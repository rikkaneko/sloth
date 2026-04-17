package modules

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Definition struct {
	Backup  BackupDefinition  `yaml:"backup"`
	Restore RestoreDefinition `yaml:"restore"`
}

type BackupDefinition struct {
	Command    []string `yaml:"command"`
	TargetFile string   `yaml:"target_file"`
}

type RestoreDefinition struct {
	BackupFile RestoreBackupFile `yaml:"backup_file"`
	Command    []string          `yaml:"command"`
}

type RestoreBackupFile struct {
	ToContainer string `yaml:"to_container"`
}

type OverrideFile struct {
	Modules map[string]Definition `yaml:"modules"`
}

func LoadOverrideDefinition(path string, moduleType string) (Definition, bool, error) {
	if path == "" {
		return Definition{}, false, nil
	}
	raw, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return Definition{}, false, fmt.Errorf("read module override %q: %w", path, err)
	}

	var override OverrideFile
	if err := yaml.Unmarshal(raw, &override); err != nil {
		return Definition{}, false, fmt.Errorf("parse module override yaml: %w", err)
	}

	def, ok := override.Modules[moduleType]
	if !ok {
		return Definition{}, false, nil
	}
	return def, true, nil
}

func MergeDefinition(base Definition, override Definition) Definition {
	merged := base

	if len(override.Backup.Command) > 0 {
		merged.Backup.Command = override.Backup.Command
	}
	if override.Backup.TargetFile != "" {
		merged.Backup.TargetFile = override.Backup.TargetFile
	}

	if len(override.Restore.Command) > 0 {
		merged.Restore.Command = override.Restore.Command
	}
	if override.Restore.BackupFile.ToContainer != "" {
		merged.Restore.BackupFile.ToContainer = override.Restore.BackupFile.ToContainer
	}

	return merged
}
