// gotunnel/pkg/config/config.go
package config

import "fmt"

// ServerConfig holds server-side configuration.
type ServerConfig struct {
	ControlPort int
	MinPort     int
	MaxPort     int
}

// DefaultServerConfig returns sensible defaults.
func DefaultServerConfig() ServerConfig {
	return ServerConfig{
		ControlPort: 7000,
		MinPort:     8000,
		MaxPort:     9000,
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

// ClientConfig holds client-side configuration.
type ClientConfig struct {
	ServerAddr string
	LocalPort  int
	RemotePort int
}

// DefaultClientConfig returns sensible defaults.
func DefaultClientConfig() ClientConfig {
	return ClientConfig{
		ServerAddr: "localhost:7000",
		LocalPort:  3000,
		RemotePort: 0,
	}
}

// Validate checks the config for correctness.
func (c ClientConfig) Validate() error {
	if c.ServerAddr == "" {
		return fmt.Errorf("server address is required")
	}
	if c.LocalPort <= 0 || c.LocalPort > 65535 {
		return fmt.Errorf("invalid local port: %d", c.LocalPort)
	}
	return nil
}
