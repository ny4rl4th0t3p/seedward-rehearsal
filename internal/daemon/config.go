// Package daemon is the coordd-connected rehearsal service (cmd/rehearsald): an ops-authed
// trigger endpoint that pulls a launch's approved input, runs the engine, signs the result fact
// and posts it back, streaming step progress. Runs are serialized — one ephemeral chain at a time.
package daemon

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/viper"
)

const (
	defaultListenAddr = ":8088"
	defaultValidators = 2
	defaultBootWait   = 90 * time.Second
)

// Config is the daemon's startup configuration. Values come from the YAML file, with env
// overrides (prefix REHEARSALD_, e.g. REHEARSALD_OPS_TOKEN) so secrets need not live on disk.
type Config struct {
	ListenAddr     string
	CoorddURL      string
	OpsToken       string
	ServiceKeyPath string
	BinaryPath     string
	Validators     int
	BootWait       time.Duration
}

// LoadConfig reads the daemon config from a YAML file (with REHEARSALD_ env overrides).
func LoadConfig(path string) (Config, error) {
	if path == "" {
		return Config{}, fmt.Errorf("config file path is required")
	}
	v := viper.New()
	v.SetConfigFile(path)
	v.SetEnvPrefix("REHEARSALD")
	v.AutomaticEnv()
	if err := v.ReadInConfig(); err != nil {
		return Config{}, fmt.Errorf("read config %s: %w", path, err)
	}

	cfg := Config{
		ListenAddr:     v.GetString("listen_addr"),
		CoorddURL:      v.GetString("coordd_url"),
		OpsToken:       v.GetString("ops_token"),
		ServiceKeyPath: v.GetString("service_key_path"),
		BinaryPath:     v.GetString("binary_path"),
		Validators:     v.GetInt("validators"),
		BootWait:       v.GetDuration("boot_wait"),
	}
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = defaultListenAddr
	}
	if cfg.Validators == 0 {
		cfg.Validators = defaultValidators
	}
	if cfg.BootWait == 0 {
		cfg.BootWait = defaultBootWait
	}

	required := []struct{ name, val string }{
		{"coordd_url", cfg.CoorddURL},
		{"ops_token", cfg.OpsToken},
		{"service_key_path", cfg.ServiceKeyPath},
		{"binary_path", cfg.BinaryPath},
	}
	for _, r := range required {
		if r.val == "" {
			return Config{}, fmt.Errorf("%s is required", r.name)
		}
	}
	return cfg, nil
}

// LoadServiceKey loads the Ed25519 service signing key from a base64 file (a 32-byte seed or a
// 64-byte private key).
func LoadServiceKey(path string) (ed25519.PrivateKey, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read service key %s: %w", path, err)
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil {
		return nil, fmt.Errorf("decode service key (base64): %w", err)
	}
	switch len(decoded) {
	case ed25519.SeedSize:
		return ed25519.NewKeyFromSeed(decoded), nil
	case ed25519.PrivateKeySize:
		return ed25519.PrivateKey(decoded), nil
	default:
		return nil, fmt.Errorf("service key must be a %d-byte seed or %d-byte key, got %d",
			ed25519.SeedSize, ed25519.PrivateKeySize, len(decoded))
	}
}
