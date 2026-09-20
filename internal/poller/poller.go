package poller

import (
	"context"
	"fmt"
	"sort"
	"time"

	"go.uber.org/zap"
	"zte-c300-monitoring/config"
	"zte-c300-monitoring/internal/device"
	"zte-c300-monitoring/internal/model"
	"zte-c300-monitoring/internal/store"
	"zte-c300-monitoring/pkg/logger"
	"zte-c300-monitoring/pkg/metrics"
)

// InventoryCollector is the read-only SNMP surface the poller needs.
type InventoryCollector interface {
	CollectPONInventory(ctx context.Context, boardID, ponID int) ([]model.ONUInfoPerBoard, error)
}

// UnconfiguredCollector is optional. When present, each cycle also walks the
// firmware unconfigured-ONU table and stores it without touching onu_samples.
type UnconfiguredCollector interface {
	DiscoverUnconfiguredONUs(ctx context.Context) (model.UnauthDiscovery, error)
}

// DeviceTarget is one OLT (or future switch) the poller walks sequentially.
type DeviceTarget struct {
	ID      string
	Vendor  string
	Family  string
	Role    string
	Boards  []int
	Pons    map[int]int
	Collect InventoryCollector
}

// Registry lists devices currently served by the collector process.
type Registry interface {
	Targets() []DeviceTarget
}

// Poller writes PON-list snapshots into the durable store. It never issues
// SNMP writes.
type Poller struct {
	store    store.Store
	registry Registry
	interval time.Duration
	delay    time.Duration
}

// New constructs a poller. interval is clamped to at least 30s by config.
func New(st store.Store, registry Registry, cfg config.PollConfig) *Poller {
	return &Poller{
		store:    st,
		registry: registry,
		interval: cfg.Interval,
		delay:    cfg.StartDelay,
	}
}

// Run blocks until ctx is cancelled. One full device pass at a time.
func (p *Poller) Run(ctx context.Context) {
	if p.delay > 0 {
		timer := time.NewTimer(p.delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
	p.cycle(ctx)
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			logger.Info("poller_stopped")
			return
		case <-ticker.C:
			p.cycle(ctx)
		}
	}
}

func (p *Poller) cycle(ctx context.Context) {
	for _, target := range p.registry.Targets() {
		if ctx.Err() != nil {
			return
		}
		p.pollDevice(ctx, target)
	}
}

func (p *Poller) pollDevice(ctx context.Context, target DeviceTarget) {
	started := time.Now().UTC()
	run := store.CollectionRun{
		DeviceID:  target.ID,
		StartedAt: started,
		Status:    "running",
	}
	id, err := p.store.InsertCollectionRun(ctx, run)
	if err != nil {
		logger.Error("poller_run_insert_failed", zap.String("device_id", target.ID), zap.Error(err))
		return
	}
	run.ID = id

	if err := p.store.UpsertDevice(ctx, store.Device{
		ID:     target.ID,
		Vendor: orDefault(target.Vendor, device.VendorZTE),
		Family: orDefault(target.Family, device.FamilyC300),
		Role:   orDefault(target.Role, device.RoleOLT),
	}); err != nil {
		p.finish(ctx, &run, started, err)
		return
	}

	slots := append([]int(nil), target.Boards...)
	sort.Ints(slots)
	for _, board := range slots {
		pons := target.Pons[board]
		for pon := 1; pon <= pons; pon++ {
			if ctx.Err() != nil {
				p.finish(ctx, &run, started, ctx.Err())
				return
			}
			sampled, err := p.pollPON(ctx, target, board, pon)
			if err != nil {
				run.PonsError++
				logger.Warn("poller_pon_failed",
					zap.String("device_id", target.ID),
					zap.Int("board", board),
					zap.Int("pon", pon),
					zap.Error(err),
				)
				continue
			}
			run.PonsOK++
			run.ONUsSampled += sampled
		}
	}
	p.discoverUnconfigured(ctx, target)
	p.finish(ctx, &run, started, nil)
}

func (p *Poller) pollPON(ctx context.Context, target DeviceTarget, board, pon int) (int, error) {
	observedAt := time.Now().UTC()
	rows, err := target.Collect.CollectPONInventory(ctx, board, pon)
	if err != nil {
		return 0, err
	}
	samples := make([]store.ONUSample, 0, len(rows))
	for _, row := range rows {
		samples = append(samples, sampleFromInventory(observedAt, target.ID, row))
	}
	if err := p.store.RecordSampleTransitions(ctx, samples); err != nil {
		return 0, fmt.Errorf("record transitions: %w", err)
	}
	if err := p.store.InsertSamples(ctx, samples); err != nil {
		return 0, fmt.Errorf("insert samples: %w", err)
	}
	if err := p.store.UpsertONUs(ctx, samples); err != nil {
		return 0, fmt.Errorf("upsert onus: %w", err)
	}
	return len(samples), nil
}

func (p *Poller) discoverUnconfigured(ctx context.Context, target DeviceTarget) {
	c, ok := target.Collect.(UnconfiguredCollector)
	if !ok {
		return
	}
	found, err := c.DiscoverUnconfiguredONUs(ctx)
	if err != nil {
		logger.Warn("poller_unauth_failed", zap.String("device_id", target.ID), zap.Error(err))
		_ = p.store.ReplaceUnauth(ctx, target.ID, store.UnauthList{
			Status:     "unavailable",
			Message:    err.Error(),
			ObservedAt: time.Now().UTC(),
			ONUs:       []store.UnauthONU{},
		})
		return
	}
	known := map[string]struct{}{}
	if states, err := p.store.ListONUStates(ctx, target.ID); err == nil {
		for _, state := range states {
			if state.Serial != "" {
				known[state.Serial] = struct{}{}
			}
		}
	}
	list := store.UnauthList{
		Status:     found.Status,
		OID:        found.OID,
		Message:    found.Message,
		ObservedAt: found.ObservedAt,
		ONUs:       []store.UnauthONU{},
	}
	for _, onu := range found.ONUs {
		if _, exists := known[onu.Serial]; exists {
			continue
		}
		list.ONUs = append(list.ONUs, store.UnauthONU{
			DeviceID:        target.ID,
			Serial:          onu.Serial,
			Board:           onu.Board,
			PON:             onu.PON,
			OnuType:         onu.OnuType,
			DiscoveryStatus: found.Status,
			FirstSeenAt:     found.ObservedAt,
			LastSeenAt:      found.ObservedAt,
		})
	}
	list.Count = len(list.ONUs)
	if err := p.store.ReplaceUnauth(ctx, target.ID, list); err != nil {
		logger.Warn("poller_unauth_store_failed", zap.String("device_id", target.ID), zap.Error(err))
	}
}

func (p *Poller) finish(ctx context.Context, run *store.CollectionRun, started time.Time, runErr error) {
	finished := time.Now().UTC()
	run.FinishedAt = &finished
	ms := int(finished.Sub(started).Milliseconds())
	run.DurationMS = &ms
	if runErr != nil {
		run.Status = "error"
		run.Error = runErr.Error()
	} else if run.PonsError > 0 {
		run.Status = "partial"
	} else {
		run.Status = "ok"
		run.Error = ""
	}
	if err := p.store.FinishCollectionRun(ctx, *run); err != nil {
		logger.Error("poller_run_finish_failed", zap.String("device_id", run.DeviceID), zap.Error(err))
	}
	metrics.RecordPollCycle(run.Status, finished.Sub(started), run.ONUsSampled)
	logger.Info("poller_cycle_finished",
		zap.String("device_id", run.DeviceID),
		zap.String("status", run.Status),
		zap.Int("pons_ok", run.PonsOK),
		zap.Int("pons_error", run.PonsError),
		zap.Int("onus_sampled", run.ONUsSampled),
		zap.Int("duration_ms", ms),
	)
}

func sampleFromInventory(observedAt time.Time, deviceID string, row model.ONUInfoPerBoard) store.ONUSample {
	sample := store.ONUSample{
		Time:     observedAt,
		DeviceID: deviceID,
		Serial:   row.SerialNumber,
		Board:    row.Board,
		PON:      row.PON,
		ONUID:    row.ID,
		Name:     row.Name,
		OnuType:  row.OnuType,
		Status:   row.Status,
		RXPower:  store.ParseOpticalPower(row.RXPower),
		TXPower:  store.ParseOpticalPower(row.TXPower),
	}
	applyEthernet(&sample, row.Ethernet)
	return sample
}

func applyEthernet(sample *store.ONUSample, eth model.ONUEthernetStatus) {
	sample.EthStatus = eth.Status
	if eth.Status != "ok" && eth.Status != "partial" {
		return
	}
	ports := make([]store.EthPortSample, 0, len(eth.Ports))
	link := ""
	admin := ""
	var speed *int
	for _, port := range eth.Ports {
		duplex := port.Duplex
		if duplex == "unknown" {
			duplex = ""
		}
		var portSpeed *int
		if port.SpeedMbps != nil {
			copied := *port.SpeedMbps
			portSpeed = &copied
		}
		ports = append(ports, store.EthPortSample{
			Port:      port.PortIndex,
			Admin:     port.AdminState,
			Link:      port.LinkState,
			SpeedMbps: portSpeed,
			Duplex:    duplex,
		})
		switch port.LinkState {
		case "up":
			link = "up"
			if speed == nil && port.SpeedMbps != nil {
				copied := *port.SpeedMbps
				speed = &copied
			}
		case "down":
			if link == "" {
				link = "down"
			}
		}
		switch port.AdminState {
		case "enabled":
			admin = "enabled"
		case "disabled":
			if admin == "" {
				admin = "disabled"
			}
		}
	}
	sample.EthPorts = ports
	sample.EthLinkState = link
	sample.EthAdminState = admin
	sample.EthSpeedMbps = speed
}

func orDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
