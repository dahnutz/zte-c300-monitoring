package config

import (
	"testing"
	"time"
)

func TestLoadConfig_PollerRequiresStore(t *testing.T) {
	t.Setenv("SNMP_HOST", "10.0.0.1")
	t.Setenv("SNMP_COMMUNITY", "public")
	t.Setenv("POLL_ENABLED", "true")
	t.Setenv("TIMESCALEDB_HOST", "")
	if _, err := LoadConfig(); err == nil {
		t.Fatal("expected POLL_ENABLED without TIMESCALEDB_HOST to fail")
	}
}

func TestLoadConfig_PollerAndStoreDefaults(t *testing.T) {
	t.Setenv("SNMP_HOST", "10.0.0.1")
	t.Setenv("SNMP_COMMUNITY", "public")
	t.Setenv("POLL_ENABLED", "true")
	t.Setenv("TIMESCALEDB_HOST", "timescaledb")
	t.Setenv("POLL_INTERVAL_SECONDS", "10")
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.PollCfg.Enabled {
		t.Fatal("poller not enabled")
	}
	if cfg.PollCfg.Interval != 30*time.Second {
		t.Fatalf("interval clamped to 30s, got %s", cfg.PollCfg.Interval)
	}
	if cfg.StoreCfg.Host != "timescaledb" || cfg.StoreCfg.DB != "c300_monitoring" {
		t.Fatalf("store = %+v", cfg.StoreCfg)
	}
	if cfg.DeviceMeta.Vendor != "zte" || cfg.DeviceMeta.Family != "c300" {
		t.Fatalf("device = %+v", cfg.DeviceMeta)
	}
}

func TestStoreConfigDSNEscapesPassword(t *testing.T) {
	cfg := StoreConfig{
		Host: "db", Port: "5432", User: "c300", Password: "p@ss:word",
		DB: "c300_monitoring", SSLMode: "disable",
	}
	dsn := cfg.DSN()
	if want := "postgres://c300:p%40ss%3Aword@db:5432/c300_monitoring?sslmode=disable"; dsn != want {
		t.Fatalf("dsn = %s want %s", dsn, want)
	}
}
