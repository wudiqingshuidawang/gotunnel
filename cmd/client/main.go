// gotunnel/cmd/client/main.go
package main

import (
	"crypto/tls"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/yan/gotunnel/pkg/config"
	"github.com/yan/gotunnel/pkg/tunnel"
)

func main() {
	var configFile string
	var serverAddr string
	var tunnelSpecs []string
	var localPort int
	var remotePort int
	var token string
	var enableTLS bool
	var insecure bool
	var httpMode bool

	rootCmd := &cobra.Command{
		Use:   "gotunnel-client",
		Short: "gotunnel client - expose local services to the internet",
		RunE: func(cmd *cobra.Command, args []string) error {
			slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
				Level: slog.LevelInfo,
			})))

			// Load config file if specified
			if configFile != "" {
				cfg, err := config.LoadClientConfig(configFile)
				if err != nil {
					return fmt.Errorf("load config: %w", err)
				}
				if !cmd.Flags().Changed("server") && cfg.ServerAddr != "" {
					serverAddr = cfg.ServerAddr
				}
				if !cmd.Flags().Changed("token") && cfg.Token != "" {
					token = cfg.Token
				}
				if !cmd.Flags().Changed("tls") && cfg.TLS {
					enableTLS = true
				}
				if !cmd.Flags().Changed("insecure") && cfg.Insecure {
					insecure = true
				}
				if !cmd.Flags().Changed("http") && cfg.HTTP {
					httpMode = true
				}
				// Convert config tunnels to tunnel specs if --tunnel not set
				if !cmd.Flags().Changed("tunnel") && len(cfg.Tunnels) > 0 {
					tunnelSpecs = nil
					for _, t := range cfg.Tunnels {
						if t.Remote > 0 {
							tunnelSpecs = append(tunnelSpecs, fmt.Sprintf("%d:%d", t.Local, t.Remote))
						} else {
							tunnelSpecs = append(tunnelSpecs, fmt.Sprintf("%d", t.Local))
						}
					}
				}
			}

			// Parse tunnel specs: --tunnel 3000:8080 --tunnel 5432
			// Fall back to --local/--remote for backward compatibility
			if len(tunnelSpecs) == 0 && localPort > 0 {
				if remotePort > 0 {
					tunnelSpecs = []string{fmt.Sprintf("%d:%d", localPort, remotePort)}
				} else {
					tunnelSpecs = []string{fmt.Sprintf("%d", localPort)}
				}
			}
			if len(tunnelSpecs) == 0 {
				return fmt.Errorf("at least one --tunnel required (e.g. --tunnel 3000 or --tunnel 3000:8080)")
			}

			client := tunnel.NewClient(serverAddr)
			for _, spec := range tunnelSpecs {
				local, remote, err := parseTunnelSpec(spec)
				if err != nil {
					return fmt.Errorf("invalid tunnel spec %q: %w", spec, err)
				}
				client.AddTunnel(local, remote)
			}

			client.SetDialTimeout(5 * time.Second)
			client.SetToken(token)
			client.SetHTTPMode(httpMode)
			if enableTLS {
				client.SetTLSConfig(&tls.Config{
					InsecureSkipVerify: insecure,
				})
				slog.Info("TLS enabled", "insecure", insecure)
			}

			slog.Info("connecting to server", "server", serverAddr, "tunnels", len(tunnelSpecs))

			for {
				err := client.Connect()
				if err != nil {
					slog.Error("connect failed", "err", err)
					time.Sleep(2 * time.Second)
					continue
				}

				ports := client.RemotePorts()
				for i, p := range ports {
					slog.Info("tunnel active", "remote_port", p, "tunnel", tunnelSpecs[i%len(tunnelSpecs)])
				}

				sigCh := make(chan os.Signal, 1)
				signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

				go func() {
					<-sigCh
					slog.Info("shutting down...")
					client.Close()
					os.Exit(0)
				}()

				client.Run()

				slog.Info("disconnected, reconnecting in 2s...")
				time.Sleep(2 * time.Second)

				// Recreate client for reconnection
				client = tunnel.NewClient(serverAddr)
				for _, spec := range tunnelSpecs {
					local, remote, _ := parseTunnelSpec(spec)
					client.AddTunnel(local, remote)
				}
				client.SetDialTimeout(5 * time.Second)
				client.SetToken(token)
				client.SetHTTPMode(httpMode)
				if enableTLS {
					client.SetTLSConfig(&tls.Config{
						InsecureSkipVerify: insecure,
					})
				}
			}
		},
	}

	rootCmd.Flags().StringVar(&configFile, "config", "", "Path to YAML config file")
	rootCmd.Flags().StringVarP(&serverAddr, "server", "s", "localhost:7000", "Server address (host:port)")
	rootCmd.Flags().StringSliceVar(&tunnelSpecs, "tunnel", nil, "Tunnel spec localPort:remotePort (repeatable, e.g. --tunnel 3000 --tunnel 5432:9000)")
	rootCmd.Flags().IntVarP(&localPort, "local", "l", 0, "Local port to expose (shorthand for --tunnel)")
	rootCmd.Flags().IntVarP(&remotePort, "remote", "r", 0, "Requested remote port (used with --local)")
	rootCmd.Flags().StringVar(&token, "token", "", "Authentication token")
	rootCmd.Flags().BoolVar(&enableTLS, "tls", false, "Enable TLS for control channel")
	rootCmd.Flags().BoolVar(&insecure, "insecure", false, "Skip TLS certificate verification (for self-signed certs)")
	rootCmd.Flags().BoolVar(&httpMode, "http", false, "Enable HTTP header injection (X-Forwarded-For, X-Real-IP)")

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// parseTunnelSpec parses "localPort" or "localPort:remotePort".
func parseTunnelSpec(spec string) (local, remote int, err error) {
	parts := strings.SplitN(spec, ":", 2)
	local, err = strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return 0, 0, fmt.Errorf("invalid local port: %w", err)
	}
	if len(parts) == 2 {
		remote, err = strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil {
			return 0, 0, fmt.Errorf("invalid remote port: %w", err)
		}
	}
	return local, remote, nil
}
