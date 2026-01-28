package mcpserver

import (
	"testing"
)

func TestTransportConfig(t *testing.T) {
	// Test stdio config (default)
	cfg := &Config{}
	transportType := getTransportType(cfg)
	if transportType != "stdio" {
		t.Errorf("Expected stdio transport by default, got %s", transportType)
	}

	// Test HTTP config
	cfg.Transport.Type = "http"
	cfg.Transport.HTTP.Host = "localhost"
	cfg.Transport.HTTP.Port = 8080

	details := getTransportDetails(cfg)
	expected := "http transport on localhost:8080"
	if details != expected {
		t.Errorf("Expected %s, got %s", expected, details)
	}
}
