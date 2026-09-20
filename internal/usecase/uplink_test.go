package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/gosnmp/gosnmp"
	"zte-c300-monitoring/config"
)

func TestToInt(t *testing.T) {
	cases := []struct {
		in   interface{}
		want int
	}{
		{int(5), 5},
		{int32(7), 7},
		{int64(9), 9},
		{uint(3), 3},
		{uint32(10000), 10000},
		{uint64(1000), 1000},
		{"not-an-int", 0},
		{[]byte("x"), 0},
	}
	for _, c := range cases {
		if got := toInt(c.in); got != c.want {
			t.Errorf("toInt(%v) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestTrailingIndex(t *testing.T) {
	if got := trailingIndex(".1.3.6.1.2.1.31.1.1.1.1.169"); got != 169 {
		t.Errorf("trailingIndex = %d, want 169", got)
	}
	if got := trailingIndex("1.3.6.1.foo"); got != -1 {
		t.Errorf("trailingIndex non-int = %d, want -1", got)
	}
}

func TestCardRole(t *testing.T) {
	cases := map[string]string{
		"16 ports GPON OLT line card.":                                "gpon",
		"Type B Level TM switch control card.":                        "control",
		"2 ports 10GE and 2 ports GE optical Ethernet interface card": "uplink",
		"General Power Card.":                                         "power",
		"Some random thing":                                           "other",
	}
	for descr, want := range cases {
		if got := cardRole(descr); got != want {
			t.Errorf("cardRole(%q) = %q, want %q", descr, got, want)
		}
	}
}

func TestGetUplinkTopology(t *testing.T) {
	// Index assignments for the fake walk.
	const (
		idxXgei    = 100 // xgei_1/19/1 (10G uplink, admin up, oper up, 10000 Mbps)
		idxGei     = 101 // gei_1/19/3  (1G uplink, admin up, oper down, 1000 Mbps)
		idxMng     = 102 // Mng1        (ignored)
		entUplink  = 30  // class 9, Ethernet interface card -> role uplink, slot 2
		entGpon    = 200 // class 9, GPON line card -> role gpon, slot 19
		entChassis = 1   // class 3 -> ignored
	)

	snmpRepo := &mockSnmpRepository{
		BulkWalkFunc: func(oid string, walkFunc func(pdu gosnmp.SnmpPDU) error) error {
			switch oid {
			case config.OidIfName:
				_ = walkFunc(gosnmp.SnmpPDU{Name: oid + ".100", Type: gosnmp.OctetString, Value: []byte("xgei_1/19/1")})
				_ = walkFunc(gosnmp.SnmpPDU{Name: oid + ".101", Type: gosnmp.OctetString, Value: []byte("gei_1/19/3")})
				_ = walkFunc(gosnmp.SnmpPDU{Name: oid + ".102", Type: gosnmp.OctetString, Value: []byte("Mng1")})
			case config.OidIfAdminStatus:
				_ = walkFunc(gosnmp.SnmpPDU{Name: oid + ".100", Type: gosnmp.Integer, Value: 1})
				_ = walkFunc(gosnmp.SnmpPDU{Name: oid + ".101", Type: gosnmp.Integer, Value: 1})
				_ = walkFunc(gosnmp.SnmpPDU{Name: oid + ".102", Type: gosnmp.Integer, Value: 1})
			case config.OidIfOperStatus:
				_ = walkFunc(gosnmp.SnmpPDU{Name: oid + ".100", Type: gosnmp.Integer, Value: 1})
				_ = walkFunc(gosnmp.SnmpPDU{Name: oid + ".101", Type: gosnmp.Integer, Value: 2})
				_ = walkFunc(gosnmp.SnmpPDU{Name: oid + ".102", Type: gosnmp.Integer, Value: 1})
			case config.OidIfHighSpeed:
				_ = walkFunc(gosnmp.SnmpPDU{Name: oid + ".100", Type: gosnmp.Gauge32, Value: uint(10000)})
				_ = walkFunc(gosnmp.SnmpPDU{Name: oid + ".101", Type: gosnmp.Gauge32, Value: uint(1000)})
			case config.OidEntPhysicalClass:
				_ = walkFunc(gosnmp.SnmpPDU{Name: oid + ".30", Type: gosnmp.Integer, Value: 9})
				_ = walkFunc(gosnmp.SnmpPDU{Name: oid + ".200", Type: gosnmp.Integer, Value: 9})
				_ = walkFunc(gosnmp.SnmpPDU{Name: oid + ".1", Type: gosnmp.Integer, Value: 3}) // chassis, ignored
			case config.OidEntPhysicalDescr:
				_ = walkFunc(gosnmp.SnmpPDU{Name: oid + ".30", Type: gosnmp.OctetString, Value: []byte("2 ports 10GE and 2 ports GE optical Ethernet interface card")})
				_ = walkFunc(gosnmp.SnmpPDU{Name: oid + ".200", Type: gosnmp.OctetString, Value: []byte("16 ports GPON OLT line card.")})
				_ = walkFunc(gosnmp.SnmpPDU{Name: oid + ".1", Type: gosnmp.OctetString, Value: []byte("ZXA10 C300 chassis")})
			}
			return nil
		},
	}

	uc := NewOnuUsecase(snmpRepo, &mockRedisRepository{}, &config.Config{}).(*onuUsecase)

	topo, err := uc.GetUplinkTopology(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 2 uplink ports (Mng1 filtered out), sorted by slot then port.
	if len(topo.Ports) != 2 {
		t.Fatalf("expected 2 uplink ports, got %d (%+v)", len(topo.Ports), topo.Ports)
	}

	// Both are slot 19; xgei port 1 should sort before gei port 3.
	p0 := topo.Ports[0]
	if p0.Name != "xgei_1/19/1" || p0.Kind != "10G" || p0.Shelf != 1 || p0.Slot != 19 || p0.Port != 1 {
		t.Errorf("port[0] parse wrong: %+v", p0)
	}
	if p0.AdminStatus != "up" || p0.OperStatus != "up" || p0.SpeedMbps != 10000 {
		t.Errorf("port[0] status/speed wrong: %+v", p0)
	}

	p1 := topo.Ports[1]
	if p1.Name != "gei_1/19/3" || p1.Kind != "1G" || p1.Slot != 19 || p1.Port != 3 {
		t.Errorf("port[1] parse wrong: %+v", p1)
	}
	if p1.AdminStatus != "up" || p1.OperStatus != "down" || p1.SpeedMbps != 1000 {
		t.Errorf("port[1] status/speed wrong: %+v", p1)
	}

	// 2 cards (chassis class 3 filtered out), sorted by slot.
	if len(topo.Cards) != 2 {
		t.Fatalf("expected 2 cards, got %d (%+v)", len(topo.Cards), topo.Cards)
	}
	// ent 30 -> slot 2 (uplink) sorts before ent 200 -> slot 19 (gpon).
	c0 := topo.Cards[0]
	if c0.EntIndex != 30 || c0.Slot != 2 || c0.Role != "uplink" {
		t.Errorf("card[0] wrong: %+v", c0)
	}
	c1 := topo.Cards[1]
	if c1.EntIndex != 200 || c1.Slot != 19 || c1.Role != "gpon" {
		t.Errorf("card[1] wrong: %+v", c1)
	}
}

var errSNMPTimeout = errors.New("snmp timeout")

func TestToInt_AllIntegerKinds(t *testing.T) {
	cases := []struct {
		in   interface{}
		want int
	}{
		{int8(5), 5},
		{int16(6), 6},
		{uint8(7), 7},
		{uint16(8), 8},
	}
	for _, c := range cases {
		if got := toInt(c.in); got != c.want {
			t.Errorf("toInt(%T %v) = %d, want %d", c.in, c.in, got, c.want)
		}
	}
}

func TestPduString(t *testing.T) {
	if got := pduString([]byte("xgei_1/19/1")); got != "xgei_1/19/1" {
		t.Errorf("pduString([]byte) = %q", got)
	}
	if got := pduString("gei_1/2/3"); got != "gei_1/2/3" {
		t.Errorf("pduString(string) = %q", got)
	}
	if got := pduString(42); got != "" {
		t.Errorf("pduString(non-string) = %q, want empty", got)
	}
}

func TestStatusString(t *testing.T) {
	if got := statusString(1); got != "up" {
		t.Errorf("statusString(1) = %q", got)
	}
	if got := statusString(2); got != "down" {
		t.Errorf("statusString(2) = %q", got)
	}
	if got := statusString(7); got != "7" {
		t.Errorf("statusString(7) = %q", got)
	}
}

// Each IF-MIB / ENTITY-MIB walk has its own error return; fail each subtree in
// turn and assert GetUplinkTopology surfaces an SNMP error.
func TestGetUplinkTopology_WalkErrors(t *testing.T) {
	failOIDs := []string{
		config.OidIfName,
		config.OidIfAdminStatus,
		config.OidIfOperStatus,
		config.OidIfHighSpeed,
		config.OidEntPhysicalClass,
		config.OidEntPhysicalDescr,
	}
	for _, failOID := range failOIDs {
		snmpRepo := &mockSnmpRepository{
			BulkWalkFunc: func(oid string, walkFunc func(pdu gosnmp.SnmpPDU) error) error {
				if oid == failOID {
					return errSNMPTimeout
				}
				// Seed one uplink port so the later walks are reached.
				if oid == config.OidIfName {
					_ = walkFunc(gosnmp.SnmpPDU{Name: oid + ".100", Type: gosnmp.OctetString, Value: []byte("xgei_1/19/1")})
				}
				return nil
			},
		}
		uc := NewOnuUsecase(snmpRepo, &mockRedisRepository{}, &config.Config{}).(*onuUsecase)
		if _, err := uc.GetUplinkTopology(context.Background()); err == nil {
			t.Errorf("failOID %s: expected error, got nil", failOID)
		}
	}
}

func TestFirmwareCardClassesAndReportedPositions(t *testing.T) {
	repo := &mockSnmpRepository{BulkWalkFunc: func(oid string, walk func(gosnmp.SnmpPDU) error) error {
		values := map[string][]gosnmp.SnmpPDU{
			config.OidEntPhysicalClass:        {{Name: config.OidEntPhysicalClass + ".1", Type: gosnmp.Integer, Value: 3}, {Name: config.OidEntPhysicalClass + ".40", Type: gosnmp.Integer, Value: 3}, {Name: config.OidEntPhysicalClass + ".10", Type: gosnmp.Integer, Value: 6}},
			config.OidEntPhysicalDescr:        {{Name: config.OidEntPhysicalDescr + ".1", Type: gosnmp.OctetString, Value: []byte("OLT chassis")}, {Name: config.OidEntPhysicalDescr + ".40", Type: gosnmp.OctetString, Value: []byte("8-port GPON OLT line card.")}, {Name: config.OidEntPhysicalDescr + ".10", Type: gosnmp.OctetString, Value: []byte("General Power Card.")}},
			config.OidEntPhysicalParentRelPos: {{Name: config.OidEntPhysicalParentRelPos + ".40", Type: gosnmp.Integer, Value: 7}, {Name: config.OidEntPhysicalParentRelPos + ".10", Type: gosnmp.Integer, Value: 0}},
		}
		for _, p := range values[oid] {
			if err := walk(p); err != nil {
				return err
			}
		}
		return nil
	}}
	uc := NewOnuUsecase(repo, &mockRedisRepository{}, &config.Config{})
	topology, err := uc.GetUplinkTopology(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(topology.Cards) != 2 {
		t.Fatalf("cards/chassis classification: %+v", topology.Cards)
	}
	if topology.Cards[0].Role != "power" || topology.Cards[0].Slot != 0 {
		t.Fatalf("power card: %+v", topology.Cards[0])
	}
	card := topology.Cards[1]
	if card.Slot != 7 || card.SlotSource != "parent_relative_position" || card.Role != "gpon" {
		t.Fatalf("reported position must win over heuristic: %+v", card)
	}
	if isCardEntity(3, "chassis") || isCardEntity(6, "power supply") {
		t.Fatal("non-card entity accepted")
	}
}

func TestClassifyUplinkName(t *testing.T) {
	cases := map[string]string{
		"xgei_1/2/1":  "10G",
		"gei_1/2/5":   "1G",
		"SmartGroup1": uplinkKindLAG,
		"smartgroup3": uplinkKindLAG,
		"gpon_1/3/1":  "",
		"Mng1":        "",
	}
	for name, want := range cases {
		got, ok := classifyUplinkName(name)
		if want == "" {
			if ok {
				t.Errorf("%s: got %q, want ignored", name, got)
			}
			continue
		}
		if !ok || got != want {
			t.Errorf("%s: got %q ok=%v, want %q", name, got, ok, want)
		}
	}
}

func TestTrailingIndices(t *testing.T) {
	got := trailingIndices(".1.3.6.1.2.1.31.1.2.1.3.553648129.285281025", 2)
	if len(got) != 2 || got[0] != 553648129 || got[1] != 285281025 {
		t.Fatalf("trailingIndices = %v", got)
	}
	if trailingIndices("1.2.3", 4) != nil {
		t.Fatal("short oid")
	}
}

func TestGetUplinkTopologySmartGroup(t *testing.T) {
	snmpRepo := &mockSnmpRepository{
		BulkWalkFunc: func(oid string, walkFunc func(pdu gosnmp.SnmpPDU) error) error {
			switch oid {
			case config.OidIfName:
				_ = walkFunc(gosnmp.SnmpPDU{Name: oid + ".100", Type: gosnmp.OctetString, Value: []byte("xgei_1/2/1")})
				_ = walkFunc(gosnmp.SnmpPDU{Name: oid + ".101", Type: gosnmp.OctetString, Value: []byte("gei_1/2/5")})
				_ = walkFunc(gosnmp.SnmpPDU{Name: oid + ".200", Type: gosnmp.OctetString, Value: []byte("SmartGroup1")})
				_ = walkFunc(gosnmp.SnmpPDU{Name: oid + ".102", Type: gosnmp.OctetString, Value: []byte("Mng1")})
			case config.OidIfAdminStatus:
				_ = walkFunc(gosnmp.SnmpPDU{Name: oid + ".100", Type: gosnmp.Integer, Value: 1})
				_ = walkFunc(gosnmp.SnmpPDU{Name: oid + ".200", Type: gosnmp.Integer, Value: 1})
			case config.OidIfOperStatus:
				_ = walkFunc(gosnmp.SnmpPDU{Name: oid + ".100", Type: gosnmp.Integer, Value: 1})
				_ = walkFunc(gosnmp.SnmpPDU{Name: oid + ".200", Type: gosnmp.Integer, Value: 1})
			case config.OidIfHighSpeed:
				_ = walkFunc(gosnmp.SnmpPDU{Name: oid + ".100", Type: gosnmp.Gauge32, Value: uint(10000)})
				_ = walkFunc(gosnmp.SnmpPDU{Name: oid + ".200", Type: gosnmp.Gauge32, Value: uint(20000)})
			case config.OidIfStackStatus:
				_ = walkFunc(gosnmp.SnmpPDU{Name: oid + ".200.100", Type: gosnmp.Integer, Value: 1})
				_ = walkFunc(gosnmp.SnmpPDU{Name: oid + ".200.101", Type: gosnmp.Integer, Value: 1})
			}
			return nil
		},
	}
	uc := NewOnuUsecase(snmpRepo, &mockRedisRepository{}, &config.Config{}).(*onuUsecase)
	topo, err := uc.GetUplinkTopology(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(topo.Ports) != 3 {
		t.Fatalf("ports = %+v", topo.Ports)
	}
	if topo.Ports[0].Name != "xgei_1/2/1" || topo.Ports[0].Lag != "SmartGroup1" {
		t.Fatalf("member xgei: %+v", topo.Ports[0])
	}
	if topo.Ports[1].Name != "gei_1/2/5" || topo.Ports[1].Lag != "SmartGroup1" {
		t.Fatalf("member gei: %+v", topo.Ports[1])
	}
	lag := topo.Ports[2]
	if lag.Name != "SmartGroup1" || lag.Kind != uplinkKindLAG || lag.Port != 1 {
		t.Fatalf("lag: %+v", lag)
	}
	if lag.SpeedMbps != 20000 || len(lag.Members) != 2 || lag.Members[0] != "gei_1/2/5" || lag.Members[1] != "xgei_1/2/1" {
		t.Fatalf("lag members/speed: %+v", lag)
	}
}
