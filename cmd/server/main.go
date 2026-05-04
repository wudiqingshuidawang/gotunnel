// gotunnel/cmd/server/main.go
package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"log/slog"
	"math/big"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/yan/gotunnel/pkg/config"
	"github.com/yan/gotunnel/pkg/registry"
	"github.com/yan/gotunnel/pkg/tunnel"
)

func main() {
	var configFile string
	var port int
	var minPort, maxPort int
	var token string
	var maxClients, maxTunnels, maxSessions int
	var clientTimeout time.Duration
	var tlsCert, tlsKey string
	var tlsAuto bool
	var dashboardPort int

	rootCmd := &cobra.Command{
		Use:   "gotunnel-server",
		Short: "gotunnel server - relay tunnel traffic",
		RunE: func(cmd *cobra.Command, args []string) error {
			slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
				Level: slog.LevelInfo,
			})))

			// Load config file if specified, CLI flags override
			if configFile != "" {
				cfg, err := config.LoadServerConfig(configFile)
				if err != nil {
					return fmt.Errorf("load config: %w", err)
				}
				// Apply config file values only if CLI flag was not explicitly set
				if !cmd.Flags().Changed("port") && cfg.ControlPort != 0 {
					port = cfg.ControlPort
				}
				if !cmd.Flags().Changed("min-port") && cfg.MinPort != 0 {
					minPort = cfg.MinPort
				}
				if !cmd.Flags().Changed("max-port") && cfg.MaxPort != 0 {
					maxPort = cfg.MaxPort
				}
				if !cmd.Flags().Changed("token") && cfg.Token != "" {
					token = cfg.Token
				}
				if !cmd.Flags().Changed("max-clients") && cfg.MaxClients != 0 {
					maxClients = cfg.MaxClients
				}
				if !cmd.Flags().Changed("max-tunnels") && cfg.MaxTunnels != 0 {
					maxTunnels = cfg.MaxTunnels
				}
				if !cmd.Flags().Changed("max-sessions") && cfg.MaxSessions != 0 {
					maxSessions = cfg.MaxSessions
				}
				if !cmd.Flags().Changed("client-timeout") && cfg.Timeout != 0 {
					clientTimeout = cfg.Timeout
				}
				if !cmd.Flags().Changed("tls-auto") && cfg.TLS.Auto {
					tlsAuto = true
				}
				if !cmd.Flags().Changed("tls-cert") && cfg.TLS.Cert != "" {
					tlsCert = cfg.TLS.Cert
				}
				if !cmd.Flags().Changed("tls-key") && cfg.TLS.Key != "" {
					tlsKey = cfg.TLS.Key
				}
				if !cmd.Flags().Changed("dashboard-port") && cfg.DashboardPort != 0 {
					dashboardPort = cfg.DashboardPort
				}
			}

			addr := fmt.Sprintf(":%d", port)
			reg := registry.New(minPort, maxPort)
			srv := tunnel.NewServerWithRegistry(addr, reg)
			srv.SetToken(token)
			srv.SetMaxClients(maxClients)
			srv.SetMaxTunnelsPerClient(maxTunnels)
			srv.SetMaxSessions(maxSessions)
			srv.SetClientTimeout(clientTimeout)

			// TLS configuration
			if tlsAuto && (tlsCert != "" || tlsKey != "") {
				return fmt.Errorf("--tls-auto and --tls-cert/--tls-key are mutually exclusive")
			}
			if tlsAuto {
				cfg, err := generateSelfSignedCert()
				if err != nil {
					return fmt.Errorf("generate self-signed cert: %w", err)
				}
				srv.SetTLSConfig(cfg)
				slog.Info("TLS enabled with auto-generated self-signed certificate")
			} else if tlsCert != "" && tlsKey != "" {
				cert, err := tls.LoadX509KeyPair(tlsCert, tlsKey)
				if err != nil {
					return fmt.Errorf("load TLS key pair: %w", err)
				}
				srv.SetTLSConfig(&tls.Config{
					Certificates: []tls.Certificate{cert},
					MinVersion:   tls.VersionTLS12,
				})
				slog.Info("TLS enabled with certificate files", "cert", tlsCert, "key", tlsKey)
			} else if tlsCert != "" || tlsKey != "" {
				return fmt.Errorf("both --tls-cert and --tls-key must be provided together")
			}

			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

			go func() {
				<-sigCh
				slog.Info("shutting down...")
				srv.Stop()
				os.Exit(0)
			}()

			slog.Info("starting gotunnel server",
				"control_port", port,
				"port_range", fmt.Sprintf("%d-%d", minPort, maxPort),
			)

			// Start dashboard if port is configured
			if dashboardPort > 0 {
				srv.StartDashboard(fmt.Sprintf(":%d", dashboardPort))
			}

			return srv.Start()
		},
	}

	rootCmd.Flags().StringVar(&configFile, "config", "", "Path to YAML config file")
	rootCmd.Flags().IntVarP(&port, "port", "p", 7000, "Control channel port")
	rootCmd.Flags().IntVar(&minPort, "min-port", 8000, "Minimum allocatable port")
	rootCmd.Flags().IntVar(&maxPort, "max-port", 9000, "Maximum allocatable port")
	rootCmd.Flags().StringVar(&token, "token", "", "Authentication token (empty = no auth)")
	rootCmd.Flags().IntVar(&maxClients, "max-clients", 0, "Max concurrent clients (0=unlimited)")
	rootCmd.Flags().IntVar(&maxTunnels, "max-tunnels", 0, "Max tunnels per client (0=unlimited)")
	rootCmd.Flags().IntVar(&maxSessions, "max-sessions", 0, "Max concurrent sessions (0=unlimited)")
	rootCmd.Flags().DurationVar(&clientTimeout, "client-timeout", 0, "Client heartbeat timeout, e.g. 90s (0=no timeout)")
	rootCmd.Flags().StringVar(&tlsCert, "tls-cert", "", "Path to TLS certificate file")
	rootCmd.Flags().StringVar(&tlsKey, "tls-key", "", "Path to TLS key file")
	rootCmd.Flags().BoolVar(&tlsAuto, "tls-auto", false, "Auto-generate self-signed TLS certificate")
	rootCmd.Flags().IntVar(&dashboardPort, "dashboard-port", 0, "Web dashboard port (0=disabled)")

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func generateSelfSignedCert() (*tls.Config, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{Organization: []string{"gotunnel"}},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:     []string{"localhost"},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}

	cert := tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  key,
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}, nil
}
