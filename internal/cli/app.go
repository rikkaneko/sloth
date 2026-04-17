package cli

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"sloth/internal/orchestrator"
	"sloth/internal/ui"
)

type App struct {
	manager orchestrator.Manager
	logger  ui.Logger
	version string
}

func NewApp(version string) App {
	normalizedVersion := strings.TrimSpace(version)
	if normalizedVersion == "" {
		normalizedVersion = "dev"
	}

	return App{
		manager: orchestrator.NewManager(),
		logger:  ui.NewLogger(),
		version: normalizedVersion,
	}
}

func (a App) Run(ctx context.Context, args []string) error {
	a.printBanner()

	if len(args) == 0 {
		a.printUsage()
		return nil
	}

	switch args[0] {
	case "backup":
		return a.runBackup(ctx, args[1:])
	case "list":
		return a.runList(ctx, args[1:])
	case "restore":
		return a.runRestore(ctx, args[1:])
	case "help", "--help", "-h":
		a.printUsage()
		return nil
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

	typeValue := flagSet.String("type", "", "service type")
	containerName := flagSet.String("container-name", "", "container name")
	engine := flagSet.String("engine", "", "engine name: docker|podman|local")
	storageName := flagSet.String("storage", "", "storage config name")
	envFile := flagSet.String("env", "", "env file path")
	moduleConfig := flagSet.String("module-config", "", "module override yaml path")
	volumeName := flagSet.String("volume-name", "", "single volume name for type=volume")
	volumeNamesRaw := flagSet.String("volume-names", "", "comma separated volume names for type=volume")

	if err := flagSet.Parse(args[1:]); err != nil {
		return err
	}

	volumeNames := splitCSV(*volumeNamesRaw)

	a.logger.Infof("Backing up service %s ...", serviceID)
	outcome, err := a.manager.Backup(ctx, orchestrator.BackupOptions{
		ServiceID:     serviceID,
		Type:          *typeValue,
		ContainerName: *containerName,
		Engine:        *engine,
		Storage:       *storageName,
		EnvFile:       *envFile,
		ModuleConfig:  *moduleConfig,
		VolumeName:    *volumeName,
		VolumeNames:   volumeNames,
	})
	if err != nil {
		return err
	}

	a.logger.Successf("Backup uploaded")
	fmt.Printf("service=%s engine=%s storage=%s version=%s\n", outcome.ServiceID, outcome.Engine, outcome.StorageName, outcome.Version)
	fmt.Printf("object=%s\n", outcome.ObjectKey)
	return nil
}

func (a App) runList(ctx context.Context, args []string) error {
	serviceID := ""
	if len(args) > 0 {
		serviceID = args[0]
	}

	outcome, err := a.manager.List(ctx, serviceID)
	if err != nil {
		return err
	}

	if serviceID == "" {
		rows := make([][]string, 0, len(outcome.Services))
		for _, service := range outcome.Services {
			rows = append(rows, []string{service.Name, service.Type, service.ContainerName, service.Storage, service.LastBackupTime})
		}
		ui.PrintSolidTable([]string{"service", "type", "container", "storage", "last_backup"}, rows)
		return nil
	}

	if len(outcome.Backups) == 0 {
		a.logger.Warnf("No backups found for service %s", serviceID)
		return nil
	}

	rows := make([][]string, 0, len(outcome.Backups))
	for _, backup := range outcome.Backups {
		rows = append(rows, []string{
			backup.Version,
			backup.LastModified.Format(time.RFC3339),
			fmt.Sprintf("%d", backup.Size),
			backup.Key,
		})
	}

	ui.PrintTable([]string{"version", "last_modified", "size", "object_key"}, rows)
	return nil
}

func (a App) runRestore(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("restore requires <service-id>")
	}

	serviceID := args[0]
	flagSet := flag.NewFlagSet("restore", flag.ContinueOnError)
	flagSet.SetOutput(os.Stdout)

	versionValue := flagSet.String("version", "latest", "backup version id or latest")
	applyFile := flagSet.String("apply", "", "apply a downloaded backup file")
	typeValue := flagSet.String("type", "", "service type")
	containerName := flagSet.String("container-name", "", "container name")
	engine := flagSet.String("engine", "", "engine name: docker|podman|local")
	storageName := flagSet.String("storage", "", "storage config name")
	envFile := flagSet.String("env", "", "env file path")
	moduleConfig := flagSet.String("module-config", "", "module override yaml path")

	if err := flagSet.Parse(args[1:]); err != nil {
		return err
	}

	if *applyFile != "" {
		a.logger.Infof("Applying backup %s to service %s ...", *applyFile, serviceID)
		outcome, err := a.manager.RestoreApply(ctx, orchestrator.RestoreApplyOptions{
			ServiceID:     serviceID,
			BackupFile:    *applyFile,
			Type:          *typeValue,
			ContainerName: *containerName,
			Engine:        *engine,
			Storage:       *storageName,
			EnvFile:       *envFile,
			ModuleConfig:  *moduleConfig,
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

	a.logger.Infof("Retrieving backup for service %s ...", serviceID)
	outcome, err := a.manager.RestoreRetrieve(ctx, orchestrator.RestoreRetrieveOptions{
		ServiceID:     serviceID,
		Version:       *versionValue,
		Type:          *typeValue,
		ContainerName: *containerName,
		Engine:        *engine,
		Storage:       *storageName,
		EnvFile:       *envFile,
		ModuleConfig:  *moduleConfig,
	})
	if err != nil {
		return err
	}

	a.logger.Successf("Backup retrieved")
	fmt.Printf("file=%s\n", outcome.DownloadedPath)
	fmt.Printf("object=%s version=%s\n", outcome.ObjectKey, outcome.Version)
	a.logger.Warnf(outcome.Guidance)
	return nil
}

func (a App) printUsage() {
	usage := []string{
		"Commands:",
		"  sloth backup <service-id> [--type --container-name --engine --storage --env --module-config]",
		"  sloth list [<service-id>]",
		"  sloth restore <service-id> [--version <id|latest>] [--type --container-name --engine --storage --env]",
		"  sloth restore <service-id> --apply <backup-file> [--type --container-name --engine --storage --env]",
		"",
	}
	fmt.Println(strings.Join(usage, "\n"))
}

func (a App) printBanner() {
	fmt.Printf("sloth-cli %s\n\n", a.version)
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
