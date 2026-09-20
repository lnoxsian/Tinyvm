package config

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Port != 8080 {
		t.Errorf("expected default port 8080, got %d", cfg.Port)
	}
	if cfg.Listen != "0.0.0.0" {
		t.Errorf("expected default listen 0.0.0.0, got %s", cfg.Listen)
	}
	if cfg.LogLevel != "error" {
		t.Errorf("expected default log level 'error', got %s", cfg.LogLevel)
	}
	if cfg.Verbose != false {
		t.Errorf("expected default verbose false, got %v", cfg.Verbose)
	}
	if cfg.Addr() != "0.0.0.0:8080" {
		t.Errorf("expected addr '0.0.0.0:8080', got %s", cfg.Addr())
	}
}

func TestLoad_Flags(t *testing.T) {
	args := []string{
		"-listen", "127.0.0.1",
		"-port", "9090",
		"-data-dir", "/tmp/tinyvm-test",
		"-log-level", "debug",
		"-log-format", "json",
	}

	cfg, _, err := Load(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Listen != "127.0.0.1" {
		t.Errorf("expected listen 127.0.0.1, got %s", cfg.Listen)
	}
	if cfg.Port != 9090 {
		t.Errorf("expected port 9090, got %d", cfg.Port)
	}
	if cfg.DataDir != "/tmp/tinyvm-test" {
		t.Errorf("expected data-dir /tmp/tinyvm-test, got %s", cfg.DataDir)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("expected log-level debug, got %s", cfg.LogLevel)
	}
	if cfg.LogFormat != "json" {
		t.Errorf("expected log-format json, got %s", cfg.LogFormat)
	}
	if cfg.Addr() != "127.0.0.1:9090" {
		t.Errorf("expected addr '127.0.0.1:9090', got %s", cfg.Addr())
	}
}

func TestLoad_Env(t *testing.T) {
	os.Setenv("TINYVM_PORT", "9999")
	os.Setenv("TINYVM_LISTEN", "10.0.0.1")
	defer func() {
		os.Unsetenv("TINYVM_PORT")
		os.Unsetenv("TINYVM_LISTEN")
	}()

	cfg, _, err := Load([]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Port != 9999 {
		t.Errorf("expected port 9999 from env, got %d", cfg.Port)
	}
	if cfg.Listen != "10.0.0.1" {
		t.Errorf("expected listen 10.0.0.1 from env, got %s", cfg.Listen)
	}
}

func TestEnsureDirs(t *testing.T) {
	tmpDir := filepath.Join(os.TempDir(), "tinyvm-test-dirs")
	defer os.RemoveAll(tmpDir)

	cfg := &Config{DataDir: tmpDir}
	if err := cfg.EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs failed: %v", err)
	}

	for _, dir := range []string{cfg.DataDir, cfg.ConfigDir(), cfg.VMsDir(), cfg.ISODir()} {
		info, err := os.Stat(dir)
		if err != nil {
			t.Errorf("directory %s does not exist: %v", dir, err)
		} else if !info.IsDir() {
			t.Errorf("%s is not a directory", dir)
		}
	}
}

func TestLoad_VerboseFlags(t *testing.T) {
	for _, arg := range []string{"-verbose", "--verbose", "-v"} {
		cfg, _, err := Load([]string{arg})
		if err != nil {
			t.Fatalf("Load([%s]) returned error: %v", arg, err)
		}
		if !cfg.Verbose {
			t.Errorf("Load([%s]) expected Verbose=true, got false", arg)
		}
		logger := cfg.SetupLogger()
		if !logger.Enabled(context.Background(), slog.LevelInfo) {
			t.Errorf("expected Info level enabled with %s", arg)
		}
	}
}

func TestLoad_VerboseEnv(t *testing.T) {
	os.Setenv("TINYVM_VERBOSE", "1")
	defer os.Unsetenv("TINYVM_VERBOSE")

	cfg, _, err := Load([]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.Verbose {
		t.Errorf("expected Verbose=true from env TINYVM_VERBOSE=1, got false")
	}

	logger := cfg.SetupLogger()
	if !logger.Enabled(context.Background(), slog.LevelInfo) {
		t.Errorf("expected Info level enabled with TINYVM_VERBOSE=1")
	}
}

func TestDefaultLogger_OnlyErrors(t *testing.T) {
	cfg := DefaultConfig()
	logger := cfg.SetupLogger()

	ctx := context.Background()
	if logger.Enabled(ctx, slog.LevelInfo) {
		t.Errorf("default logger should not enable Info level")
	}
	if logger.Enabled(ctx, slog.LevelWarn) {
		t.Errorf("default logger should not enable Warn level")
	}
	if !logger.Enabled(ctx, slog.LevelError) {
		t.Errorf("default logger must enable Error level")
	}
}
