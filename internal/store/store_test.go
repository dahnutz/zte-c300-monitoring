package store

import (
	"errors"
	"testing"
	"time"
)

func TestParseRxPower(t *testing.T) {
	if got := ParseRxPower(""); got != nil {
		t.Fatalf("empty = %v, want nil", got)
	}
	if got := ParseRxPower("  "); got != nil {
		t.Fatalf("blank = %v, want nil", got)
	}
	if got := ParseRxPower("nope"); got != nil {
		t.Fatalf("invalid = %v, want nil", got)
	}
	got := ParseRxPower("-18.50")
	if got == nil || *got != -18.50 {
		t.Fatalf("got %v", got)
	}
}

func TestMemoryStoreRoundTrip(t *testing.T) {
	mem := NewMemory()
	ctx := t.Context()
	rx := -20.0
	samples := []ONUSample{{
		Time: time.Now().UTC(), DeviceID: "default", Serial: "ZTEG1", Board: 3, PON: 1, ONUID: 1,
		Name: "A", Status: "Online", RXPower: &rx,
	}}
	if err := mem.UpsertDevice(ctx, Device{ID: "default", Vendor: "zte", Family: "c300", Role: "olt"}); err != nil {
		t.Fatal(err)
	}
	if err := mem.InsertSamples(ctx, samples); err != nil {
		t.Fatal(err)
	}
	if err := mem.UpsertONUs(ctx, samples); err != nil {
		t.Fatal(err)
	}
	got, err := mem.ListSamples(ctx, SampleQuery{DeviceID: "default", Serial: "ZTEG1", Limit: 10})
	if err != nil || len(got) != 1 {
		t.Fatalf("list = %v err=%v", got, err)
	}
}

func TestSplitSQLSkipsComments(t *testing.T) {
	stmts := splitSQL("-- comment\nCREATE TABLE a (id int);\n-- x\nCREATE TABLE b (id int);")
	if len(stmts) != 2 {
		t.Fatalf("stmts = %#v", stmts)
	}
}

func TestMemoryListSamplesByLatestRunStatusAndCount(t *testing.T) {
	mem := NewMemory()
	ctx := t.Context()
	start := time.Now().UTC().Add(-time.Minute)
	end := time.Now().UTC()
	id, err := mem.InsertCollectionRun(ctx, CollectionRun{DeviceID: "default", StartedAt: start, Status: "running"})
	if err != nil {
		t.Fatal(err)
	}
	if err := mem.FinishCollectionRun(ctx, CollectionRun{
		ID: id, DeviceID: "default", StartedAt: start, FinishedAt: &end, Status: "ok", ONUsSampled: 2,
	}); err != nil {
		t.Fatal(err)
	}
	if err := mem.InsertSamples(ctx, []ONUSample{
		{Time: start.Add(time.Second), DeviceID: "default", Serial: "ON", Board: 3, PON: 1, ONUID: 1, Status: "Online", EthLinkState: "up"},
		{Time: start.Add(2 * time.Second), DeviceID: "default", Serial: "OFF", Board: 8, PON: 1, ONUID: 11, Status: "Offline"},
		{Time: start.Add(-time.Hour), DeviceID: "default", Serial: "OLD", Board: 3, PON: 1, ONUID: 2, Status: "Offline"},
	}); err != nil {
		t.Fatal(err)
	}

	offline, err := mem.ListSamples(ctx, SampleQuery{DeviceID: "default", LatestRun: true, Status: "offline"})
	if err != nil || len(offline) != 1 || offline[0].Serial != "OFF" {
		t.Fatalf("offline last run = %#v err=%v", offline, err)
	}

	counts, err := mem.ListSampleCounts(ctx, SampleQuery{DeviceID: "default", LatestRun: true, CountBy: CountByStatus})
	if err != nil {
		t.Fatal(err)
	}
	if counts.Total != 2 || counts.RunID != id || len(counts.Counts) != 2 {
		t.Fatalf("counts = %#v", counts)
	}

	byBoard, err := mem.ListSampleCounts(ctx, SampleQuery{DeviceID: "default", LatestRun: true, CountBy: CountByBoard})
	if err != nil || byBoard.Total != 2 {
		t.Fatalf("by board = %#v err=%v", byBoard, err)
	}

	byEth, err := mem.ListSampleCounts(ctx, SampleQuery{DeviceID: "default", LatestRun: true, CountBy: CountByEthLink})
	if err != nil || byEth.Total != 2 || byEth.GroupBy != CountByEthLink {
		t.Fatalf("by eth = %#v err=%v", byEth, err)
	}

	_, err = mem.GetCollectionRun(ctx, "default", 999)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing run err=%v", err)
	}
}

func TestMemoryRecordsStatusAndEthTransitions(t *testing.T) {
	mem := NewMemory()
	ctx := t.Context()
	t1 := time.Now().UTC().Add(-time.Minute)
	t2 := time.Now().UTC()
	first := []ONUSample{{
		Time: t1, DeviceID: "default", Serial: "ZTEG1", Board: 3, PON: 1, ONUID: 1,
		OnuType: "F668", Status: "Online",
		EthPorts: []EthPortSample{{Port: 1, Admin: "enabled", Link: "up"}},
	}}
	if err := mem.RecordSampleTransitions(ctx, first); err != nil {
		t.Fatal(err)
	}
	second := []ONUSample{{
		Time: t2, DeviceID: "default", Serial: "ZTEG1", Board: 3, PON: 1, ONUID: 1,
		OnuType: "F668", Status: "Offline",
		EthPorts: []EthPortSample{{Port: 1, Admin: "enabled", Link: "down"}},
	}}
	if err := mem.RecordSampleTransitions(ctx, second); err != nil {
		t.Fatal(err)
	}
	status, err := mem.ListStatusEvents(ctx, EventQuery{DeviceID: "default", Serial: "ZTEG1"})
	if err != nil || len(status) < 2 {
		t.Fatalf("status events = %#v err=%v", status, err)
	}
	eth, err := mem.ListEthEvents(ctx, EventQuery{DeviceID: "default", Serial: "ZTEG1", Port: 1})
	if err != nil || len(eth) < 2 {
		t.Fatalf("eth events = %#v err=%v", eth, err)
	}
	states, err := mem.ListONUStates(ctx, "default")
	if err != nil || len(states) != 1 || states[0].Status != "Offline" || states[0].PreviousStatus != "Online" {
		t.Fatalf("state = %#v err=%v", states, err)
	}
	if states[0].ExpectedEthPorts != 4 {
		t.Fatalf("expected ports = %d", states[0].ExpectedEthPorts)
	}
}

func TestNoDiscoveryIsUnavailable(t *testing.T) {
	list, err := NewMemory().ListUnauth(t.Context(), "example")
	if err != nil || list.Status != "unavailable" || !list.ObservedAt.IsZero() {
		t.Fatalf("no discovery must not assert a fresh empty result: %+v err=%v", list, err)
	}
}
