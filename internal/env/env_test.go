package env

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadEnvWithInterpolationAndQuotes(t *testing.T) {
	directory := t.TempDir()
	envPath := filepath.Join(directory, ".env")

	content := "DB_USER=user\nDB_PASS=secret\nDB_NAME=app\nDATABASE_URL=postgres://${DB_USER}:${DB_PASS}@localhost/${DB_NAME}\nQUOTED='hello world'\n"
	if err := os.WriteFile(envPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	loader := NewLoader()
	values, err := loader.Load(envPath)
	if err != nil {
		t.Fatalf("load env: %v", err)
	}

	if values["DATABASE_URL"] != "postgres://user:secret@localhost/app" {
		t.Fatalf("unexpected DATABASE_URL: %s", values["DATABASE_URL"])
	}
	if values["QUOTED"] != "hello world" {
		t.Fatalf("unexpected QUOTED: %s", values["QUOTED"])
	}
}

func TestLoadEnvDefaultMissingFileReturnsEmptyMap(t *testing.T) {
	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer os.Chdir(originalWD)

	workingDir := t.TempDir()
	if err := os.Chdir(workingDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	loader := NewLoader()
	values, err := loader.Load("")
	if err != nil {
		t.Fatalf("expected no error when default .env is missing, got %v", err)
	}
	if len(values) != 0 {
		t.Fatalf("expected empty env map, got %d entries", len(values))
	}
}
