package model

import "time"

// UnauthDiscovery is a read-only list of OLT-seen, unprovisioned ONUs.
type UnauthDiscovery struct {
	Status     string      `json:"status"`
	OID        string      `json:"oid,omitempty"`
	Message    string      `json:"message,omitempty"`
	ObservedAt time.Time   `json:"observed_at"`
	ONUs       []UnauthONU `json:"onus"`
}

// UnauthONU is one discovered serial that is not in the provisioned inventory.
type UnauthONU struct {
	Serial  string `json:"serial_number"`
	Board   int    `json:"board,omitempty"`
	PON     int    `json:"pon,omitempty"`
	OnuType string `json:"onu_type,omitempty"`
}
