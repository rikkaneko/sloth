package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadServiceConfigUsesDefaultConfigHomePath(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Cleanup(func() {
		if err := SetConfigHomeOverride(""); err != nil {
			t.Fatalf("reset config home override: %v", err)
		}
	})

	servicePath := filepath.Join(homeDir, ".config", "sloth", "service.yaml")
	if err := os.MkdirAll(filepath.Dir(servicePath), 0o755); err != nil {
		t.Fatalf("mkdir home service dir: %v", err)
	}
	if err := os.WriteFile(servicePath, []byte("service:\n  - name: home\n    type: mysql\n"), 0o600); err != nil {
		t.Fatalf("write home service config: %v", err)
	}

	result, err := LoadServiceConfig()
	if err != nil {
		t.Fatalf("load service config: %v", err)
	}

	if result.Source != servicePath {
		t.Fatalf("expected source %s, got %s", servicePath, result.Source)
	}
	if len(result.Config.Service) != 1 || result.Config.Service[0].Name != "home" {
		t.Fatalf("expected default home service config to be loaded")
	}
}

func TestLoadServiceConfigUsesOverrideConfigHomePath(t *testing.T) {
	homeDir := t.TempDir()
	overrideHome := t.TempDir()

	t.Setenv("HOME", homeDir)
	t.Cleanup(func() {
		if err := SetConfigHomeOverride(""); err != nil {
			t.Fatalf("reset config home override: %v", err)
		}
	})

	if err := SetConfigHomeOverride(overrideHome); err != nil {
		t.Fatalf("set config home override: %v", err)
	}

	servicePath := filepath.Join(overrideHome, "service.yaml")
	if err := os.WriteFile(servicePath, []byte("service:\n  - name: custom\n    type: pgsql\n"), 0o600); err != nil {
		t.Fatalf("write override service config: %v", err)
	}

	result, err := LoadServiceConfig()
	if err != nil {
		t.Fatalf("load service config: %v", err)
	}

	if result.Source != servicePath {
		t.Fatalf("expected source %s, got %s", servicePath, result.Source)
	}
	if len(result.Config.Service) != 1 || result.Config.Service[0].Name != "custom" {
		t.Fatalf("expected override service config to be loaded")
	}
}

func TestLoadServiceConfigReturnsResolvedPathWhenMissing(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Cleanup(func() {
		if err := SetConfigHomeOverride(""); err != nil {
			t.Fatalf("reset config home override: %v", err)
		}
	})

	result, err := LoadServiceConfig()
	if err != nil {
		t.Fatalf("load service config: %v", err)
	}

	expectedSource := filepath.Join(homeDir, ".config", "sloth", "service.yaml")
	if result.Source != expectedSource {
		t.Fatalf("expected source %s, got %s", expectedSource, result.Source)
	}
	if len(result.Config.Service) != 0 {
		t.Fatalf("expected empty service config")
	}
}

func TestLoadServiceConfigIgnoresLegacyLocalSlothFile(t *testing.T) {
	homeDir := t.TempDir()
	workingDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Cleanup(func() {
		if err := SetConfigHomeOverride(""); err != nil {
			t.Fatalf("reset config home override: %v", err)
		}
	})

	if err := os.WriteFile(filepath.Join(workingDir, ".sloth.yaml"), []byte("service:\n  - name: legacy\n    type: mysql\n"), 0o600); err != nil {
		t.Fatalf("write legacy local config: %v", err)
	}

	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer os.Chdir(originalWD)
	if err := os.Chdir(workingDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	result, err := LoadServiceConfig()
	if err != nil {
		t.Fatalf("load service config: %v", err)
	}

	expectedSource := filepath.Join(homeDir, ".config", "sloth", "service.yaml")
	if result.Source != expectedSource {
		t.Fatalf("expected source %s, got %s", expectedSource, result.Source)
	}
	if len(result.Config.Service) != 0 {
		t.Fatalf("expected empty service config when only .sloth.yaml exists")
	}
}

func TestSaveServiceConfigDefaultWritesToConfigHomeServicePath(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Cleanup(func() {
		if err := SetConfigHomeOverride(""); err != nil {
			t.Fatalf("reset config home override: %v", err)
		}
	})

	cfg := ServiceConfig{
		Service: []ServiceEntry{
			{Name: "svc", Type: "mysql"},
		},
	}
	if err := SaveServiceConfig("", cfg); err != nil {
		t.Fatalf("save service config: %v", err)
	}

	expectedPath := filepath.Join(homeDir, ".config", "sloth", "service.yaml")
	if _, err := os.Stat(expectedPath); err != nil {
		t.Fatalf("expected service config at %s: %v", expectedPath, err)
	}
}

func TestNormalizeBasePath(t *testing.T) {
	got := NormalizeBasePath("/backup/")
	if got != "backup" {
		t.Fatalf("expected backup, got %s", got)
	}

	got = NormalizeBasePath("")
	if got != "backup" {
		t.Fatalf("expected backup for empty base path, got %s", got)
	}
}

func TestMainConfigValidateAcceptsCommonFileDeltaCheck(t *testing.T) {
	cfg := MainConfig{
		Storage: []StorageConfig{
			{
				Name:            "default",
				Type:            "s3",
				Endpoint:        "https://example.com",
				Bucket:          "backup",
				AccessKeyID:     "key",
				SecretAccessKey: "secret",
			},
		},
		Common: CommonConfig{
			FileDeltaCheck: "file_size",
		},
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("validate main config: %v", err)
	}
	if cfg.ResolveFileDeltaCheck() != "file_size" {
		t.Fatalf("expected file_size mode")
	}
}

func TestMainConfigValidateRejectsInvalidCommonFileDeltaCheck(t *testing.T) {
	cfg := MainConfig{
		Storage: []StorageConfig{
			{
				Name:            "default",
				Type:            "s3",
				Endpoint:        "https://example.com",
				Bucket:          "backup",
				AccessKeyID:     "key",
				SecretAccessKey: "secret",
			},
		},
		Common: CommonConfig{
			FileDeltaCheck: "invalid",
		},
	}

	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected invalid file_delta_check error")
	}
}

func TestResolveConfigHomeUsesOverride(t *testing.T) {
	t.Cleanup(func() {
		if err := SetConfigHomeOverride(""); err != nil {
			t.Fatalf("reset config home override: %v", err)
		}
	})

	if err := SetConfigHomeOverride("/tmp/sloth-config"); err != nil {
		t.Fatalf("set config home override: %v", err)
	}

	configHome, err := ResolveConfigHome()
	if err != nil {
		t.Fatalf("resolve config home: %v", err)
	}
	if configHome != "/tmp/sloth-config" {
		t.Fatalf("expected /tmp/sloth-config, got %s", configHome)
	}
}
