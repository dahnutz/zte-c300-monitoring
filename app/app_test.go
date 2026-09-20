package app

import (
	"context"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

func TestNew(t *testing.T) {
	app := New()

	if app == nil {
		t.Fatal("Expected non-nil app instance")
	}

	if app.router != nil {
		t.Error("Expected router to be nil before Start is called")
	}
}

func TestNew_ReturnsAppStruct(t *testing.T) {
	app1 := New()
	app2 := New()

	if app1 == nil || app2 == nil {
		t.Error("Expected both app instances to be non-nil")
	}

	if app1 == app2 {
		t.Error("Expected different app instances")
	}
}

func TestApp_Start_MissingConfig(t *testing.T) {
	t.Setenv("API_KEY", "test-only-key")
	t.Setenv("SNMP_HOST", "")
	t.Setenv("SNMP_COMMUNITY", "")

	app := New()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := app.Start(ctx)
	if err == nil {
		t.Error("Expected error for missing SNMP config")
	}
}

func TestApp_Start_SNMPSetupFailure(t *testing.T) {
	// Use miniredis for a working Redis, but invalid SNMP host to trigger SNMP failure
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("Failed to start miniredis: %v", err)
	}
	defer mr.Close()

	t.Setenv("API_KEY", "test-only-key")
	t.Setenv("SNMP_HOST", "invalid-host-!@#")
	t.Setenv("SNMP_COMMUNITY", "public")
	t.Setenv("SNMP_PORT", "161")
	t.Setenv("REDIS_HOST", mr.Host())
	t.Setenv("REDIS_PORT", mr.Port())
	t.Setenv("REDIS_PASSWORD", "")

	app := New()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Covers: config OK → Redis init → Redis ping SUCCESS → Redis close →
	// SNMP setup FAILURE → return err
	err = app.Start(ctx)
	if err == nil {
		t.Error("Expected error for invalid SNMP host")
	}
}

func TestApp_Start_RedisPingFailure(t *testing.T) {
	// Start miniredis then close it before app.Start to trigger ping failure
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("Failed to start miniredis: %v", err)
	}
	host, port := mr.Host(), mr.Port()
	mr.Close() // Close before Start — ping will fail

	// Start UDP listener for SNMP
	udpListener, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to start UDP listener: %v", err)
	}
	defer func() { _ = udpListener.Close() }()
	snmpAddr := udpListener.LocalAddr().(*net.UDPAddr)

	t.Setenv("API_KEY", "test-only-key")
	t.Setenv("SNMP_HOST", "127.0.0.1")
	t.Setenv("SNMP_COMMUNITY", "public")
	t.Setenv("SNMP_PORT", strconv.Itoa(snmpAddr.Port))
	t.Setenv("REDIS_HOST", host)
	t.Setenv("REDIS_PORT", port)
	t.Setenv("REDIS_PASSWORD", "")
	t.Setenv("CACHE_PREWARM", "false")

	app := New()
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(300 * time.Millisecond)
		cancel()
	}()

	// Covers: config OK → Redis init → Redis ping FAILURE (non-fatal) →
	// SNMP connect OK → server start → cancel → shutdown
	_ = app.Start(ctx)
}

func TestApp_Start_RedisCloseError(t *testing.T) {
	// Test that Redis close error is handled (covers defer close error branch)
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("Failed to start miniredis: %v", err)
	}

	udpListener, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to start UDP listener: %v", err)
	}
	defer func() { _ = udpListener.Close() }()
	snmpAddr := udpListener.LocalAddr().(*net.UDPAddr)

	t.Setenv("API_KEY", "test-only-key")
	t.Setenv("SNMP_HOST", "127.0.0.1")
	t.Setenv("SNMP_COMMUNITY", "public")
	t.Setenv("SNMP_PORT", strconv.Itoa(snmpAddr.Port))
	t.Setenv("REDIS_HOST", mr.Host())
	t.Setenv("REDIS_PORT", mr.Port())
	t.Setenv("REDIS_PASSWORD", "")
	t.Setenv("SERVER_PORT", "0")
	t.Setenv("CACHE_PREWARM", "false")

	app := New()
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		time.Sleep(200 * time.Millisecond)
		// Close miniredis before app shutdown to trigger Redis close error
		mr.Close()
		cancel()
	}()

	_ = app.Start(ctx)
}

func TestApp_Start_FullLifecycle(t *testing.T) {
	// Use miniredis for Redis + local UDP listener for SNMP
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("Failed to start miniredis: %v", err)
	}
	defer mr.Close()

	// Start a local UDP listener to simulate SNMP agent
	udpListener, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to start UDP listener: %v", err)
	}
	defer func() { _ = udpListener.Close() }()
	snmpAddr := udpListener.LocalAddr().(*net.UDPAddr)

	t.Setenv("API_KEY", "test-only-key")
	t.Setenv("SNMP_HOST", "127.0.0.1")
	t.Setenv("SNMP_COMMUNITY", "public")
	t.Setenv("SNMP_PORT", strconv.Itoa(snmpAddr.Port))
	t.Setenv("REDIS_HOST", mr.Host())
	t.Setenv("REDIS_PORT", mr.Port())
	t.Setenv("REDIS_PASSWORD", "")
	t.Setenv("CACHE_PREWARM", "false")
	// Don't set SERVER_PORT — let it fall back to default "8081" to cover that branch

	app := New()
	ctx, cancel := context.WithCancel(context.Background())

	// Cancel after short delay to trigger graceful shutdown
	go func() {
		time.Sleep(300 * time.Millisecond)
		cancel()
	}()

	// Covers full lifecycle: config → Redis init → Redis ping OK →
	// SNMP connect OK → repository init → handler init → router init →
	// server start → context cancel → graceful shutdown → Redis close → SNMP close
	_ = app.Start(ctx)
}

func TestAppRequiresAuthentication(t *testing.T) {
	t.Setenv("SNMP_HOST", "127.0.0.1")
	t.Setenv("SNMP_COMMUNITY", "fixture-only")
	t.Setenv("API_KEY", "")
	t.Setenv("API_USERS", "")
	err := New().Start(context.Background())
	if err == nil || err.Error() != "API_KEY or API_USERS is required" {
		t.Fatalf("expected authentication configuration error, got %v", err)
	}
}
