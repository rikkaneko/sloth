package cli

import (
	"context"
	"flag"
	"fmt"
	"math"
	"os"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"sloth/internal/orchestrator"
	"sloth/internal/ui"
)

type App struct {
	manager manager
	logger  ui.Logger
	version string
}

type manager interface {
	Backup(ctx context.Context, options orchestrator.BackupOptions) (orchestrator.BackupOutcome, error)
	List(ctx context.Context, options orchestrator.ListOptions) (orchestrator.ListOutcome, error)
	RestoreRetrieve(ctx context.Context, options orchestrator.RestoreRetrieveOptions) (orchestrator.RestoreRetrieveOutcome, error)
	RestoreApply(ctx context.Context, options orchestrator.RestoreApplyOptions) (orchestrator.RestoreApplyOutcome, error)
}

func NewApp(version string) App {
	normalizedVersion := resolveDisplayVersion(version)

	return App{
		manager: orchestrator.NewManager(),
		logger:  ui.NewLogger(),
		version: normalizedVersion,
	}
}

func (a App) Run(ctx context.Context, args []string) error {
	a.printBanner()

	if len(args) == 0 {
		a.printRootHelp()
		return nil
	}

	if isRootHelpArg(args[0]) {
		a.printRootHelp()
		return nil
	}

	if args[0] == "help" {
		if len(args) == 1 {
			a.printRootHelp()
			return nil
		}
		return a.printCommandHelp(args[1])
	}

	switch args[0] {
	case "backup":
		if hasHelpFlag(args[1:]) {
			a.printBackupHelp()
			return nil
		}
		return a.runBackup(ctx, args[1:])
	case "list":
		if hasHelpFlag(args[1:]) {
			a.printListHelp()
			return nil
		}
		return a.runList(ctx, args[1:])
	case "restore":
		if hasHelpFlag(args[1:]) {
			a.printRestoreHelp()
			return nil
		}
		return a.runRestore(ctx, args[1:])
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func (a App) runBackup(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("backup requires <service-id>")
	}

	serviceID := args[0]
	flagSet := flag.NewFlagSet("backup", flag.ContinueOnError)
	flagSet.SetOutput(os.Stdout)

	var typeValue string
	var containerName string
	var engine string
	var local bool
	var storageName string
	var envFile string
	var moduleConfig string
	var volumeName string
	var volumeNamesRaw string
	var force bool
	var useChecksum bool
	var useFileSizeCheck bool
	var debugMode bool

	flagSet.StringVar(&typeValue, "type", "", "service type")
	flagSet.StringVar(&typeValue, "t", "", "service type")
	flagSet.StringVar(&containerName, "container-name", "", "container name")
	flagSet.StringVar(&containerName, "c", "", "container name")
	flagSet.StringVar(&engine, "engine", "", "engine name: docker|podman")
	flagSet.StringVar(&engine, "E", "", "engine name: docker|podman")
	flagSet.BoolVar(&local, "local", false, "run in local mode")
	flagSet.BoolVar(&local, "l", false, "run in local mode")
	flagSet.StringVar(&storageName, "storage", "", "storage config name")
	flagSet.StringVar(&storageName, "s", "", "storage config name")
	flagSet.StringVar(&envFile, "env", "", "env file path")
	flagSet.StringVar(&envFile, "e", "", "env file path")
	flagSet.StringVar(&moduleConfig, "module-config", "", "module override yaml path")
	flagSet.StringVar(&moduleConfig, "m", "", "module override yaml path")
	flagSet.StringVar(&volumeName, "volume-name", "", "single volume name for type=volume")
	flagSet.StringVar(&volumeName, "n", "", "single volume name for type=volume")
	flagSet.StringVar(&volumeNamesRaw, "volume-names", "", "comma separated volume names for type=volume")
	flagSet.StringVar(&volumeNamesRaw, "N", "", "comma separated volume names for type=volume")
	flagSet.BoolVar(&force, "force", false, "force upload even when delta check matches")
	flagSet.BoolVar(&useChecksum, "use-checksum", false, "enable checksum delta check")
	flagSet.BoolVar(&useFileSizeCheck, "use-file-size-check", false, "enable file-size delta check")
	flagSet.BoolVar(&debugMode, "debug", false, "show debug logs")
	flagSet.BoolVar(&debugMode, "d", false, "show debug logs")

	if err := flagSet.Parse(args[1:]); err != nil {
		return err
	}

	ui.SetDebug(debugMode)
	if local && strings.TrimSpace(engine) != "" {
		return fmt.Errorf("cannot use --local with --engine")
	}
	if strings.EqualFold(strings.TrimSpace(engine), "local") {
		return fmt.Errorf("use --local instead of --engine local")
	}

	volumeNames := splitCSV(volumeNamesRaw)

	outcome, err := a.manager.Backup(ctx, orchestrator.BackupOptions{
		ServiceID:     serviceID,
		Type:          typeValue,
		ContainerName: containerName,
		Engine:        engine,
		Local:         local,
		Force:         force,
		UseChecksum:   useChecksum,
		UseFileSize:   useFileSizeCheck,
		Storage:       storageName,
		EnvFile:       envFile,
		ModuleConfig:  moduleConfig,
		VolumeName:    volumeName,
		VolumeNames:   volumeNames,
	})
	if err != nil {
		return err
	}

	listOutcome, err := a.manager.List(ctx, orchestrator.ListOptions{ServiceID: outcome.ServiceID})
	if err != nil {
		return err
	}
	printBackupObjectsTable(listOutcome.Backups, false)
	return nil
}

func (a App) runList(ctx context.Context, args []string) error {
	serviceID := ""
	flagArgs := make([]string, 0, len(args))
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			flagArgs = append(flagArgs, arg)
			continue
		}
		if serviceID != "" {
			return fmt.Errorf("list accepts at most one <service-id>")
		}
		serviceID = strings.TrimSpace(arg)
	}

	flagSet := flag.NewFlagSet("list", flag.ContinueOnError)
	flagSet.SetOutput(os.Stdout)
	var debugMode bool
	var showObjectKey bool
	var remote bool
	flagSet.BoolVar(&debugMode, "debug", false, "show debug logs")
	flagSet.BoolVar(&debugMode, "d", false, "show debug logs")
	flagSet.BoolVar(&showObjectKey, "show-object-key", false, "show object_key column")
	flagSet.BoolVar(&remote, "remote", false, "list services/backups from remote storage")
	if err := flagSet.Parse(flagArgs); err != nil {
		return err
	}
	ui.SetDebug(debugMode)

	if len(flagSet.Args()) > 0 {
		return fmt.Errorf("list accepts at most one <service-id>")
	}

	outcome, err := a.manager.List(ctx, orchestrator.ListOptions{
		ServiceID: serviceID,
		Remote:    remote,
	})
	if err != nil {
		return err
	}

	if remote {
		if serviceID == "" {
			if len(outcome.RemoteServiceGroups) == 0 {
				a.logger.Warnf("No remote services found")
				return nil
			}
			printRemoteServiceGroups(outcome.RemoteServiceGroups, showObjectKey)
			return nil
		}

		if len(outcome.RemoteBackupGroups) == 0 {
			a.logger.Warnf("No backups found for service %s", serviceID)
			return nil
		}
		printRemoteBackupGroups(outcome.RemoteBackupGroups, showObjectKey)
		return nil
	}

	if serviceID == "" {
		rows := make([][]string, 0, len(outcome.Services))
		for _, service := range outcome.Services {
			storageName := strings.TrimSpace(service.Storage)
			if storageName == "" {
				storageName = "default"
			}
			rows = append(rows, []string{service.Name, service.Type, storageName, service.LastBackupTime})
		}
		ui.PrintSolidTable([]string{"service", "type", "storage", "last_backup"}, rows)
		return nil
	}

	if len(outcome.Backups) == 0 {
		a.logger.Warnf("No backups found for service %s", serviceID)
		return nil
	}

	printBackupObjectsTable(outcome.Backups, showObjectKey)
	return nil
}

func (a App) runRestore(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("restore requires <service-id>")
	}

	serviceID := args[0]
	flagSet := flag.NewFlagSet("restore", flag.ContinueOnError)
	flagSet.SetOutput(os.Stdout)

	var versionValue string
	var applyFile string
	var typeValue string
	var containerName string
	var engine string
	var local bool
	var storageName string
	var envFile string
	var moduleConfig string
	var debugMode bool

	flagSet.StringVar(&versionValue, "version", "latest", "backup version id or latest")
	flagSet.StringVar(&versionValue, "v", "latest", "backup version id or latest")
	flagSet.StringVar(&applyFile, "apply", "", "apply a downloaded backup file")
	flagSet.StringVar(&applyFile, "a", "", "apply a downloaded backup file")
	flagSet.StringVar(&typeValue, "type", "", "service type")
	flagSet.StringVar(&typeValue, "t", "", "service type")
	flagSet.StringVar(&containerName, "container-name", "", "container name")
	flagSet.StringVar(&containerName, "c", "", "container name")
	flagSet.StringVar(&engine, "engine", "", "engine name: docker|podman")
	flagSet.StringVar(&engine, "E", "", "engine name: docker|podman")
	flagSet.BoolVar(&local, "local", false, "run in local mode")
	flagSet.BoolVar(&local, "l", false, "run in local mode")
	flagSet.StringVar(&storageName, "storage", "", "storage config name")
	flagSet.StringVar(&storageName, "s", "", "storage config name")
	flagSet.StringVar(&envFile, "env", "", "env file path")
	flagSet.StringVar(&envFile, "e", "", "env file path")
	flagSet.StringVar(&moduleConfig, "module-config", "", "module override yaml path")
	flagSet.StringVar(&moduleConfig, "m", "", "module override yaml path")
	flagSet.BoolVar(&debugMode, "debug", false, "show debug logs")
	flagSet.BoolVar(&debugMode, "d", false, "show debug logs")

	if err := flagSet.Parse(args[1:]); err != nil {
		return err
	}

	ui.SetDebug(debugMode)
	if local && strings.TrimSpace(engine) != "" {
		return fmt.Errorf("cannot use --local with --engine")
	}
	if strings.EqualFold(strings.TrimSpace(engine), "local") {
		return fmt.Errorf("use --local instead of --engine local")
	}

	if applyFile != "" {
		a.logger.Infof("Applying backup %s to service %s ...", applyFile, serviceID)
		outcome, err := a.manager.RestoreApply(ctx, orchestrator.RestoreApplyOptions{
			ServiceID:     serviceID,
			BackupFile:    applyFile,
			Type:          typeValue,
			ContainerName: containerName,
			Engine:        engine,
			Local:         local,
			Storage:       storageName,
			EnvFile:       envFile,
			ModuleConfig:  moduleConfig,
		})
		if err != nil {
			return err
		}
		if outcome.Guidance != "" {
			a.logger.Warnf(outcome.Guidance)
		}
		a.logger.Successf("Restore apply completed via %s", outcome.Engine)
		return nil
	}

	outcome, err := a.manager.RestoreRetrieve(ctx, orchestrator.RestoreRetrieveOptions{
		ServiceID:     serviceID,
		Version:       versionValue,
		Type:          typeValue,
		ContainerName: containerName,
		Engine:        engine,
		Local:         local,
		Storage:       storageName,
		EnvFile:       envFile,
		ModuleConfig:  moduleConfig,
	})
	if err != nil {
		return err
	}

	a.logger.Infof("Retrieving backup for service %s (Version %s) ...", serviceID, outcome.Version)
	a.logger.Infof("Downloaded backup file to %q", outcome.DownloadedPath)
	a.logger.Warnf(outcome.Guidance)
	return nil
}

func (a App) printBanner() {
	fmt.Println("Copyright (c) rikkaneko <rikkaneko23@gmail.com>")
	fmt.Printf("Sloth CLI (version %s, go %s)\n\n", a.version, runtime.Version())
}

func splitCSV(raw string) []string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}

	values := strings.Split(trimmed, ",")
	out := make([]string, 0, len(values))
	for _, value := range values {
		item := strings.TrimSpace(value)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}

func ExitWithError(err error) {
	if err == nil {
		return
	}
	fmt.Fprintf(os.Stderr, "error: %v\n", err)
	os.Exit(1)
}

func printBackupObjectsTable(backups []orchestrator.BackupObject, showObjectKey bool) {
	headers := []string{"version", "last_modified", "size"}
	rows := make([][]string, 0, len(backups))
	for _, backup := range backups {
		row := []string{
			backup.Version,
			backup.LastModified.Format(time.RFC3339),
			humanReadableBytes(backup.Size),
		}
		if showObjectKey {
			row = append(row, backup.Key)
		}
		rows = append(rows, row)
	}
	if showObjectKey {
		headers = append(headers, "object_key")
	}
	ui.PrintSolidTable(headers, rows)
}

func printRemoteServiceGroups(groups []orchestrator.RemoteServiceGroup, showObjectKey bool) {
	for _, group := range groups {
		fmt.Printf("Storage: %s\n", group.Storage)

		headers := []string{"service", "last_backup"}
		rows := make([][]string, 0, len(group.Rows))
		for _, row := range group.Rows {
			record := []string{row.Service, row.LastBackup.Format(time.RFC3339)}
			if showObjectKey {
				record = append(record, row.ObjectKey)
			}
			rows = append(rows, record)
		}

		if showObjectKey {
			headers = append(headers, "object_key")
		}

		ui.PrintSolidTable(headers, rows)
	}
}

func printRemoteBackupGroups(groups []orchestrator.RemoteBackupGroup, showObjectKey bool) {
	for _, group := range groups {
		fmt.Printf("Storage: %s\n", group.Storage)
		printBackupObjectsTable(group.Backups, showObjectKey)
	}
}

func humanReadableBytes(size int64) string {
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

func resolveDisplayVersion(version string) string {
	normalized := strings.TrimSpace(version)
	if normalized != "" && normalized != "dev" {
		return normalized
	}

	info, ok := debug.ReadBuildInfo()
	if ok {
		buildVersion := strings.TrimSpace(info.Main.Version)
		if buildVersion != "" && buildVersion != "(devel)" {
			return buildVersion
		}
		revision := ""
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" {
				revision = strings.TrimSpace(setting.Value)
				break
			}
		}
		if revision != "" {
			if len(revision) > 12 {
				return revision[:12]
			}
			return revision
		}
	}

	if normalized == "" {
		return "dev"
	}
	return normalized
}
