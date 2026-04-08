package main

import (
	"context"
	"log"
	"net"
	"sync"
	"time"

	"github.com/ghibranalj/signifer/db/sqlc"
	"github.com/google/uuid"
	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

type Pinger struct {
	repo            sqlc.Queries
	intervalSeconds int
	failedThreshold int
	cancel          context.CancelFunc
	wg              sync.WaitGroup
	discord         *Discord // Discord webhook client
	dbMutex         sync.Mutex // Protects database writes (SQLite doesn't support concurrent writes)
}

// NewPinger creates a new Pinger with the required dependencies
func NewPinger(repo *sqlc.Queries, intervalSeconds int, failedThreshold int, discord *Discord) *Pinger {
	p := &Pinger{
		repo:            *repo,
		intervalSeconds: intervalSeconds,
		failedThreshold: failedThreshold,
		discord:         discord,
	}

	return p
}

// Start begins the background ping service
func (p *Pinger) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel

	p.wg.Go(func() {
		ticker := time.NewTicker(time.Duration(p.intervalSeconds) * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				log.Println("Pinger stopped")
				return
			case <-ticker.C:
				p.pingAllDevices()
			}
		}
	})

	log.Printf("Pinger started with %d second interval, failed threshold: %d", p.intervalSeconds, p.failedThreshold)
}

// Stop gracefully shuts down the pinger
func (p *Pinger) Stop() {
	if p.cancel != nil {
		p.cancel()
		p.wg.Wait()
	}
}

// pingAllDevices fetches all devices and pings them concurrently
func (p *Pinger) pingAllDevices() {
	devices, err := p.repo.GetDevices(context.Background())
	if err != nil {
		log.Printf("Error fetching devices: %v", err)
		return
	}

	var wg sync.WaitGroup
	for _, device := range devices {
		wg.Add(1)
		go func(d sqlc.Device) {
			defer wg.Done()
			p.pingDevice(d)
		}(device)
	}
	wg.Wait()
}

// pingDevice pings a single device using ICMP and updates the database
func (p *Pinger) pingDevice(device sqlc.Device) {
	log.Printf("Pinging %s @ %s", device.DeviceName, device.Hostname)

	// Resolve the hostname
	dst, err := net.ResolveIPAddr("ip", device.Hostname)
	if err != nil {
		log.Printf("Ping failed: %s @ %s - DNS resolution error", device.DeviceName, device.Hostname)
		p.markDeviceDown(device)
		return
	}

	// Perform ping
	isUp, latency := p.icmpPing(dst)

	// Log ping result
	if isUp {
		log.Printf("Ping success: %s @ %s - latency: %dms", device.DeviceName, device.Hostname, latency)
	} else {
		log.Printf("Ping failed: %s @ %s - timeout", device.DeviceName, device.Hostname)
	}

	// Calculate failed pings count and new alerted state
	failedPings := int64(0)
	alertedDown := device.AlertedDown

	if !isUp {
		failedPings = device.FailedPings + 1
	}

	// No state change - just update and return
	if device.IsUp == isUp {
		p.updateDeviceState(device.ID, isUp, latency, failedPings, alertedDown)
		return
	}

	// Device went down - notify after threshold reached
	if !isUp && failedPings >= int64(p.failedThreshold) && !device.AlertedDown {
		alertedDown = true
		p.notifyStateChange(device, true, false, latency, "ping timeout")
		p.updateDeviceState(device.ID, isUp, latency, failedPings, alertedDown)
		return
	}

	// Device recovered - notify if we had alerted
	if isUp && device.AlertedDown {
		alertedDown = false
		p.notifyStateChange(device, false, true, latency, "ping successful")
		p.updateDeviceState(device.ID, isUp, latency, failedPings, alertedDown)
		return
	}

	// State changed but threshold not reached - just update
	p.updateDeviceState(device.ID, isUp, latency, failedPings, alertedDown)
}

// updateDeviceState updates the device state in the database
func (p *Pinger) updateDeviceState(id uuid.UUID, isUp bool, latency int64, failedPings int64, alertedDown bool) {
	params := sqlc.SetDeviceStateAndLatencyParams{
		ID:              id,
		IsUp:            isUp,
		LastPingLatency: latency,
		FailedPings:     failedPings,
		AlertedDown:     alertedDown,
	}

	p.dbMutex.Lock()
	defer p.dbMutex.Unlock()

	if _, err := p.repo.SetDeviceStateAndLatency(context.Background(), params); err != nil {
		log.Printf("Error updating device: %v", err)
	}
}

// icmpPing sends an ICMP echo request and returns success status and latency in ms
func (p *Pinger) icmpPing(dst *net.IPAddr) (bool, int64) {
	start := time.Now()

	// Determine IP version
	var network string
	var icmpType icmp.Type
	if dst.IP.To4() != nil {
		// IPv4
		network = "ip4:icmp"
		icmpType = ipv4.ICMPTypeEcho
	} else {
		// IPv6
		network = "ip6:ipv6-icmp"
		icmpType = ipv6.ICMPTypeEchoRequest
	}

	// Create ICMP connection
	conn, err := icmp.ListenPacket(network, "")
	if err != nil {
		log.Printf("Error creating ICMP connection: %v (requires root/CAP_NET_RAW)", err)
		return false, -1
	}
	defer conn.Close()

	// Create ICMP echo request message
	msg := &icmp.Message{
		Type: icmpType,
		Code: 0,
		Body: &icmp.Echo{
			ID:   1,
			Seq:  1,
			Data: []byte("signifer"),
		},
	}

	data, err := msg.Marshal(nil)
	if err != nil {
		return false, -1
	}

	// Set deadline for response
	deadline := time.Now().Add(5 * time.Second)
	conn.SetWriteDeadline(deadline)
	conn.SetReadDeadline(deadline)

	// Send the packet
	if _, err := conn.WriteTo(data, dst); err != nil {
		return false, -1
	}

	// Wait for response
	reply := make([]byte, 1500)
	n, _, err := conn.ReadFrom(reply)
	if err != nil {
		return false, -1
	}

	latency := time.Since(start).Milliseconds()

	// Parse response
	rm, err := icmp.ParseMessage(1, reply[:n])
	if err != nil {
		return false, -1
	}

	// Check if it's an echo reply
	switch rm.Type {
	case ipv4.ICMPTypeEchoReply, ipv6.ICMPTypeEchoReply:
		return true, latency
	default:
		return false, -1
	}
}

// markDeviceDown updates device as offline when unreachable
func (p *Pinger) markDeviceDown(device sqlc.Device) {
	log.Printf("Ping failed: %s @ %s - unreachable", device.DeviceName, device.Hostname)

	failedPings := device.FailedPings + 1
	alertedDown := device.AlertedDown

	// Already down - just increment counter
	if !device.IsUp {
		p.updateDeviceState(device.ID, false, -1, failedPings, alertedDown)
		return
	}

	// Was up, now down - check threshold
	shouldAlert := failedPings >= int64(p.failedThreshold) && !device.AlertedDown
	if shouldAlert {
		alertedDown = true
		p.notifyStateChange(device, true, false, -1, "DNS resolution error")
	}

	p.updateDeviceState(device.ID, false, -1, failedPings, alertedDown)
}

// notifyStateChange sends a Discord notification when device status changes
func (p *Pinger) notifyStateChange(device sqlc.Device, wasUp, isNowUp bool, latency int64, reason string) {
	change := DeviceStatusChange{
		DeviceName: device.DeviceName,
		Hostname:   device.Hostname,
		WasUp:      wasUp,
		IsNowUp:    isNowUp,
		Timestamp:  time.Now(),
		Latency:    latency,
		Reason:     reason,
	}

	if err := p.discord.Notify(change); err != nil {
		log.Printf("Error sending Discord notification for device %s: %v", device.DeviceName, err)
	} else {
		log.Printf("Sent Discord notification: %s status changed from %s to %s",
			device.DeviceName,
			map[bool]string{true: "online", false: "offline"}[wasUp],
			map[bool]string{true: "online", false: "offline"}[isNowUp])
	}
}
