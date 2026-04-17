package versioning

import (
	"testing"
	"time"

	"sloth/internal/storage"
)

func TestNextVersionID(t *testing.T) {
	prefix := "backup/service-a"
	objects := []storage.ObjectInfo{
		{Key: "backup/service-a/1/file.sql"},
		{Key: "backup/service-a/3/file.sql"},
		{Key: "backup/service-a/2/file.sql"},
		{Key: "backup/service-a/not-a-version/file.sql"},
	}

	got := NextVersionID(objects, prefix)
	if got != "4" {
		t.Fatalf("expected next version 4, got %s", got)
	}
}

func TestExtractVersionFromKey(t *testing.T) {
	got := ExtractVersionFromKey("backup/service-a/12/file.sql", "backup/service-a")
	if got != "12" {
		t.Fatalf("expected 12, got %s", got)
	}

	missing := ExtractVersionFromKey("backup/service-a/file.sql", "backup/service-a")
	if missing != "" {
		t.Fatalf("expected empty version, got %s", missing)
	}
}

func TestSelectLatestVersion(t *testing.T) {
	now := time.Now()
	objects := []storage.ObjectInfo{
		{Key: "backup/service-a/1/file.sql", LastModified: now.Add(-2 * time.Hour)},
		{Key: "backup/service-a/5/file.sql", LastModified: now.Add(-1 * time.Hour)},
		{Key: "backup/service-a/3/file.sql", LastModified: now.Add(-3 * time.Hour)},
	}

	latest, err := SelectLatestVersion(objects, "backup/service-a")
	if err != nil {
		t.Fatalf("select latest version: %v", err)
	}
	if latest != "5" {
		t.Fatalf("expected latest version 5, got %s", latest)
	}
}
