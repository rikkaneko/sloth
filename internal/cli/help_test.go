package cli

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunRootHelpWithDynamicValues(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	mainConfigPath := filepath.Join(homeDir, ".config", "sloth", "main.yaml")
	if err := os.MkdirAll(filepath.Dir(mainConfigPath), 0o755); err != nil {
		t.Fatalf("mkdir home config dir: %v", err)
	}

	mainConfig := "storage:\n" +
		"  - name: default\n" +
		"    type: s3\n" +
		"    endpoint: https://example.com\n" +
		"    region: auto\n" +
		"    bucket: backups\n" +
		"    access_key_id: key\n" +
		"    secret_access_key: secret\n" +
		"  - name: archive\n" +
		"    type: s3\n" +
		"    endpoint: https://archive.example.com\n" +
		"    region: auto\n" +
		"    bucket: archive\n" +
		"    access_key_id: key\n" +
		"    secret_access_key: secret\n"
	if err := os.WriteFile(mainConfigPath, []byte(mainConfig), 0o600); err != nil {
		t.Fatalf("write main config: %v", err)
	}

	app := NewApp("test")
	output, err := runWithCapturedStdout(t, func() error {
		return app.Run(context.Background(), []string{"--help"})
	})
	if err != nil {
		t.Fatalf("run help: %v", err)
	}

	assertContains(t, output, "SYNOPSIS")
	assertContains(t, output, "GLOBAL OPTIONS")
	assertContains(t, output, "Default compiled-in and discovered parameters:")
	assertContains(t, output, "Available service types:")
	assertContains(t, output, "volume")
	assertContains(t, output, "Available container engines: docker, podman")
	assertContains(t, output, "archive")
	assertContains(t, output, "default")
}

func TestRunBackupHelpWithoutServiceID(t *testing.T) {
	app := NewApp("test")
	output, err := runWithCapturedStdout(t, func() error {
		return app.Run(context.Background(), []string{"backup", "--help"})
	})
	if err != nil {
		t.Fatalf("run backup help: %v", err)
	}

	assertContains(t, output, "sloth backup <service-id> [options]")
	assertContains(t, output, "GLOBAL OPTIONS")
	assertContains(t, output, "-t, --type <service-type>")
	assertContains(t, output, "-l, --local")
	assertContains(t, output, "-k, --keep")
	assertContains(t, output, "--dry-run")
	assertContains(t, output, "--force")
	assertContains(t, output, "--use-checksum")
	assertContains(t, output, "--use-file-size-check")
	assertContains(t, output, "-d, --debug")
	assertContains(t, output, "available:")
}

func TestRunHelpSubcommandForRestore(t *testing.T) {
	app := NewApp("test")
	output, err := runWithCapturedStdout(t, func() error {
		return app.Run(context.Background(), []string{"help", "restore"})
	})
	if err != nil {
		t.Fatalf("run help restore: %v", err)
	}

	assertContains(t, output, "sloth restore <service-id>")
	assertContains(t, output, "GLOBAL OPTIONS")
	assertContains(t, output, "-E, --engine <container-engine>")
}

func TestRunRootHelpGracefulWhenStorageConfigMissing(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	app := NewApp("test")
	output, err := runWithCapturedStdout(t, func() error {
		return app.Run(context.Background(), []string{"-h"})
	})
	if err != nil {
		t.Fatalf("run root help: %v", err)
	}

	assertContains(t, output, "Available storage names: unavailable")
}

func TestRunRootHelpUsesConfigHomeOverride(t *testing.T) {
	homeDir := t.TempDir()
	overrideConfigHome := t.TempDir()
	t.Setenv("HOME", homeDir)

	mainConfigPath := filepath.Join(overrideConfigHome, "main.yaml")
	mainConfig := "storage:\n" +
		"  - name: override\n" +
		"    type: s3\n" +
		"    endpoint: https://example.com\n" +
		"    region: auto\n" +
		"    bucket: backups\n" +
		"    access_key_id: key\n" +
		"    secret_access_key: secret\n"
	if err := os.WriteFile(mainConfigPath, []byte(mainConfig), 0o600); err != nil {
		t.Fatalf("write override main config: %v", err)
	}

	app := NewApp("test")
	output, err := runWithCapturedStdout(t, func() error {
		return app.Run(context.Background(), []string{"-C", overrideConfigHome, "--help"})
	})
	if err != nil {
		t.Fatalf("run root help with config home override: %v", err)
	}

	assertContains(t, output, "override")
}

func TestRunListHelpIncludesShowObjectKey(t *testing.T) {
	app := NewApp("test")
	output, err := runWithCapturedStdout(t, func() error {
		return app.Run(context.Background(), []string{"list", "--help"})
	})
	if err != nil {
		t.Fatalf("run list help: %v", err)
	}

	assertContains(t, output, "--show-object-key")
	assertContains(t, output, "--remote")
	assertContains(t, output, "GLOBAL OPTIONS")
	assertContains(t, output, "sloth list [--remote] [<service-id>]")
}

func TestRunListHelpAcceptsConfigHomeAfterSubcommand(t *testing.T) {
	homeDir := t.TempDir()
	overrideConfigHome := t.TempDir()
	t.Setenv("HOME", homeDir)

	mainConfigPath := filepath.Join(overrideConfigHome, "main.yaml")
	mainConfig := "storage:\n" +
		"  - name: after-subcommand\n" +
		"    type: s3\n" +
		"    endpoint: https://example.com\n" +
		"    region: auto\n" +
		"    bucket: backups\n" +
		"    access_key_id: key\n" +
		"    secret_access_key: secret\n"
	if err := os.WriteFile(mainConfigPath, []byte(mainConfig), 0o600); err != nil {
		t.Fatalf("write override main config: %v", err)
	}

	app := NewApp("test")
	output, err := runWithCapturedStdout(t, func() error {
		return app.Run(context.Background(), []string{"list", "-C", overrideConfigHome, "--help"})
	})
	if err != nil {
		t.Fatalf("run list help with config-home after subcommand: %v", err)
	}

	assertContains(t, output, "after-subcommand")
}

func TestRunHelpUnknownTopic(t *testing.T) {
	app := NewApp("test")
	_, err := runWithCapturedStdout(t, func() error {
		return app.Run(context.Background(), []string{"help", "unknown"})
	})
	if err == nil {
		t.Fatalf("expected error for unknown help topic")
	}
}

func runWithCapturedStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()

	originalStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stdout pipe: %v", err)
	}

	os.Stdout = writer
	runErr := fn()
	writer.Close()
	os.Stdout = originalStdout

	var buffer bytes.Buffer
	if _, err := io.Copy(&buffer, reader); err != nil {
		t.Fatalf("copy captured stdout: %v", err)
	}
	return buffer.String(), runErr
}

func assertContains(t *testing.T, output string, expected string) {
	t.Helper()
	if !strings.Contains(output, expected) {
		t.Fatalf("expected output to contain %q\n%s", expected, output)
	}
}
