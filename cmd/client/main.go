// gotunnel/cmd/client/main.go
package main

import (
	"crypto/tls"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/yan/gotunnel/pkg/tunnel"
)

func main() {
	var serverAddr string
	var localPort int
	var remotePort int
	var token string
	var enableTLS bool
	var insecure bool

	rootCmd := &cobra.Command{
		Use:   "gotunnel-client",
		Short: "gotunnel client - expose local services to the internet",
		RunE: func(cmd *cobra.Command, args []string) error {
			slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
				Level: slog.LevelInfo,
			})))

			client := tunnel.NewClient(serverAddr, localPort, remotePort)
			client.SetDialTimeout(5 * time.Second)
			client.SetToken(token)
			if enableTLS {
				client.SetTLSConfig(&tls.Config{
					InsecureSkipVerify: insecure,
				})
				slog.Info("TLS enabled", "insecure", insecure)
			}

			slog.Info("connecting to server",
				"server", serverAddr,
				"local_port", localPort,
			)

			for {
				err := client.Connect()
				if err != nil {
					slog.Error("connect failed", "err", err)
					time.Sleep(2 * time.Second)
					continue
				}

				slog.Info("connected! tunnel active",
					"remote_port", client.RemotePort(),
					"local_port", localPort,
				)

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
				client = tunnel.NewClient(serverAddr, localPort, remotePort)
				client.SetDialTimeout(5 * time.Second)
				client.SetToken(token)
				if enableTLS {
					client.SetTLSConfig(&tls.Config{
						InsecureSkipVerify: insecure,
					})
				}
			}
		},
	}

	rootCmd.Flags().StringVarP(&serverAddr, "server", "s", "localhost:7000", "Server address (host:port)")
	rootCmd.Flags().IntVarP(&localPort, "local", "l", 3000, "Local port to expose")
	rootCmd.Flags().IntVarP(&remotePort, "remote", "r", 0, "Requested remote port (0 = auto)")
	rootCmd.Flags().StringVar(&token, "token", "", "Authentication token")
	rootCmd.Flags().BoolVar(&enableTLS, "tls", false, "Enable TLS for control channel")
	rootCmd.Flags().BoolVar(&insecure, "insecure", false, "Skip TLS certificate verification (for self-signed certs)")

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
