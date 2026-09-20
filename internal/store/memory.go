package store

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

// Memory is an in-process Store for tests. It is not a production backend.
type Memory struct {
	mu           sync.Mutex
	devices      map[string]Device
	onus         map[string]ONUSample
	onuState     map[string]ONUState
	samples      []ONUSample
	runs         []CollectionRun
	statusEvents []StatusEvent
	ethEvents    []EthEvent
	ethState     map[string]EthPortState
	unauth       map[string]UnauthList
	nextRun      int64
}

// NewMemory returns an empty in-memory store.
func NewMemory() *Memory {
	return &Memory{
		devices:  make(map[string]Device),
		onus:     make(map[string]ONUSample),
		onuState: make(map[string]ONUState),
		ethState: make(map[string]EthPortState),
		unauth:   make(map[string]UnauthList),
	}
}

func (m *Memory) Ping(context.Context) error { return nil }

func (m *Memory) Close() {}

func (m *Memory) UpsertDevice(_ context.Context, device Device) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.devices[device.ID] = device
	return nil
}

func (m *Memory) InsertSamples(_ context.Context, samples []ONUSample) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.samples = append(m.samples, samples...)
	return nil
}

func (m *Memory) UpsertONUs(_ context.Context, samples []ONUSample) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, sample := range samples {
		if sample.Serial == "" {
			continue
		}
		m.onus[sample.DeviceID+"|"+sample.Serial] = sample
	}
	return nil
}

func (m *Memory) InsertCollectionRun(_ context.Context, run CollectionRun) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextRun++
	run.ID = m.nextRun
	m.runs = append(m.runs, run)
	return run.ID, nil
}

func (m *Memory) FinishCollectionRun(_ context.Context, run CollectionRun) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.runs {
		if m.runs[i].ID == run.ID {
			m.runs[i] = run
			return nil
		}
	}
	m.runs = append(m.runs, run)
	return nil
}

func (m *Memory) ListSamples(ctx context.Context, q SampleQuery) ([]ONUSample, error) {
	if err := ApplyRunWindow(ctx, m, &q); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]ONUSample, 0, len(m.samples))
	for _, sample := range m.samples {
		if !matchSample(sample, q) {
			continue
		}
		out = append(out, sample)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Time.After(out[j].Time) })
	limit := q.Limit
	if limit <= 0 {
		limit = 500
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (m *Memory) ListSampleCounts(ctx context.Context, q SampleQuery) (CountResult, error) {
	if err := ApplyRunWindow(ctx, m, &q); err != nil {
		return CountResult{}, err
	}
	from, to := sampleWindow(q)
	result := CountResult{GroupBy: q.CountBy, RunID: q.ResolvedRunID, From: from, To: to}
	if result.GroupBy == "" {
		result.GroupBy = CountByStatus
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	tally := map[string]int{}
	for _, sample := range m.samples {
		if !matchSample(sample, q) {
			continue
		}
		tally[sampleGroupKey(sample, result.GroupBy)]++
		result.Total++
	}
	result.Counts = make([]CountBucket, 0, len(tally))
	for key, n := range tally {
		result.Counts = append(result.Counts, CountBucket{Key: key, Count: n})
	}
	sort.Slice(result.Counts, func(i, j int) bool {
		if result.Counts[i].Count == result.Counts[j].Count {
			return result.Counts[i].Key < result.Counts[j].Key
		}
		return result.Counts[i].Count > result.Counts[j].Count
	})
	return result, nil
}

func (m *Memory) GetCollectionRun(_ context.Context, deviceID string, id int64) (CollectionRun, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, run := range m.runs {
		if run.ID == id && (deviceID == "" || run.DeviceID == deviceID) {
			return run, nil
		}
	}
	return CollectionRun{}, fmt.Errorf("%w: collection run %d", ErrNotFound, id)
}

func (m *Memory) LatestCollectionRun(_ context.Context, deviceID string) (CollectionRun, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := len(m.runs) - 1; i >= 0; i-- {
		if deviceID != "" && m.runs[i].DeviceID != deviceID {
			continue
		}
		return m.runs[i], nil
	}
	return CollectionRun{}, fmt.Errorf("%w: no collection runs", ErrNotFound)
}

func (m *Memory) ListCollectionRuns(_ context.Context, deviceID string, limit int) ([]CollectionRun, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]CollectionRun, 0, len(m.runs))
	for i := len(m.runs) - 1; i >= 0; i-- {
		if deviceID != "" && m.runs[i].DeviceID != deviceID {
			continue
		}
		out = append(out, m.runs[i])
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (m *Memory) ListONUStates(_ context.Context, deviceID string) ([]ONUState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []ONUState
	for _, state := range m.onuState {
		if deviceID == "" || state.DeviceID == deviceID {
			out = append(out, state)
		}
	}
	return out, nil
}

func (m *Memory) ListEthState(_ context.Context, deviceID, serial string) ([]EthPortState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []EthPortState
	for _, state := range m.ethState {
		if (deviceID == "" || state.DeviceID == deviceID) && (serial == "" || state.Serial == serial) {
			out = append(out, state)
		}
	}
	return out, nil
}

func (m *Memory) ListStatusEvents(_ context.Context, q EventQuery) ([]StatusEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []StatusEvent
	for i := len(m.statusEvents) - 1; i >= 0; i-- {
		event := m.statusEvents[i]
		if q.DeviceID != "" && event.DeviceID != q.DeviceID {
			continue
		}
		if q.Serial != "" && event.Serial != q.Serial {
			continue
		}
		out = append(out, event)
		if len(out) >= eventLimit(q.Limit) {
			break
		}
	}
	return out, nil
}

func (m *Memory) ListEthEvents(_ context.Context, q EventQuery) ([]EthEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []EthEvent
	for i := len(m.ethEvents) - 1; i >= 0; i-- {
		event := m.ethEvents[i]
		if q.DeviceID != "" && event.DeviceID != q.DeviceID {
			continue
		}
		if q.Serial != "" && event.Serial != q.Serial {
			continue
		}
		if q.Port != 0 && event.Port != q.Port {
			continue
		}
		out = append(out, event)
		if len(out) >= eventLimit(q.Limit) {
			break
		}
	}
	return out, nil
}

func (m *Memory) RecordSampleTransitions(_ context.Context, samples []ONUSample) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, sample := range samples {
		applyExpectedPorts(&sample)
		key := sample.DeviceID + "|" + sample.Serial
		prev := m.onuState[key]
		if statusChanged(prev, sample) {
			m.statusEvents = append(m.statusEvents, statusEventFrom(prev, sample, "poller"))
			changed := sample.Time
			prev.PreviousStatus = prev.Status
			prev.StatusChangedAt = &changed
		}
		prev.DeviceID = sample.DeviceID
		prev.Serial = sample.Serial
		prev.Name = sample.Name
		prev.OnuType = sample.OnuType
		prev.Board = sample.Board
		prev.PON = sample.PON
		prev.ONUID = sample.ONUID
		prev.Status = sample.Status
		prev.ExpectedEthPorts = sample.ExpectedEthPorts
		seen := sample.Time
		prev.LastSeenAt = &seen
		m.onuState[key] = prev
		ethPrev := map[int]EthPortState{}
		for _, state := range m.ethState {
			if state.DeviceID == sample.DeviceID && state.Serial == sample.Serial {
				ethPrev[state.Port] = state
			}
		}
		m.ethEvents = append(m.ethEvents, ethEventsFrom(ethPrev, sample, "poller")...)
		for _, port := range sample.EthPorts {
			pkey := fmt.Sprintf("%s|%d", key, port.Port)
			old := m.ethState[pkey]
			changed := sample.Time
			if old.Serial == "" || old.LinkState != port.Link || old.AdminState != port.Admin {
				old.LastChangedAt = &changed
			}
			old.DeviceID = sample.DeviceID
			old.Serial = sample.Serial
			old.Port = port.Port
			old.AdminState = port.Admin
			old.LinkState = port.Link
			old.SpeedMbps = port.SpeedMbps
			old.Duplex = port.Duplex
			old.LastSeenAt = &changed
			m.ethState[pkey] = old
		}
	}
	return nil
}

func (m *Memory) ReplaceUnauth(_ context.Context, deviceID string, list UnauthList) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if list.ONUs == nil {
		list.ONUs = []UnauthONU{}
	}
	if (list.Status == "unsupported" || list.Status == "unavailable") && len(list.ONUs) == 0 {
		if prev, ok := m.unauth[deviceID]; ok {
			list.ONUs = prev.ONUs
			list.Count = len(prev.ONUs)
		}
	}
	m.unauth[deviceID] = list
	return nil
}

func (m *Memory) ListUnauth(_ context.Context, deviceID string) (UnauthList, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if list, ok := m.unauth[deviceID]; ok {
		return list, nil
	}
	return UnauthList{Status: "unavailable", Message: "no unconfigured-ONU discovery has run yet", ONUs: []UnauthONU{}}, nil
}
