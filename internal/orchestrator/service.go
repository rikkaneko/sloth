package orchestrator

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
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
	Force         bool
	UseChecksum   bool
	UseFileSize   bool
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
	Skipped     bool
}

type ListOptions struct {
	ServiceID string
	Remote    bool
}

type ListOutcome struct {
	Services            []config.ServiceEntry
	Backups             []BackupObject
	RemoteServiceGroups []RemoteServiceGroup
	RemoteBackupGroups  []RemoteBackupGroup
}

type BackupObject struct {
	Key          string
	Storage      string
	Version      string
	LastModified time.Time
	Size         int64
}

type RemoteServiceGroup struct {
	Storage string
	Rows    []RemoteServiceRow
}

type RemoteServiceRow struct {
	Service    string
	LastBackup time.Time
	ObjectKey  string
}

type RemoteBackupGroup struct {
	Storage string
	Backups []BackupObject
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
	latestObject := backupObjectCandidate{}
	shouldUseChecksum, shouldUseFileSize := resolveFileDeltaChecks(mainConfig, options)

	if storageConfig.UseNativeObjectVersioning {
		objectKey = path.Join(servicePrefix, backupResult.ArtifactName)
		versions, err := provider.ListObjectVersions(ctx, objectKey)
		if err != nil {
			return BackupOutcome{}, fmt.Errorf("list existing backup object versions: %w", err)
		}
		latestObject = selectLatestNativeCandidate(versions, objectKey)
	} else {
		existing, err := provider.ListObjects(ctx, servicePrefix+"/")
		if err != nil {
			return BackupOutcome{}, fmt.Errorf("list existing backup objects: %w", err)
		}
		version = versioning.NextVersionID(existing, servicePrefix)
		objectKey = path.Join(servicePrefix, version, backupResult.ArtifactName)
		latestObject = selectLatestVersionedCandidate(existing, servicePrefix, backupResult.ArtifactName)
	}

	if !options.Force && latestObject.Exists {
		upToDate, err := isBackupArtifactUpToDate(ctx, provider, backupResult.LocalPath, fileInfo.Size(), latestObject, shouldUseChecksum, shouldUseFileSize)
		if err != nil {
			return BackupOutcome{}, err
		}
		if upToDate {
			ui.Infof("Backup file is already up-to-date. Skipped.")
			return BackupOutcome{
				ServiceID:   resolved.Service.Name,
				StorageName: storageConfigName,
				Engine:      engine.Name(),
				ObjectKey:   latestObject.ObjectKey,
				Version:     latestObject.VersionLabel,
				Skipped:     true,
			}, nil
		}
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
		Skipped:     false,
	}, nil
}

func (m Manager) List(ctx context.Context, options ListOptions) (ListOutcome, error) {
	serviceID := strings.TrimSpace(options.ServiceID)
	if options.Remote {
		return m.listRemote(ctx, serviceID)
	}

	serviceResult, err := config.LoadServiceConfig()
	if err != nil {
		return ListOutcome{}, err
	}

	if serviceID == "" {
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

	backups := buildBackupObjects(objects, servicePrefix, storageConfig.UseNativeObjectVersioning, "")
	return ListOutcome{Backups: backups}, nil
}

func (m Manager) listRemote(ctx context.Context, serviceID string) (ListOutcome, error) {
	mainConfig, _, err := config.LoadMainConfig()
	if err != nil {
		return ListOutcome{}, err
	}

	if serviceID == "" {
		groups := make([]RemoteServiceGroup, 0, len(mainConfig.Storage))

		for _, storageConfig := range mainConfig.Storage {
			storageName := strings.TrimSpace(storageConfig.Name)
			if storageName == "" {
				continue
			}

			provider, err := m.storageFactory.Build(storageConfig)
			if err != nil {
				return ListOutcome{}, err
			}

			basePath := config.NormalizeBasePath(storageConfig.BasePath)
			objects, err := provider.ListObjects(ctx, basePath+"/")
			if err != nil {
				return ListOutcome{}, err
			}

			rows := buildRemoteServiceRows(objects, basePath)
			groups = append(groups, RemoteServiceGroup{
				Storage: storageName,
				Rows:    rows,
			})
		}

		sort.Slice(groups, func(i int, j int) bool {
			return groups[i].Storage < groups[j].Storage
		})
		return ListOutcome{RemoteServiceGroups: groups}, nil
	}

	backupGroups := make([]RemoteBackupGroup, 0, len(mainConfig.Storage))
	for _, storageConfig := range mainConfig.Storage {
		storageName := strings.TrimSpace(storageConfig.Name)
		if storageName == "" {
			continue
		}

		provider, err := m.storageFactory.Build(storageConfig)
		if err != nil {
			return ListOutcome{}, err
		}

		basePath := config.NormalizeBasePath(storageConfig.BasePath)
		servicePrefix := versioning.BuildVersionedPrefix(basePath, serviceID)

		objects := []storage.ObjectInfo{}
		if storageConfig.UseNativeObjectVersioning {
			objects, err = provider.ListObjectVersions(ctx, servicePrefix+"/")
		} else {
			objects, err = provider.ListObjects(ctx, servicePrefix+"/")
		}
		if err != nil {
			return ListOutcome{}, err
		}

		backups := buildBackupObjects(objects, servicePrefix, storageConfig.UseNativeObjectVersioning, storageName)
		if len(backups) == 0 {
			continue
		}

		backupGroups = append(backupGroups, RemoteBackupGroup{
			Storage: storageName,
			Backups: backups,
		})
	}

	sort.Slice(backupGroups, func(i int, j int) bool {
		return backupGroups[i].Storage < backupGroups[j].Storage
	})
	return ListOutcome{RemoteBackupGroups: backupGroups}, nil
}

func (m Manager) RestoreRetrieve(ctx context.Context, options RestoreRetrieveOptions) (RestoreRetrieveOutcome, error) {
	serviceID := strings.TrimSpace(options.ServiceID)
	if serviceID == "" {
		return RestoreRetrieveOutcome{}, fmt.Errorf("service id is required")
	}

	mainConfig, _, err := config.LoadMainConfig()
	if err != nil {
		return RestoreRetrieveOutcome{}, err
	}

	serviceResult, err := config.LoadServiceConfig()
	if err != nil {
		return RestoreRetrieveOutcome{}, err
	}
	service, _, found := serviceResult.Config.Find(serviceID)

	storageConfigName := "default"
	if found && strings.TrimSpace(service.Storage) != "" {
		storageConfigName = service.Storage
	}
	if strings.TrimSpace(options.Storage) != "" {
		storageConfigName = options.Storage
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
	servicePrefix := versioning.BuildVersionedPrefix(basePath, serviceID)

	selectedVersion := strings.TrimSpace(options.Version)
	if selectedVersion == "" {
		selectedVersion = "latest"
	}

	objectKey := ""
	objectTime := m.now()
	nativeVersionID := ""

	if storageConfig.UseNativeObjectVersioning {
		versions, err := provider.ListObjectVersions(ctx, servicePrefix+"/")
		if err != nil {
			return RestoreRetrieveOutcome{}, err
		}
		if len(versions) == 0 {
			return RestoreRetrieveOutcome{}, fmt.Errorf("no backup versions found for service %s", serviceID)
		}

		selectedObject, foundObject := selectNativeRestoreObject(versions, selectedVersion)
		if !foundObject {
			return RestoreRetrieveOutcome{}, fmt.Errorf("native object version %q not found for service %s", selectedVersion, serviceID)
		}

		objectKey = selectedObject.Key
		nativeVersionID = selectedObject.VersionID
		objectTime = selectedObject.LastModified

		if selectedVersion == "latest" {
			selectedVersion = resolveNativeVersionNumber(versions, selectedObject)
		} else if strings.TrimSpace(selectedObject.VersionID) != "" {
			selectedVersion = selectedObject.VersionID
		}
	} else {
		objects, err := provider.ListObjects(ctx, servicePrefix+"/")
		if err != nil {
			return RestoreRetrieveOutcome{}, err
		}
		if len(objects) == 0 {
			return RestoreRetrieveOutcome{}, fmt.Errorf("no backup versions found for service %s", serviceID)
		}

		if selectedVersion == "latest" {
			selectedVersion, err = versioning.SelectLatestVersion(objects, servicePrefix)
			if err != nil {
				return RestoreRetrieveOutcome{}, err
			}
		}

		selectedObject, foundObject := selectVersionedRestoreObject(objects, servicePrefix, selectedVersion)
		if !foundObject {
			return RestoreRetrieveOutcome{}, fmt.Errorf("backup version %q not found for service %s", selectedVersion, serviceID)
		}

		objectKey = selectedObject.Key
		objectTime = selectedObject.LastModified
	}

	versionLabel := selectedVersion
	if versionLabel == "" {
		versionLabel = "native"
	}

	extension := extractObjectExtension(objectKey)
	downloadName := fmt.Sprintf("%s-backup-%s-%s.%s", serviceID, objectTime.Format("20060102-150405"), versionLabel, extension)
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

type backupObjectCandidate struct {
	Exists       bool
	ObjectKey    string
	VersionID    string
	VersionLabel string
	Size         int64
	LastModified time.Time
}

func resolveFileDeltaChecks(mainConfig config.MainConfig, options BackupOptions) (bool, bool) {
	useChecksum := options.UseChecksum
	useFileSize := options.UseFileSize

	if !useChecksum && !useFileSize {
		switch mainConfig.ResolveFileDeltaCheck() {
		case "file_size":
			useFileSize = true
		default:
			useChecksum = true
		}
	}

	return useChecksum, useFileSize
}

func selectLatestVersionedCandidate(objects []storage.ObjectInfo, servicePrefix string, artifactName string) backupObjectCandidate {
	bestVersion := -1
	best := backupObjectCandidate{}

	for _, object := range objects {
		if !strings.HasSuffix(object.Key, "/"+artifactName) {
			continue
		}
		versionText := versioning.ExtractVersionFromKey(object.Key, servicePrefix)
		if versionText == "" {
			continue
		}
		versionNumber, err := strconv.Atoi(versionText)
		if err != nil {
			continue
		}

		if versionNumber > bestVersion || (versionNumber == bestVersion && object.LastModified.After(best.LastModified)) {
			bestVersion = versionNumber
			best = backupObjectCandidate{
				Exists:       true,
				ObjectKey:    object.Key,
				VersionLabel: versionText,
				Size:         object.Size,
				LastModified: object.LastModified,
			}
		}
	}

	return best
}

func selectLatestNativeCandidate(versions []storage.ObjectInfo, objectKey string) backupObjectCandidate {
	filtered := make([]storage.ObjectInfo, 0, len(versions))
	for _, versionObject := range versions {
		if versionObject.Key == objectKey {
			filtered = append(filtered, versionObject)
		}
	}
	if len(filtered) == 0 {
		return backupObjectCandidate{}
	}

	versioning.SortByLastModifiedDesc(filtered)
	latest := filtered[0]
	versionLabel := latest.VersionID
	if strings.TrimSpace(versionLabel) == "" {
		versionLabel = "latest"
	}

	return backupObjectCandidate{
		Exists:       true,
		ObjectKey:    latest.Key,
		VersionID:    latest.VersionID,
		VersionLabel: versionLabel,
		Size:         latest.Size,
		LastModified: latest.LastModified,
	}
}

func isBackupArtifactUpToDate(
	ctx context.Context,
	provider storage.Provider,
	localPath string,
	localSize int64,
	candidate backupObjectCandidate,
	useChecksum bool,
	useFileSize bool,
) (bool, error) {
	metadata, err := provider.HeadObject(ctx, candidate.ObjectKey, candidate.VersionID)
	if err != nil {
		return false, fmt.Errorf("read latest backup object metadata for delta check: %w", err)
	}

	if useChecksum {
		localChecksum, err := checksumFileSHA256Base64(localPath)
		if err != nil {
			return false, err
		}
		remoteChecksum := strings.TrimSpace(metadata.ChecksumSHA256)
		if remoteChecksum != "" {
			return localChecksum == remoteChecksum, nil
		}
		ui.Warnf("Remote checksum is unavailable, fallback to file-size check")
	}

	if useFileSize {
		return localSize == metadata.Size, nil
	}

	if !useChecksum {
		return false, nil
	}

	return localSize == metadata.Size, nil
}

func checksumFileSHA256Base64(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open file for checksum: %w", err)
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", fmt.Errorf("calculate file checksum: %w", err)
	}
	return base64.StdEncoding.EncodeToString(hasher.Sum(nil)), nil
}

func selectNativeRestoreObject(versions []storage.ObjectInfo, requestedVersion string) (storage.ObjectInfo, bool) {
	if len(versions) == 0 {
		return storage.ObjectInfo{}, false
	}

	candidates := append([]storage.ObjectInfo{}, versions...)
	versioning.SortByLastModifiedDesc(candidates)

	if requestedVersion == "latest" {
		return candidates[0], true
	}

	for _, candidate := range candidates {
		if candidate.VersionID == requestedVersion {
			return candidate, true
		}
	}
	return storage.ObjectInfo{}, false
}

func selectVersionedRestoreObject(objects []storage.ObjectInfo, servicePrefix string, requestedVersion string) (storage.ObjectInfo, bool) {
	candidates := make([]storage.ObjectInfo, 0, len(objects))
	for _, object := range objects {
		if versioning.ExtractVersionFromKey(object.Key, servicePrefix) == requestedVersion {
			candidates = append(candidates, object)
		}
	}

	if len(candidates) == 0 {
		return storage.ObjectInfo{}, false
	}

	versioning.SortByLastModifiedDesc(candidates)
	return candidates[0], true
}

func resolveNativeVersionNumber(versions []storage.ObjectInfo, selected storage.ObjectInfo) string {
	if len(versions) == 0 {
		return "1"
	}

	ordered := append([]storage.ObjectInfo{}, versions...)
	sort.Slice(ordered, func(i int, j int) bool {
		if ordered[i].LastModified.Equal(ordered[j].LastModified) {
			if ordered[i].Key == ordered[j].Key {
				return ordered[i].VersionID < ordered[j].VersionID
			}
			return ordered[i].Key < ordered[j].Key
		}
		return ordered[i].LastModified.Before(ordered[j].LastModified)
	})

	for index, object := range ordered {
		if object.Key == selected.Key && object.VersionID == selected.VersionID && object.LastModified.Equal(selected.LastModified) {
			return strconv.Itoa(index + 1)
		}
	}

	return strconv.Itoa(len(ordered))
}

func extractObjectExtension(objectKey string) string {
	ext := strings.TrimSpace(path.Ext(objectKey))
	if ext == "" {
		return "bin"
	}

	trimmed := strings.TrimPrefix(ext, ".")
	if trimmed == "" {
		return "bin"
	}
	return trimmed
}

func buildBackupObjects(objects []storage.ObjectInfo, servicePrefix string, useNativeObjectVersioning bool, storageName string) []BackupObject {
	backups := make([]BackupObject, 0, len(objects))
	for _, object := range objects {
		versionValue := object.VersionID
		if versionValue == "" {
			versionValue = versioning.ExtractVersionFromKey(object.Key, servicePrefix)
			if versionValue == "" && useNativeObjectVersioning {
				versionValue = "latest"
			}
		}

		backups = append(backups, BackupObject{
			Key:          object.Key,
			Storage:      storageName,
			Version:      versionValue,
			LastModified: object.LastModified,
			Size:         object.Size,
		})
	}

	sort.Slice(backups, func(i int, j int) bool {
		if backups[i].LastModified.Equal(backups[j].LastModified) {
			if backups[i].Version == backups[j].Version {
				return backups[i].Key < backups[j].Key
			}
			return backups[i].Version < backups[j].Version
		}
		return backups[i].LastModified.After(backups[j].LastModified)
	})

	return backups
}

func buildRemoteServiceRows(objects []storage.ObjectInfo, basePath string) []RemoteServiceRow {
	latestByService := map[string]RemoteServiceRow{}

	for _, object := range objects {
		serviceID, ok := extractServiceIDFromObjectKey(object.Key, basePath)
		if !ok {
			continue
		}

		current, exists := latestByService[serviceID]
		if !exists || object.LastModified.After(current.LastBackup) || (object.LastModified.Equal(current.LastBackup) && object.Key < current.ObjectKey) {
			latestByService[serviceID] = RemoteServiceRow{
				Service:    serviceID,
				LastBackup: object.LastModified,
				ObjectKey:  object.Key,
			}
		}
	}

	rows := make([]RemoteServiceRow, 0, len(latestByService))
	for _, row := range latestByService {
		rows = append(rows, row)
	}

	sort.Slice(rows, func(i int, j int) bool {
		if rows[i].Service == rows[j].Service {
			return rows[i].ObjectKey < rows[j].ObjectKey
		}
		return rows[i].Service < rows[j].Service
	})

	return rows
}

func extractServiceIDFromObjectKey(key string, basePath string) (string, bool) {
	normalizedKey := strings.Trim(strings.TrimSpace(key), "/")
	if normalizedKey == "" {
		return "", false
	}

	normalizedBasePath := strings.Trim(strings.TrimSpace(basePath), "/")
	if normalizedBasePath != "" {
		prefix := normalizedBasePath + "/"
		if !strings.HasPrefix(normalizedKey, prefix) {
			return "", false
		}
		normalizedKey = strings.TrimPrefix(normalizedKey, prefix)
	}

	if normalizedKey == "" {
		return "", false
	}

	segments := strings.SplitN(normalizedKey, "/", 2)
	serviceID := strings.TrimSpace(segments[0])
	if serviceID == "" {
		return "", false
	}

	return serviceID, true
}
