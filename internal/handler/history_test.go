package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"zte-c300-monitoring/internal/reqctx"
	"zte-c300-monitoring/internal/store"
	"zte-c300-monitoring/internal/utils"
)

func TestHistoryHandlerListSamples(t *testing.T) {
	mem := store.NewMemory()
	rx := -14.6
	_ = mem.InsertSamples(t.Context(), []store.ONUSample{{
		Time: time.Now().UTC(), DeviceID: "default", Serial: "ZTEG1",
		Board: 8, PON: 1, ONUID: 11, Status: "Online", RXPower: &rx,
	}})
	h := NewHistoryHandler(mem)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/history/samples?serial=ZTEG1", nil)
	req = req.WithContext(reqctx.WithDeviceID(req.Context(), "default"))
	rr := httptest.NewRecorder()
	h.ListSamples(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var body utils.WebResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
}

func TestHistoryHandlerMissingStore(t *testing.T) {
	h := NewHistoryHandler(nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/history/runs", nil)
	req = req.WithContext(reqctx.WithDeviceID(req.Context(), "default"))
	rr := httptest.NewRecorder()
	h.ListCollectionRuns(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestHistoryHandlerLastRunOfflineAndCountBy(t *testing.T) {
	mem := store.NewMemory()
	ctx := t.Context()
	start := time.Now().UTC().Add(-time.Minute)
	end := time.Now().UTC()
	id, err := mem.InsertCollectionRun(ctx, store.CollectionRun{DeviceID: "default", StartedAt: start, Status: "running"})
	if err != nil {
		t.Fatal(err)
	}
	_ = mem.FinishCollectionRun(ctx, store.CollectionRun{
		ID: id, DeviceID: "default", StartedAt: start, FinishedAt: &end, Status: "ok",
	})
	_ = mem.InsertSamples(ctx, []store.ONUSample{
		{Time: start.Add(time.Second), DeviceID: "default", Serial: "ON", Board: 3, PON: 1, ONUID: 1, Status: "Online"},
		{Time: start.Add(2 * time.Second), DeviceID: "default", Serial: "OFF", Board: 3, PON: 2, ONUID: 4, Status: "Offline"},
	})
	h := NewHistoryHandler(mem)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/history/samples?run=latest&status=Offline", nil)
	req = req.WithContext(reqctx.WithDeviceID(req.Context(), "default"))
	rr := httptest.NewRecorder()
	h.ListSamples(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var listed struct {
		Data []store.ONUSample `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Data) != 1 || listed.Data[0].Serial != "OFF" {
		t.Fatalf("listed = %#v", listed.Data)
	}

	countReq := httptest.NewRequest(http.MethodGet, "/api/v1/history/samples?run=latest&count_by=status", nil)
	countReq = countReq.WithContext(reqctx.WithDeviceID(countReq.Context(), "default"))
	countRR := httptest.NewRecorder()
	h.ListSamples(countRR, countReq)
	if countRR.Code != http.StatusOK {
		t.Fatalf("count status=%d body=%s", countRR.Code, countRR.Body.String())
	}
	var counted struct {
		Data store.CountResult `json:"data"`
	}
	if err := json.Unmarshal(countRR.Body.Bytes(), &counted); err != nil {
		t.Fatal(err)
	}
	if counted.Data.Total != 2 || counted.Data.GroupBy != "status" {
		t.Fatalf("counts = %#v", counted.Data)
	}

	ethReq := httptest.NewRequest(http.MethodGet, "/api/v1/history/samples?run=latest&count_by=eth_link", nil)
	ethReq = ethReq.WithContext(reqctx.WithDeviceID(ethReq.Context(), "default"))
	ethRR := httptest.NewRecorder()
	h.ListSamples(ethRR, ethReq)
	if ethRR.Code != http.StatusOK {
		t.Fatalf("eth_link status=%d body=%s", ethRR.Code, ethRR.Body.String())
	}
}

func TestHistoryHandlerInvalidCountByAndMissingRun(t *testing.T) {
	h := NewHistoryHandler(store.NewMemory())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/history/samples?count_by=vendor", nil)
	req = req.WithContext(reqctx.WithDeviceID(req.Context(), "default"))
	rr := httptest.NewRecorder()
	h.ListSamples(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}

	missing := httptest.NewRequest(http.MethodGet, "/api/v1/history/samples?run=latest", nil)
	missing = missing.WithContext(reqctx.WithDeviceID(missing.Context(), "default"))
	missRR := httptest.NewRecorder()
	h.ListSamples(missRR, missing)
	if missRR.Code != http.StatusNotFound {
		t.Fatalf("missing run status=%d body=%s", missRR.Code, missRR.Body.String())
	}
}

func TestHistoryHandlerEventsAndUnauth(t *testing.T) {
	mem := store.NewMemory()
	ctx := t.Context()
	now := time.Now().UTC()
	_ = mem.RecordSampleTransitions(ctx, []store.ONUSample{{
		Time: now, DeviceID: "default", Serial: "ZTEG1", Status: "Online",
		EthPorts: []store.EthPortSample{{Port: 1, Admin: "enabled", Link: "up"}},
	}})
	_ = mem.ReplaceUnauth(ctx, "default", store.UnauthList{
		Status: "ok", ObservedAt: now, Count: 1,
		ONUs: []store.UnauthONU{{Serial: "ZTEGNEW", FirstSeenAt: now, LastSeenAt: now}},
	})
	h := NewHistoryHandler(mem)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/history/status-events?serial=ZTEG1", nil)
	req = req.WithContext(reqctx.WithDeviceID(req.Context(), "default"))
	rr := httptest.NewRecorder()
	h.ListStatusEvents(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status events=%d body=%s", rr.Code, rr.Body.String())
	}

	ethReq := httptest.NewRequest(http.MethodGet, "/api/v1/history/eth-events?serial=ZTEG1&port=1", nil)
	ethReq = ethReq.WithContext(reqctx.WithDeviceID(ethReq.Context(), "default"))
	ethRR := httptest.NewRecorder()
	h.ListEthEvents(ethRR, ethReq)
	if ethRR.Code != http.StatusOK {
		t.Fatalf("eth events=%d body=%s", ethRR.Code, ethRR.Body.String())
	}

	unauthReq := httptest.NewRequest(http.MethodGet, "/api/v1/history/unauth", nil)
	unauthReq = unauthReq.WithContext(reqctx.WithDeviceID(unauthReq.Context(), "default"))
	unauthRR := httptest.NewRecorder()
	h.ListUnauth(unauthRR, unauthReq)
	if unauthRR.Code != http.StatusOK {
		t.Fatalf("unauth=%d body=%s", unauthRR.Code, unauthRR.Body.String())
	}
	var body struct {
		Data store.UnauthList `json:"data"`
	}
	if err := json.Unmarshal(unauthRR.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.Count != 1 || len(body.Data.ONUs) != 1 {
		t.Fatalf("unauth = %#v", body.Data)
	}
}
