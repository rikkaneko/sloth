package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	defaultConfigHomeRelPath = ".config/sloth"
	mainConfigFileName       = "main.yaml"
	serviceConfigFileName    = "service.yaml"
)

var configHomeOverride string

type ServiceConfig struct {
	Service []ServiceEntry `yaml:"service"`
}

type ServiceEntry struct {
	Name           string            `yaml:"name"`
	ContainerName  string            `yaml:"container_name"`
	Type           string            `yaml:"type"`
	Engine         string            `yaml:"engine,omitempty"`
	Storage        string            `yaml:"storage,omitempty"`
	LastBackupTime string            `yaml:"last_backup_time,omitempty"`
	EnvFile        string            `yaml:"env_file,omitempty"`
	ModuleConfig   string            `yaml:"module_config,omitempty"`
	VolumeName     string            `yaml:"volume_name,omitempty"`
	VolumeNames    []string          `yaml:"volume_names,omitempty"`
	Meta           map[string]string `yaml:"meta,omitempty"`
}

type MainConfig struct {
	Storage []StorageConfig `yaml:"storage"`
	Common  CommonConfig    `yaml:"common,omitempty"`
}

type CommonConfig struct {
	FileDeltaCheck string `yaml:"file_delta_check,omitempty"`
}

type StorageConfig struct {
	Name                      string `yaml:"name"`
	Type                      string `yaml:"type"`
	Endpoint                  string `yaml:"endpoint"`
	Region                    string `yaml:"region"`
	Bucket                    string `yaml:"bucket"`
	Backet                    string `yaml:"backet"`
	AccessKeyID               string `yaml:"access_key_id"`
	SecretAccessKey           string `yaml:"secret_access_key"`
	UseNativeObjectVersioning bool   `yaml:"use_native_object_versioning"`
	BasePath                  string `yaml:"base_path"`
}

type ServiceLoadResult struct {
	Config ServiceConfig
	Source string
}

func LoadMainConfig() (MainConfig, string, error) {
	path, err := resolveMainConfigPath()
	if err != nil {
		return MainConfig{}, "", err
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return MainConfig{}, "", fmt.Errorf("read main config: %w", err)
	}
	var cfg MainConfig
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return MainConfig{}, "", fmt.Errorf("parse main config yaml: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return MainConfig{}, "", err
	}

	return cfg, path, nil
}

func LoadServiceConfig() (ServiceLoadResult, error) {
	path, err := resolveServiceConfigPath()
	if err != nil {
		return ServiceLoadResult{}, err
	}

	cfg, err := readServiceConfig(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ServiceLoadResult{Config: ServiceConfig{}, Source: path}, nil
		}
		return ServiceLoadResult{}, err
	}
	return ServiceLoadResult{Config: cfg, Source: path}, nil
}

func readServiceConfig(path string) (ServiceConfig, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ServiceConfig{}, fmt.Errorf("read service config: %w", err)
	}

	var cfg ServiceConfig
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return ServiceConfig{}, fmt.Errorf("parse service config yaml: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return ServiceConfig{}, err
	}

	return cfg, nil
}

func (cfg MainConfig) Validate() error {
	if len(cfg.Storage) == 0 {
		return errors.New("main config must include at least one storage entry")
	}

	switch strings.ToLower(strings.TrimSpace(cfg.Common.FileDeltaCheck)) {
	case "", "checksum", "file_size":
	default:
		return fmt.Errorf("common.file_delta_check %q unsupported; use checksum or file_size", cfg.Common.FileDeltaCheck)
	}

	names := map[string]struct{}{}
	for i := range cfg.Storage {
		s := cfg.Storage[i]
		if s.Bucket == "" && s.Backet != "" {
			cfg.Storage[i].Bucket = s.Backet
			s.Bucket = s.Backet
		}
		if s.Name == "" {
			return fmt.Errorf("storage[%d] name is required", i)
		}
		if _, exists := names[s.Name]; exists {
			return fmt.Errorf("duplicate storage name: %s", s.Name)
		}
		names[s.Name] = struct{}{}
		if s.Type == "" {
			return fmt.Errorf("storage[%s] type is required", s.Name)
		}
		if strings.ToLower(s.Type) != "s3" {
			return fmt.Errorf("storage[%s] type %q unsupported; only s3 is supported", s.Name, s.Type)
		}
		if s.Endpoint == "" {
			return fmt.Errorf("storage[%s] endpoint is required", s.Name)
		}
		if s.Bucket == "" {
			return fmt.Errorf("storage[%s] bucket/backet is required", s.Name)
		}
		if s.AccessKeyID == "" || s.SecretAccessKey == "" {
			return fmt.Errorf("storage[%s] access_key_id and secret_access_key are required", s.Name)
		}
	}

	return nil
}

func (cfg MainConfig) ResolveFileDeltaCheck() string {
	mode := strings.ToLower(strings.TrimSpace(cfg.Common.FileDeltaCheck))
	if mode == "file_size" {
		return "file_size"
	}
	return "checksum"
}

func (cfg ServiceConfig) Validate() error {
	names := map[string]struct{}{}
	for i := range cfg.Service {
		s := cfg.Service[i]
		if s.Name == "" {
			return fmt.Errorf("service[%d] name is required", i)
		}
		if _, exists := names[s.Name]; exists {
			return fmt.Errorf("duplicate service name: %s", s.Name)
		}
		names[s.Name] = struct{}{}

		if s.Type == "" {
			return fmt.Errorf("service[%s] type is required", s.Name)
		}
	}
	return nil
}

func (cfg ServiceConfig) Find(name string) (ServiceEntry, int, bool) {
	for i := range cfg.Service {
		if cfg.Service[i].Name == name {
			return cfg.Service[i], i, true
		}
	}
	return ServiceEntry{}, -1, false
}

func (cfg ServiceConfig) ResolveStorage(service ServiceEntry, override string) string {
	if override != "" {
		return override
	}
	if service.Storage != "" {
		return service.Storage
	}
	return "default"
}

func (cfg MainConfig) FindStorage(name string) (StorageConfig, bool) {
	for _, s := range cfg.Storage {
		if s.Name == name {
			if s.Bucket == "" {
				s.Bucket = s.Backet
			}
			if s.Region == "" {
				s.Region = "auto"
			}
			if s.BasePath == "" {
				s.BasePath = "/backup"
			}
			return s, true
		}
	}
	return StorageConfig{}, false
}

func SaveServiceConfig(path string, cfg ServiceConfig) error {
	if path == "" {
		resolvedPath, err := resolveServiceConfigPath()
		if err != nil {
			return err
		}
		path = resolvedPath
	}

	if err := cfg.Validate(); err != nil {
		return err
	}

	raw, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal service config: %w", err)
	}

	dir := filepath.Dir(path)
	if dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("ensure config directory: %w", err)
		}
	}

	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return fmt.Errorf("write service config: %w", err)
	}
	return nil
}

func NormalizeBasePath(basePath string) string {
	cleaned := strings.TrimSpace(basePath)
	if cleaned == "" {
		return "backup"
	}
	cleaned = strings.TrimPrefix(cleaned, "/")
	cleaned = strings.TrimSuffix(cleaned, "/")
	return cleaned
}

func SetConfigHomeOverride(path string) error {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		configHomeOverride = ""
		return nil
	}

	resolved, err := resolveInputPath(trimmed)
	if err != nil {
		return err
	}

	configHomeOverride = resolved
	return nil
}

func ResolveConfigHome() (string, error) {
	if strings.TrimSpace(configHomeOverride) != "" {
		return configHomeOverride, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home: %w", err)
	}
	return filepath.Join(home, defaultConfigHomeRelPath), nil
}

func resolveMainConfigPath() (string, error) {
	configHome, err := ResolveConfigHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(configHome, mainConfigFileName), nil
}

func resolveServiceConfigPath() (string, error) {
	configHome, err := ResolveConfigHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(configHome, serviceConfigFileName), nil
}

func resolveInputPath(path string) (string, error) {
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve user home: %w", err)
		}
		if path == "~" {
			return home, nil
		}
		return filepath.Join(home, strings.TrimPrefix(path, "~/")), nil
	}
	return filepath.Clean(path), nil
}
