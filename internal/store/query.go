package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrNotFound is returned when a collection run (or similar row) is missing.
var ErrNotFound = errors.New("not found")

// CountByStatus and related values are accepted by ?count_by=.
const (
	CountByStatus  = "status"
	CountByBoard   = "board"
	CountByPON     = "pon"
	CountByOnuType = "onu_type"
	CountByEthLink = "eth_link"
)

// ValidCountBy reports whether count_by is empty or a supported grouping.
func ValidCountBy(value string) bool {
	switch value {
	case "", CountByStatus, CountByBoard, CountByPON, CountByOnuType, CountByEthLink:
		return true
	default:
		return false
	}
}

// ApplyRunWindow sets From/To from a collection run when RunID or LatestRun is set.
func ApplyRunWindow(ctx context.Context, st interface {
	GetCollectionRun(context.Context, string, int64) (CollectionRun, error)
	LatestCollectionRun(context.Context, string) (CollectionRun, error)
}, q *SampleQuery) error {
	if q.RunID == 0 && !q.LatestRun {
		return nil
	}
	var run CollectionRun
	var err error
	if q.LatestRun {
		run, err = st.LatestCollectionRun(ctx, q.DeviceID)
	} else {
		run, err = st.GetCollectionRun(ctx, q.DeviceID, q.RunID)
	}
	if err != nil {
		return err
	}
	q.From = run.StartedAt
	if run.FinishedAt != nil {
		q.To = *run.FinishedAt
	} else {
		q.To = time.Now().UTC()
	}
	q.ResolvedRunID = run.ID
	if q.Limit <= 0 {
		q.Limit = 2000
	}
	return nil
}

func sampleWindow(q SampleQuery) (time.Time, time.Time) {
	from, to := q.From, q.To
	if to.IsZero() {
		to = time.Now().UTC()
	}
	if from.IsZero() {
		from = to.Add(-24 * time.Hour)
	}
	return from, to
}

func matchSample(sample ONUSample, q SampleQuery) bool {
	if q.DeviceID != "" && sample.DeviceID != q.DeviceID {
		return false
	}
	if q.Serial != "" && sample.Serial != q.Serial {
		return false
	}
	if q.Board != 0 && sample.Board != q.Board {
		return false
	}
	if q.PON != 0 && sample.PON != q.PON {
		return false
	}
	if q.ONUID != 0 && sample.ONUID != q.ONUID {
		return false
	}
	if q.Status != "" && !strings.EqualFold(sample.Status, q.Status) {
		return false
	}
	from, to := sampleWindow(q)
	if sample.Time.Before(from) || sample.Time.After(to) {
		return false
	}
	return true
}

func sampleGroupKey(sample ONUSample, countBy string) string {
	switch countBy {
	case CountByBoard:
		return fmt.Sprintf("%d", sample.Board)
	case CountByPON:
		return fmt.Sprintf("%d", sample.PON)
	case CountByEthLink:
		if sample.EthLinkState == "" {
			return "(empty)"
		}
		return sample.EthLinkState
	case CountByOnuType:
		if sample.OnuType == "" {
			return "(empty)"
		}
		return sample.OnuType
	default:
		if sample.Status == "" {
			return "(empty)"
		}
		return sample.Status
	}
}
