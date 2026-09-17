package config

import (
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
	if cfg.LogLevel != "info" {
		t.Errorf("expected default log level 'info', got %s", cfg.LogLevel)
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
