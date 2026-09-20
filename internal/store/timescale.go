package store

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"zte-c300-monitoring/config"
	"zte-c300-monitoring/internal/model"
)

//go:embed schema.sql
var schemaSQL string

const schemaVersion = 3

// Timescale is a Store backed by TimescaleDB (PostgreSQL).
type Timescale struct {
	pool *pgxpool.Pool
}

// OpenTimescale connects, runs schema migrations, and returns a Store.
func OpenTimescale(ctx context.Context, cfg config.StoreConfig) (*Timescale, error) {
	if cfg.Host == "" {
		return nil, fmt.Errorf("timescaledb host is empty")
	}
	pool, err := pgxpool.New(ctx, cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("timescaledb connect: %w", err)
	}
	store := &Timescale{pool: pool}
	if err := store.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	if err := store.migrate(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return store, nil
}

func (t *Timescale) Ping(ctx context.Context) error {
	return t.pool.Ping(ctx)
}

func (t *Timescale) Close() {
	if t != nil && t.pool != nil {
		t.pool.Close()
	}
}

func (t *Timescale) migrate(ctx context.Context) error {
	for _, stmt := range splitSQL(schemaSQL) {
		if _, err := t.pool.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("apply schema: %w", err)
		}
	}
	var current int
	err := t.pool.QueryRow(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&current)
	if err != nil {
		return fmt.Errorf("schema version: %w", err)
	}
	if current < 1 {
		if _, err := t.pool.Exec(ctx, `SELECT create_hypertable('onu_samples', 'time', if_not_exists => TRUE)`); err != nil {
			return fmt.Errorf("create hypertable: %w", err)
		}
		if _, err := t.pool.Exec(ctx, `CREATE INDEX IF NOT EXISTS onu_samples_device_serial_time_idx ON onu_samples (device_id, serial, time DESC)`); err != nil {
			return fmt.Errorf("serial index: %w", err)
		}
		if _, err := t.pool.Exec(ctx, `CREATE INDEX IF NOT EXISTS onu_samples_device_loc_time_idx ON onu_samples (device_id, board, pon, onu_id, time DESC)`); err != nil {
			return fmt.Errorf("location index: %w", err)
		}
		if _, err := t.pool.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES (1) ON CONFLICT DO NOTHING`); err != nil {
			return fmt.Errorf("record migration 1: %w", err)
		}
		current = 1
	}
	if current < 2 {
		for _, stmt := range []string{
			`ALTER TABLE onu_samples ADD COLUMN IF NOT EXISTS tx_power double precision`,
			`ALTER TABLE onu_samples ADD COLUMN IF NOT EXISTS eth_status text NOT NULL DEFAULT ''`,
			`ALTER TABLE onu_samples ADD COLUMN IF NOT EXISTS eth_link_state text NOT NULL DEFAULT ''`,
			`ALTER TABLE onu_samples ADD COLUMN IF NOT EXISTS eth_admin_state text NOT NULL DEFAULT ''`,
			`ALTER TABLE onu_samples ADD COLUMN IF NOT EXISTS eth_speed_mbps integer`,
			`ALTER TABLE onu_samples ADD COLUMN IF NOT EXISTS eth_ports jsonb`,
		} {
			if _, err := t.pool.Exec(ctx, stmt); err != nil {
				return fmt.Errorf("migrate v2: %w", err)
			}
		}
		if _, err := t.pool.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES (2) ON CONFLICT DO NOTHING`); err != nil {
			return fmt.Errorf("record migration 2: %w", err)
		}
		current = 2
	}
	if current < 3 {
		if err := t.migrateV3(ctx); err != nil {
			return err
		}
	}
	_ = schemaVersion
	return nil
}

func splitSQL(script string) []string {
	var lines []string
	for _, line := range strings.Split(script, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "--") {
			continue
		}
		lines = append(lines, trimmed)
	}
	var out []string
	for _, stmt := range strings.Split(strings.Join(lines, "\n"), ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt != "" {
			out = append(out, stmt)
		}
	}
	return out
}

func (t *Timescale) UpsertDevice(ctx context.Context, device Device) error {
	_, err := t.pool.Exec(ctx, `
INSERT INTO devices (id, vendor, family, role, updated_at)
VALUES ($1, $2, $3, $4, now())
ON CONFLICT (id) DO UPDATE SET
    vendor = EXCLUDED.vendor,
    family = EXCLUDED.family,
    role = EXCLUDED.role,
    updated_at = now()`,
		device.ID, device.Vendor, device.Family, device.Role)
	return err
}

func (t *Timescale) InsertSamples(ctx context.Context, samples []ONUSample) error {
	if len(samples) == 0 {
		return nil
	}
	rows := make([][]any, 0, len(samples))
	for _, sample := range samples {
		applyExpectedPorts(&sample)
		ports, err := json.Marshal(sample.EthPorts)
		if err != nil {
			return err
		}
		if len(sample.EthPorts) == 0 {
			ports = nil
		}
		rows = append(rows, []any{
			sample.Time, sample.DeviceID, sample.Serial, sample.Board, sample.PON, sample.ONUID,
			sample.Name, sample.OnuType, sample.Status, sample.RXPower, sample.TXPower,
			sample.EthStatus, sample.EthLinkState, sample.EthAdminState, sample.EthSpeedMbps, ports,
		})
	}
	_, err := t.pool.CopyFrom(ctx,
		pgx.Identifier{"onu_samples"},
		[]string{
			"time", "device_id", "serial", "board", "pon", "onu_id",
			"name", "onu_type", "status", "rx_power", "tx_power",
			"eth_status", "eth_link_state", "eth_admin_state", "eth_speed_mbps", "eth_ports",
		},
		pgx.CopyFromRows(rows),
	)
	return err
}

func (t *Timescale) UpsertONUs(ctx context.Context, samples []ONUSample) error {
	for _, sample := range samples {
		if strings.TrimSpace(sample.Serial) == "" {
			continue
		}
		if _, err := t.pool.Exec(ctx, `
INSERT INTO onus (device_id, serial, name, onu_type, last_board, last_pon, last_onu_id, last_status, previous_status, status_changed_at, expected_eth_ports, last_seen_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, '', $9, $10, $9, now())
ON CONFLICT (device_id, serial) DO UPDATE SET
    name = EXCLUDED.name,
    onu_type = EXCLUDED.onu_type,
    last_board = EXCLUDED.last_board,
    last_pon = EXCLUDED.last_pon,
    last_onu_id = EXCLUDED.last_onu_id,
    previous_status = CASE WHEN onus.last_status IS DISTINCT FROM EXCLUDED.last_status THEN onus.last_status ELSE onus.previous_status END,
    status_changed_at = CASE WHEN onus.last_status IS DISTINCT FROM EXCLUDED.last_status THEN EXCLUDED.last_seen_at ELSE COALESCE(onus.status_changed_at, EXCLUDED.last_seen_at) END,
    last_status = EXCLUDED.last_status,
    expected_eth_ports = EXCLUDED.expected_eth_ports,
    last_seen_at = EXCLUDED.last_seen_at,
    updated_at = now()`,
			sample.DeviceID, sample.Serial, sample.Name, sample.OnuType,
			sample.Board, sample.PON, sample.ONUID, sample.Status, sample.Time,
			model.ExpectedEthernetPorts(sample.OnuType),
		); err != nil {
			return err
		}
	}
	return nil
}

func (t *Timescale) InsertCollectionRun(ctx context.Context, run CollectionRun) (int64, error) {
	var id int64
	err := t.pool.QueryRow(ctx, `
INSERT INTO collection_runs (device_id, started_at, status, pons_ok, pons_error, onus_sampled, error)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id`,
		run.DeviceID, run.StartedAt, run.Status, run.PonsOK, run.PonsError, run.ONUsSampled, run.Error,
	).Scan(&id)
	return id, err
}

func (t *Timescale) FinishCollectionRun(ctx context.Context, run CollectionRun) error {
	_, err := t.pool.Exec(ctx, `
UPDATE collection_runs
SET finished_at = $2, status = $3, pons_ok = $4, pons_error = $5, onus_sampled = $6, duration_ms = $7, error = $8
WHERE id = $1`,
		run.ID, run.FinishedAt, run.Status, run.PonsOK, run.PonsError, run.ONUsSampled, run.DurationMS, run.Error)
	return err
}

func sampleLimit(q SampleQuery) int {
	limit := q.Limit
	if limit <= 0 {
		limit = 500
	}
	if limit > 2000 {
		limit = 2000
	}
	return limit
}

func countGroupExpr(countBy string) string {
	switch countBy {
	case CountByBoard:
		return "board::text"
	case CountByPON:
		return "pon::text"
	case CountByEthLink:
		return "COALESCE(NULLIF(eth_link_state, ''), '(empty)')"
	case CountByOnuType:
		return "COALESCE(NULLIF(onu_type, ''), '(empty)')"
	default:
		return "COALESCE(NULLIF(status, ''), '(empty)')"
	}
}

func (t *Timescale) ListSamples(ctx context.Context, q SampleQuery) ([]ONUSample, error) {
	if err := ApplyRunWindow(ctx, t, &q); err != nil {
		return nil, err
	}
	from, to := sampleWindow(q)
	limit := sampleLimit(q)
	rows, err := t.pool.Query(ctx, `
SELECT time, device_id, serial, board, pon, onu_id, name, onu_type, status, rx_power,
       tx_power, eth_status, eth_link_state, eth_admin_state, eth_speed_mbps, eth_ports
FROM onu_samples
WHERE device_id = $1
  AND time >= $2 AND time <= $3
  AND ($4 = '' OR serial = $4)
  AND ($5 = 0 OR board = $5)
  AND ($6 = 0 OR pon = $6)
  AND ($7 = 0 OR onu_id = $7)
  AND ($8 = '' OR lower(status) = lower($8))
ORDER BY time DESC
LIMIT $9`,
		q.DeviceID, from, to, q.Serial, q.Board, q.PON, q.ONUID, q.Status, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ONUSample{}
	for rows.Next() {
		var sample ONUSample
		var ports []byte
		if err := rows.Scan(
			&sample.Time, &sample.DeviceID, &sample.Serial, &sample.Board, &sample.PON, &sample.ONUID,
			&sample.Name, &sample.OnuType, &sample.Status, &sample.RXPower, &sample.TXPower,
			&sample.EthStatus, &sample.EthLinkState, &sample.EthAdminState, &sample.EthSpeedMbps, &ports,
		); err != nil {
			return nil, err
		}
		if len(ports) > 0 {
			if err := json.Unmarshal(ports, &sample.EthPorts); err != nil {
				return nil, err
			}
		}
		out = append(out, sample)
	}
	return out, rows.Err()
}

func (t *Timescale) ListSampleCounts(ctx context.Context, q SampleQuery) (CountResult, error) {
	if err := ApplyRunWindow(ctx, t, &q); err != nil {
		return CountResult{}, err
	}
	from, to := sampleWindow(q)
	groupBy := q.CountBy
	if groupBy == "" {
		groupBy = CountByStatus
	}
	query := fmt.Sprintf(`
SELECT %s AS key, COUNT(*)::int
FROM onu_samples
WHERE device_id = $1
  AND time >= $2 AND time <= $3
  AND ($4 = '' OR serial = $4)
  AND ($5 = 0 OR board = $5)
  AND ($6 = 0 OR pon = $6)
  AND ($7 = 0 OR onu_id = $7)
  AND ($8 = '' OR lower(status) = lower($8))
GROUP BY 1
ORDER BY 2 DESC, 1`, countGroupExpr(groupBy))
	rows, err := t.pool.Query(ctx, query,
		q.DeviceID, from, to, q.Serial, q.Board, q.PON, q.ONUID, q.Status)
	if err != nil {
		return CountResult{}, err
	}
	defer rows.Close()
	result := CountResult{
		GroupBy: groupBy,
		RunID:   q.ResolvedRunID,
		From:    from,
		To:      to,
		Counts:  []CountBucket{},
	}
	for rows.Next() {
		var bucket CountBucket
		if err := rows.Scan(&bucket.Key, &bucket.Count); err != nil {
			return CountResult{}, err
		}
		result.Total += bucket.Count
		result.Counts = append(result.Counts, bucket)
	}
	return result, rows.Err()
}

func (t *Timescale) ListCollectionRuns(ctx context.Context, deviceID string, limit int) ([]CollectionRun, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := t.pool.Query(ctx, `
SELECT id, device_id, started_at, finished_at, status, pons_ok, pons_error, onus_sampled, duration_ms, error
FROM collection_runs
WHERE device_id = $1
ORDER BY started_at DESC
LIMIT $2`, deviceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CollectionRun
	for rows.Next() {
		var run CollectionRun
		if err := rows.Scan(
			&run.ID, &run.DeviceID, &run.StartedAt, &run.FinishedAt, &run.Status,
			&run.PonsOK, &run.PonsError, &run.ONUsSampled, &run.DurationMS, &run.Error,
		); err != nil {
			return nil, err
		}
		out = append(out, run)
	}
	return out, rows.Err()
}

func (t *Timescale) GetCollectionRun(ctx context.Context, deviceID string, id int64) (CollectionRun, error) {
	row := t.pool.QueryRow(ctx, `
SELECT id, device_id, started_at, finished_at, status, pons_ok, pons_error, onus_sampled, duration_ms, error
FROM collection_runs
WHERE device_id = $1 AND id = $2`, deviceID, id)
	run, err := scanCollectionRun(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return CollectionRun{}, fmt.Errorf("%w: collection run %d", ErrNotFound, id)
	}
	return run, err
}

func (t *Timescale) LatestCollectionRun(ctx context.Context, deviceID string) (CollectionRun, error) {
	row := t.pool.QueryRow(ctx, `
SELECT id, device_id, started_at, finished_at, status, pons_ok, pons_error, onus_sampled, duration_ms, error
FROM collection_runs
WHERE device_id = $1
ORDER BY started_at DESC
LIMIT 1`, deviceID)
	run, err := scanCollectionRun(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return CollectionRun{}, fmt.Errorf("%w: no collection runs", ErrNotFound)
	}
	return run, err
}

func scanCollectionRun(row interface{ Scan(dest ...any) error }) (CollectionRun, error) {
	var run CollectionRun
	err := row.Scan(
		&run.ID, &run.DeviceID, &run.StartedAt, &run.FinishedAt, &run.Status,
		&run.PonsOK, &run.PonsError, &run.ONUsSampled, &run.DurationMS, &run.Error,
	)
	return run, err
}
