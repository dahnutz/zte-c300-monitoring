package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

func (t *Timescale) ListDuplicateSerials(ctx context.Context, deviceID, scope string) (DuplicateSerialList, error) {
	scope = NormalizeDuplicateScope(scope)
	if scope == "" {
		scope = DuplicateScopeCurrent
	}
	latest, err := t.latestFinishedCollectionRun(ctx, deviceID)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return DuplicateSerialList{}, err
	}
	if scope == DuplicateScopeCurrent && latest.ID == 0 {
		return DuplicateSerialList{Scope: scope, Serials: []DuplicateSerial{}}, nil
	}

	var from, to time.Time
	if latest.FinishedAt != nil {
		from, to = latest.StartedAt, *latest.FinishedAt
	}

	rows, err := t.pool.Query(ctx, `
WITH pos AS (
    SELECT serial, board, pon, onu_id,
           MAX(time) AS last_seen,
           (ARRAY_AGG(status ORDER BY time DESC))[1] AS status,
           (ARRAY_AGG(name ORDER BY time DESC))[1] AS name,
           COUNT(*)::int AS samples,
           BOOL_OR($2::timestamptz IS NOT NULL AND time >= $2 AND time <= $3) AS in_latest
    FROM onu_samples
    WHERE device_id = $1 AND serial <> ''
      AND ($4 = $5 OR ($2::timestamptz IS NOT NULL AND time >= $2 AND time <= $3))
    GROUP BY serial, board, pon, onu_id
)
SELECT serial, board, pon, onu_id, last_seen, COALESCE(status, ''), COALESCE(name, ''), samples, in_latest
FROM pos
WHERE serial IN (
    SELECT serial FROM pos GROUP BY serial
    HAVING COUNT(*) > 1 OR COUNT(*) FILTER (WHERE in_latest) > 1
)`, deviceID, nullTime(from), nullTime(to), scope, DuplicateScopeHistory)
	if err != nil {
		return DuplicateSerialList{}, err
	}
	defer rows.Close()

	var acc []duplicatePosRow
	for rows.Next() {
		var row duplicatePosRow
		if err := rows.Scan(
			&row.Serial, &row.Board, &row.PON, &row.ONUID, &row.LastSeen,
			&row.Status, &row.Name, &row.Samples, &row.InLatest,
		); err != nil {
			return DuplicateSerialList{}, err
		}
		acc = append(acc, row)
	}
	if err := rows.Err(); err != nil {
		return DuplicateSerialList{}, err
	}
	return assembleDuplicateSerials(deviceID, latest, scope, acc), nil
}

func nullTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
}

func (t *Timescale) latestFinishedCollectionRun(ctx context.Context, deviceID string) (CollectionRun, error) {
	row := t.pool.QueryRow(ctx, `
SELECT id, device_id, started_at, finished_at, status, pons_ok, pons_error, onus_sampled, duration_ms, error
FROM collection_runs
WHERE device_id = $1 AND finished_at IS NOT NULL AND status <> 'running'
ORDER BY id DESC
LIMIT 1`, deviceID)
	run, err := scanCollectionRun(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return CollectionRun{}, fmt.Errorf("%w: no collection runs", ErrNotFound)
	}
	return run, err
}
