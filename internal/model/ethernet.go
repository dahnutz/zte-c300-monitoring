package model

// ONUEthernetStatus distinguishes a failed/unavailable read from link-down.
// Observation time is when the collector read the OLT, not a device change time.
type ONUEthernetStatus struct {
	Status     string            `json:"status"`
	ObservedAt string            `json:"observed_at"`
	Ports      []ONUEthernetPort `json:"ports"`
}

type ONUEthernetPort struct {
	PortIndex  int    `json:"port_index"`  // raw UNI index, scoped to this ONU
	AdminState string `json:"admin_state"` // enabled, disabled, unknown
	LinkState  string `json:"link_state"`  // up, down, unknown
	SpeedMbps  *int   `json:"speed_mbps"`  // null unless link is up and speed known
	Duplex     string `json:"duplex"`      // full, half, unknown
	AdminRaw   *int   `json:"admin_raw"`
	OperRaw    *int   `json:"oper_raw"`
	SpeedRaw   *int   `json:"speed_raw"`
}
