// gotunnel/pkg/config/config_test.go
package config

import (
	"testing"
)

func TestServerConfigDefaults(t *testing.T) {
	cfg := DefaultServerConfig()

	if cfg.ControlPort != 7000 {
		t.Errorf("ControlPort = %d, want 7000", cfg.ControlPort)
	}
	if cfg.MinPort != 8000 {
		t.Errorf("MinPort = %d, want 8000", cfg.MinPort)
	}
	if cfg.MaxPort != 9000 {
		t.Errorf("MaxPort = %d, want 9000", cfg.MaxPort)
	}
}

func TestClientConfigDefaults(t *testing.T) {
	cfg := DefaultClientConfig()

	if cfg.ServerAddr != "localhost:7000" {
		t.Errorf("ServerAddr = %s, want localhost:7000", cfg.ServerAddr)
	}
	if cfg.LocalPort != 3000 {
		t.Errorf("LocalPort = %d, want 3000", cfg.LocalPort)
	}
	if cfg.RemotePort != 0 {
		t.Errorf("RemotePort = %d, want 0", cfg.RemotePort)
	}
}

func TestServerConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		cfg     ServerConfig
		wantErr bool
	}{
		{"valid", ServerConfig{ControlPort: 7000, MinPort: 8000, MaxPort: 9000}, false},
		{"min > max", ServerConfig{ControlPort: 7000, MinPort: 9000, MaxPort: 8000}, true},
		{"zero port", ServerConfig{ControlPort: 0, MinPort: 8000, MaxPort: 9000}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestClientConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		cfg     ClientConfig
		wantErr bool
	}{
		{"valid", ClientConfig{ServerAddr: "vps:7000", LocalPort: 3000}, false},
		{"empty server", ClientConfig{ServerAddr: "", LocalPort: 3000}, true},
		{"zero local port", ClientConfig{ServerAddr: "vps:7000", LocalPort: 0}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
