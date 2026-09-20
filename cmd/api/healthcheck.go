package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"time"
)

// checkHealth checks process liveness, not OLT reachability. It also works in
// the runtime image, which intentionally has no shell or curl executable.
func checkHealth() error {
	port := os.Getenv("SERVER_PORT")
	if port == "" {
		port = "8081"
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://" + net.JoinHostPort("127.0.0.1", port) + "/healthz")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health status: %d", resp.StatusCode)
	}
	return nil
}
