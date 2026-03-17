package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

// Discord handles webhook notifications for device status changes
type Discord struct {
	webhookURL string
}

// NewDiscord creates a new Discord webhook client
func NewDiscord(webhookURL string) *Discord {
	return &Discord{
		webhookURL: webhookURL,
	}
}

// DeviceStatusChange represents the data needed for a notification
type DeviceStatusChange struct {
	DeviceName string
	Hostname   string
	WasUp      bool
	IsNowUp    bool
	Timestamp  time.Time
	Latency    int64  // -1 if device is down
	Reason     string // Reason for status change (e.g., "DNS resolution error", "timeout")
}

// DiscordEmbed represents a Discord embed object
type DiscordEmbed struct {
	Title       string  `json:"title"`
	Description string  `json:"description"`
	Color       int     `json:"color"`
	Timestamp   string  `json:"timestamp"`
	Fields      []Field `json:"fields"`
}

// Field represents a field in a Discord embed
type Field struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline"`
}

// WebhookPayload represents the Discord webhook JSON structure
type WebhookPayload struct {
	Content string         `json:"content,omitempty"` // Optional text content
	Embeds  []DiscordEmbed `json:"embeds"`            // Rich embeds
}

// Notify sends a Discord webhook notification for a device status change
func (d *Discord) Notify(change DeviceStatusChange) error {
	// Determine status and color
	var title, description string
	var color int

	if change.IsNowUp {
		title = "Device Back Online"
		description = change.DeviceName + " is now responding to pings."
		color = 0x00FF00 // Green
	} else {
		title = "Device Offline"
		description = change.DeviceName + " is no longer responding to pings."
		color = 0xFF0000 // Red
	}

	// Build embed fields
	fields := []Field{
		{Name: "Hostname", Value: change.Hostname, Inline: true},
		{Name: "Previous State", Value: boolToString(change.WasUp), Inline: true},
		{Name: "Current State", Value: boolToString(change.IsNowUp), Inline: true},
	}

	// Add reason if provided
	if change.Reason != "" {
		fields = append(fields, Field{Name: "Reason", Value: change.Reason, Inline: false})
	}

	// Add latency field if device is up
	if change.IsNowUp && change.Latency >= 0 {
		fields = append(fields, Field{Name: "Latency", Value: formatLatency(change.Latency), Inline: true})
	}

	// Create embed
	embed := DiscordEmbed{
		Title:       title,
		Description: description,
		Color:       color,
		Timestamp:   change.Timestamp.Format(time.RFC3339),
		Fields:      fields,
	}

	// Create webhook payload
	payload := WebhookPayload{Embeds: []DiscordEmbed{embed}}

	// Marshal to JSON
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	// Send HTTP POST request
	resp, err := http.Post(d.webhookURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Check for non-2xx status codes
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.Printf("Discord webhook returned status %d", resp.StatusCode)
	}

	return nil
}

func boolToString(b bool) string {
	if b {
		return "Online"
	}
	return "Offline"
}

func formatLatency(latency int64) string {
	if latency < 0 {
		return "N/A"
	}
	return fmt.Sprintf("%d ms", latency)
}
