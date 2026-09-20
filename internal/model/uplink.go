package model

// UplinkPort represents a single uplink ethernet port or LAG discovered via SNMP
// (ifName starting with "xgei_" for 10G, "gei_" for 1G, or "SmartGroupN" for LAG).
type UplinkPort struct {
	Name        string   `json:"name"`              // ifName, e.g. "xgei_1/19/1" or "SmartGroup1"
	Shelf       int      `json:"shelf"`             // parsed shelf (0 if name didn't parse)
	Slot        int      `json:"slot"`              // parsed slot (0 if name didn't parse)
	Port        int      `json:"port"`              // parsed port, or SmartGroup id
	Kind        string   `json:"kind"`              // "10G", "1G", or "lag"
	AdminStatus string   `json:"admin_status"`      // "up" / "down" / numeric string
	OperStatus  string   `json:"oper_status"`       // "up" / "down" / numeric string
	SpeedMbps   int      `json:"speed_mbps"`        // ifHighSpeed in Mbps
	Lag         string   `json:"lag,omitempty"`     // aggregator ifName when this port is a member
	Members     []string `json:"members,omitempty"` // member ifNames when kind is lag
}

// UplinkCard represents a physical card/module discovered via ENTITY-MIB
// (standard modules, plus explicitly recognized firmware card classes).
type UplinkCard struct {
	EntIndex   int    `json:"ent_index"`   // raw entPhysical index (heuristic-free reference)
	Slot       int    `json:"slot"`        // parent-relative position, or inherited fallback
	SlotSource string `json:"slot_source"` // parent_relative_position or index_heuristic
	Type       string `json:"type"`        // entPhysicalDescr text
	Role       string `json:"role"`        // "gpon" / "control" / "uplink" / "power" / "other"
}

// UplinkTopology is the auto-detected OLT card + uplink-port topology returned
// by the /uplinks endpoint (Phase 1: detection only, no config writes).
type UplinkTopology struct {
	Cards []UplinkCard `json:"cards"`
	Ports []UplinkPort `json:"ports"`
}
