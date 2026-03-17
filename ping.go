package main

import (
	"context"
	"log"
	"net"
	"sync"
	"time"

	"github.com/ghibranalj/signifer/db/sqlc"
	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

type Pinger struct {
	repo            sqlc.Queries
	intervalSeconds int
	cancel          context.CancelFunc
	wg              sync.WaitGroup
	discord         *Discord             // Discord webhook client
	previousStates  map[interface{}]bool // Track previous device states (ID -> IsUp)
	stateMutex      sync.RWMutex         // Protects previousStates map
}

// NewPinger creates a new Pinger with the required dependencies
func NewPinger(repo *sqlc.Queries, intervalSeconds int, discord *Discord) *Pinger {
	p := &Pinger{
		repo:            *repo,
		intervalSeconds: intervalSeconds,
		discord:         discord,
		previousStates:  make(map[interface{}]bool),
	}

	// Initialize previous states from database
	p.initializePreviousStates()

	return p
}

// Start begins the background ping service
func (p *Pinger) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel

	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
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
	}()

	log.Printf("Pinger started with %d second interval", p.intervalSeconds)
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

	// Get previous state before ping
	wasUp := p.getPreviousState(device.ID)

	// Perform ping
	isUp, latency := p.icmpPing(dst)

	// Log ping result
	if isUp {
		log.Printf("Ping success: %s @ %s - latency: %dms", device.DeviceName, device.Hostname, latency)
	} else {
		log.Printf("Ping failed: %s @ %s - timeout", device.DeviceName, device.Hostname)
	}

	// Check for state change
	if wasUp != isUp {
		reason := "timeout"
		if !isUp {
			reason = "ping timeout"
		} else {
			reason = "ping successful"
		}
		p.notifyStateChange(device, wasUp, isUp, latency, reason)
	}

	// Update device state in database
	params := sqlc.SetDeviceStateAndLatencyParams{
		ID:              device.ID,
		IsUp:            isUp,
		LastPingLatency: latency,
	}

	if _, err := p.repo.SetDeviceStateAndLatency(context.Background(), params); err != nil {
		log.Printf("Error updating device %s: %v", device.DeviceName, err)
		return
	}

	// Update our tracked state
	p.setPreviousState(device.ID, isUp)
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
	// Get previous state before marking down
	wasUp := p.getPreviousState(device.ID)

	log.Printf("Ping failed: %s @ %s - unreachable", device.DeviceName, device.Hostname)

	// Notify if state changed (was up, now down)
	if wasUp {
		p.notifyStateChange(device, wasUp, false, -1, "DNS resolution error")
	}

	params := sqlc.SetDeviceStateAndLatencyParams{
		ID:              device.ID,
		IsUp:            false,
		LastPingLatency: -1,
	}

	if _, err := p.repo.SetDeviceStateAndLatency(context.Background(), params); err != nil {
		log.Printf("Error updating device %s: %v", device.DeviceName, err)
		return
	}

	// Update our tracked state
	p.setPreviousState(device.ID, false)
}

// initializePreviousStates loads current device states from database
func (p *Pinger) initializePreviousStates() {
	devices, err := p.repo.GetDevices(context.Background())
	if err != nil {
		log.Printf("Error initializing device states: %v", err)
		return
	}

	p.stateMutex.Lock()
	defer p.stateMutex.Unlock()

	for _, device := range devices {
		p.previousStates[device.ID] = device.IsUp
	}

	log.Printf("Initialized states for %d devices", len(devices))
}

// getPreviousState returns the previous state of a device (thread-safe)
func (p *Pinger) getPreviousState(deviceID interface{}) bool {
	p.stateMutex.RLock()
	defer p.stateMutex.RUnlock()
	return p.previousStates[deviceID]
}

// setPreviousState updates the previous state of a device (thread-safe)
func (p *Pinger) setPreviousState(deviceID interface{}, isUp bool) {
	p.stateMutex.Lock()
	defer p.stateMutex.Unlock()
	p.previousStates[deviceID] = isUp
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
