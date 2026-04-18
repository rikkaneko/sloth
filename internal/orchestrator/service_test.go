package orchestrator

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sloth/internal/config"
	"sloth/internal/modules"
	"sloth/internal/storage"
)

type fakeEnvLoader struct {
	values map[string]string
	err    error
}

func (f fakeEnvLoader) Load(path string) (map[string]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.values, nil
}

type fakeModuleRegistry struct {
	module modules.Module
	err    error
}

func (f fakeModuleRegistry) Resolve(serviceType string, overridePath string) (modules.Module, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.module, nil
}

type fakeStorageProvider struct {
	listedObjects  []storage.ObjectInfo
	listedVersions []storage.ObjectInfo
	putKey         string
	putFile        string
	putCalls       int
	getKey         string
	getVersionID   string
	getDestPath    string
	getCalls       int
	getBody        []byte
}

func (f *fakeStorageProvider) Put(ctx context.Context, key string, localPath string) error {
	f.putKey = key
	f.putFile = localPath
	f.putCalls++
	return nil
}

func (f *fakeStorageProvider) Get(ctx context.Context, key string, versionID string, localPath string) error {
	f.getKey = key
	f.getVersionID = versionID
	f.getDestPath = localPath
	f.getCalls++
	body := f.getBody
	if len(body) == 0 {
		body = []byte("backup-data")
	}
	return os.WriteFile(localPath, body, 0o600)
}

func (f *fakeStorageProvider) ListObjects(ctx context.Context, prefix string) ([]storage.ObjectInfo, error) {
	return f.listedObjects, nil
}

func (f *fakeStorageProvider) ListObjectVersions(ctx context.Context, prefix string) ([]storage.ObjectInfo, error) {
	return f.listedVersions, nil
}

type testStorageFactory struct {
	provider storage.Provider
}

func (t testStorageFactory) Build(storageConfig config.StorageConfig) (storage.Provider, error) {
	return t.provider, nil
}

type fakeModule struct {
	artifactName string
	backupFile   string
}

func (f fakeModule) Type() string {
	return "mysql"
}

func (f fakeModule) SupportsLocal() bool {
	return true
}

func (f fakeModule) ArtifactFileName(service config.ServiceEntry) string {
	return f.artifactName
}

func (f fakeModule) Backup(ctx context.Context, req modules.BackupRequest) (modules.BackupResult, error) {
	return modules.BackupResult{
		LocalPath:    f.backupFile,
		ArtifactName: f.artifactName,
		ArtifactExt:  "sql",
	}, nil
}

func (f fakeModule) Restore(ctx context.Context, req modules.RestoreRequest) (modules.RestoreResult, error) {
	return modules.RestoreResult{}, nil
}

func TestResolveServiceForOperationCreatesLocalConfig(t *testing.T) {
	homeDir := t.TempDir()
	workingDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer os.Chdir(originalWD)
	if err := os.Chdir(workingDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	resolved, err := resolveServiceForOperation("svc", serviceResolutionOptions{
		Type:          "mysql",
		ContainerName: "db",
		Engine:        "local",
		AllowCreate:   true,
	})
	if err != nil {
		t.Fatalf("resolve service: %v", err)
	}

	if resolved.Service.Name != "svc" {
		t.Fatalf("expected service name svc, got %s", resolved.Service.Name)
	}
	if _, err := os.Stat(filepath.Join(workingDir, ".sloth.yaml")); err != nil {
		t.Fatalf("expected .sloth.yaml to be created: %v", err)
	}
}

func TestResolveServiceForOperationRequiresDefinitionOnCreate(t *testing.T) {
	homeDir := t.TempDir()
	workingDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer os.Chdir(originalWD)
	if err := os.Chdir(workingDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	_, err = resolveServiceForOperation("svc", serviceResolutionOptions{AllowCreate: true})
	if err == nil {
		t.Fatalf("expected error when type/container-name are missing")
	}
}

func TestBackupNonNativeUsesIncrementedVersion(t *testing.T) {
	homeDir := t.TempDir()
	workingDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	if err := os.MkdirAll(filepath.Join(homeDir, ".config", "sloth"), 0o755); err != nil {
		t.Fatalf("mkdir home config dir: %v", err)
	}
	mainConfig := "storage:\n  - name: default\n    type: s3\n    endpoint: https://example.com\n    region: us-east-1\n    bucket: backups\n    access_key_id: key\n    secret_access_key: secret\n    use_native_object_versioning: false\n    base_path: /backup\n"
	if err := os.WriteFile(filepath.Join(homeDir, ".config", "sloth", "main.yaml"), []byte(mainConfig), 0o600); err != nil {
		t.Fatalf("write main config: %v", err)
	}

	artifact := filepath.Join(workingDir, "artifact.sql")
	if err := os.WriteFile(artifact, []byte("data"), 0o600); err != nil {
		t.Fatalf("write artifact: %v", err)
	}

	provider := &fakeStorageProvider{
		listedObjects: []storage.ObjectInfo{
			{Key: "backup/app/1/app-mysql-backup.sql"},
			{Key: "backup/app/2/app-mysql-backup.sql"},
		},
	}

	manager := Manager{
		envLoader:      fakeEnvLoader{values: map[string]string{}},
		moduleRegistry: fakeModuleRegistry{module: fakeModule{artifactName: "app-mysql-backup.sql", backupFile: artifact}},
		storageFactory: testStorageFactory{provider: provider},
		now: func() time.Time {
			return time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
		},
	}

	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer os.Chdir(originalWD)
	if err := os.Chdir(workingDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	outcome, err := manager.Backup(context.Background(), BackupOptions{
		ServiceID:     "app",
		Type:          "mysql",
		ContainerName: "app-db",
		Local:         true,
	})
	if err != nil {
		t.Fatalf("backup: %v", err)
	}

	if outcome.Version != "3" {
		t.Fatalf("expected version 3, got %s", outcome.Version)
	}
	if provider.putKey != "backup/app/3/app-mysql-backup.sql" {
		t.Fatalf("unexpected object key %s", provider.putKey)
	}

	savedConfig, err := os.ReadFile(filepath.Join(workingDir, ".sloth.yaml"))
	if err != nil {
		t.Fatalf("read saved local config: %v", err)
	}
	if !strings.Contains(string(savedConfig), "last_backup_time") {
		t.Fatalf("expected last_backup_time to be persisted")
	}
}

func TestBackupSkipsUploadWhenChecksumMatches(t *testing.T) {
	homeDir := t.TempDir()
	workingDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	if err := os.MkdirAll(filepath.Join(homeDir, ".config", "sloth"), 0o755); err != nil {
		t.Fatalf("mkdir home config dir: %v", err)
	}
	mainConfig := "storage:\n  - name: default\n    type: s3\n    endpoint: https://example.com\n    region: us-east-1\n    bucket: backups\n    access_key_id: key\n    secret_access_key: secret\n    use_native_object_versioning: false\n    base_path: /backup\n"
	if err := os.WriteFile(filepath.Join(homeDir, ".config", "sloth", "main.yaml"), []byte(mainConfig), 0o600); err != nil {
		t.Fatalf("write main config: %v", err)
	}

	content := []byte("same-checksum")
	artifact := filepath.Join(workingDir, "artifact.sql")
	if err := os.WriteFile(artifact, content, 0o600); err != nil {
		t.Fatalf("write artifact: %v", err)
	}

	provider := &fakeStorageProvider{
		listedObjects: []storage.ObjectInfo{
			{
				Key:          "backup/app/1/app-mysql-backup.sql",
				Size:         int64(len(content)),
				LastModified: time.Date(2026, 4, 17, 11, 0, 0, 0, time.UTC),
			},
		},
		getBody: content,
	}

	manager := Manager{
		envLoader:      fakeEnvLoader{values: map[string]string{}},
		moduleRegistry: fakeModuleRegistry{module: fakeModule{artifactName: "app-mysql-backup.sql", backupFile: artifact}},
		storageFactory: testStorageFactory{provider: provider},
		now: func() time.Time {
			return time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
		},
	}

	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer os.Chdir(originalWD)
	if err := os.Chdir(workingDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	outcome, err := manager.Backup(context.Background(), BackupOptions{
		ServiceID:     "app",
		Type:          "mysql",
		ContainerName: "app-db",
		Local:         true,
	})
	if err != nil {
		t.Fatalf("backup: %v", err)
	}

	if !outcome.Skipped {
		t.Fatalf("expected backup to be skipped")
	}
	if provider.putCalls != 0 {
		t.Fatalf("expected no upload call when checksum matches")
	}
	if provider.getCalls == 0 {
		t.Fatalf("expected checksum compare to download latest object")
	}
}

func TestBackupSkipsUploadWhenFileSizeCheckEnabled(t *testing.T) {
	homeDir := t.TempDir()
	workingDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	if err := os.MkdirAll(filepath.Join(homeDir, ".config", "sloth"), 0o755); err != nil {
		t.Fatalf("mkdir home config dir: %v", err)
	}
	mainConfig := "storage:\n  - name: default\n    type: s3\n    endpoint: https://example.com\n    region: us-east-1\n    bucket: backups\n    access_key_id: key\n    secret_access_key: secret\n    use_native_object_versioning: false\n    base_path: /backup\n"
	if err := os.WriteFile(filepath.Join(homeDir, ".config", "sloth", "main.yaml"), []byte(mainConfig), 0o600); err != nil {
		t.Fatalf("write main config: %v", err)
	}

	content := []byte("size-match")
	artifact := filepath.Join(workingDir, "artifact.sql")
	if err := os.WriteFile(artifact, content, 0o600); err != nil {
		t.Fatalf("write artifact: %v", err)
	}

	provider := &fakeStorageProvider{
		listedObjects: []storage.ObjectInfo{
			{
				Key:          "backup/app/1/app-mysql-backup.sql",
				Size:         int64(len(content)),
				LastModified: time.Date(2026, 4, 17, 11, 0, 0, 0, time.UTC),
			},
		},
		getBody: []byte("different-content"),
	}

	manager := Manager{
		envLoader:      fakeEnvLoader{values: map[string]string{}},
		moduleRegistry: fakeModuleRegistry{module: fakeModule{artifactName: "app-mysql-backup.sql", backupFile: artifact}},
		storageFactory: testStorageFactory{provider: provider},
		now: func() time.Time {
			return time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
		},
	}

	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer os.Chdir(originalWD)
	if err := os.Chdir(workingDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	outcome, err := manager.Backup(context.Background(), BackupOptions{
		ServiceID:     "app",
		Type:          "mysql",
		ContainerName: "app-db",
		Local:         true,
		UseFileSize:   true,
	})
	if err != nil {
		t.Fatalf("backup: %v", err)
	}

	if !outcome.Skipped {
		t.Fatalf("expected backup to be skipped by file-size check")
	}
	if provider.putCalls != 0 {
		t.Fatalf("expected no upload call for file-size skip")
	}
	if provider.getCalls != 0 {
		t.Fatalf("expected no checksum download when only file-size check is used")
	}
}

func TestBackupForceUploadsEvenWhenDeltaMatches(t *testing.T) {
	homeDir := t.TempDir()
	workingDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	if err := os.MkdirAll(filepath.Join(homeDir, ".config", "sloth"), 0o755); err != nil {
		t.Fatalf("mkdir home config dir: %v", err)
	}
	mainConfig := "storage:\n  - name: default\n    type: s3\n    endpoint: https://example.com\n    region: us-east-1\n    bucket: backups\n    access_key_id: key\n    secret_access_key: secret\n    use_native_object_versioning: false\n    base_path: /backup\n"
	if err := os.WriteFile(filepath.Join(homeDir, ".config", "sloth", "main.yaml"), []byte(mainConfig), 0o600); err != nil {
		t.Fatalf("write main config: %v", err)
	}

	content := []byte("same-checksum")
	artifact := filepath.Join(workingDir, "artifact.sql")
	if err := os.WriteFile(artifact, content, 0o600); err != nil {
		t.Fatalf("write artifact: %v", err)
	}

	provider := &fakeStorageProvider{
		listedObjects: []storage.ObjectInfo{
			{
				Key:          "backup/app/1/app-mysql-backup.sql",
				Size:         int64(len(content)),
				LastModified: time.Date(2026, 4, 17, 11, 0, 0, 0, time.UTC),
			},
		},
		getBody: content,
	}

	manager := Manager{
		envLoader:      fakeEnvLoader{values: map[string]string{}},
		moduleRegistry: fakeModuleRegistry{module: fakeModule{artifactName: "app-mysql-backup.sql", backupFile: artifact}},
		storageFactory: testStorageFactory{provider: provider},
		now: func() time.Time {
			return time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
		},
	}

	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer os.Chdir(originalWD)
	if err := os.Chdir(workingDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	outcome, err := manager.Backup(context.Background(), BackupOptions{
		ServiceID:     "app",
		Type:          "mysql",
		ContainerName: "app-db",
		Local:         true,
		Force:         true,
	})
	if err != nil {
		t.Fatalf("backup: %v", err)
	}

	if outcome.Skipped {
		t.Fatalf("expected backup to upload when force is enabled")
	}
	if provider.putCalls == 0 {
		t.Fatalf("expected upload call when force is enabled")
	}
	if provider.getCalls != 0 {
		t.Fatalf("expected force to bypass delta compare download")
	}
}
