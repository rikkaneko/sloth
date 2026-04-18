package modules

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"sloth/internal/config"
)

type CommandModule struct {
	moduleType    string
	artifactExt   string
	supportsLocal bool
	definition    Definition
}

func NewCommandModule(moduleType string, artifactExt string, supportsLocal bool, definition Definition) CommandModule {
	return CommandModule{
		moduleType:    moduleType,
		artifactExt:   artifactExt,
		supportsLocal: supportsLocal,
		definition:    definition,
	}
}

func (m CommandModule) Type() string {
	return m.moduleType
}

func (m CommandModule) SupportsLocal() bool {
	return m.supportsLocal
}

func (m CommandModule) ArtifactFileName(service config.ServiceEntry) string {
	return fmt.Sprintf("%s-%s-backup.%s", service.Name, m.moduleType, m.artifactExt)
}

func (m CommandModule) Backup(ctx context.Context, req BackupRequest) (BackupResult, error) {
	if req.Engine.Name() == "local" && !m.supportsLocal {
		return BackupResult{}, fmt.Errorf("%s does not support local mode in v1", m.moduleType)
	}

	artifactName := m.ArtifactFileName(req.Service)
	localOutputPath := filepath.Join(req.TempDir, artifactName)

	templateValues := map[string]string{}
	containerTarget := m.definition.Backup.TargetFile
	if containerTarget == "" {
		containerTarget = "/tmp/" + artifactName
	}

	if req.Engine.Name() == "local" {
		templateValues["target_file"] = localOutputPath
		commands := renderCommands(m.definition.Backup.Command, templateValues)
		if err := runModuleCommands(ctx, req, commands); err != nil {
			return BackupResult{}, err
		}

		return BackupResult{
			LocalPath:    localOutputPath,
			ArtifactName: artifactName,
			ArtifactExt:  m.artifactExt,
		}, nil
	}

	if req.Service.ContainerName == "" {
		return BackupResult{}, fmt.Errorf("container_name is required for %s backup", m.moduleType)
	}

	templateValues["target_file"] = containerTarget
	commands := renderCommands(m.definition.Backup.Command, templateValues)
	if err := runModuleCommands(ctx, req, commands); err != nil {
		return BackupResult{}, err
	}

	if err := req.Engine.CopyFrom(ctx, req.Service.ContainerName, containerTarget, localOutputPath); err != nil {
		return BackupResult{}, fmt.Errorf("copy backup file from container: %w", err)
	}

	return BackupResult{
		LocalPath:    localOutputPath,
		ArtifactName: artifactName,
		ArtifactExt:  m.artifactExt,
	}, nil
}

func (m CommandModule) Restore(ctx context.Context, req RestoreRequest) (RestoreResult, error) {
	if req.Engine.Name() == "local" && !m.supportsLocal {
		return RestoreResult{}, fmt.Errorf("%s does not support local mode in v1", m.moduleType)
	}

	templateValues := map[string]string{}

	if req.Engine.Name() == "local" {
		templateValues["backup_file"] = req.BackupFile
		commands := renderCommands(m.definition.Restore.Command, templateValues)
		if err := runModuleRestoreCommands(ctx, req, commands); err != nil {
			return RestoreResult{}, err
		}
		return RestoreResult{}, nil
	}

	if req.Service.ContainerName == "" {
		return RestoreResult{}, fmt.Errorf("container_name is required for %s restore", m.moduleType)
	}

	toContainer := m.definition.Restore.BackupFile.ToContainer
	if toContainer == "" {
		toContainer = m.definition.Backup.TargetFile
	}
	if toContainer == "" {
		toContainer = "/tmp/" + filepath.Base(req.BackupFile)
	}

	if err := req.Engine.CopyTo(ctx, req.Service.ContainerName, req.BackupFile, toContainer); err != nil {
		return RestoreResult{}, fmt.Errorf("copy backup file into container: %w", err)
	}

	templateValues["backup_file"] = toContainer
	commands := renderCommands(m.definition.Restore.Command, templateValues)
	if err := runModuleRestoreCommands(ctx, req, commands); err != nil {
		return RestoreResult{}, err
	}

	return RestoreResult{}, nil
}

func runModuleCommands(ctx context.Context, req BackupRequest, commands []string) error {
	for _, command := range commands {
		if strings.TrimSpace(command) == "" {
			continue
		}
		if err := req.Engine.Exec(ctx, req.Service.ContainerName, command, req.Env, nil, nil, nil); err != nil {
			return err
		}
	}
	return nil
}

func runModuleRestoreCommands(ctx context.Context, req RestoreRequest, commands []string) error {
	for _, command := range commands {
		if strings.TrimSpace(command) == "" {
			continue
		}
		if err := req.Engine.Exec(ctx, req.Service.ContainerName, command, req.Env, nil, nil, nil); err != nil {
			return err
		}
	}
	return nil
}

func renderCommands(commands []string, values map[string]string) []string {
	out := make([]string, 0, len(commands))
	for _, command := range commands {
		rendered := command
		for key, value := range values {
			rendered = strings.ReplaceAll(rendered, "{{"+key+"}}", value)
		}
		out = append(out, rendered)
	}
	return out
}
