package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadServiceConfigPrefersHomePath(t *testing.T) {
	homeDir := t.TempDir()
	workingDir := t.TempDir()

	t.Setenv("HOME", homeDir)

	homeServicePath := filepath.Join(homeDir, ".config", "sloth", "service.yaml")
	if err := os.MkdirAll(filepath.Dir(homeServicePath), 0o755); err != nil {
		t.Fatalf("mkdir home service dir: %v", err)
	}
	if err := os.WriteFile(homeServicePath, []byte("service:\n  - name: home\n    type: mysql\n"), 0o600); err != nil {
		t.Fatalf("write home service config: %v", err)
	}

	if err := os.WriteFile(filepath.Join(workingDir, ".sloth.yaml"), []byte("service:\n  - name: local\n    type: pgsql\n"), 0o600); err != nil {
		t.Fatalf("write local service config: %v", err)
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

	if result.Source != homeServicePath {
		t.Fatalf("expected source %s, got %s", homeServicePath, result.Source)
	}
	if len(result.Config.Service) != 1 || result.Config.Service[0].Name != "home" {
		t.Fatalf("expected home service config to be loaded")
	}
}

func TestLoadServiceConfigFallsBackToLocalPath(t *testing.T) {
	homeDir := t.TempDir()
	workingDir := t.TempDir()

	t.Setenv("HOME", homeDir)

	localPath := filepath.Join(workingDir, ".sloth.yaml")
	if err := os.WriteFile(localPath, []byte("service:\n  - name: local\n    type: pgsql\n"), 0o600); err != nil {
		t.Fatalf("write local service config: %v", err)
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

	if result.Source != ".sloth.yaml" {
		t.Fatalf("expected local source .sloth.yaml, got %s", result.Source)
	}
	if len(result.Config.Service) != 1 || result.Config.Service[0].Name != "local" {
		t.Fatalf("expected local service config to be loaded")
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
