package config

import (
	"net/url"
	"time"
)

// StoreConfig is the durable TimescaleDB connection. Empty Host means the
// collector runs without history (API + Redis only).
type StoreConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	DB       string
	SSLMode  string
}

// PollConfig controls the read-only PON-list poller that writes ONU samples.
type PollConfig struct {
	Enabled    bool
	Interval   time.Duration
	StartDelay time.Duration
}

// DeviceMeta is stored with each telemetry stream so later Huawei OLT or
// Cisco switch collectors can share the same tables.
type DeviceMeta struct {
	Vendor string
	Family string
	Role   string
}

// Enabled reports whether a history database is configured.
func (c StoreConfig) Enabled() bool {
	return c.Host != ""
}

// DSN builds a Postgres URL. The password is URL-escaped.
func (c StoreConfig) DSN() string {
	user := url.UserPassword(c.User, c.Password)
	host := c.Host
	if c.Port != "" {
		host = c.Host + ":" + c.Port
	}
	u := url.URL{
		Scheme: "postgres",
		User:   user,
		Host:   host,
		Path:   "/" + c.DB,
	}
	query := url.Values{}
	ssl := c.SSLMode
	if ssl == "" {
		ssl = "disable"
	}
	query.Set("sslmode", ssl)
	u.RawQuery = query.Encode()
	return u.String()
}
