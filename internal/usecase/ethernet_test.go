package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/gosnmp/gosnmp"
	"zte-c300-monitoring/config"
	"zte-c300-monitoring/internal/model"
)

// All device identities, positions and measurements in this file are synthetic.
func TestEthernetPortsIsolationAndStates(t *testing.T) {
	calls := 0
	repo := &mockSnmpRepository{BulkWalkFunc: func(root string, walk func(gosnmp.SnmpPDU) error) error {
		calls++
		if !strings.HasSuffix(root, ".285278724.6") {
			t.Fatalf("wrong ONU scope: %s", root)
		}
		values := []int{1, 1}
		if strings.Contains(root, config.OnuEthOperPrefix+".") {
			values = []int{1, 2}
		}
		if strings.Contains(root, config.OnuEthSpeedPrefix+".") {
			values = []int{6, 6}
		}
		// Deliberately unsorted ports. Port 2's stale speed must not look live.
		for _, port := range []int{2, 1} {
			suffix := ".1"
			if port == 2 {
				suffix = ".2"
			}
			if err := walk(gosnmp.SnmpPDU{Name: root + suffix, Type: gosnmp.Integer, Value: values[port-1]}); err != nil {
				return err
			}
		}
		// Malbehaving agent returns a different ONU and a prefix-collision ONU 60.
		_ = walk(gosnmp.SnmpPDU{Name: strings.TrimSuffix(root, ".6") + ".7.1", Type: gosnmp.Integer, Value: 1})
		_ = walk(gosnmp.SnmpPDU{Name: root + "0.1", Type: gosnmp.Integer, Value: 1})
		return nil
	}}
	cfg := &config.Config{OltCfg: config.OltConfig{BaseOID1: config.BaseOID1}}
	uc := NewOnuUsecase(repo, &mockRedisRepository{}, cfg).(*onuUsecase)
	got := uc.getEthernetPorts(context.Background(), 2, 4, 6, "Online", "")
	if calls != 3 || got.Status != "ok" || len(got.Ports) != 2 || got.ObservedAt == "" {
		t.Fatalf("unexpected collection: %+v (%d calls)", got, calls)
	}
	up, down := got.Ports[0], got.Ports[1]
	if up.PortIndex != 1 || up.AdminState != "enabled" || up.LinkState != "up" || up.SpeedMbps == nil || *up.SpeedMbps != 1000 || up.Duplex != "full" {
		t.Fatalf("online port: %+v", up)
	}
	if down.LinkState != "down" || down.SpeedMbps != nil || down.Duplex != "unknown" || *down.SpeedRaw != 6 {
		t.Fatalf("stale speed must not become a live rate: %+v", down)
	}
}

func TestEthernetUnavailableAndPartial(t *testing.T) {
	for _, mode := range []string{"absent", "timeout", "partial", "sentinel", "malformed", "auto"} {
		t.Run(mode, func(t *testing.T) {
			repo := &mockSnmpRepository{BulkWalkFunc: func(root string, walk func(gosnmp.SnmpPDU) error) error {
				if mode == "absent" {
					return nil
				}
				if mode == "timeout" {
					return errors.New("timeout")
				}
				if mode == "partial" && strings.Contains(root, config.OnuEthOperPrefix+".") {
					return errors.New("timeout")
				}
				value := 1
				if strings.Contains(root, config.OnuEthSpeedPrefix+".") {
					value = 6
				}
				if mode == "sentinel" {
					value = 65535
				}
				if mode == "auto" && strings.Contains(root, config.OnuEthSpeedPrefix+".") {
					value = 1
				}
				p := gosnmp.SnmpPDU{Name: root + ".1", Type: gosnmp.Integer, Value: value}
				if mode == "malformed" {
					p.Type = gosnmp.OctetString
					p.Value = []byte("bad")
				}
				return walk(p)
			}}
			uc := NewOnuUsecase(repo, &mockRedisRepository{}, &config.Config{}).(*onuUsecase)
			got := uc.getEthernetPorts(context.Background(), 2, 4, 6, "Online", "")
			want := "partial"
			if mode == "absent" || mode == "timeout" || mode == "malformed" {
				want = "unavailable"
			}
			if got.Status != want {
				t.Fatalf("got %+v; want %s", got, want)
			}
			for _, port := range got.Ports {
				if port.SpeedMbps != nil || port.LinkState == "down" {
					t.Fatalf("missing data reported as a measurement: %+v", port)
				}
			}
		})
	}
}

func TestEthernetSkipsOfflineAndCancelledQueries(t *testing.T) {
	repo := &mockSnmpRepository{BulkWalkFunc: func(string, func(gosnmp.SnmpPDU) error) error { t.Fatal("unexpected SNMP request"); return nil }}
	uc := NewOnuUsecase(repo, &mockRedisRepository{}, &config.Config{}).(*onuUsecase)
	for _, state := range []string{"Offline", "LOS", "Unknown", ""} {
		got := uc.getEthernetPorts(context.Background(), 2, 4, 6, state, "")
		if got.Status == "ok" || len(got.Ports) != 0 {
			t.Fatalf("offline data exposed: %+v", got)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got := uc.getEthernetPorts(ctx, 2, 4, 6, "Online", ""); got.Status != "unavailable" {
		t.Fatal(got)
	}
}

func TestCorrectedONUTransmitOIDAndSentinel(t *testing.T) {
	bp, err := config.GenerateBoardPonOID(2, 4)
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []int{16000, 65535} {
		repo := &mockSnmpRepository{GetFunc: func(oids []string) (*gosnmp.SnmpPacket, error) {
			want := ".1.3.6.1.4.1.3902.1082.500.20.2.2.2.1.14.285278724.6.1"
			if len(oids) != 1 || oids[0] != want {
				t.Fatalf("TX must use ANI tree and ONU index: %v", oids)
			}
			return &gosnmp.SnmpPacket{Variables: []gosnmp.SnmpPDU{{Name: want, Type: gosnmp.Integer, Value: raw}}}, nil
		}}
		uc := NewOnuUsecase(repo, &mockRedisRepository{}, &config.Config{OltCfg: config.OltConfig{BaseOID1: config.BaseOID1, BaseOID2: config.BaseOID2}}).(*onuUsecase)
		value, err := uc.getTxPower(bp.OnuTxPowerOID, "6")
		if raw == 65535 {
			if err == nil || value != "" {
				t.Fatalf("unavailable optical sentinel converted to %q", value)
			}
		} else if err != nil || value != "2.00" {
			t.Fatalf("TX conversion: %q %v", value, err)
		}
	}
}

func TestCollectPONInventoryTxAndEthernet(t *testing.T) {
	bp, err := config.GenerateBoardPonOID(2, 4)
	if err != nil {
		t.Fatal(err)
	}
	ponIndex := config.OnuIDIfIndexBase + 2*config.OnuIDSlotStride + 4*config.OnuIDIncrement
	walks := 0
	repo := &mockSnmpRepository{
		BulkWalkFunc: func(root string, walk func(gosnmp.SnmpPDU) error) error {
			walks++
			if strings.Contains(root, config.OnuEthAdminPrefix) ||
				strings.Contains(root, config.OnuEthOperPrefix) ||
				strings.Contains(root, config.OnuEthSpeedPrefix) {
				if strings.Contains(root, fmt.Sprintf(".%d.", ponIndex)) {
					t.Fatalf("ethernet walk must be PON-scoped, got %s", root)
				}
				values := []int{1, 1}
				if strings.Contains(root, config.OnuEthOperPrefix) {
					values = []int{1, 2}
				}
				if strings.Contains(root, config.OnuEthSpeedPrefix) {
					values = []int{6, 6}
				}
				if err := walk(gosnmp.SnmpPDU{Name: root + ".6.1", Type: gosnmp.Integer, Value: values[0]}); err != nil {
					return err
				}
				// Stale UNI on an offline ONU and an unknown subscriber must not leak.
				if err := walk(gosnmp.SnmpPDU{Name: root + ".7.1", Type: gosnmp.Integer, Value: values[1]}); err != nil {
					return err
				}
				return walk(gosnmp.SnmpPDU{Name: root + ".99.1", Type: gosnmp.Integer, Value: 1})
			}
			if err := walk(gosnmp.SnmpPDU{Name: root + ".6", Type: gosnmp.OctetString, Value: []byte("OnlineONU")}); err != nil {
				return err
			}
			return walk(gosnmp.SnmpPDU{Name: root + ".7", Type: gosnmp.OctetString, Value: []byte("OfflineONU")})
		},
		GetFunc: func(oids []string) (*gosnmp.SnmpPacket, error) {
			if len(oids) != 5 {
				t.Fatalf("PON GET batch must include TX as 5th OID: %v", oids)
			}
			if !strings.Contains(oids[4], config.OnuTxPowerPrefix) || !strings.HasSuffix(oids[4], ".1") {
				t.Fatalf("TX OID = %s", oids[4])
			}
			status := 4
			if strings.HasSuffix(oids[3], ".7") {
				status = 7
			}
			return &gosnmp.SnmpPacket{Variables: []gosnmp.SnmpPDU{
				{Name: oids[0], Type: gosnmp.OctetString, Value: []byte("SYNTHETIC-ONU")},
				{Name: oids[1], Type: gosnmp.OctetString, Value: []byte("TEST00000001")},
				{Name: oids[2], Type: gosnmp.Integer, Value: 5000},
				{Name: oids[3], Type: gosnmp.Integer, Value: status},
				{Name: oids[4], Type: gosnmp.Integer, Value: 16000},
			}}, nil
		},
	}
	cfg := &config.Config{
		OltCfg:      config.OltConfig{BaseOID1: config.BaseOID1, BaseOID2: config.BaseOID2},
		CacheCfg:    config.CacheConfig{ONUInfoTTL: 60},
		BoardPonMap: map[config.BoardPonKey]*config.BoardPonConfig{{BoardID: 2, PonID: 4}: bp},
	}
	uc := NewOnuUsecase(repo, &mockRedisRepository{}, cfg)
	got, err := uc.CollectPONInventory(context.Background(), 2, 4)
	if err != nil {
		t.Fatal(err)
	}
	if walks != 4 {
		t.Fatalf("walks = %d, want 1 name + 3 ethernet", walks)
	}
	if len(got) != 2 {
		t.Fatalf("onus = %d", len(got))
	}
	online, offline := got[0], got[1]
	if online.ID != 6 || online.TXPower != "2.00" || online.RXPower != "-20.00" || online.Status != "Online" {
		t.Fatalf("online = %+v", online)
	}
	if online.Ethernet.Status != "ok" || len(online.Ethernet.Ports) != 1 || online.Ethernet.Ports[0].LinkState != "up" {
		t.Fatalf("online ethernet = %+v", online.Ethernet)
	}
	if offline.ID != 7 || offline.Status != "Offline" || offline.Ethernet.Status != "onu_not_online" || len(offline.Ethernet.Ports) != 0 {
		t.Fatalf("offline ethernet leaked: %+v", offline)
	}
}

func TestAttachPONEthernetSkipsWalkWhenAllOffline(t *testing.T) {
	repo := &mockSnmpRepository{BulkWalkFunc: func(string, func(gosnmp.SnmpPDU) error) error {
		t.Fatal("ethernet walk must be skipped when no ONU is online")
		return nil
	}}
	uc := NewOnuUsecase(repo, &mockRedisRepository{}, &config.Config{OltCfg: config.OltConfig{BaseOID1: config.BaseOID1}}).(*onuUsecase)
	onus := []model.ONUInfoPerBoard{{ID: 1, Status: "Offline"}, {ID: 2, Status: "LOS"}}
	uc.attachPONEthernet(context.Background(), 2, 4, onus)
	if onus[0].Ethernet.Status != "onu_not_online" || onus[1].Ethernet.Status != "onu_not_online" {
		t.Fatalf("%+v", onus)
	}
}
