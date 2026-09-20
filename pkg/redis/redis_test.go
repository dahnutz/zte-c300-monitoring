package redis

import (
	"testing"

	"zte-c300-monitoring/config"
)

func TestNewRedisClient_FromConfig(t *testing.T) {
	// Clear environment variables to ensure we use config
	t.Setenv("APP_ENV", "")
	t.Setenv("REDIS_HOST", "")
	t.Setenv("REDIS_PORT", "")
	t.Setenv("REDIS_PASSWORD", "")
	t.Setenv("REDIS_DB", "")
	t.Setenv("REDIS_MIN_IDLE_CONNECTIONS", "")
	t.Setenv("REDIS_POOL_SIZE", "")
	t.Setenv("REDIS_POOL_TIMEOUT", "")

	cfg := &config.Config{
		RedisCfg: config.RedisConfig{
			Host:               "localhost",
			Port:               "6379",
			Password:           "testpass",
			DB:                 1,
			MinIdleConnections: 10,
			PoolSize:           100,
			PoolTimeout:        30,
		},
	}

	client := NewRedisClient(cfg)

	if client == nil {
		t.Error("Expected non-nil Redis client")
	}

	opts := client.Options()

	expectedAddr := "localhost:6379"
	if opts.Addr != expectedAddr {
		t.Errorf("Expected address %s, got %s", expectedAddr, opts.Addr)
	}

	if opts.Password != "testpass" {
		t.Errorf("Expected password 'testpass', got %s", opts.Password)
	}

	if opts.DB != 1 {
		t.Errorf("Expected DB 1, got %d", opts.DB)
	}

	if opts.MinIdleConns != 10 {
		t.Errorf("Expected MinIdleConns 10, got %d", opts.MinIdleConns)
	}

	if opts.PoolSize != 100 {
		t.Errorf("Expected PoolSize 100, got %d", opts.PoolSize)
	}
}

func TestNewRedisClient_WithEmptyPassword(t *testing.T) {
	t.Setenv("APP_ENV", "")

	cfg := &config.Config{
		RedisCfg: config.RedisConfig{
			Host:     "localhost",
			Port:     "6379",
			Password: "",
			DB:       0,
		},
	}

	client := NewRedisClient(cfg)

	if client == nil {
		t.Error("Expected non-nil Redis client")
	}

	opts := client.Options()

	if opts.Password != "" {
		t.Errorf("Expected empty password, got %s", opts.Password)
	}
}

func TestNewRedisClient_InvalidEnvironmentIntegers(t *testing.T) {
	// Set environment with invalid integer values
	t.Setenv("APP_ENV", "development")
	t.Setenv("REDIS_HOST", "localhost")
	t.Setenv("REDIS_PORT", "6379")
	t.Setenv("REDIS_DB", "invalid")
	t.Setenv("REDIS_MIN_IDLE_CONNECTIONS", "not-a-number")
	t.Setenv("REDIS_POOL_SIZE", "abc")
	t.Setenv("REDIS_POOL_TIMEOUT", "xyz")

	t.Setenv("SNMP_HOST", "127.0.0.1")
	t.Setenv("SNMP_COMMUNITY", "fixture-only")
	cfg, err := config.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}

	client := NewRedisClient(cfg)

	if client == nil {
		t.Error("Expected non-nil Redis client")
	}

	// Client should still be created even with invalid values
	// LoadConfig applies its documented integer defaults.
	opts := client.Options()

	// Just verify client was created successfully
	if opts.Addr != "localhost:6379" {
		t.Errorf("Expected address 'localhost:6379', got %s", opts.Addr)
	}
}

// Regression: production used to drop the default port when REDIS_PORT was unset.
func TestProductionKeepsResolvedDefaults(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("REDIS_PORT", "")
	t.Setenv("SNMP_HOST", "127.0.0.1")
	t.Setenv("SNMP_COMMUNITY", "fixture-only")
	cfg, err := config.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	client := NewRedisClient(cfg)
	defer client.Close()
	if client.Options().Addr != "localhost:6379" {
		t.Fatalf("defaults lost: %s", client.Options().Addr)
	}
}
