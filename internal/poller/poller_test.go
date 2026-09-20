package poller

import (
	"context"
	"errors"
	"testing"
	"time"

	"zte-c300-monitoring/config"
	"zte-c300-monitoring/internal/model"
	"zte-c300-monitoring/internal/store"
)

type fakeCollector struct {
	rows []model.ONUInfoPerBoard
	err  error
}

func (f fakeCollector) CollectPONInventory(context.Context, int, int) ([]model.ONUInfoPerBoard, error) {
	return f.rows, f.err
}

type staticRegistry struct {
	targets []DeviceTarget
}

func (s staticRegistry) Targets() []DeviceTarget { return s.targets }

func TestPollerWritesSamples(t *testing.T) {
	mem := store.NewMemory()
	rx := -18.5
	speed := 1000
	collector := fakeCollector{rows: []model.ONUInfoPerBoard{{
		Board: 2, PON: 1, ID: 4, Name: "TEST", OnuType: "SYNTHETIC-ONU",
		SerialNumber: "ZTEGTEST0001", RXPower: "-18.50", TXPower: "2.00", Status: "Online",
		Ethernet: model.ONUEthernetStatus{
			Status: "ok",
			Ports: []model.ONUEthernetPort{{
				PortIndex: 1, AdminState: "enabled", LinkState: "up",
				SpeedMbps: &speed, Duplex: "full",
			}},
		},
	}}}
	p := New(mem, staticRegistry{targets: []DeviceTarget{{
		ID: "default", Vendor: "zte", Family: "c300", Role: "olt",
		Boards:  []int{2},
		Pons:    map[int]int{2: 1},
		Collect: collector,
	}}}, config.PollConfig{Interval: time.Minute})

	ctx := context.Background()
	p.cycle(ctx)

	samples, err := mem.ListSamples(ctx, store.SampleQuery{DeviceID: "default", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != 1 {
		t.Fatalf("samples = %d, want 1", len(samples))
	}
	if samples[0].Serial != "ZTEGTEST0001" || samples[0].Status != "Online" {
		t.Fatalf("sample = %+v", samples[0])
	}
	if samples[0].RXPower == nil || *samples[0].RXPower != rx {
		t.Fatalf("rx = %v want %v", samples[0].RXPower, rx)
	}
	if samples[0].TXPower == nil || *samples[0].TXPower != 2.00 {
		t.Fatalf("tx = %v", samples[0].TXPower)
	}
	if samples[0].EthStatus != "ok" || samples[0].EthLinkState != "up" || samples[0].EthAdminState != "enabled" {
		t.Fatalf("eth summary = %+v", samples[0])
	}
	if samples[0].EthSpeedMbps == nil || *samples[0].EthSpeedMbps != 1000 || len(samples[0].EthPorts) != 1 {
		t.Fatalf("eth ports = %+v", samples[0].EthPorts)
	}
	runs, err := mem.ListCollectionRuns(ctx, "default", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].Status != "ok" || runs[0].ONUsSampled != 1 {
		t.Fatalf("runs = %+v", runs)
	}
}

func TestPollerPONErrorIsPartial(t *testing.T) {
	mem := store.NewMemory()
	p := New(mem, staticRegistry{targets: []DeviceTarget{{
		ID:     "default",
		Boards: []int{3},
		Pons:   map[int]int{3: 1},
		Collect: fakeCollector{
			err: errors.New("snmp timeout"),
		},
	}}}, config.PollConfig{Interval: time.Minute})

	p.cycle(context.Background())
	runs, err := mem.ListCollectionRuns(context.Background(), "default", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 {
		t.Fatalf("runs = %d", len(runs))
	}
	if runs[0].Status != "partial" || runs[0].PonsError != 1 {
		t.Fatalf("run = %+v", runs[0])
	}
}
