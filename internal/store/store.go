package store

import (
	"context"
	"time"
)

// Device is one managed network element (an OLT today, a switch later).
type Device struct {
	ID     string
	Vendor string
	Family string
	Role   string
}

// ONUSample is one PON-list observation. Optical fields are nil when the
// firmware did not return a usable value.
type ONUSample struct {
	Time             time.Time       `json:"time"`
	DeviceID         string          `json:"device_id"`
	Serial           string          `json:"serial_number"`
	Board            int             `json:"board"`
	PON              int             `json:"pon"`
	ONUID            int             `json:"onu_id"`
	Name             string          `json:"name"`
	OnuType          string          `json:"onu_type"`
	Status           string          `json:"status"`
	RXPower          *float64        `json:"rx_power"`
	TXPower          *float64        `json:"tx_power"`
	EthStatus        string          `json:"eth_status,omitempty"`
	EthLinkState     string          `json:"eth_link_state,omitempty"`
	EthAdminState    string          `json:"eth_admin_state,omitempty"`
	EthSpeedMbps     *int            `json:"eth_speed_mbps,omitempty"`
	EthPorts         []EthPortSample `json:"eth_ports,omitempty"`
	StatusChangedAt  *time.Time      `json:"status_changed_at,omitempty"`
	PreviousStatus   string          `json:"previous_status,omitempty"`
	ExpectedEthPorts int             `json:"expected_eth_ports,omitempty"`
}

// EthPortSample is the compact UNI snapshot stored with history samples.
type EthPortSample struct {
	Port      int    `json:"port"`
	Admin     string `json:"admin"`
	Link      string `json:"link"`
	SpeedMbps *int   `json:"speed_mbps,omitempty"`
	Duplex    string `json:"duplex,omitempty"`
}

// CollectionRun records the cost and outcome of one poller cycle on a device.
type CollectionRun struct {
	ID          int64      `json:"id"`
	DeviceID    string     `json:"device_id"`
	StartedAt   time.Time  `json:"started_at"`
	FinishedAt  *time.Time `json:"finished_at,omitempty"`
	Status      string     `json:"status"`
	PonsOK      int        `json:"pons_ok"`
	PonsError   int        `json:"pons_error"`
	ONUsSampled int        `json:"onus_sampled"`
	DurationMS  *int       `json:"duration_ms,omitempty"`
	Error       string     `json:"error,omitempty"`
}

// SampleQuery selects ONU samples for the history API.
type SampleQuery struct {
	DeviceID      string
	Serial        string
	Board         int
	PON           int
	ONUID         int
	Status        string // case-insensitive, e.g. Online / Offline
	From          time.Time
	To            time.Time
	Limit         int
	RunID         int64 // 0 = unset
	LatestRun     bool
	CountBy       string // status, board, pon, onu_type, eth_link
	ResolvedRunID int64
}

// CountBucket is one group in a count_by response.
type CountBucket struct {
	Key   string `json:"key"`
	Count int    `json:"count"`
}

// CountResult is an aggregate over matching samples.
type CountResult struct {
	GroupBy string        `json:"group_by"`
	RunID   int64         `json:"run_id,omitempty"`
	From    time.Time     `json:"from"`
	To      time.Time     `json:"to"`
	Total   int           `json:"total"`
	Counts  []CountBucket `json:"counts"`
}

// Store is durable telemetry storage. The Timescale implementation is used
// in deployment; Memory is for tests.
type Store interface {
	Ping(ctx context.Context) error
	Close()
	UpsertDevice(ctx context.Context, device Device) error
	InsertSamples(ctx context.Context, samples []ONUSample) error
	UpsertONUs(ctx context.Context, samples []ONUSample) error
	InsertCollectionRun(ctx context.Context, run CollectionRun) (int64, error)
	FinishCollectionRun(ctx context.Context, run CollectionRun) error
	ListSamples(ctx context.Context, q SampleQuery) ([]ONUSample, error)
	ListSampleCounts(ctx context.Context, q SampleQuery) (CountResult, error)
	ListCollectionRuns(ctx context.Context, deviceID string, limit int) ([]CollectionRun, error)
	GetCollectionRun(ctx context.Context, deviceID string, id int64) (CollectionRun, error)
	LatestCollectionRun(ctx context.Context, deviceID string) (CollectionRun, error)
	ListONUStates(ctx context.Context, deviceID string) ([]ONUState, error)
	ListEthState(ctx context.Context, deviceID, serial string) ([]EthPortState, error)
	ListStatusEvents(ctx context.Context, q EventQuery) ([]StatusEvent, error)
	ListEthEvents(ctx context.Context, q EventQuery) ([]EthEvent, error)
	RecordSampleTransitions(ctx context.Context, samples []ONUSample) error
	ReplaceUnauth(ctx context.Context, deviceID string, list UnauthList) error
	ListUnauth(ctx context.Context, deviceID string) (UnauthList, error)
}
