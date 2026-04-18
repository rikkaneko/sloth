package orchestrator

import (
	"context"
	"fmt"
	"math"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"sloth/internal/config"
	"sloth/internal/container"
	envloader "sloth/internal/env"
	"sloth/internal/modules"
	"sloth/internal/storage"
	s3storage "sloth/internal/storage/s3"
	"sloth/internal/ui"
	"sloth/internal/versioning"
)

type StorageFactory interface {
	Build(storageConfig config.StorageConfig) (storage.Provider, error)
}

type EnvLoader interface {
	Load(path string) (map[string]string, error)
}

type ModuleRegistry interface {
	Resolve(serviceType string, overridePath string) (modules.Module, error)
}

type DefaultStorageFactory struct{}

func (DefaultStorageFactory) Build(storageConfig config.StorageConfig) (storage.Provider, error) {
	switch strings.ToLower(storageConfig.Type) {
	case "s3":
		return s3storage.NewProvider(storageConfig)
	default:
		return nil, fmt.Errorf("unsupported storage type %q", storageConfig.Type)
	}
}

type Manager struct {
	envLoader      EnvLoader
	moduleRegistry ModuleRegistry
	storageFactory StorageFactory
	now            func() time.Time
}

func NewManager() Manager {
	return Manager{
		envLoader:      envloader.NewLoader(),
		moduleRegistry: modules.NewRegistry(),
		storageFactory: DefaultStorageFactory{},
		now:            time.Now,
	}
}

type BackupOptions struct {
	ServiceID     string
	Type          string
	ContainerName string
	Engine        string
	Local         bool
	Storage       string
	EnvFile       string
	ModuleConfig  string
	VolumeName    string
	VolumeNames   []string
}

type BackupOutcome struct {
	ServiceID   string
	StorageName string
	Engine      string
	ObjectKey   string
	Version     string
}

type ListOutcome struct {
	Services []config.ServiceEntry
	Backups  []BackupObject
}

type BackupObject struct {
	Key          string
	Version      string
	LastModified time.Time
	Size         int64
}

type RestoreRetrieveOptions struct {
	ServiceID     string
	Version       string
	Type          string
	ContainerName string
	Engine        string
	Local         bool
	Storage       string
	EnvFile       string
	ModuleConfig  string
}

type RestoreRetrieveOutcome struct {
	DownloadedPath string
	ObjectKey      string
	Version        string
	Guidance       string
}

type RestoreApplyOptions struct {
	ServiceID     string
	BackupFile    string
	Type          string
	ContainerName string
	Engine        string
	Local         bool
	Storage       string
	EnvFile       string
	ModuleConfig  string
}

type RestoreApplyOutcome struct {
	Guidance string
	Engine   string
}

func (m Manager) Backup(ctx context.Context, options BackupOptions) (BackupOutcome, error) {
	if strings.TrimSpace(options.ServiceID) == "" {
		return BackupOutcome{}, fmt.Errorf("service id is required")
	}

	mainConfig, _, err := config.LoadMainConfig()
	if err != nil {
		return BackupOutcome{}, err
	}

	resolved, err := resolveServiceForOperation(options.ServiceID, serviceResolutionOptions{
		Type:          options.Type,
		ContainerName: options.ContainerName,
		Engine:        options.Engine,
		Local:         options.Local,
		Storage:       options.Storage,
		EnvFile:       options.EnvFile,
		ModuleConfig:  options.ModuleConfig,
		VolumeName:    options.VolumeName,
		VolumeNames:   options.VolumeNames,
		AllowCreate:   true,
	})
	if err != nil {
		return BackupOutcome{}, err
	}

	resolution, err := container.ResolveEngine(
		ctx,
		options.Engine,
		resolved.Service.Engine,
		options.ContainerName,
		resolved.Service.ContainerName,
		resolved.Service.Name,
		options.Local,
	)
	if err != nil {
		return BackupOutcome{}, err
	}
	engine := resolution.Engine
	if resolution.ContainerName != "" {
		resolved.Service.ContainerName = resolution.ContainerName
	}

	ui.Infof("Found service %s%s [%s]", resolved.Service.Name, renderContainerSuffix(resolved.Service.Name, resolved.Service.ContainerName), engine.Name())
	ui.Infof("Running %s modules for %s ...", resolved.Service.Type, resolved.Service.Name)

	envMap, err := m.loadEnv(resolved.Service, options.EnvFile)
	if err != nil {
		return BackupOutcome{}, err
	}

	module, err := m.moduleRegistry.Resolve(resolved.Service.Type, resolved.Service.ModuleConfig)
	if err != nil {
		return BackupOutcome{}, err
	}

	tempDir, err := os.MkdirTemp("", "sloth-backup-*")
	if err != nil {
		return BackupOutcome{}, fmt.Errorf("create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	backupResult, err := module.Backup(ctx, modules.BackupRequest{
		Service: resolved.Service,
		Engine:  engine,
		Env:     envMap,
		TempDir: tempDir,
	})
	if err != nil {
		return BackupOutcome{}, err
	}

	fileInfo, err := os.Stat(backupResult.LocalPath)
	if err != nil {
		return BackupOutcome{}, fmt.Errorf("stat backup artifact: %w", err)
	}
	ui.Infof("Created backup file for %s (%s)", resolved.Service.Name, humanReadableSize(fileInfo.Size()))

	storageConfigName := resolved.Service.Storage
	if options.Storage != "" {
		storageConfigName = options.Storage
	}
	if storageConfigName == "" {
		storageConfigName = "default"
	}

	storageConfig, ok := mainConfig.FindStorage(storageConfigName)
	if !ok {
		return BackupOutcome{}, fmt.Errorf("storage %q not found", storageConfigName)
	}

	provider, err := m.storageFactory.Build(storageConfig)
	if err != nil {
		return BackupOutcome{}, err
	}

	basePath := config.NormalizeBasePath(storageConfig.BasePath)
	servicePrefix := versioning.BuildVersionedPrefix(basePath, resolved.Service.Name)

	var objectKey string
	version := "native"

	if storageConfig.UseNativeObjectVersioning {
		objectKey = path.Join(servicePrefix, backupResult.ArtifactName)
	} else {
		existing, err := provider.ListObjects(ctx, servicePrefix+"/")
		if err != nil {
			return BackupOutcome{}, fmt.Errorf("list existing backup objects: %w", err)
		}
		version = versioning.NextVersionID(existing, servicePrefix)
		objectKey = path.Join(servicePrefix, version, backupResult.ArtifactName)
	}

	ui.Infof("Uploading backup to %s (Version %s) ...", storageConfigName, version)
	if err := provider.Put(ctx, objectKey, backupResult.LocalPath); err != nil {
		return BackupOutcome{}, err
	}
	ui.Infof("Uploaded")

	resolved.Service.LastBackupTime = m.now().Format(time.RFC3339)
	if err := saveServiceResolution(resolved); err != nil {
		return BackupOutcome{}, err
	}

	return BackupOutcome{
		ServiceID:   resolved.Service.Name,
		StorageName: storageConfigName,
		Engine:      engine.Name(),
		ObjectKey:   objectKey,
		Version:     version,
	}, nil
}

func (m Manager) List(ctx context.Context, serviceID string) (ListOutcome, error) {
	serviceResult, err := config.LoadServiceConfig()
	if err != nil {
		return ListOutcome{}, err
	}

	if strings.TrimSpace(serviceID) == "" {
		services := append([]config.ServiceEntry{}, serviceResult.Config.Service...)
		sort.Slice(services, func(i int, j int) bool {
			return services[i].Name < services[j].Name
		})
		return ListOutcome{Services: services}, nil
	}

	service, _, found := serviceResult.Config.Find(serviceID)
	if !found {
		return ListOutcome{}, fmt.Errorf("service %q not found", serviceID)
	}

	mainConfig, _, err := config.LoadMainConfig()
	if err != nil {
		return ListOutcome{}, err
	}

	storageConfigName := service.Storage
	if storageConfigName == "" {
		storageConfigName = "default"
	}

	storageConfig, ok := mainConfig.FindStorage(storageConfigName)
	if !ok {
		return ListOutcome{}, fmt.Errorf("storage %q not found", storageConfigName)
	}

	provider, err := m.storageFactory.Build(storageConfig)
	if err != nil {
		return ListOutcome{}, err
	}

	basePath := config.NormalizeBasePath(storageConfig.BasePath)
	servicePrefix := versioning.BuildVersionedPrefix(basePath, service.Name)

	objects := []storage.ObjectInfo{}
	if storageConfig.UseNativeObjectVersioning {
		objects, err = provider.ListObjectVersions(ctx, servicePrefix+"/")
	} else {
		objects, err = provider.ListObjects(ctx, servicePrefix+"/")
	}
	if err != nil {
		return ListOutcome{}, err
	}

	backups := make([]BackupObject, 0, len(objects))
	for _, obj := range objects {
		versionValue := obj.VersionID
		if versionValue == "" {
			versionValue = versioning.ExtractVersionFromKey(obj.Key, servicePrefix)
			if versionValue == "" && storageConfig.UseNativeObjectVersioning {
				versionValue = "latest"
			}
		}
		backups = append(backups, BackupObject{
			Key:          obj.Key,
			Version:      versionValue,
			LastModified: obj.LastModified,
			Size:         obj.Size,
		})
	}

	sort.Slice(backups, func(i int, j int) bool {
		return backups[i].LastModified.After(backups[j].LastModified)
	})

	return ListOutcome{Backups: backups}, nil
}

func (m Manager) RestoreRetrieve(ctx context.Context, options RestoreRetrieveOptions) (RestoreRetrieveOutcome, error) {
	if strings.TrimSpace(options.ServiceID) == "" {
		return RestoreRetrieveOutcome{}, fmt.Errorf("service id is required")
	}

	resolved, err := resolveServiceForOperation(options.ServiceID, serviceResolutionOptions{
		Type:          options.Type,
		ContainerName: options.ContainerName,
		Engine:        options.Engine,
		Local:         options.Local,
		Storage:       options.Storage,
		EnvFile:       options.EnvFile,
		ModuleConfig:  options.ModuleConfig,
		AllowCreate:   false,
	})
	if err != nil {
		return RestoreRetrieveOutcome{}, err
	}

	module, err := m.moduleRegistry.Resolve(resolved.Service.Type, resolved.Service.ModuleConfig)
	if err != nil {
		return RestoreRetrieveOutcome{}, err
	}

	mainConfig, _, err := config.LoadMainConfig()
	if err != nil {
		return RestoreRetrieveOutcome{}, err
	}

	storageConfigName := resolved.Service.Storage
	if options.Storage != "" {
		storageConfigName = options.Storage
	}
	if storageConfigName == "" {
		storageConfigName = "default"
	}

	storageConfig, ok := mainConfig.FindStorage(storageConfigName)
	if !ok {
		return RestoreRetrieveOutcome{}, fmt.Errorf("storage %q not found", storageConfigName)
	}

	provider, err := m.storageFactory.Build(storageConfig)
	if err != nil {
		return RestoreRetrieveOutcome{}, err
	}

	basePath := config.NormalizeBasePath(storageConfig.BasePath)
	servicePrefix := versioning.BuildVersionedPrefix(basePath, resolved.Service.Name)
	artifactName := module.ArtifactFileName(resolved.Service)

	objectKey := ""
	selectedVersion := options.Version
	objectTime := m.now()
	nativeVersionID := ""

	if storageConfig.UseNativeObjectVersioning {
		objectKey = path.Join(servicePrefix, artifactName)
		versions, err := provider.ListObjectVersions(ctx, objectKey)
		if err != nil {
			return RestoreRetrieveOutcome{}, err
		}
		if len(versions) > 0 {
			versioning.SortByLastModifiedDesc(versions)
			if selectedVersion == "" || selectedVersion == "latest" {
				nativeVersionID = versions[0].VersionID
				objectTime = versions[0].LastModified
				selectedVersion = "latest"
			} else {
				foundVersion := false
				for _, versionObject := range versions {
					if versionObject.VersionID == selectedVersion {
						nativeVersionID = selectedVersion
						objectTime = versionObject.LastModified
						foundVersion = true
						break
					}
				}
				if !foundVersion {
					return RestoreRetrieveOutcome{}, fmt.Errorf("native object version %q not found for service %s", selectedVersion, resolved.Service.Name)
				}
			}
		} else if selectedVersion != "" && selectedVersion != "latest" {
			return RestoreRetrieveOutcome{}, fmt.Errorf("native object versions not found for service %s", resolved.Service.Name)
		}
	} else {
		objects, err := provider.ListObjects(ctx, servicePrefix+"/")
		if err != nil {
			return RestoreRetrieveOutcome{}, err
		}

		if selectedVersion == "" || selectedVersion == "latest" {
			selectedVersion, err = versioning.SelectLatestVersion(objects, servicePrefix)
			if err != nil {
				return RestoreRetrieveOutcome{}, err
			}
		}

		objectKey = path.Join(servicePrefix, selectedVersion, artifactName)

		for _, object := range objects {
			if object.Key == objectKey {
				objectTime = object.LastModified
				break
			}
		}
	}

	versionLabel := selectedVersion
	if versionLabel == "" {
		versionLabel = "native"
	}
	extension := artifactExtension(artifactName)
	downloadName := fmt.Sprintf("%s-backup-%s-%s.%s", resolved.Service.Name, objectTime.Format("20060102-150405"), versionLabel, extension)
	destination := filepath.Join(".", downloadName)

	if err := provider.Get(ctx, objectKey, nativeVersionID, destination); err != nil {
		return RestoreRetrieveOutcome{}, err
	}

	guidance := "Download completed. Clean up old container/bind mounts/volumes before applying restore with --apply."

	return RestoreRetrieveOutcome{
		DownloadedPath: destination,
		ObjectKey:      objectKey,
		Version:        versionLabel,
		Guidance:       guidance,
	}, nil
}

func (m Manager) RestoreApply(ctx context.Context, options RestoreApplyOptions) (RestoreApplyOutcome, error) {
	if strings.TrimSpace(options.ServiceID) == "" {
		return RestoreApplyOutcome{}, fmt.Errorf("service id is required")
	}
	if strings.TrimSpace(options.BackupFile) == "" {
		return RestoreApplyOutcome{}, fmt.Errorf("backup file is required")
	}

	resolved, err := resolveServiceForOperation(options.ServiceID, serviceResolutionOptions{
		Type:          options.Type,
		ContainerName: options.ContainerName,
		Engine:        options.Engine,
		Local:         options.Local,
		Storage:       options.Storage,
		EnvFile:       options.EnvFile,
		ModuleConfig:  options.ModuleConfig,
		AllowCreate:   true,
	})
	if err != nil {
		return RestoreApplyOutcome{}, err
	}

	resolution, err := container.ResolveEngine(
		ctx,
		options.Engine,
		resolved.Service.Engine,
		options.ContainerName,
		resolved.Service.ContainerName,
		resolved.Service.Name,
		options.Local,
	)
	if err != nil {
		return RestoreApplyOutcome{}, err
	}
	engine := resolution.Engine
	if resolution.ContainerName != "" {
		resolved.Service.ContainerName = resolution.ContainerName
	}

	ui.Infof("Found service %s%s [%s]", resolved.Service.Name, renderContainerSuffix(resolved.Service.Name, resolved.Service.ContainerName), engine.Name())
	ui.Infof("Running %s modules for %s ...", resolved.Service.Type, resolved.Service.Name)

	envMap, err := m.loadEnv(resolved.Service, options.EnvFile)
	if err != nil {
		return RestoreApplyOutcome{}, err
	}

	module, err := m.moduleRegistry.Resolve(resolved.Service.Type, resolved.Service.ModuleConfig)
	if err != nil {
		return RestoreApplyOutcome{}, err
	}

	absoluteBackupPath, err := filepath.Abs(options.BackupFile)
	if err != nil {
		return RestoreApplyOutcome{}, fmt.Errorf("resolve backup file path: %w", err)
	}

	restoreResult, err := module.Restore(ctx, modules.RestoreRequest{
		Service:    resolved.Service,
		Engine:     engine,
		Env:        envMap,
		BackupFile: absoluteBackupPath,
		WorkingDir: ".",
	})
	if err != nil {
		return RestoreApplyOutcome{}, err
	}

	return RestoreApplyOutcome{
		Guidance: restoreResult.Guidance,
		Engine:   engine.Name(),
	}, nil
}

func (m Manager) loadEnv(service config.ServiceEntry, overridePath string) (map[string]string, error) {
	envPath := overridePath
	if envPath == "" {
		envPath = service.EnvFile
	}
	return m.envLoader.Load(envPath)
}

type serviceResolutionOptions struct {
	Type          string
	ContainerName string
	Engine        string
	Local         bool
	Storage       string
	EnvFile       string
	ModuleConfig  string
	VolumeName    string
	VolumeNames   []string
	AllowCreate   bool
}

type resolvedService struct {
	Service      config.ServiceEntry
	Config       config.ServiceConfig
	ConfigSource string
	ServiceIndex int
}

func resolveServiceForOperation(serviceID string, options serviceResolutionOptions) (resolvedService, error) {
	serviceLoad, err := config.LoadServiceConfig()
	if err != nil {
		return resolvedService{}, err
	}

	current, idx, found := serviceLoad.Config.Find(serviceID)
	if !found {
		if !options.AllowCreate {
			return resolvedService{}, fmt.Errorf("service %q not found", serviceID)
		}
		if strings.TrimSpace(options.Type) == "" {
			return resolvedService{}, fmt.Errorf("service %q not found; provide --type to create local .sloth.yaml entry", serviceID)
		}

		engineName := options.Engine
		if options.Local {
			engineName = "local"
		}

		current = config.ServiceEntry{
			Name:          serviceID,
			Type:          options.Type,
			ContainerName: options.ContainerName,
			Engine:        engineName,
			Storage:       options.Storage,
			EnvFile:       options.EnvFile,
			ModuleConfig:  options.ModuleConfig,
			VolumeName:    options.VolumeName,
			VolumeNames:   options.VolumeNames,
		}

		serviceLoad.Config.Service = append(serviceLoad.Config.Service, current)
		idx = len(serviceLoad.Config.Service) - 1

		serviceLoad.Source = ".sloth.yaml"
		if err := config.SaveServiceConfig(serviceLoad.Source, serviceLoad.Config); err != nil {
			return resolvedService{}, err
		}
	}

	if options.Type != "" {
		current.Type = options.Type
	}
	if options.ContainerName != "" {
		current.ContainerName = options.ContainerName
	}
	if options.Engine != "" {
		current.Engine = options.Engine
	}
	if options.Local {
		current.Engine = "local"
	}
	if options.Storage != "" {
		current.Storage = options.Storage
	}
	if options.EnvFile != "" {
		current.EnvFile = options.EnvFile
	}
	if options.ModuleConfig != "" {
		current.ModuleConfig = options.ModuleConfig
	}
	if options.VolumeName != "" {
		current.VolumeName = options.VolumeName
	}
	if len(options.VolumeNames) > 0 {
		current.VolumeNames = options.VolumeNames
	}

	serviceLoad.Config.Service[idx] = current

	return resolvedService{
		Service:      current,
		Config:       serviceLoad.Config,
		ConfigSource: serviceLoad.Source,
		ServiceIndex: idx,
	}, nil
}

func saveServiceResolution(resolved resolvedService) error {
	if resolved.ServiceIndex < 0 || resolved.ServiceIndex >= len(resolved.Config.Service) {
		return fmt.Errorf("invalid service index while saving service config")
	}
	resolved.Config.Service[resolved.ServiceIndex] = resolved.Service
	return config.SaveServiceConfig(resolved.ConfigSource, resolved.Config)
}

func renderContainerSuffix(serviceID string, containerName string) string {
	trimmed := strings.TrimSpace(containerName)
	if trimmed == "" || trimmed == serviceID {
		return ""
	}
	return fmt.Sprintf(" (%s)", trimmed)
}

func humanReadableSize(size int64) string {
	units := []string{"B", "KB", "MB", "GB", "TB"}
	value := float64(size)
	index := 0
	for value >= 1024 && index < len(units)-1 {
		value /= 1024
		index++
	}

	if index == 0 {
		return fmt.Sprintf("%d %s", size, units[index])
	}
	value = math.Round(value*10) / 10
	return fmt.Sprintf("%s %s", strconv.FormatFloat(value, 'f', 1, 64), units[index])
}

func artifactExtension(fileName string) string {
	idx := strings.Index(fileName, ".")
	if idx < 0 {
		return "bin"
	}
	return fileName[idx+1:]
}
