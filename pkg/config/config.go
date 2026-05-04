// gotunnel/pkg/config/config.go
package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// ServerConfig holds server-side configuration.
type ServerConfig struct {
	ControlPort int           `yaml:"port"`
	MinPort     int           `yaml:"min_port"`
	MaxPort     int           `yaml:"max_port"`
	Token       string        `yaml:"token"`
	TLS         TLSConfig     `yaml:"tls"`
	MaxClients  int           `yaml:"max_clients"`
	MaxTunnels  int           `yaml:"max_tunnels"`
	MaxSessions int           `yaml:"max_sessions"`
	Timeout     time.Duration `yaml:"client_timeout"`
}

type TLSConfig struct {
	Cert string `yaml:"cert"`
	Key  string `yaml:"key"`
	Auto bool   `yaml:"auto"`
}

// ClientConfig holds client-side configuration.
type ClientConfig struct {
	ServerAddr string         `yaml:"server"`
	Tunnels    []TunnelConfig `yaml:"tunnels"`
	Token      string         `yaml:"token"`
	TLS        bool           `yaml:"tls"`
	Insecure   bool           `yaml:"insecure"`
}

type TunnelConfig struct {
	Local  int `yaml:"local"`
	Remote int `yaml:"remote"`
}

// DefaultServerConfig returns sensible defaults.
func DefaultServerConfig() ServerConfig {
	return ServerConfig{
		ControlPort: 7000,
		MinPort:     8000,
		MaxPort:     9000,
	}
}

// DefaultClientConfig returns sensible defaults.
func DefaultClientConfig() ClientConfig {
	return ClientConfig{
		ServerAddr: "localhost:7000",
	}
}

// Validate checks the config for correctness.
func (c ServerConfig) Validate() error {
	if c.ControlPort <= 0 || c.ControlPort > 65535 {
		return fmt.Errorf("invalid control port: %d", c.ControlPort)
	}
	if c.MinPort > c.MaxPort {
		return fmt.Errorf("min_port (%d) > max_port (%d)", c.MinPort, c.MaxPort)
	}
	return nil
}

// Validate checks the config for correctness.
func (c ClientConfig) Validate() error {
	if c.ServerAddr == "" {
		return fmt.Errorf("server address is required")
	}
	for i, t := range c.Tunnels {
		if t.Local <= 0 || t.Local > 65535 {
			return fmt.Errorf("tunnel %d: invalid local port: %d", i, t.Local)
		}
	}
	return nil
}

// LoadServerConfig reads a YAML file and returns a ServerConfig.
// Fields not set in the file keep their zero values (use defaults from CLI).
func LoadServerConfig(path string) (ServerConfig, error) {
	var cfg ServerConfig
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("read config: %w", err)
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse config: %w", err)
	}
	return cfg, nil
}

// LoadClientConfig reads a YAML file and returns a ClientConfig.
func LoadClientConfig(path string) (ClientConfig, error) {
	var cfg ClientConfig
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("read config: %w", err)
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse config: %w", err)
	}
	return cfg, nil
}
