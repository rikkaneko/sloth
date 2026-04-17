package modules

import (
	"context"

	"sloth/internal/config"
	"sloth/internal/container"
)

type BackupRequest struct {
	Service config.ServiceEntry
	Engine  container.Engine
	Env     map[string]string
	TempDir string
}

type RestoreRequest struct {
	Service    config.ServiceEntry
	Engine     container.Engine
	Env        map[string]string
	BackupFile string
	WorkingDir string
}

type BackupResult struct {
	LocalPath    string
	ArtifactName string
	ArtifactExt  string
}

type RestoreResult struct {
	Guidance string
}

type Module interface {
	Type() string
	SupportsLocal() bool
	ArtifactFileName(service config.ServiceEntry) string
	Backup(ctx context.Context, req BackupRequest) (BackupResult, error)
	Restore(ctx context.Context, req RestoreRequest) (RestoreResult, error)
}
