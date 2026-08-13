package cli

import (
	"context"
	"strings"
	"testing"
	"time"

	"sloth/internal/config"
	"sloth/internal/orchestrator"
)

type fakeManager struct {
	backupOptions   orchestrator.BackupOptions
	restoreOptions  orchestrator.RestoreApplyOptions
	retrieveOptions orchestrator.RestoreRetrieveOptions
	listOptions     orchestrator.ListOptions
	versionOptions  orchestrator.VersionOptions

	backupOutcome  orchestrator.BackupOutcome
	listOutcome    orchestrator.ListOutcome
	versionOutcome orchestrator.VersionOutcome
	restoreOutcome orchestrator.RestoreApplyOutcome
	retrieveResult orchestrator.RestoreRetrieveOutcome
}

func (f *fakeManager) Backup(ctx context.Context, options orchestrator.BackupOptions) (orchestrator.BackupOutcome, error) {
	f.backupOptions = options
	return f.backupOutcome, nil
}

func (f *fakeManager) List(ctx context.Context, options orchestrator.ListOptions) (orchestrator.ListOutcome, error) {
	f.listOptions = options
	return f.listOutcome, nil
}

func (f *fakeManager) Version(ctx context.Context, options orchestrator.VersionOptions) (orchestrator.VersionOutcome, error) {
	f.versionOptions = options
	return f.versionOutcome, nil
}

func (f *fakeManager) RestoreRetrieve(ctx context.Context, options orchestrator.RestoreRetrieveOptions) (orchestrator.RestoreRetrieveOutcome, error) {
	f.retrieveOptions = options
	return f.retrieveResult, nil
}

func (f *fakeManager) RestoreApply(ctx context.Context, options orchestrator.RestoreApplyOptions) (orchestrator.RestoreApplyOutcome, error) {
	f.restoreOptions = options
	return f.restoreOutcome, nil
}

func TestRunBackupRejectsEngineLocalFlag(t *testing.T) {
	app := NewApp("test")
	_, err := runWithCapturedStdout(t, func() error {
		return app.Run(context.Background(), []string{"backup", "svc", "--engine", "local"})
	})
	if err == nil || !strings.Contains(err.Error(), "use --local") {
		t.Fatalf("expected local engine rejection, got %v", err)
	}
}

func TestRunRestoreRejectsEngineLocalFlag(t *testing.T) {
	app := NewApp("test")
	_, err := runWithCapturedStdout(t, func() error {
		return app.Run(context.Background(), []string{"restore", "svc", "--engine", "local"})
	})
	if err == nil || !strings.Contains(err.Error(), "use --local") {
		t.Fatalf("expected local engine rejection, got %v", err)
	}
}

func TestRunRestoreRetrieveUsesRefinedLogs(t *testing.T) {
	manager := &fakeManager{
		retrieveResult: orchestrator.RestoreRetrieveOutcome{
			DownloadedPath: "./svc-backup.sql",
			ObjectKey:      "backup/svc/3/svc.sql",
			Version:        "3",
			Guidance:       "cleanup old data before apply",
		},
	}

	app := NewApp("test")
	app.manager = manager

	output, err := runWithCapturedStdout(t, func() error {
		return app.Run(context.Background(), []string{"restore", "svc", "--version", "3"})
	})
	if err != nil {
		t.Fatalf("run restore retrieve: %v", err)
	}

	if manager.retrieveOptions.Version != "3" {
		t.Fatalf("expected version 3 to be forwarded, got %+v", manager.retrieveOptions)
	}
	assertContains(t, output, "Retrieving backup for service svc (Version 3) ...")
	assertContains(t, output, "Downloaded backup file to \"./svc-backup.sql\"")
	assertNotContains(t, output, "file=./svc-backup.sql")
	assertNotContains(t, output, "object=backup/svc/3/svc.sql")
}

func TestRunRestoreRetrieveLogsResolvedVersionInsteadOfLatest(t *testing.T) {
	manager := &fakeManager{
		retrieveResult: orchestrator.RestoreRetrieveOutcome{
			DownloadedPath: "./svc-backup.sql",
			ObjectKey:      "backup/svc/5/svc.sql",
			Version:        "5",
			Guidance:       "cleanup old data before apply",
		},
	}

	app := NewApp("test")
	app.manager = manager

	output, err := runWithCapturedStdout(t, func() error {
		return app.Run(context.Background(), []string{"restore", "svc"})
	})
	if err != nil {
		t.Fatalf("run restore retrieve latest: %v", err)
	}

	if manager.retrieveOptions.Version != "latest" {
		t.Fatalf("expected latest request to be forwarded, got %+v", manager.retrieveOptions)
	}
	assertContains(t, output, "Retrieving backup for service svc (Version 5) ...")
	assertNotContains(t, output, "Retrieving backup for service svc (Version latest) ...")
}

func TestRunBackupAcceptsShortFlagsAndPrintsBackupTable(t *testing.T) {
	manager := &fakeManager{
		backupOutcome: orchestrator.BackupOutcome{ServiceID: "svc"},
		listOutcome: orchestrator.ListOutcome{
			Backups: []orchestrator.BackupObject{
				{
					Key:          "backup/svc/1/svc.sql",
					Version:      "1",
					Size:         128,
					LastModified: time.Date(2026, 4, 18, 10, 0, 0, 0, time.UTC),
				},
			},
		},
	}

	app := NewApp("test")
	app.manager = manager
	output, err := runWithCapturedStdout(t, func() error {
		return app.Run(
			context.Background(),
			[]string{"backup", "svc", "-t", "mysql", "-c", "svc-db", "-E", "docker", "-s", "archive", "-e", ".env.local", "-m", "mod.yaml", "-k", "--dry-run", "--force", "--use-file-size-check", "--use-checksum", "--override-latest-version", "-d"},
		)
	})
	if err != nil {
		t.Fatalf("run backup: %v", err)
	}

	if manager.backupOptions.Type != "mysql" || manager.backupOptions.ContainerName != "svc-db" || manager.backupOptions.Engine != "docker" {
		t.Fatalf("unexpected backup options: %+v", manager.backupOptions)
	}
	if !manager.backupOptions.UseChecksum || !manager.backupOptions.UseFileSize {
		t.Fatalf("expected delta-check flags to be passed: %+v", manager.backupOptions)
	}
	if !manager.backupOptions.Force {
		t.Fatalf("expected force flag to be passed: %+v", manager.backupOptions)
	}
	if !manager.backupOptions.Keep || !manager.backupOptions.DryRun {
		t.Fatalf("expected keep and dry-run flags to be passed: %+v", manager.backupOptions)
	}
	if !manager.backupOptions.OverrideLatest {
		t.Fatalf("expected override latest flag to be passed: %+v", manager.backupOptions)
	}
	if !strings.Contains(output, "| version ") {
		t.Fatalf("expected backup table output\n%s", output)
	}
	assertNotContains(t, output, "object_key")
	assertNotContains(t, output, "service=")
}

func TestRunBackupForwardsGlobalSudoFlagsAndPreservesSubcommandShortFlags(t *testing.T) {
	manager := &fakeManager{
		backupOutcome: orchestrator.BackupOutcome{ServiceID: "svc"},
		listOutcome: orchestrator.ListOutcome{
			Backups: []orchestrator.BackupObject{
				{
					Key:          "backup/svc/1/svc.sql",
					Version:      "1",
					Size:         128,
					LastModified: time.Date(2026, 4, 18, 10, 0, 0, 0, time.UTC),
				},
			},
		},
	}

	app := NewApp("test")
	app.manager = manager

	_, err := runWithCapturedStdout(t, func() error {
		return app.Run(
			context.Background(),
			[]string{"-S", "--sudo-program", "doas", "backup", "svc", "-c", "svc-db", "-s", "archive"},
		)
	})
	if err != nil {
		t.Fatalf("run backup: %v", err)
	}

	if !manager.backupOptions.UseSudo || manager.backupOptions.SudoProgram != "doas" {
		t.Fatalf("expected sudo runtime options to be forwarded: %+v", manager.backupOptions)
	}
	if manager.backupOptions.ContainerName != "svc-db" || manager.backupOptions.Storage != "archive" {
		t.Fatalf("expected subcommand short flags preserved: %+v", manager.backupOptions)
	}
}

func TestRunRestoreApplyForwardsGlobalSudoFlags(t *testing.T) {
	manager := &fakeManager{
		restoreOutcome: orchestrator.RestoreApplyOutcome{
			Engine: "docker",
		},
	}

	app := NewApp("test")
	app.manager = manager

	_, err := runWithCapturedStdout(t, func() error {
		return app.Run(
			context.Background(),
			[]string{"-S", "restore", "svc", "--apply", "./svc.sql"},
		)
	})
	if err != nil {
		t.Fatalf("run restore apply: %v", err)
	}

	if !manager.restoreOptions.UseSudo || manager.restoreOptions.SudoProgram != "sudo" {
		t.Fatalf("expected restore apply sudo options: %+v", manager.restoreOptions)
	}
}

func TestRunBackupAcceptsGlobalFlagsAfterSubcommand(t *testing.T) {
	manager := &fakeManager{
		backupOutcome: orchestrator.BackupOutcome{ServiceID: "svc"},
		listOutcome: orchestrator.ListOutcome{
			Backups: []orchestrator.BackupObject{
				{
					Key:          "backup/svc/1/svc.sql",
					Version:      "1",
					Size:         128,
					LastModified: time.Date(2026, 4, 18, 10, 0, 0, 0, time.UTC),
				},
			},
		},
	}

	app := NewApp("test")
	app.manager = manager

	_, err := runWithCapturedStdout(t, func() error {
		return app.Run(
			context.Background(),
			[]string{"backup", "svc", "-c", "svc-db", "--sudo-program=doas", "-S"},
		)
	})
	if err != nil {
		t.Fatalf("run backup with trailing global flags: %v", err)
	}

	if !manager.backupOptions.UseSudo || manager.backupOptions.SudoProgram != "doas" {
		t.Fatalf("expected trailing global flags to be forwarded: %+v", manager.backupOptions)
	}
	if manager.backupOptions.ContainerName != "svc-db" {
		t.Fatalf("expected container-name flag to be preserved: %+v", manager.backupOptions)
	}
}

func TestRunRestoreApplyAcceptsGlobalFlagsAfterSubcommand(t *testing.T) {
	manager := &fakeManager{
		restoreOutcome: orchestrator.RestoreApplyOutcome{Engine: "podman"},
	}

	app := NewApp("test")
	app.manager = manager

	_, err := runWithCapturedStdout(t, func() error {
		return app.Run(
			context.Background(),
			[]string{"restore", "svc", "--apply", "./svc.sql", "-S", "--sudo-program", "doas"},
		)
	})
	if err != nil {
		t.Fatalf("run restore apply with trailing global flags: %v", err)
	}

	if !manager.restoreOptions.UseSudo || manager.restoreOptions.SudoProgram != "doas" {
		t.Fatalf("expected trailing global flags to be forwarded: %+v", manager.restoreOptions)
	}
}

func TestRunListWithoutServiceIDRemovesContainerColumnAndShowsDefaultStorage(t *testing.T) {
	manager := &fakeManager{
		listOutcome: orchestrator.ListOutcome{
			Services: []config.ServiceEntry{
				{
					Name:           "svc",
					Type:           "mysql",
					Storage:        "",
					LastBackupTime: "2026-04-18T10:00:00Z",
				},
			},
		},
	}
	app := NewApp("test")
	app.manager = manager

	output, err := runWithCapturedStdout(t, func() error {
		return app.Run(context.Background(), []string{"list"})
	})
	if err != nil {
		t.Fatalf("run list: %v", err)
	}

	assertContains(t, output, "| service ")
	assertContains(t, output, "| storage ")
	assertContains(t, output, "default")
	assertNotContains(t, output, "| container ")
}

func TestRunListWithoutServiceIDShowsMessageWhenEmpty(t *testing.T) {
	manager := &fakeManager{
		listOutcome: orchestrator.ListOutcome{
			Services: []config.ServiceEntry{},
		},
	}
	app := NewApp("test")
	app.manager = manager

	output, err := runWithCapturedStdout(t, func() error {
		return app.Run(context.Background(), []string{"list"})
	})
	if err != nil {
		t.Fatalf("run list empty: %v", err)
	}

	assertContains(t, output, "No service backup found")
	assertNotContains(t, output, "[warn] No service backup found")
	assertNotContains(t, output, "| service ")
}

func TestRunListWithoutServiceIDFormatsUnixLastBackupTimestamp(t *testing.T) {
	manager := &fakeManager{
		listOutcome: orchestrator.ListOutcome{
			Services: []config.ServiceEntry{
				{
					Name:           "svc",
					Type:           "mysql",
					Storage:        "default",
					LastBackupTime: "1713355200",
				},
			},
		},
	}
	app := NewApp("test")
	app.manager = manager

	output, err := runWithCapturedStdout(t, func() error {
		return app.Run(context.Background(), []string{"list"})
	})
	if err != nil {
		t.Fatalf("run list with unix timestamp: %v", err)
	}

	assertContains(t, output, "2024-04-17T12:00:00Z")
}

func TestRunListWithServiceIDUsesSolidTable(t *testing.T) {
	manager := &fakeManager{
		listOutcome: orchestrator.ListOutcome{
			Backups: []orchestrator.BackupObject{
				{
					Key:          "backup/svc/1/svc.sql",
					Version:      "1",
					Size:         128,
					LastModified: time.Date(2026, 4, 18, 10, 0, 0, 0, time.UTC),
				},
			},
		},
	}
	app := NewApp("test")
	app.manager = manager

	output, err := runWithCapturedStdout(t, func() error {
		return app.Run(context.Background(), []string{"list", "svc"})
	})
	if err != nil {
		t.Fatalf("run list service: %v", err)
	}

	assertContains(t, output, "+---------")
	assertContains(t, output, "| version ")
	assertContains(t, output, "128 B")
	assertNotContains(t, output, "object_key")
}

func TestRunListWithServiceIDShowObjectKey(t *testing.T) {
	manager := &fakeManager{
		listOutcome: orchestrator.ListOutcome{
			Backups: []orchestrator.BackupObject{
				{
					Key:          "backup/svc/1/svc.sql",
					Version:      "1",
					Size:         2048,
					LastModified: time.Date(2026, 4, 18, 10, 0, 0, 0, time.UTC),
				},
			},
		},
	}
	app := NewApp("test")
	app.manager = manager

	output, err := runWithCapturedStdout(t, func() error {
		return app.Run(context.Background(), []string{"list", "svc", "--show-object-key"})
	})
	if err != nil {
		t.Fatalf("run list service with object key: %v", err)
	}

	assertContains(t, output, "object_key")
	assertContains(t, output, "backup/svc/1/svc.sql")
	assertContains(t, output, "2.0 KB")
}

func TestRunVersionForwardsDeleteAndStorage(t *testing.T) {
	manager := &fakeManager{
		versionOutcome: orchestrator.VersionOutcome{
			Deleted: []orchestrator.DeletedVersion{
				{
					Storage:   "archive",
					Version:   "2",
					ObjectKey: "backup/svc/2/svc.sql",
				},
			},
			Groups: []orchestrator.RemoteBackupGroup{
				{
					Storage: "archive",
					Backups: []orchestrator.BackupObject{
						{
							Key:          "backup/svc/1/svc.sql",
							Version:      "1",
							Size:         1024,
							LastModified: time.Date(2026, 4, 18, 10, 0, 0, 0, time.UTC),
						},
					},
				},
			},
		},
	}
	app := NewApp("test")
	app.manager = manager

	manager.versionOutcome.DryRun = true
	output, err := runWithCapturedStdout(t, func() error {
		return app.Run(context.Background(), []string{"version", "svc", "--storage", "archive", "--delete", "2", "--dry-run", "-d"})
	})
	if err != nil {
		t.Fatalf("run version delete: %v", err)
	}

	if manager.versionOptions.ServiceID != "svc" || manager.versionOptions.Storage != "archive" || manager.versionOptions.Delete != "2" || !manager.versionOptions.DryRun {
		t.Fatalf("unexpected version options: %+v", manager.versionOptions)
	}
	assertContains(t, output, "Versions marked to delete")
	assertContains(t, output, "Storage: archive")
	assertContains(t, output, "| version ")
}

func TestRunVersionForwardsKeepLast(t *testing.T) {
	manager := &fakeManager{}
	app := NewApp("test")
	app.manager = manager

	_, err := runWithCapturedStdout(t, func() error {
		return app.Run(context.Background(), []string{"version", "svc", "--keep-last", "3"})
	})
	if err != nil {
		t.Fatalf("run version keep-last: %v", err)
	}

	if manager.versionOptions.ServiceID != "svc" || manager.versionOptions.KeepLast != 3 {
		t.Fatalf("unexpected version options: %+v", manager.versionOptions)
	}
}

func TestRunVersionForwardsDeleteBefore(t *testing.T) {
	manager := &fakeManager{}
	app := NewApp("test")
	app.manager = manager

	_, err := runWithCapturedStdout(t, func() error {
		return app.Run(context.Background(), []string{"version", "svc", "--delete-before", "2026-04-18T11:00:00Z", "--dry-run"})
	})
	if err != nil {
		t.Fatalf("run version delete-before: %v", err)
	}

	if manager.versionOptions.ServiceID != "svc" || manager.versionOptions.DeleteBefore != "2026-04-18T11:00:00Z" || !manager.versionOptions.DryRun {
		t.Fatalf("unexpected version options: %+v", manager.versionOptions)
	}
}

func TestRunVersionForwardsKeepAfter(t *testing.T) {
	manager := &fakeManager{}
	app := NewApp("test")
	app.manager = manager

	_, err := runWithCapturedStdout(t, func() error {
		return app.Run(context.Background(), []string{"version", "svc", "--keep-after", "1713355200"})
	})
	if err != nil {
		t.Fatalf("run version keep-after: %v", err)
	}

	if manager.versionOptions.ServiceID != "svc" || manager.versionOptions.KeepAfter != "1713355200" {
		t.Fatalf("unexpected version options: %+v", manager.versionOptions)
	}
}

func TestRunVersionRejectsDeleteWithKeepLast(t *testing.T) {
	app := NewApp("test")
	_, err := runWithCapturedStdout(t, func() error {
		return app.Run(context.Background(), []string{"version", "svc", "--delete", "1", "--keep-last", "2", "--delete-before", "2026-04-18T11:00:00Z"})
	})
	if err == nil || !strings.Contains(err.Error(), "version accepts only one") {
		t.Fatalf("expected mutually exclusive selector error, got %v", err)
	}
}

func TestRunListRemoteGroupedByStorage(t *testing.T) {
	manager := &fakeManager{
		listOutcome: orchestrator.ListOutcome{
			RemoteServiceGroups: []orchestrator.RemoteServiceGroup{
				{
					Storage: "archive",
					Rows: []orchestrator.RemoteServiceRow{
						{
							Service:    "svc-a",
							LastBackup: time.Date(2026, 4, 18, 8, 0, 0, 0, time.UTC),
							ObjectKey:  "backup/svc-a/1/svc-a.sql",
						},
					},
				},
				{
					Storage: "default",
					Rows: []orchestrator.RemoteServiceRow{
						{
							Service:    "svc-b",
							LastBackup: time.Date(2026, 4, 18, 9, 0, 0, 0, time.UTC),
							ObjectKey:  "backup/svc-b/1/svc-b.sql",
						},
					},
				},
				{
					Storage: "empty",
					Rows:    []orchestrator.RemoteServiceRow{},
				},
			},
		},
	}

	app := NewApp("test")
	app.manager = manager

	output, err := runWithCapturedStdout(t, func() error {
		return app.Run(context.Background(), []string{"list", "--remote"})
	})
	if err != nil {
		t.Fatalf("run list remote: %v", err)
	}

	if !manager.listOptions.Remote || manager.listOptions.ServiceID != "" {
		t.Fatalf("unexpected list options: %+v", manager.listOptions)
	}

	assertContains(t, output, "Storage: archive")
	assertContains(t, output, "Storage: default")
	assertContains(t, output, "Storage: empty")
	assertContains(t, output, "| service ")
	assertContains(t, output, "| last_backup ")
	assertContains(t, output, "No service backup found")
	assertNotContains(t, output, "object_key")
}

func TestRunListRemoteShowObjectKey(t *testing.T) {
	manager := &fakeManager{
		listOutcome: orchestrator.ListOutcome{
			RemoteServiceGroups: []orchestrator.RemoteServiceGroup{
				{
					Storage: "default",
					Rows: []orchestrator.RemoteServiceRow{
						{
							Service:    "svc",
							LastBackup: time.Date(2026, 4, 18, 9, 0, 0, 0, time.UTC),
							ObjectKey:  "backup/svc/3/svc.sql",
						},
					},
				},
			},
		},
	}

	app := NewApp("test")
	app.manager = manager

	output, err := runWithCapturedStdout(t, func() error {
		return app.Run(context.Background(), []string{"list", "--remote", "--show-object-key"})
	})
	if err != nil {
		t.Fatalf("run list remote show object key: %v", err)
	}

	assertContains(t, output, "object_key")
	assertContains(t, output, "backup/svc/3/svc.sql")
}

func TestRunListRemoteServiceIDGroupedByStorage(t *testing.T) {
	manager := &fakeManager{
		listOutcome: orchestrator.ListOutcome{
			RemoteBackupGroups: []orchestrator.RemoteBackupGroup{
				{
					Storage: "archive",
					Backups: []orchestrator.BackupObject{
						{
							Key:          "backup/svc/2/svc.sql",
							Version:      "2",
							Size:         4096,
							LastModified: time.Date(2026, 4, 18, 11, 0, 0, 0, time.UTC),
						},
					},
				},
				{
					Storage: "default",
					Backups: []orchestrator.BackupObject{
						{
							Key:          "backup/svc/1/svc.sql",
							Version:      "1",
							Size:         1024,
							LastModified: time.Date(2026, 4, 18, 10, 0, 0, 0, time.UTC),
						},
					},
				},
			},
		},
	}

	app := NewApp("test")
	app.manager = manager

	output, err := runWithCapturedStdout(t, func() error {
		return app.Run(context.Background(), []string{"list", "svc", "--remote"})
	})
	if err != nil {
		t.Fatalf("run list remote service id: %v", err)
	}

	if !manager.listOptions.Remote || manager.listOptions.ServiceID != "svc" {
		t.Fatalf("unexpected list options: %+v", manager.listOptions)
	}
	assertContains(t, output, "Storage: archive")
	assertContains(t, output, "Storage: default")
	assertContains(t, output, "| version ")
	assertContains(t, output, "4.0 KB")
	assertContains(t, output, "1.0 KB")
}

func TestRunListRemoteAllStoragesEmptyShowsPerStorageMessage(t *testing.T) {
	manager := &fakeManager{
		listOutcome: orchestrator.ListOutcome{
			RemoteServiceGroups: []orchestrator.RemoteServiceGroup{
				{
					Storage: "archive",
					Rows:    []orchestrator.RemoteServiceRow{},
				},
				{
					Storage: "default",
					Rows:    []orchestrator.RemoteServiceRow{},
				},
			},
		},
	}

	app := NewApp("test")
	app.manager = manager

	output, err := runWithCapturedStdout(t, func() error {
		return app.Run(context.Background(), []string{"list", "--remote"})
	})
	if err != nil {
		t.Fatalf("run list remote all empty: %v", err)
	}

	assertContains(t, output, "Storage: archive")
	assertContains(t, output, "Storage: default")
	assertContains(t, output, "No service backup found")
	assertNotContains(t, output, "[warn] No service backup found")
	assertNotContains(t, output, "| service ")
}

func assertNotContains(t *testing.T, output string, unexpected string) {
	t.Helper()
	if strings.Contains(output, unexpected) {
		t.Fatalf("expected output to not contain %q\n%s", unexpected, output)
	}
}
