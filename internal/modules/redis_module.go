package modules

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"sloth/internal/config"
)

type RedisModule struct {
	backupModule  CommandModule
	supportsLocal bool
	restorePath   string
}

func NewRedisModule(definition Definition, supportsLocal bool) RedisModule {
	restorePath := definition.Restore.BackupFile.ToContainer
	return RedisModule{
		backupModule:  NewCommandModule("redis", "rdb", supportsLocal, definition),
		supportsLocal: supportsLocal,
		restorePath:   restorePath,
	}
}

func (m RedisModule) Type() string {
	return "redis"
}

func (m RedisModule) SupportsLocal() bool {
	return m.supportsLocal
}

func (m RedisModule) ArtifactFileName(service config.ServiceEntry) string {
	return m.backupModule.ArtifactFileName(service)
}

func (m RedisModule) Backup(ctx context.Context, req BackupRequest) (BackupResult, error) {
	return m.backupModule.Backup(ctx, req)
}

func (m RedisModule) Restore(ctx context.Context, req RestoreRequest) (RestoreResult, error) {
	if req.Engine.Name() == "local" {
		if !m.supportsLocal {
			return RestoreResult{}, fmt.Errorf("redis restore does not support local mode in v1")
		}

		targetPath := m.resolveRestorePath(req, true)
		if err := copyFile(req.BackupFile, targetPath); err != nil {
			return RestoreResult{}, fmt.Errorf("copy redis dump file for local restore: %w", err)
		}

		guidance := fmt.Sprintf("Redis dump file copied to %s. Restart Redis service to load the snapshot.", targetPath)
		return RestoreResult{Guidance: guidance}, nil
	}
	if req.Service.ContainerName == "" {
		return RestoreResult{}, fmt.Errorf("container_name is required for redis restore")
	}

	targetPath := m.resolveRestorePath(req, false)
	if err := req.Engine.CopyTo(ctx, req.Service.ContainerName, req.BackupFile, targetPath); err != nil {
		return RestoreResult{}, fmt.Errorf("copy redis dump file into container: %w", err)
	}

	guidance := fmt.Sprintf("Redis dump file copied to %s. Restart the Redis container/service to load the snapshot.", targetPath)
	return RestoreResult{Guidance: guidance}, nil
}

func (m RedisModule) resolveRestorePath(req RestoreRequest, local bool) string {
	if override := strings.TrimSpace(req.Env["REDIS_RDB_PATH"]); override != "" {
		return m.resolveWorkingPath(req, override)
	}

	if req.Service.Meta != nil {
		if metaPath := strings.TrimSpace(req.Service.Meta["redis_rdb_path"]); metaPath != "" {
			return m.resolveWorkingPath(req, metaPath)
		}
	}

	configured := strings.TrimSpace(m.restorePath)
	if configured != "" {
		return m.resolveWorkingPath(req, configured)
	}

	if local {
		return m.resolveWorkingPath(req, "dump.rdb")
	}
	return "/data/dump.rdb"
}

func (m RedisModule) resolveWorkingPath(req RestoreRequest, target string) string {
	if filepath.IsAbs(target) {
		return target
	}
	if req.WorkingDir != "" {
		return filepath.Join(req.WorkingDir, target)
	}
	return target
}

func copyFile(src string, dest string) error {
	sourceFile, err := os.Open(filepath.Clean(src))
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	directory := filepath.Dir(dest)
	if directory != "." {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			return err
		}
	}

	destinationFile, err := os.Create(filepath.Clean(dest))
	if err != nil {
		return err
	}
	defer destinationFile.Close()

	if _, err := io.Copy(destinationFile, sourceFile); err != nil {
		return err
	}
	return nil
}
