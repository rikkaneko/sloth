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
	listServiceID   string

	backupOutcome  orchestrator.BackupOutcome
	listOutcome    orchestrator.ListOutcome
	restoreOutcome orchestrator.RestoreApplyOutcome
	retrieveResult orchestrator.RestoreRetrieveOutcome
}

func (f *fakeManager) Backup(ctx context.Context, options orchestrator.BackupOptions) (orchestrator.BackupOutcome, error) {
	f.backupOptions = options
	return f.backupOutcome, nil
}

func (f *fakeManager) List(ctx context.Context, serviceID string) (orchestrator.ListOutcome, error) {
	f.listServiceID = serviceID
	return f.listOutcome, nil
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
			[]string{"backup", "svc", "-t", "mysql", "-c", "svc-db", "-E", "docker", "-s", "archive", "-e", ".env.local", "-m", "mod.yaml", "-d"},
		)
	})
	if err != nil {
		t.Fatalf("run backup: %v", err)
	}

	if manager.backupOptions.Type != "mysql" || manager.backupOptions.ContainerName != "svc-db" || manager.backupOptions.Engine != "docker" {
		t.Fatalf("unexpected backup options: %+v", manager.backupOptions)
	}
	if !strings.Contains(output, "| version ") {
		t.Fatalf("expected backup table output\n%s", output)
	}
	assertNotContains(t, output, "service=")
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
}

func assertNotContains(t *testing.T, output string, unexpected string) {
	t.Helper()
	if strings.Contains(output, unexpected) {
		t.Fatalf("expected output to not contain %q\n%s", unexpected, output)
	}
}
