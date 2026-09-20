// Package device names vendor/family/role values stored with telemetry.
// Stage 1 only collects ZTE C300 OLTs; other values are reserved so Huawei
// OLTs and Cisco uplink switches can share the same sample tables later.
package device

const (
	VendorZTE    = "zte"
	VendorHuawei = "huawei"
	VendorCisco  = "cisco"

	FamilyC300 = "c300"
	FamilyC320 = "c320"

	RoleOLT    = "olt"
	RoleSwitch = "switch"
)
