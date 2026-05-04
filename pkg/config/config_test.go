// gotunnel/pkg/config/config_test.go
package config

import (
	"os"
	"path/filepath"
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
		{"valid", ClientConfig{ServerAddr: "vps:7000", Tunnels: []TunnelConfig{{Local: 3000}}}, false},
		{"empty server", ClientConfig{ServerAddr: "", Tunnels: []TunnelConfig{{Local: 3000}}}, true},
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

func TestLoadServerConfig(t *testing.T) {
	content := `
port: 8000
min_port: 9000
max_port: 9100
token: "test-token"
max_clients: 10
max_tunnels: 3
client_timeout: 60s
tls:
  auto: true
`
	dir := t.TempDir()
	path := filepath.Join(dir, "server.yaml")
	os.WriteFile(path, []byte(content), 0644)

	cfg, err := LoadServerConfig(path)
	if err != nil {
		t.Fatalf("LoadServerConfig: %v", err)
	}
	if cfg.ControlPort != 8000 {
		t.Errorf("ControlPort = %d, want 8000", cfg.ControlPort)
	}
	if cfg.Token != "test-token" {
		t.Errorf("Token = %s, want test-token", cfg.Token)
	}
	if cfg.MaxClients != 10 {
		t.Errorf("MaxClients = %d, want 10", cfg.MaxClients)
	}
	if !cfg.TLS.Auto {
		t.Error("TLS.Auto should be true")
	}
}

func TestLoadClientConfig(t *testing.T) {
	content := `
server: "vps:7000"
token: "my-token"
tls: true
insecure: true
tunnels:
  - local: 3000
  - local: 5432
    remote: 9000
`
	dir := t.TempDir()
	path := filepath.Join(dir, "client.yaml")
	os.WriteFile(path, []byte(content), 0644)

	cfg, err := LoadClientConfig(path)
	if err != nil {
		t.Fatalf("LoadClientConfig: %v", err)
	}
	if cfg.ServerAddr != "vps:7000" {
		t.Errorf("ServerAddr = %s, want vps:7000", cfg.ServerAddr)
	}
	if !cfg.TLS {
		t.Error("TLS should be true")
	}
	if len(cfg.Tunnels) != 2 {
		t.Fatalf("Tunnels len = %d, want 2", len(cfg.Tunnels))
	}
	if cfg.Tunnels[0].Local != 3000 || cfg.Tunnels[0].Remote != 0 {
		t.Errorf("Tunnels[0] = %+v", cfg.Tunnels[0])
	}
	if cfg.Tunnels[1].Local != 5432 || cfg.Tunnels[1].Remote != 9000 {
		t.Errorf("Tunnels[1] = %+v", cfg.Tunnels[1])
	}
}
