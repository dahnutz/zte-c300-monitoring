package store

import (
	"sort"
	"strings"
	"time"
)

const (
	DuplicateScopeCurrent = "current"
	DuplicateScopeHistory = "history"
)

// DuplicatePosition is one board/PON/ONU-ID where a serial was observed.
type DuplicatePosition struct {
	Board    int       `json:"board"`
	PON      int       `json:"pon"`
	ONUID    int       `json:"onu_id"`
	Name     string    `json:"name"`
	Status   string    `json:"status"`
	LastSeen time.Time `json:"last_seen"`
	Samples  int       `json:"samples"`
	InLatest bool      `json:"in_latest_run"`
}

// DuplicateSerial is one serial seen at more than one provisioned position.
type DuplicateSerial struct {
	DeviceID      string              `json:"device_id"`
	Serial        string              `json:"serial_number"`
	Name          string              `json:"name"`
	CurrentCount  int                 `json:"current_count"`
	PositionCount int                 `json:"position_count"`
	Positions     []DuplicatePosition `json:"positions"`
}

// DuplicateSerialList is the history-only duplicate-serial report.
type DuplicateSerialList struct {
	Scope   string            `json:"scope"`
	RunID   int64             `json:"run_id,omitempty"`
	From    time.Time         `json:"from,omitempty"`
	To      time.Time         `json:"to,omitempty"`
	Count   int               `json:"count"`
	Serials []DuplicateSerial `json:"serials"`
}

type duplicatePosRow struct {
	Serial string
	DuplicatePosition
}

func NormalizeDuplicateScope(scope string) string {
	switch strings.ToLower(strings.TrimSpace(scope)) {
	case "", DuplicateScopeCurrent:
		return DuplicateScopeCurrent
	case DuplicateScopeHistory:
		return DuplicateScopeHistory
	default:
		return ""
	}
}

func BuildDuplicateSerials(deviceID string, samples []ONUSample, latest CollectionRun, scope string) DuplicateSerialList {
	type key struct {
		serial string
		board  int
		pon    int
		onu    int
	}
	acc := map[key]*duplicatePosRow{}
	for _, sample := range samples {
		serial := strings.TrimSpace(sample.Serial)
		if serial == "" || (deviceID != "" && sample.DeviceID != deviceID) {
			continue
		}
		k := key{serial: serial, board: sample.Board, pon: sample.PON, onu: sample.ONUID}
		row, ok := acc[k]
		if !ok {
			row = &duplicatePosRow{Serial: serial, DuplicatePosition: DuplicatePosition{
				Board: sample.Board, PON: sample.PON, ONUID: sample.ONUID,
			}}
			acc[k] = row
		}
		row.Samples++
		if sample.Time.After(row.LastSeen) {
			row.LastSeen = sample.Time
			row.Status = sample.Status
			row.Name = sample.Name
		}
		if inLatestRun(sample.Time, latest) {
			row.InLatest = true
		}
	}
	rows := make([]duplicatePosRow, 0, len(acc))
	for _, row := range acc {
		rows = append(rows, *row)
	}
	return assembleDuplicateSerials(deviceID, latest, scope, rows)
}

func assembleDuplicateSerials(deviceID string, latest CollectionRun, scope string, rows []duplicatePosRow) DuplicateSerialList {
	scope = NormalizeDuplicateScope(scope)
	if scope == "" {
		scope = DuplicateScopeCurrent
	}
	out := DuplicateSerialList{Scope: scope, Serials: []DuplicateSerial{}}
	if latest.ID != 0 {
		out.RunID = latest.ID
		out.From = latest.StartedAt
		if latest.FinishedAt != nil {
			out.To = *latest.FinishedAt
		}
	}
	bySerial := map[string]*DuplicateSerial{}
	for _, row := range rows {
		item, ok := bySerial[row.Serial]
		if !ok {
			item = &DuplicateSerial{DeviceID: deviceID, Serial: row.Serial}
			bySerial[row.Serial] = item
		}
		item.Positions = append(item.Positions, row.DuplicatePosition)
		if row.InLatest {
			item.CurrentCount++
		}
	}
	for _, item := range bySerial {
		item.PositionCount = len(item.Positions)
		sort.Slice(item.Positions, func(i, j int) bool {
			a, b := item.Positions[i], item.Positions[j]
			if a.InLatest != b.InLatest {
				return a.InLatest
			}
			if !a.LastSeen.Equal(b.LastSeen) {
				return a.LastSeen.After(b.LastSeen)
			}
			if a.Board != b.Board {
				return a.Board < b.Board
			}
			if a.PON != b.PON {
				return a.PON < b.PON
			}
			return a.ONUID < b.ONUID
		})
		if len(item.Positions) > 0 {
			item.Name = item.Positions[0].Name
		}
		keep := item.PositionCount > 1
		if scope == DuplicateScopeCurrent {
			keep = item.CurrentCount > 1
		}
		if keep {
			out.Serials = append(out.Serials, *item)
		}
	}
	sort.Slice(out.Serials, func(i, j int) bool {
		if out.Serials[i].CurrentCount != out.Serials[j].CurrentCount {
			return out.Serials[i].CurrentCount > out.Serials[j].CurrentCount
		}
		return out.Serials[i].Serial < out.Serials[j].Serial
	})
	out.Count = len(out.Serials)
	return out
}

func inLatestRun(observed time.Time, latest CollectionRun) bool {
	if latest.ID == 0 || latest.StartedAt.IsZero() {
		return false
	}
	if observed.Before(latest.StartedAt) {
		return false
	}
	if latest.FinishedAt != nil && observed.After(*latest.FinishedAt) {
		return false
	}
	return true
}

func latestFinishedRun(runs []CollectionRun, deviceID string) CollectionRun {
	var best CollectionRun
	for _, run := range runs {
		if deviceID != "" && run.DeviceID != deviceID {
			continue
		}
		if run.FinishedAt == nil || run.Status == "running" {
			continue
		}
		if best.ID == 0 || run.ID > best.ID {
			best = run
		}
	}
	return best
}
