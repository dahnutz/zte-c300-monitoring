package store

import (
	"strings"
	"time"

	"zte-c300-monitoring/internal/model"
)

// ONUState is the current identity row in onus.
type ONUState struct {
	DeviceID         string     `json:"device_id"`
	Serial           string     `json:"serial_number"`
	Name             string     `json:"name"`
	OnuType          string     `json:"onu_type"`
	Board            int        `json:"board"`
	PON              int        `json:"pon"`
	ONUID            int        `json:"onu_id"`
	Status           string     `json:"status"`
	PreviousStatus   string     `json:"previous_status,omitempty"`
	StatusChangedAt  *time.Time `json:"status_changed_at,omitempty"`
	ExpectedEthPorts int        `json:"expected_eth_ports,omitempty"`
	LastSeenAt       *time.Time `json:"last_seen_at,omitempty"`
}

// StatusEvent is one ONU operational-state transition.
type StatusEvent struct {
	Time           time.Time `json:"time"`
	DeviceID       string    `json:"device_id"`
	Serial         string    `json:"serial_number"`
	Board          int       `json:"board"`
	PON            int       `json:"pon"`
	ONUID          int       `json:"onu_id"`
	Status         string    `json:"status"`
	PreviousStatus string    `json:"previous_status,omitempty"`
	Source         string    `json:"source,omitempty"`
}

// EthPortState is the last known UNI row.
type EthPortState struct {
	DeviceID      string     `json:"device_id"`
	Serial        string     `json:"serial_number"`
	Port          int        `json:"port"`
	AdminState    string     `json:"admin_state"`
	LinkState     string     `json:"link_state"`
	SpeedMbps     *int       `json:"speed_mbps,omitempty"`
	Duplex        string     `json:"duplex,omitempty"`
	LastChangedAt *time.Time `json:"last_changed_at,omitempty"`
	LastSeenAt    *time.Time `json:"last_seen_at,omitempty"`
}

// EthEvent is one UNI admin/link change.
type EthEvent struct {
	Time          time.Time `json:"time"`
	DeviceID      string    `json:"device_id"`
	Serial        string    `json:"serial_number"`
	Board         int       `json:"board"`
	PON           int       `json:"pon"`
	ONUID         int       `json:"onu_id"`
	Port          int       `json:"port"`
	LinkState     string    `json:"link_state"`
	AdminState    string    `json:"admin_state"`
	PreviousLink  string    `json:"previous_link,omitempty"`
	PreviousAdmin string    `json:"previous_admin,omitempty"`
	SpeedMbps     *int      `json:"speed_mbps,omitempty"`
	Source        string    `json:"source,omitempty"`
}

// UnauthONU is an OLT-discovered serial that is not provisioned.
type UnauthONU struct {
	DeviceID        string     `json:"device_id"`
	Serial          string     `json:"serial_number"`
	Board           int        `json:"board,omitempty"`
	PON             int        `json:"pon,omitempty"`
	OnuType         string     `json:"onu_type,omitempty"`
	DiscoveryStatus string     `json:"discovery_status,omitempty"`
	FirstSeenAt     time.Time  `json:"first_seen_at"`
	LastSeenAt      time.Time  `json:"last_seen_at"`
	GoneAt          *time.Time `json:"gone_at,omitempty"`
}

// UnauthList is the current unconfigured-ONU observation.
type UnauthList struct {
	Status     string      `json:"status"`
	OID        string      `json:"oid,omitempty"`
	Message    string      `json:"message,omitempty"`
	ObservedAt time.Time   `json:"observed_at"`
	Count      int         `json:"count"`
	ONUs       []UnauthONU `json:"onus"`
}

// EventQuery selects status or Ethernet events.
type EventQuery struct {
	DeviceID string
	Serial   string
	Port     int
	Limit    int
}

func eventLimit(limit int) int {
	if limit <= 0 {
		return 50
	}
	if limit > 500 {
		return 500
	}
	return limit
}

func statusChanged(prev ONUState, sample ONUSample) bool {
	if strings.TrimSpace(sample.Serial) == "" {
		return false
	}
	if prev.Serial == "" {
		return true
	}
	return !strings.EqualFold(prev.Status, sample.Status)
}

func statusEventFrom(prev ONUState, sample ONUSample, source string) StatusEvent {
	return StatusEvent{
		Time:           sample.Time,
		DeviceID:       sample.DeviceID,
		Serial:         sample.Serial,
		Board:          sample.Board,
		PON:            sample.PON,
		ONUID:          sample.ONUID,
		Status:         sample.Status,
		PreviousStatus: prev.Status,
		Source:         source,
	}
}

func ethEventsFrom(prev map[int]EthPortState, sample ONUSample, source string) []EthEvent {
	var out []EthEvent
	seen := map[int]struct{}{}
	for _, port := range sample.EthPorts {
		seen[port.Port] = struct{}{}
		old := prev[port.Port]
		if old.Serial != "" && old.LinkState == port.Link && old.AdminState == port.Admin {
			continue
		}
		if old.Serial == "" && port.Link == "" && port.Admin == "" {
			continue
		}
		out = append(out, EthEvent{
			Time:          sample.Time,
			DeviceID:      sample.DeviceID,
			Serial:        sample.Serial,
			Board:         sample.Board,
			PON:           sample.PON,
			ONUID:         sample.ONUID,
			Port:          port.Port,
			LinkState:     port.Link,
			AdminState:    port.Admin,
			PreviousLink:  old.LinkState,
			PreviousAdmin: old.AdminState,
			SpeedMbps:     port.SpeedMbps,
			Source:        source,
		})
	}
	return out
}

func applyExpectedPorts(sample *ONUSample) {
	n := model.ExpectedEthernetPorts(sample.OnuType)
	sample.ExpectedEthPorts = n
	if n <= 0 || len(sample.EthPorts) >= n {
		return
	}
	by := map[int]EthPortSample{}
	for _, port := range sample.EthPorts {
		by[port.Port] = port
	}
	out := make([]EthPortSample, 0, n)
	for i := 1; i <= n; i++ {
		if port, ok := by[i]; ok {
			out = append(out, port)
			continue
		}
		out = append(out, EthPortSample{Port: i, Admin: "unknown", Link: "unknown"})
	}
	sample.EthPorts = out
}
