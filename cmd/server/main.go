// gotunnel/cmd/server/main.go
package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/yan/gotunnel/pkg/registry"
	"github.com/yan/gotunnel/pkg/tunnel"
)

func main() {
	var port int
	var minPort, maxPort int

	rootCmd := &cobra.Command{
		Use:   "gotunnel-server",
		Short: "gotunnel server - relay tunnel traffic",
		RunE: func(cmd *cobra.Command, args []string) error {
			slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
				Level: slog.LevelInfo,
			})))

			addr := fmt.Sprintf(":%d", port)
			reg := registry.New(minPort, maxPort)
			srv := tunnel.NewServerWithRegistry(addr, reg)

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

			return srv.Start()
		},
	}

	rootCmd.Flags().IntVarP(&port, "port", "p", 7000, "Control channel port")
	rootCmd.Flags().IntVar(&minPort, "min-port", 8000, "Minimum allocatable port")
	rootCmd.Flags().IntVar(&maxPort, "max-port", 9000, "Maximum allocatable port")

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
