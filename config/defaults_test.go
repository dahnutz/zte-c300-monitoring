package config

import "testing"

func TestLocalTestDefaults(t *testing.T) {
	for _, key := range []string{
		"OLTS", "OLTS_FILE", "API_USERS", "DEFAULT_OLT",
		"CACHE_PREWARM", "OLT_TIMEZONE",
	} {
		t.Setenv(key, "")
	}
	t.Setenv("SNMP_HOST", "127.0.0.1")
	t.Setenv("SNMP_COMMUNITY", "test-only")
	t.Setenv("OLT_BOARDS", "3:16,5:8")
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CacheCfg.PreWarm {
		t.Fatal("background collection must be opt-in")
	}
	if cfg.OltCfg.Timezone != "UTC" {
		t.Fatalf("default timezone = %q, want UTC", cfg.OltCfg.Timezone)
	}
	if _, err := cfg.GetBoardPonConfig(5, 8); err != nil {
		t.Fatal(err)
	}
	if _, err := cfg.GetBoardPonConfig(5, 9); err == nil {
		t.Fatal("an 8-PON card must reject PON 9")
	}
}
