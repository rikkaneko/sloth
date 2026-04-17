package modules

import (
	"embed"
	"fmt"
	"io/fs"
	"path"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

type Registry struct{}

func NewRegistry() Registry {
	return Registry{}
}

func (Registry) Resolve(serviceType string, overridePath string) (Module, error) {
	builtins, err := builtInDefinitions()
	if err != nil {
		return nil, err
	}

	if serviceType == "volume" {
		return NewVolumeModule(), nil
	}

	baseDef, ok := builtins[serviceType]
	if !ok {
		return nil, fmt.Errorf("unsupported service type %q", serviceType)
	}

	if overridePath != "" {
		overrideDef, exists, err := LoadOverrideDefinition(overridePath, serviceType)
		if err != nil {
			return nil, err
		}
		if exists {
			baseDef.definition = MergeDefinition(baseDef.definition, overrideDef)
		}
	}

	switch serviceType {
	case "redis":
		return NewRedisModule(baseDef.definition, baseDef.localSupported), nil
	default:
		return NewCommandModule(serviceType, baseDef.ext, baseDef.localSupported, baseDef.definition), nil
	}
}

type builtInDefinition struct {
	ext            string
	localSupported bool
	definition     Definition
}

type builtInDefinitionYAML struct {
	ArtifactExt   string            `yaml:"artifact_ext"`
	SupportsLocal bool              `yaml:"supports_local"`
	Backup        BackupDefinition  `yaml:"backup"`
	Restore       RestoreDefinition `yaml:"restore"`
}

var (
	builtInDefinitionsOnce sync.Once
	builtInDefinitionsData map[string]builtInDefinition
	builtInDefinitionsErr  error
)

//go:embed yaml/*.yaml
var builtInDefinitionsFS embed.FS

func builtInDefinitions() (map[string]builtInDefinition, error) {
	builtInDefinitionsOnce.Do(func() {
		entries, err := fs.ReadDir(builtInDefinitionsFS, "yaml")
		if err != nil {
			builtInDefinitionsErr = fmt.Errorf("read embedded module yaml directory: %w", err)
			return
		}

		if len(entries) == 0 {
			builtInDefinitionsErr = fmt.Errorf("embedded module yaml directory has no service definitions")
			return
		}

		loaded := make(map[string]builtInDefinition, len(entries))
		for _, entryInfo := range entries {
			if entryInfo.IsDir() {
				continue
			}

			filename := entryInfo.Name()
			if !strings.HasSuffix(filename, ".yaml") {
				continue
			}

			moduleType := strings.TrimSuffix(filename, path.Ext(filename))
			if moduleType == "" {
				builtInDefinitionsErr = fmt.Errorf("invalid module yaml filename %q", filename)
				return
			}
			if _, exists := loaded[moduleType]; exists {
				builtInDefinitionsErr = fmt.Errorf("duplicate module type from embedded yaml %q", moduleType)
				return
			}

			raw, err := builtInDefinitionsFS.ReadFile(path.Join("yaml", filename))
			if err != nil {
				builtInDefinitionsErr = fmt.Errorf("read embedded module yaml %q: %w", filename, err)
				return
			}

			var entry builtInDefinitionYAML
			if err := yaml.Unmarshal(raw, &entry); err != nil {
				builtInDefinitionsErr = fmt.Errorf("parse embedded module yaml %q: %w", filename, err)
				return
			}

			if entry.ArtifactExt == "" {
				builtInDefinitionsErr = fmt.Errorf("module %q missing artifact_ext", moduleType)
				return
			}
			loaded[moduleType] = builtInDefinition{
				ext:            entry.ArtifactExt,
				localSupported: entry.SupportsLocal,
				definition: Definition{
					Backup:  entry.Backup,
					Restore: entry.Restore,
				},
			}
		}

		if len(loaded) == 0 {
			builtInDefinitionsErr = fmt.Errorf("no valid embedded module yaml files found")
			return
		}

		builtInDefinitionsData = loaded
	})

	if builtInDefinitionsErr != nil {
		return nil, builtInDefinitionsErr
	}
	return builtInDefinitionsData, nil
}
