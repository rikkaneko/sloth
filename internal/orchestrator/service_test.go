package orchestrator

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
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
	listedObjects       []storage.ObjectInfo
	listedVersions      []storage.ObjectInfo
	listObjectsPrefixes []string
	listVersionPrefixes []string
	headMetadata        storage.ObjectMetadata
	headCalls           int
	headKey             string
	headVersionID       string
	putKey              string
	putFile             string
	putCalls            int
	getKey              string
	getVersionID        string
	getDestPath         string
	getCalls            int
	getBody             []byte
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
	f.listObjectsPrefixes = append(f.listObjectsPrefixes, prefix)
	return f.listedObjects, nil
}

func (f *fakeStorageProvider) ListObjectVersions(ctx context.Context, prefix string) ([]storage.ObjectInfo, error) {
	f.listVersionPrefixes = append(f.listVersionPrefixes, prefix)
	return f.listedVersions, nil
}

func (f *fakeStorageProvider) HeadObject(ctx context.Context, key string, versionID string) (storage.ObjectMetadata, error) {
	f.headCalls++
	f.headKey = key
	f.headVersionID = versionID
	return f.headMetadata, nil
}

type testStorageFactory struct {
	provider storage.Provider
}

func (t testStorageFactory) Build(storageConfig config.StorageConfig) (storage.Provider, error) {
	return t.provider, nil
}

type namedTestStorageFactory struct {
	providers map[string]storage.Provider
}

func (t namedTestStorageFactory) Build(storageConfig config.StorageConfig) (storage.Provider, error) {
	provider, ok := t.providers[storageConfig.Name]
	if !ok {
		return nil, fmt.Errorf("provider for storage %q not found", storageConfig.Name)
	}
	return provider, nil
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
		headMetadata: storage.ObjectMetadata{
			Size:           int64(len(content)),
			ChecksumSHA256: checksumSHA256Base64(content),
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

	if !outcome.Skipped {
		t.Fatalf("expected backup to be skipped")
	}
	if provider.putCalls != 0 {
		t.Fatalf("expected no upload call when checksum matches")
	}
	if provider.headCalls == 0 {
		t.Fatalf("expected checksum compare to use head object metadata")
	}
	if provider.getCalls != 0 {
		t.Fatalf("expected no object download for checksum compare")
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
		headMetadata: storage.ObjectMetadata{
			Size:           int64(len(content)),
			ChecksumSHA256: "",
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
	if provider.headCalls == 0 {
		t.Fatalf("expected file-size compare to use head object metadata")
	}
	if provider.getCalls != 0 {
		t.Fatalf("expected no object download when only file-size check is used")
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
		headMetadata: storage.ObjectMetadata{
			Size:           int64(len(content)),
			ChecksumSHA256: checksumSHA256Base64(content),
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
	if provider.headCalls != 0 {
		t.Fatalf("expected force to bypass delta compare")
	}
	if provider.getCalls != 0 {
		t.Fatalf("expected force to avoid object download for delta compare")
	}
}

func TestBackupChecksumMissingFallsBackToFileSizeAndSkipsWhenSizeMatches(t *testing.T) {
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

	content := []byte("checksum-missing-size-match")
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
		headMetadata: storage.ObjectMetadata{
			Size:           int64(len(content)),
			ChecksumSHA256: "",
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

	output, err := captureStdout(t, func() error {
		_, backupErr := manager.Backup(context.Background(), BackupOptions{
			ServiceID:     "app",
			Type:          "mysql",
			ContainerName: "app-db",
			Local:         true,
			UseChecksum:   true,
		})
		return backupErr
	})
	if err != nil {
		t.Fatalf("backup: %v", err)
	}

	if provider.putCalls != 0 {
		t.Fatalf("expected skip when size matches after checksum fallback")
	}
	if !strings.Contains(output, "Remote checksum is unavailable, fallback to file-size check") {
		t.Fatalf("expected fallback warning in output, got: %s", output)
	}
}

func TestBackupChecksumMissingFallsBackToFileSizeAndUploadsWhenSizeDiffers(t *testing.T) {
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

	content := []byte("checksum-missing-size-different-local")
	artifact := filepath.Join(workingDir, "artifact.sql")
	if err := os.WriteFile(artifact, content, 0o600); err != nil {
		t.Fatalf("write artifact: %v", err)
	}

	provider := &fakeStorageProvider{
		listedObjects: []storage.ObjectInfo{
			{
				Key:          "backup/app/1/app-mysql-backup.sql",
				Size:         5,
				LastModified: time.Date(2026, 4, 17, 11, 0, 0, 0, time.UTC),
			},
		},
		headMetadata: storage.ObjectMetadata{
			Size:           5,
			ChecksumSHA256: "",
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

	output, err := captureStdout(t, func() error {
		_, backupErr := manager.Backup(context.Background(), BackupOptions{
			ServiceID:     "app",
			Type:          "mysql",
			ContainerName: "app-db",
			Local:         true,
			UseChecksum:   true,
		})
		return backupErr
	})
	if err != nil {
		t.Fatalf("backup: %v", err)
	}

	if provider.putCalls == 0 {
		t.Fatalf("expected upload when fallback file size differs")
	}
	if !strings.Contains(output, "Remote checksum is unavailable, fallback to file-size check") {
		t.Fatalf("expected fallback warning in output, got: %s", output)
	}
}

func TestListRemoteGroupsServicesByStorage(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	mainConfigPath := filepath.Join(homeDir, ".config", "sloth", "main.yaml")
	if err := os.MkdirAll(filepath.Dir(mainConfigPath), 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	mainConfig := "" +
		"storage:\n" +
		"  - name: archive\n" +
		"    type: s3\n" +
		"    endpoint: https://archive.example.com\n" +
		"    region: us-east-1\n" +
		"    bucket: archive\n" +
		"    access_key_id: key\n" +
		"    secret_access_key: secret\n" +
		"    use_native_object_versioning: false\n" +
		"    base_path: /backup\n" +
		"  - name: default\n" +
		"    type: s3\n" +
		"    endpoint: https://default.example.com\n" +
		"    region: us-east-1\n" +
		"    bucket: default\n" +
		"    access_key_id: key\n" +
		"    secret_access_key: secret\n" +
		"    use_native_object_versioning: false\n" +
		"    base_path: /backup\n" +
		"  - name: empty\n" +
		"    type: s3\n" +
		"    endpoint: https://empty.example.com\n" +
		"    region: us-east-1\n" +
		"    bucket: empty\n" +
		"    access_key_id: key\n" +
		"    secret_access_key: secret\n" +
		"    use_native_object_versioning: false\n" +
		"    base_path: /backup\n"
	if err := os.WriteFile(mainConfigPath, []byte(mainConfig), 0o600); err != nil {
		t.Fatalf("write main config: %v", err)
	}

	archiveProvider := &fakeStorageProvider{
		listedObjects: []storage.ObjectInfo{
			{
				Key:          "backup/svc-b/1/svc-b.sql",
				LastModified: time.Date(2026, 4, 18, 8, 0, 0, 0, time.UTC),
			},
			{
				Key:          "backup/svc-a/1/svc-a.sql",
				LastModified: time.Date(2026, 4, 18, 9, 0, 0, 0, time.UTC),
			},
			{
				Key:          "backup/svc-a/2/svc-a.sql",
				LastModified: time.Date(2026, 4, 18, 10, 0, 0, 0, time.UTC),
			},
		},
	}
	defaultProvider := &fakeStorageProvider{
		listedObjects: []storage.ObjectInfo{
			{
				Key:          "backup/svc-c/1/svc-c.sql",
				LastModified: time.Date(2026, 4, 18, 11, 0, 0, 0, time.UTC),
			},
		},
	}
	emptyProvider := &fakeStorageProvider{}

	manager := Manager{
		storageFactory: namedTestStorageFactory{
			providers: map[string]storage.Provider{
				"archive": archiveProvider,
				"default": defaultProvider,
				"empty":   emptyProvider,
			},
		},
	}

	outcome, err := manager.List(context.Background(), ListOptions{Remote: true})
	if err != nil {
		t.Fatalf("list remote: %v", err)
	}

	if len(outcome.RemoteServiceGroups) != 2 {
		t.Fatalf("expected 2 non-empty storage groups, got %d", len(outcome.RemoteServiceGroups))
	}
	if outcome.RemoteServiceGroups[0].Storage != "archive" || outcome.RemoteServiceGroups[1].Storage != "default" {
		t.Fatalf("unexpected group order: %+v", outcome.RemoteServiceGroups)
	}

	archiveRows := outcome.RemoteServiceGroups[0].Rows
	if len(archiveRows) != 2 {
		t.Fatalf("expected 2 archive rows, got %d", len(archiveRows))
	}
	if archiveRows[0].Service != "svc-a" || archiveRows[1].Service != "svc-b" {
		t.Fatalf("expected service rows sorted by name, got %+v", archiveRows)
	}
	if archiveRows[0].ObjectKey != "backup/svc-a/2/svc-a.sql" {
		t.Fatalf("expected latest object key for svc-a, got %q", archiveRows[0].ObjectKey)
	}

	if len(archiveProvider.listObjectsPrefixes) != 1 || archiveProvider.listObjectsPrefixes[0] != "backup/" {
		t.Fatalf("expected archive list prefix backup/, got %+v", archiveProvider.listObjectsPrefixes)
	}
	if len(defaultProvider.listObjectsPrefixes) != 1 || defaultProvider.listObjectsPrefixes[0] != "backup/" {
		t.Fatalf("expected default list prefix backup/, got %+v", defaultProvider.listObjectsPrefixes)
	}
	if len(emptyProvider.listObjectsPrefixes) != 1 || emptyProvider.listObjectsPrefixes[0] != "backup/" {
		t.Fatalf("expected empty list prefix backup/, got %+v", emptyProvider.listObjectsPrefixes)
	}
}

func TestListRemoteServiceIDGroupsBackupsByStorage(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	mainConfigPath := filepath.Join(homeDir, ".config", "sloth", "main.yaml")
	if err := os.MkdirAll(filepath.Dir(mainConfigPath), 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	mainConfig := "" +
		"storage:\n" +
		"  - name: archive\n" +
		"    type: s3\n" +
		"    endpoint: https://archive.example.com\n" +
		"    region: us-east-1\n" +
		"    bucket: archive\n" +
		"    access_key_id: key\n" +
		"    secret_access_key: secret\n" +
		"    use_native_object_versioning: false\n" +
		"    base_path: /backup\n" +
		"  - name: versioned\n" +
		"    type: s3\n" +
		"    endpoint: https://versioned.example.com\n" +
		"    region: us-east-1\n" +
		"    bucket: versioned\n" +
		"    access_key_id: key\n" +
		"    secret_access_key: secret\n" +
		"    use_native_object_versioning: true\n" +
		"    base_path: /backup\n"
	if err := os.WriteFile(mainConfigPath, []byte(mainConfig), 0o600); err != nil {
		t.Fatalf("write main config: %v", err)
	}

	archiveProvider := &fakeStorageProvider{
		listedObjects: []storage.ObjectInfo{
			{
				Key:          "backup/svc/1/svc.sql",
				LastModified: time.Date(2026, 4, 18, 9, 0, 0, 0, time.UTC),
			},
			{
				Key:          "backup/svc/2/svc.sql",
				LastModified: time.Date(2026, 4, 18, 10, 0, 0, 0, time.UTC),
			},
		},
	}
	versionedProvider := &fakeStorageProvider{
		listedVersions: []storage.ObjectInfo{
			{
				Key:          "backup/svc/svc.sql",
				VersionID:    "v1",
				LastModified: time.Date(2026, 4, 18, 11, 0, 0, 0, time.UTC),
			},
			{
				Key:          "backup/svc/svc.sql",
				VersionID:    "v2",
				LastModified: time.Date(2026, 4, 18, 12, 0, 0, 0, time.UTC),
			},
		},
	}

	manager := Manager{
		storageFactory: namedTestStorageFactory{
			providers: map[string]storage.Provider{
				"archive":   archiveProvider,
				"versioned": versionedProvider,
			},
		},
	}

	outcome, err := manager.List(context.Background(), ListOptions{
		ServiceID: "svc",
		Remote:    true,
	})
	if err != nil {
		t.Fatalf("list remote service: %v", err)
	}

	if len(outcome.RemoteBackupGroups) != 2 {
		t.Fatalf("expected 2 backup groups, got %d", len(outcome.RemoteBackupGroups))
	}
	if outcome.RemoteBackupGroups[0].Storage != "archive" || outcome.RemoteBackupGroups[1].Storage != "versioned" {
		t.Fatalf("unexpected storage order: %+v", outcome.RemoteBackupGroups)
	}

	archiveBackups := outcome.RemoteBackupGroups[0].Backups
	if len(archiveBackups) != 2 {
		t.Fatalf("expected archive backups, got %d", len(archiveBackups))
	}
	if archiveBackups[0].Version != "2" || archiveBackups[1].Version != "1" {
		t.Fatalf("expected archive backups sorted by latest, got %+v", archiveBackups)
	}
	if archiveBackups[0].Storage != "archive" {
		t.Fatalf("expected archive backup storage metadata, got %+v", archiveBackups[0])
	}

	versionedBackups := outcome.RemoteBackupGroups[1].Backups
	if len(versionedBackups) != 2 {
		t.Fatalf("expected versioned backups, got %d", len(versionedBackups))
	}
	if versionedBackups[0].Version != "v2" || versionedBackups[1].Version != "v1" {
		t.Fatalf("expected native versions sorted by latest, got %+v", versionedBackups)
	}
	if versionedBackups[0].Storage != "versioned" {
		t.Fatalf("expected versioned backup storage metadata, got %+v", versionedBackups[0])
	}

	if len(archiveProvider.listObjectsPrefixes) != 1 || archiveProvider.listObjectsPrefixes[0] != "backup/svc/" {
		t.Fatalf("expected archive service prefix backup/svc/, got %+v", archiveProvider.listObjectsPrefixes)
	}
	if len(archiveProvider.listVersionPrefixes) != 0 {
		t.Fatalf("expected no archive version listing, got %+v", archiveProvider.listVersionPrefixes)
	}
	if len(versionedProvider.listVersionPrefixes) != 1 || versionedProvider.listVersionPrefixes[0] != "backup/svc/" {
		t.Fatalf("expected versioned service prefix backup/svc/, got %+v", versionedProvider.listVersionPrefixes)
	}
	if len(versionedProvider.listObjectsPrefixes) != 0 {
		t.Fatalf("expected no versioned object listing, got %+v", versionedProvider.listObjectsPrefixes)
	}
}

func TestRestoreRetrieveWithoutServiceConfigUsesDefaultStorage(t *testing.T) {
	homeDir := t.TempDir()
	workingDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	mainConfigPath := filepath.Join(homeDir, ".config", "sloth", "main.yaml")
	if err := os.MkdirAll(filepath.Dir(mainConfigPath), 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	mainConfig := "" +
		"storage:\n" +
		"  - name: default\n" +
		"    type: s3\n" +
		"    endpoint: https://example.com\n" +
		"    region: us-east-1\n" +
		"    bucket: backups\n" +
		"    access_key_id: key\n" +
		"    secret_access_key: secret\n" +
		"    use_native_object_versioning: false\n" +
		"    base_path: /backup\n"
	if err := os.WriteFile(mainConfigPath, []byte(mainConfig), 0o600); err != nil {
		t.Fatalf("write main config: %v", err)
	}

	provider := &fakeStorageProvider{
		listedObjects: []storage.ObjectInfo{
			{
				Key:          "backup/app/1/app-backup.sql",
				LastModified: time.Date(2026, 4, 19, 0, 0, 0, 0, time.UTC),
			},
			{
				Key:          "backup/app/2/app-backup.sql",
				LastModified: time.Date(2026, 4, 19, 1, 0, 0, 0, time.UTC),
			},
		},
	}
	manager := Manager{
		storageFactory: testStorageFactory{provider: provider},
		now: func() time.Time {
			return time.Date(2026, 4, 19, 2, 0, 0, 0, time.UTC)
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

	outcome, err := manager.RestoreRetrieve(context.Background(), RestoreRetrieveOptions{
		ServiceID: "app",
		Version:   "latest",
	})
	if err != nil {
		t.Fatalf("restore retrieve: %v", err)
	}

	if provider.getKey != "backup/app/2/app-backup.sql" {
		t.Fatalf("expected latest object key, got %q", provider.getKey)
	}
	if provider.getVersionID != "" {
		t.Fatalf("expected empty native version id for non-native storage, got %q", provider.getVersionID)
	}
	if outcome.Version != "2" {
		t.Fatalf("expected resolved version 2, got %q", outcome.Version)
	}
	if !strings.HasSuffix(outcome.DownloadedPath, "-2.sql") {
		t.Fatalf("expected sql extension and version suffix in download path, got %q", outcome.DownloadedPath)
	}
	if len(provider.listObjectsPrefixes) != 1 || provider.listObjectsPrefixes[0] != "backup/app/" {
		t.Fatalf("expected list prefix backup/app/, got %+v", provider.listObjectsPrefixes)
	}
}

func TestRestoreRetrieveWithoutServiceConfigNativeVersioning(t *testing.T) {
	homeDir := t.TempDir()
	workingDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	mainConfigPath := filepath.Join(homeDir, ".config", "sloth", "main.yaml")
	if err := os.MkdirAll(filepath.Dir(mainConfigPath), 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	mainConfig := "" +
		"storage:\n" +
		"  - name: default\n" +
		"    type: s3\n" +
		"    endpoint: https://example.com\n" +
		"    region: us-east-1\n" +
		"    bucket: backups\n" +
		"    access_key_id: key\n" +
		"    secret_access_key: secret\n" +
		"    use_native_object_versioning: true\n" +
		"    base_path: /backup\n"
	if err := os.WriteFile(mainConfigPath, []byte(mainConfig), 0o600); err != nil {
		t.Fatalf("write main config: %v", err)
	}

	provider := &fakeStorageProvider{
		listedVersions: []storage.ObjectInfo{
			{
				Key:          "backup/app/app-backup.tar",
				VersionID:    "v1",
				LastModified: time.Date(2026, 4, 19, 0, 0, 0, 0, time.UTC),
			},
			{
				Key:          "backup/app/app-backup.tar",
				VersionID:    "v2",
				LastModified: time.Date(2026, 4, 19, 1, 0, 0, 0, time.UTC),
			},
		},
	}
	manager := Manager{
		storageFactory: testStorageFactory{provider: provider},
		now: func() time.Time {
			return time.Date(2026, 4, 19, 2, 0, 0, 0, time.UTC)
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

	outcome, err := manager.RestoreRetrieve(context.Background(), RestoreRetrieveOptions{
		ServiceID: "app",
		Version:   "v1",
	})
	if err != nil {
		t.Fatalf("restore retrieve native: %v", err)
	}

	if provider.getKey != "backup/app/app-backup.tar" {
		t.Fatalf("expected native key, got %q", provider.getKey)
	}
	if provider.getVersionID != "v1" {
		t.Fatalf("expected native version id v1, got %q", provider.getVersionID)
	}
	if outcome.Version != "v1" {
		t.Fatalf("expected output version v1, got %q", outcome.Version)
	}
	if !strings.HasSuffix(outcome.DownloadedPath, "-v1.tar") {
		t.Fatalf("expected tar extension and native version suffix, got %q", outcome.DownloadedPath)
	}
	if len(provider.listVersionPrefixes) != 1 || provider.listVersionPrefixes[0] != "backup/app/" {
		t.Fatalf("expected version list prefix backup/app/, got %+v", provider.listVersionPrefixes)
	}
}

func TestRestoreRetrieveWithoutServiceConfigNativeLatestResolvesNumericVersion(t *testing.T) {
	homeDir := t.TempDir()
	workingDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	mainConfigPath := filepath.Join(homeDir, ".config", "sloth", "main.yaml")
	if err := os.MkdirAll(filepath.Dir(mainConfigPath), 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	mainConfig := "" +
		"storage:\n" +
		"  - name: default\n" +
		"    type: s3\n" +
		"    endpoint: https://example.com\n" +
		"    region: us-east-1\n" +
		"    bucket: backups\n" +
		"    access_key_id: key\n" +
		"    secret_access_key: secret\n" +
		"    use_native_object_versioning: true\n" +
		"    base_path: /backup\n"
	if err := os.WriteFile(mainConfigPath, []byte(mainConfig), 0o600); err != nil {
		t.Fatalf("write main config: %v", err)
	}

	provider := &fakeStorageProvider{
		listedVersions: []storage.ObjectInfo{
			{
				Key:          "backup/app/app-backup.tar",
				VersionID:    "v1",
				LastModified: time.Date(2026, 4, 19, 0, 0, 0, 0, time.UTC),
			},
			{
				Key:          "backup/app/app-backup.tar",
				VersionID:    "v2",
				LastModified: time.Date(2026, 4, 19, 1, 0, 0, 0, time.UTC),
			},
		},
	}
	manager := Manager{
		storageFactory: testStorageFactory{provider: provider},
		now: func() time.Time {
			return time.Date(2026, 4, 19, 2, 0, 0, 0, time.UTC)
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

	outcome, err := manager.RestoreRetrieve(context.Background(), RestoreRetrieveOptions{
		ServiceID: "app",
		Version:   "latest",
	})
	if err != nil {
		t.Fatalf("restore retrieve native latest: %v", err)
	}

	if outcome.Version != "2" {
		t.Fatalf("expected numeric latest version 2, got %q", outcome.Version)
	}
	if provider.getVersionID != "v2" {
		t.Fatalf("expected native get version v2, got %q", provider.getVersionID)
	}
	if !strings.HasSuffix(outcome.DownloadedPath, "-2.tar") {
		t.Fatalf("expected numeric version suffix in filename, got %q", outcome.DownloadedPath)
	}
}

func checksumSHA256Base64(content []byte) string {
	sum := sha256.Sum256(content)
	return base64.StdEncoding.EncodeToString(sum[:])
}

func captureStdout(t *testing.T, fn func() error) (string, error) {
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
