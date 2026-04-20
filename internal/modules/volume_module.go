package modules

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"sloth/internal/config"
	"sloth/internal/container"
)

type VolumeModule struct{}

func NewVolumeModule() VolumeModule {
	return VolumeModule{}
}

func (VolumeModule) Type() string {
	return "volume"
}

func (VolumeModule) SupportsLocal() bool {
	return false
}

func (VolumeModule) ArtifactFileName(service config.ServiceEntry) string {
	return fmt.Sprintf("%s-volume-backup.tar.gz", service.Name)
}

func (m VolumeModule) Backup(ctx context.Context, req BackupRequest) (BackupResult, error) {
	if req.Engine.Name() == "local" {
		return BackupResult{}, fmt.Errorf("volume backup does not support local mode")
	}

	volumeNames := resolveVolumeNames(req.Service)
	if len(volumeNames) == 0 {
		return BackupResult{}, fmt.Errorf("volume backup requires volume_name or volume_names")
	}

	artifactName := m.ArtifactFileName(req.Service)
	outputPath := filepath.Join(req.TempDir, artifactName)

	mountFlags := make([]string, 0, len(volumeNames))
	for _, volumeName := range volumeNames {
		mountFlags = append(mountFlags, fmt.Sprintf("-v %s:/volumes/%s:ro", volumeName, volumeName))
	}

	archiveCommand := fmt.Sprintf(
		"%s run --rm %s alpine:3.20 sh -lc 'tar -czf - -C /volumes .' > %s",
		req.Engine.RuntimeCommand(),
		strings.Join(mountFlags, " "),
		shellQuote(outputPath),
	)

	if err := container.RunHostShell(ctx, archiveCommand, req.Env, nil, nil, nil); err != nil {
		return BackupResult{}, fmt.Errorf("archive volume backup: %w", err)
	}

	return BackupResult{
		LocalPath:    outputPath,
		ArtifactName: artifactName,
		ArtifactExt:  "tar.gz",
	}, nil
}

func (m VolumeModule) Restore(ctx context.Context, req RestoreRequest) (RestoreResult, error) {
	if req.Engine.Name() == "local" {
		return RestoreResult{}, fmt.Errorf("volume restore does not support local mode")
	}

	volumeNames := resolveVolumeNames(req.Service)
	if len(volumeNames) == 0 {
		return RestoreResult{}, fmt.Errorf("volume restore requires volume_name or volume_names")
	}

	mountFlags := make([]string, 0, len(volumeNames))
	for _, volumeName := range volumeNames {
		mountFlags = append(mountFlags, fmt.Sprintf("-v %s:/restore/%s", volumeName, volumeName))
	}

	restoreCommand := fmt.Sprintf(
		"cat %s | %s run --rm -i %s alpine:3.20 sh -lc 'tar -xzf - -C /restore'",
		shellQuote(req.BackupFile),
		req.Engine.RuntimeCommand(),
		strings.Join(mountFlags, " "),
	)

	if err := container.RunHostShell(ctx, restoreCommand, req.Env, nil, nil, nil); err != nil {
		return RestoreResult{}, fmt.Errorf("restore volume archive: %w", err)
	}

	return RestoreResult{}, nil
}

func resolveVolumeNames(service config.ServiceEntry) []string {
	if len(service.VolumeNames) > 0 {
		return service.VolumeNames
	}
	if service.VolumeName != "" {
		return []string{service.VolumeName}
	}
	return nil
}

func shellQuote(path string) string {
	return "'" + strings.ReplaceAll(path, "'", "'\\''") + "'"
}
