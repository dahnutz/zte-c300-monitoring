package model

import "testing"

func TestExpectedEthernetPorts(t *testing.T) {
	if got := ExpectedEthernetPorts("ZTE-F668"); got != 4 {
		t.Fatalf("F668 = %d", got)
	}
	if got := ExpectedEthernetPorts("F601"); got != 1 {
		t.Fatalf("F601 = %d", got)
	}
	if got := ExpectedEthernetPorts("mystery"); got != 0 {
		t.Fatalf("unknown = %d", got)
	}
}

func TestPadEthernetPortsF668(t *testing.T) {
	ports := PadEthernetPorts("F668", []ONUEthernetPort{{
		PortIndex: 1, AdminState: "enabled", LinkState: "up",
	}})
	if len(ports) != 4 || ports[0].LinkState != "up" || ports[3].PortIndex != 4 || ports[3].LinkState != "unknown" {
		t.Fatalf("%+v", ports)
	}
}
