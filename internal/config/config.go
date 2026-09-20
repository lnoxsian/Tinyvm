package config

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Config represents the runtime configuration for MiniVM.
type Config struct {
	Listen    string `json:"listen"`
	Port      int    `json:"port"`
	DataDir   string `json:"data_dir"`
	LogLevel  string `json:"log_level"`
	LogFormat string `json:"log_format"`
	APIToken  string `json:"api_token,omitempty"`
	Verbose   bool   `json:"verbose"`
}

// DefaultConfig returns a Config with standard defaults.
func DefaultConfig() *Config {
	dataDir := "/var/lib/tinyvm"
	// If running as non-root and /var/lib/tinyvm is not writable, fallback to ~/.local/share/tinyvm or current dir
	if os.Geteuid() != 0 {
		if home, err := os.UserHomeDir(); err == nil {
			dataDir = filepath.Join(home, ".local", "share", "tinyvm")
		} else {
			dataDir = "./data"
		}
	}

	return &Config{
		Listen:    "0.0.0.0",
		Port:      8080,
		DataDir:   dataDir,
		LogLevel:  "error",
		LogFormat: "text",
		Verbose:   false,
	}
}

// VMsDir returns the directory where individual VM files are stored.
func (c *Config) VMsDir() string {
	return filepath.Join(c.DataDir, "vms")
}

// ISODir returns the directory where ISO images are stored.
func (c *Config) ISODir() string {
	return filepath.Join(c.DataDir, "iso")
}

// ConfigDir returns the configuration directory.
func (c *Config) ConfigDir() string {
	return filepath.Join(c.DataDir, "config")
}

// Addr returns the formatted network listen address.
func (c *Config) Addr() string {
	return fmt.Sprintf("%s:%d", c.Listen, c.Port)
}

// EnsureDirs ensures that all necessary storage directories exist.
func (c *Config) EnsureDirs() error {
	dirs := []string{
		c.DataDir,
		c.ConfigDir(),
		c.VMsDir(),
		c.ISODir(),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0750); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}
	return nil
}

func getEnv(keys ...string) string {
	for _, k := range keys {
		if val := os.Getenv(k); val != "" {
			return val
		}
	}
	return ""
}

// Load loads configuration from environment variables and command-line flags.
func Load(args []string) (*Config, *flag.FlagSet, error) {
	cfg := DefaultConfig()

	// Environment variable overrides (TINYVM_* with MINIVM_* fallback)
	if env := getEnv("TINYVM_LISTEN", "MINIVM_LISTEN"); env != "" {
		cfg.Listen = env
	}
	if env := getEnv("TINYVM_PORT", "MINIVM_PORT"); env != "" {
		if p, err := strconv.Atoi(env); err == nil && p > 0 && p <= 65535 {
			cfg.Port = p
		}
	}
	if env := getEnv("TINYVM_DATA_DIR", "MINIVM_DATA_DIR"); env != "" {
		cfg.DataDir = env
	}
	if env := getEnv("TINYVM_LOG_LEVEL", "MINIVM_LOG_LEVEL"); env != "" {
		cfg.LogLevel = env
	}
	if env := getEnv("TINYVM_LOG_FORMAT", "MINIVM_LOG_FORMAT"); env != "" {
		cfg.LogFormat = env
	}
	if env := getEnv("TINYVM_API_TOKEN", "MINIVM_API_TOKEN"); env != "" {
		cfg.APIToken = env
	}
	if env := getEnv("TINYVM_VERBOSE", "MINIVM_VERBOSE"); env != "" {
		if v, err := strconv.ParseBool(env); err == nil {
			cfg.Verbose = v
		} else if env == "1" {
			cfg.Verbose = true
		}
	}

	// FlagSet for CLI flags
	fs := flag.NewFlagSet("tinyvm serve", flag.ContinueOnError)
	fs.StringVar(&cfg.Listen, "listen", cfg.Listen, "IP address to listen on")
	fs.IntVar(&cfg.Port, "port", cfg.Port, "HTTP server port")
	fs.StringVar(&cfg.DataDir, "data-dir", cfg.DataDir, "Base data directory")
	fs.StringVar(&cfg.LogLevel, "log-level", cfg.LogLevel, "Log level: debug, info, warn, error")
	fs.StringVar(&cfg.LogFormat, "log-format", cfg.LogFormat, "Log format: text or json")
	fs.StringVar(&cfg.APIToken, "api-token", cfg.APIToken, "API bearer authentication token (optional)")
	fs.BoolVar(&cfg.Verbose, "verbose", cfg.Verbose, "Enable verbose logging (shows info and warnings; default: errors only)")
	fs.BoolVar(&cfg.Verbose, "v", cfg.Verbose, "Enable verbose logging (shorthand)")

	if err := fs.Parse(args); err != nil {
		return nil, nil, err
	}

	return cfg, fs, nil
}

// SetupLogger initializes the global slog logger according to the config.
func (c *Config) SetupLogger() *slog.Logger {
	var level slog.Level
	switch strings.ToLower(c.LogLevel) {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn", "warning":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelError
	}

	if c.Verbose && level > slog.LevelInfo {
		level = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level: level,
	}

	var handler slog.Handler
	if strings.ToLower(c.LogFormat) == "json" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)
	return logger
}
