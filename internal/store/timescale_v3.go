package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"zte-c300-monitoring/internal/model"
)

func (t *Timescale) migrateV3(ctx context.Context) error {
	for _, stmt := range []string{
		`ALTER TABLE onus ADD COLUMN IF NOT EXISTS previous_status text NOT NULL DEFAULT ''`,
		`ALTER TABLE onus ADD COLUMN IF NOT EXISTS status_changed_at timestamptz`,
		`ALTER TABLE onus ADD COLUMN IF NOT EXISTS expected_eth_ports integer`,
		`SELECT create_hypertable('onu_status_events', 'time', if_not_exists => TRUE)`,
		`SELECT create_hypertable('onu_eth_events', 'time', if_not_exists => TRUE)`,
		`SELECT create_hypertable('onu_unauth_samples', 'time', if_not_exists => TRUE)`,
		`CREATE INDEX IF NOT EXISTS onu_status_events_serial_time_idx ON onu_status_events (device_id, serial, time DESC)`,
		`CREATE INDEX IF NOT EXISTS onu_eth_events_serial_port_time_idx ON onu_eth_events (device_id, serial, port, time DESC)`,
		`CREATE INDEX IF NOT EXISTS onu_unauth_samples_device_time_idx ON onu_unauth_samples (device_id, time DESC)`,
	} {
		if _, err := t.pool.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("migrate v3 ddl: %w", err)
		}
	}
	if err := t.backfillEventsFromSamples(ctx); err != nil {
		return fmt.Errorf("migrate v3 backfill: %w", err)
	}
	if _, err := t.pool.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES (3) ON CONFLICT DO NOTHING`); err != nil {
		return fmt.Errorf("record migration 3: %w", err)
	}
	return nil
}

func (t *Timescale) backfillEventsFromSamples(ctx context.Context) error {
	if _, err := t.pool.Exec(ctx, `DELETE FROM onu_status_events WHERE source = 'backfill'`); err != nil {
		return err
	}
	if _, err := t.pool.Exec(ctx, `DELETE FROM onu_eth_events WHERE source = 'backfill'`); err != nil {
		return err
	}
	if _, err := t.pool.Exec(ctx, `TRUNCATE onu_eth_state`); err != nil {
		return err
	}
	rows, err := t.pool.Query(ctx, `
SELECT time, device_id, serial, board, pon, onu_id, onu_type, status, eth_ports
FROM onu_samples
WHERE serial <> ''
ORDER BY device_id, serial, time`)
	if err != nil {
		return err
	}
	defer rows.Close()
	prevStatus := map[string]ONUState{}
	prevEth := map[string]map[int]EthPortState{}
	latest := map[string]ONUSample{}
	var statusEvents []StatusEvent
	var ethEvents []EthEvent
	for rows.Next() {
		var sample ONUSample
		var ports []byte
		if err := rows.Scan(
			&sample.Time, &sample.DeviceID, &sample.Serial, &sample.Board, &sample.PON, &sample.ONUID,
			&sample.OnuType, &sample.Status, &ports,
		); err != nil {
			return err
		}
		if len(ports) > 0 {
			if err := json.Unmarshal(ports, &sample.EthPorts); err != nil {
				return err
			}
		}
		applyExpectedPorts(&sample)
		key := sample.DeviceID + "|" + sample.Serial
		if statusChanged(prevStatus[key], sample) {
			statusEvents = append(statusEvents, statusEventFrom(prevStatus[key], sample, "backfill"))
		}
		ethEvents = append(ethEvents, ethEventsFrom(prevEth[key], sample, "backfill")...)
		prevStatus[key] = ONUState{Serial: sample.Serial, Status: sample.Status}
		if prevEth[key] == nil {
			prevEth[key] = map[int]EthPortState{}
		}
		for _, port := range sample.EthPorts {
			prevEth[key][port.Port] = EthPortState{
				Serial: sample.Serial, Port: port.Port, AdminState: port.Admin, LinkState: port.Link,
			}
		}
		latest[key] = sample
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if err := t.insertStatusEvents(ctx, statusEvents); err != nil {
		return err
	}
	if err := t.insertEthEvents(ctx, ethEvents); err != nil {
		return err
	}
	latestSamples := make([]ONUSample, 0, len(latest))
	for _, sample := range latest {
		latestSamples = append(latestSamples, sample)
	}
	if err := t.upsertEthState(ctx, latestSamples); err != nil {
		return err
	}
	if _, err := t.pool.Exec(ctx, `
UPDATE onus o SET
    previous_status = COALESCE(e.previous_status, o.previous_status),
    status_changed_at = COALESCE(e.time, o.status_changed_at, o.last_seen_at)
FROM (
    SELECT DISTINCT ON (device_id, serial) device_id, serial, time, previous_status
    FROM onu_status_events
    ORDER BY device_id, serial, time DESC
) e
WHERE o.device_id = e.device_id AND o.serial = e.serial`); err != nil {
		return err
	}
	if _, err := t.pool.Exec(ctx, `
UPDATE onus SET status_changed_at = COALESCE(status_changed_at, last_seen_at)
WHERE status_changed_at IS NULL`); err != nil {
		return err
	}
	return t.backfillExpectedPorts(ctx)
}

func (t *Timescale) backfillExpectedPorts(ctx context.Context) error {
	rows, err := t.pool.Query(ctx, `SELECT device_id, serial, onu_type FROM onus`)
	if err != nil {
		return err
	}
	defer rows.Close()
	type row struct {
		device, serial, onuType string
	}
	var onus []row
	for rows.Next() {
		var item row
		if err := rows.Scan(&item.device, &item.serial, &item.onuType); err != nil {
			return err
		}
		onus = append(onus, item)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, item := range onus {
		n := model.ExpectedEthernetPorts(item.onuType)
		if n <= 0 {
			continue
		}
		if _, err := t.pool.Exec(ctx, `
UPDATE onus SET expected_eth_ports = $3
WHERE device_id = $1 AND serial = $2 AND (expected_eth_ports IS NULL OR expected_eth_ports = 0)`,
			item.device, item.serial, n); err != nil {
			return err
		}
	}
	return nil
}

func (t *Timescale) insertStatusEvents(ctx context.Context, events []StatusEvent) error {
	if len(events) == 0 {
		return nil
	}
	rows := make([][]any, 0, len(events))
	for _, event := range events {
		rows = append(rows, []any{
			event.Time, event.DeviceID, event.Serial, event.Board, event.PON, event.ONUID,
			event.Status, event.PreviousStatus, event.Source,
		})
	}
	_, err := t.pool.CopyFrom(ctx,
		pgx.Identifier{"onu_status_events"},
		[]string{"time", "device_id", "serial", "board", "pon", "onu_id", "status", "previous_status", "source"},
		pgx.CopyFromRows(rows),
	)
	return err
}

func (t *Timescale) insertEthEvents(ctx context.Context, events []EthEvent) error {
	if len(events) == 0 {
		return nil
	}
	rows := make([][]any, 0, len(events))
	for _, event := range events {
		rows = append(rows, []any{
			event.Time, event.DeviceID, event.Serial, event.Board, event.PON, event.ONUID, event.Port,
			event.LinkState, event.AdminState, event.PreviousLink, event.PreviousAdmin, event.SpeedMbps, event.Source,
		})
	}
	_, err := t.pool.CopyFrom(ctx,
		pgx.Identifier{"onu_eth_events"},
		[]string{"time", "device_id", "serial", "board", "pon", "onu_id", "port", "link_state", "admin_state", "previous_link", "previous_admin", "speed_mbps", "source"},
		pgx.CopyFromRows(rows),
	)
	return err
}

func (t *Timescale) upsertEthState(ctx context.Context, samples []ONUSample) error {
	for _, sample := range samples {
		for _, port := range sample.EthPorts {
			if _, err := t.pool.Exec(ctx, `
INSERT INTO onu_eth_state (device_id, serial, port, admin_state, link_state, speed_mbps, duplex, last_changed_at, last_seen_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$8)
ON CONFLICT (device_id, serial, port) DO UPDATE SET
    last_changed_at = CASE
        WHEN onu_eth_state.link_state IS DISTINCT FROM EXCLUDED.link_state
          OR onu_eth_state.admin_state IS DISTINCT FROM EXCLUDED.admin_state
        THEN EXCLUDED.last_seen_at
        ELSE onu_eth_state.last_changed_at
    END,
    admin_state = EXCLUDED.admin_state,
    link_state = EXCLUDED.link_state,
    speed_mbps = EXCLUDED.speed_mbps,
    duplex = EXCLUDED.duplex,
    last_seen_at = EXCLUDED.last_seen_at`,
				sample.DeviceID, sample.Serial, port.Port, port.Admin, port.Link, port.SpeedMbps, port.Duplex, sample.Time,
			); err != nil {
				return err
			}
		}
	}
	return nil
}

func (t *Timescale) RecordSampleTransitions(ctx context.Context, samples []ONUSample) error {
	if len(samples) == 0 {
		return nil
	}
	states, err := t.ListONUStates(ctx, samples[0].DeviceID)
	if err != nil {
		return err
	}
	bySerial := map[string]ONUState{}
	for _, state := range states {
		bySerial[state.Serial] = state
	}
	var statusEvents []StatusEvent
	var ethEvents []EthEvent
	for i := range samples {
		applyExpectedPorts(&samples[i])
		sample := samples[i]
		if statusChanged(bySerial[sample.Serial], sample) {
			statusEvents = append(statusEvents, statusEventFrom(bySerial[sample.Serial], sample, "poller"))
		}
		eth, err := t.ListEthState(ctx, sample.DeviceID, sample.Serial)
		if err != nil {
			return err
		}
		prev := map[int]EthPortState{}
		for _, port := range eth {
			prev[port.Port] = port
		}
		ethEvents = append(ethEvents, ethEventsFrom(prev, sample, "poller")...)
	}
	if err := t.insertStatusEvents(ctx, statusEvents); err != nil {
		return err
	}
	if err := t.insertEthEvents(ctx, ethEvents); err != nil {
		return err
	}
	return t.upsertEthState(ctx, samples)
}

func (t *Timescale) ListONUStates(ctx context.Context, deviceID string) ([]ONUState, error) {
	rows, err := t.pool.Query(ctx, `
SELECT device_id, serial, name, onu_type, COALESCE(last_board,0), COALESCE(last_pon,0), COALESCE(last_onu_id,0),
       last_status, previous_status, status_changed_at, COALESCE(expected_eth_ports,0), last_seen_at
FROM onus
WHERE device_id = $1
ORDER BY last_board, last_pon, last_onu_id`, deviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ONUState
	for rows.Next() {
		var state ONUState
		if err := rows.Scan(
			&state.DeviceID, &state.Serial, &state.Name, &state.OnuType, &state.Board, &state.PON, &state.ONUID,
			&state.Status, &state.PreviousStatus, &state.StatusChangedAt, &state.ExpectedEthPorts, &state.LastSeenAt,
		); err != nil {
			return nil, err
		}
		out = append(out, state)
	}
	return out, rows.Err()
}

func (t *Timescale) ListEthState(ctx context.Context, deviceID, serial string) ([]EthPortState, error) {
	rows, err := t.pool.Query(ctx, `
SELECT device_id, serial, port, admin_state, link_state, speed_mbps, duplex, last_changed_at, last_seen_at
FROM onu_eth_state
WHERE device_id = $1 AND ($2 = '' OR serial = $2)
ORDER BY serial, port`, deviceID, serial)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []EthPortState
	for rows.Next() {
		var state EthPortState
		if err := rows.Scan(
			&state.DeviceID, &state.Serial, &state.Port, &state.AdminState, &state.LinkState,
			&state.SpeedMbps, &state.Duplex, &state.LastChangedAt, &state.LastSeenAt,
		); err != nil {
			return nil, err
		}
		out = append(out, state)
	}
	return out, rows.Err()
}

func (t *Timescale) ListStatusEvents(ctx context.Context, q EventQuery) ([]StatusEvent, error) {
	rows, err := t.pool.Query(ctx, `
SELECT time, device_id, serial, board, pon, onu_id, status, previous_status, source
FROM onu_status_events
WHERE device_id = $1 AND ($2 = '' OR serial = $2)
ORDER BY time DESC
LIMIT $3`, q.DeviceID, q.Serial, eventLimit(q.Limit))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []StatusEvent
	for rows.Next() {
		var event StatusEvent
		if err := rows.Scan(
			&event.Time, &event.DeviceID, &event.Serial, &event.Board, &event.PON, &event.ONUID,
			&event.Status, &event.PreviousStatus, &event.Source,
		); err != nil {
			return nil, err
		}
		out = append(out, event)
	}
	return out, rows.Err()
}

func (t *Timescale) ListEthEvents(ctx context.Context, q EventQuery) ([]EthEvent, error) {
	rows, err := t.pool.Query(ctx, `
SELECT time, device_id, serial, board, pon, onu_id, port, link_state, admin_state, previous_link, previous_admin, speed_mbps, source
FROM onu_eth_events
WHERE device_id = $1 AND ($2 = '' OR serial = $2) AND ($3 = 0 OR port = $3)
ORDER BY time DESC
LIMIT $4`, q.DeviceID, q.Serial, q.Port, eventLimit(q.Limit))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []EthEvent
	for rows.Next() {
		var event EthEvent
		if err := rows.Scan(
			&event.Time, &event.DeviceID, &event.Serial, &event.Board, &event.PON, &event.ONUID, &event.Port,
			&event.LinkState, &event.AdminState, &event.PreviousLink, &event.PreviousAdmin, &event.SpeedMbps, &event.Source,
		); err != nil {
			return nil, err
		}
		out = append(out, event)
	}
	return out, rows.Err()
}

func (t *Timescale) ReplaceUnauth(ctx context.Context, deviceID string, list UnauthList) error {
	tx, err := t.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `
INSERT INTO onu_unauth_discovery (device_id, status, oid, message, observed_at, count)
VALUES ($1,$2,$3,$4,$5,$6)
ON CONFLICT (device_id) DO UPDATE SET
    status = EXCLUDED.status, oid = EXCLUDED.oid, message = EXCLUDED.message,
    observed_at = EXCLUDED.observed_at, count = EXCLUDED.count`,
		deviceID, list.Status, list.OID, list.Message, list.ObservedAt, list.Count,
	); err != nil {
		return err
	}
	if list.Status == "ok" || list.Status == "empty" {
		if _, err := tx.Exec(ctx, `UPDATE onu_unauth SET gone_at = $2 WHERE device_id = $1 AND gone_at IS NULL`, deviceID, list.ObservedAt); err != nil {
			return err
		}
	}
	for _, onu := range list.ONUs {
		if _, err := tx.Exec(ctx, `
INSERT INTO onu_unauth (device_id, serial, board, pon, onu_type, discovery_status, first_seen_at, last_seen_at, gone_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$7,NULL)
ON CONFLICT (device_id, serial) DO UPDATE SET
    board = EXCLUDED.board, pon = EXCLUDED.pon, onu_type = EXCLUDED.onu_type,
    discovery_status = EXCLUDED.discovery_status, last_seen_at = EXCLUDED.last_seen_at, gone_at = NULL`,
			deviceID, onu.Serial, onu.Board, onu.PON, onu.OnuType, list.Status, list.ObservedAt,
		); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
INSERT INTO onu_unauth_samples (time, device_id, serial, board, pon, onu_type, discovery_status)
VALUES ($1,$2,$3,$4,$5,$6,$7)`,
			list.ObservedAt, deviceID, onu.Serial, onu.Board, onu.PON, onu.OnuType, list.Status,
		); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (t *Timescale) ListUnauth(ctx context.Context, deviceID string) (UnauthList, error) {
	list := UnauthList{ONUs: []UnauthONU{}, Status: "unavailable"}
	row := t.pool.QueryRow(ctx, `
SELECT status, oid, message, observed_at, count FROM onu_unauth_discovery WHERE device_id = $1`, deviceID)
	if err := row.Scan(&list.Status, &list.OID, &list.Message, &list.ObservedAt, &list.Count); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			list.Status = "unavailable"
			list.Message = "no unconfigured-ONU discovery has run yet"
			return list, nil
		}
		return UnauthList{}, err
	}
	rows, err := t.pool.Query(ctx, `
SELECT device_id, serial, board, pon, onu_type, discovery_status, first_seen_at, last_seen_at, gone_at
FROM onu_unauth
WHERE device_id = $1 AND gone_at IS NULL
ORDER BY board, pon, serial`, deviceID)
	if err != nil {
		return UnauthList{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var onu UnauthONU
		if err := rows.Scan(
			&onu.DeviceID, &onu.Serial, &onu.Board, &onu.PON, &onu.OnuType, &onu.DiscoveryStatus,
			&onu.FirstSeenAt, &onu.LastSeenAt, &onu.GoneAt,
		); err != nil {
			return UnauthList{}, err
		}
		list.ONUs = append(list.ONUs, onu)
	}
	list.Count = len(list.ONUs)
	return list, rows.Err()
}

// AttachONUState copies current status-change fields onto matching samples.
func AttachONUState(samples []ONUSample, states []ONUState) {
	by := map[string]ONUState{}
	for _, state := range states {
		by[state.Serial] = state
	}
	for i := range samples {
		applyExpectedPorts(&samples[i])
		state, ok := by[samples[i].Serial]
		if !ok {
			continue
		}
		samples[i].StatusChangedAt = state.StatusChangedAt
		samples[i].PreviousStatus = state.PreviousStatus
		if state.ExpectedEthPorts > 0 {
			samples[i].ExpectedEthPorts = state.ExpectedEthPorts
		}
	}
}
