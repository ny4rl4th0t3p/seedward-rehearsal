// Command rehearsald is the coordd-connected rehearsal daemon: a long-running service whose
// ops-authed trigger endpoint pulls a launch's approved input from coordd, runs the rehearsal
// engine, signs the result fact and posts it back — streaming step progress as NDJSON. It never
// initiates toward coordd on its own (DEC-7); a run happens only when triggered.
package main

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/ny4rl4th0t3p/seedward-gentool/pkg/rehearse"

	"github.com/ny4rl4th0t3p/seedward-rehearsal/internal/bridge"
	"github.com/ny4rl4th0t3p/seedward-rehearsal/internal/daemon"
	"github.com/ny4rl4th0t3p/seedward-rehearsal/internal/version"
)

// readHeaderTimeout bounds the header read to mitigate slowloris (gosec G112).
const readHeaderTimeout = 10 * time.Second

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	var cfgPath string
	cmd := &cobra.Command{
		Use:           "rehearsald --config <rehearsald.yaml>",
		Short:         "Run the coordd-connected rehearsal daemon.",
		Version:       version.Version,
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(_ *cobra.Command, _ []string) error {
			return serve(cfgPath)
		},
	}
	cmd.Flags().StringVar(&cfgPath, "config", "", "path to the rehearsald YAML config (required)")
	_ = cmd.MarkFlagRequired("config")
	return cmd
}

func serve(cfgPath string) error {
	log := slog.New(slog.NewJSONHandler(os.Stderr, nil))

	cfg, err := daemon.LoadConfig(cfgPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	signer, err := daemon.LoadServiceKey(cfg.ServiceKeyPath)
	if err != nil {
		return fmt.Errorf("load service key: %w", err)
	}

	client := bridge.NewClient(cfg.CoorddURL, cfg.OpsToken, nil)
	engine := rehearse.New(
		rehearse.NewProcessRuntime(cfg.BinaryPath),
		rehearse.WithValidators(cfg.Validators),
		rehearse.WithBootWait(cfg.BootWait),
	)
	srv := daemon.NewServer(client, engine, signer, cfg.BinaryPath, version.Version, cfg.OpsToken, log)

	httpServer := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: readHeaderTimeout,
	}
	log.Info("rehearsald listening", "addr", cfg.ListenAddr, "coordd", cfg.CoorddURL)
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("server: %w", err)
	}
	return nil
}
