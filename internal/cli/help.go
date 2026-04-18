package cli

import (
	"fmt"
	"sort"
	"strings"

	"sloth/internal/config"
	"sloth/internal/modules"
)

var supportedEngines = []string{"docker", "podman", "local"}

type helpCatalog struct {
	serviceTypes  []string
	engines       []string
	storages      []string
	storageSource string
	storageErr    string
}

func isRootHelpArg(arg string) bool {
	return arg == "--help" || arg == "-h"
}

func hasHelpFlag(args []string) bool {
	for _, arg := range args {
		if isRootHelpArg(arg) {
			return true
		}
	}
	return false
}

func (a App) printCommandHelp(command string) error {
	switch command {
	case "", "help", "--help", "-h":
		a.printRootHelp()
		return nil
	case "backup":
		a.printBackupHelp()
		return nil
	case "restore":
		a.printRestoreHelp()
		return nil
	case "list":
		a.printListHelp()
		return nil
	default:
		return fmt.Errorf("unknown help topic %q", command)
	}
}

func (a App) printRootHelp() {
	catalog := buildHelpCatalog()
	lines := []string{
		"NAME",
		"    sloth - cloud backup tool for containerized and local services",
		"",
		"SYNOPSIS",
		"    sloth <command> [<args>]",
		"    sloth help [backup|restore|list]",
		"",
		"COMMANDS",
		"    backup   create and upload a backup artifact",
		"    restore  retrieve or apply a backup artifact",
		"    list     list configured services or backup versions",
		"    help     show help for root or subcommand",
		"",
		"OPTIONS",
		"    -h, --help   show this help message",
		"",
	}
	lines = append(lines, formatDefaultsSection(catalog)...)
	fmt.Println(strings.Join(lines, "\n"))
}

func (a App) printBackupHelp() {
	catalog := buildHelpCatalog()
	serviceTypes := joinValues(catalog.serviceTypes)
	engines := joinValues(catalog.engines)
	storages := joinValues(catalog.storages)

	lines := []string{
		"NAME",
		"    sloth backup - create backup artifact and upload to storage",
		"",
		"SYNOPSIS",
		"    sloth backup <service-id> [options]",
		"",
		"OPTIONS",
		fmt.Sprintf("    --type <service-type>            service type (available: %s)", serviceTypes),
		"    --container-name <name>          target container name",
		fmt.Sprintf("    --engine <container-engine>      engine name (available: %s)", engines),
		fmt.Sprintf("    --storage <storage-name>         storage name (available: %s)", storages),
		"    --env <env-file>                 env file path (default: .env)",
		"    --module-config <yaml-path>      module override yaml path",
		"    --volume-name <volume-name>      single volume name for type=volume",
		"    --volume-names <n1,n2>           comma-separated volume names for type=volume",
		"    -h, --help                       show this help message",
		"",
	}
	lines = append(lines, formatDefaultsSection(catalog)...)
	fmt.Println(strings.Join(lines, "\n"))
}

func (a App) printRestoreHelp() {
	catalog := buildHelpCatalog()
	serviceTypes := joinValues(catalog.serviceTypes)
	engines := joinValues(catalog.engines)
	storages := joinValues(catalog.storages)

	lines := []string{
		"NAME",
		"    sloth restore - retrieve or apply backups",
		"",
		"SYNOPSIS",
		"    sloth restore <service-id> [--version <version|latest>] [options]",
		"    sloth restore <service-id> --apply <backup-file> [options]",
		"",
		"OPTIONS",
		"    --version <version|latest>       backup version to retrieve (default: latest)",
		"    --apply <backup-file>            apply a downloaded backup file",
		fmt.Sprintf("    --type <service-type>            service type (available: %s)", serviceTypes),
		"    --container-name <name>          target container name",
		fmt.Sprintf("    --engine <container-engine>      engine name (available: %s)", engines),
		fmt.Sprintf("    --storage <storage-name>         storage name (available: %s)", storages),
		"    --env <env-file>                 env file path (default: .env)",
		"    --module-config <yaml-path>      module override yaml path",
		"    -h, --help                       show this help message",
		"",
	}
	lines = append(lines, formatDefaultsSection(catalog)...)
	fmt.Println(strings.Join(lines, "\n"))
}

func (a App) printListHelp() {
	catalog := buildHelpCatalog()
	lines := []string{
		"NAME",
		"    sloth list - list configured services or backup versions",
		"",
		"SYNOPSIS",
		"    sloth list [<service-id>]",
		"",
		"OPTIONS",
		"    -h, --help   show this help message",
		"",
	}
	lines = append(lines, formatDefaultsSection(catalog)...)
	fmt.Println(strings.Join(lines, "\n"))
}

func buildHelpCatalog() helpCatalog {
	serviceTypes, err := modules.AvailableServiceTypes()
	if err != nil {
		serviceTypes = []string{"volume"}
	}

	storages, source, storageErr := loadStorageNames()

	engines := append([]string{}, supportedEngines...)

	return helpCatalog{
		serviceTypes:  serviceTypes,
		engines:       engines,
		storages:      storages,
		storageSource: source,
		storageErr:    storageErr,
	}
}

func loadStorageNames() ([]string, string, string) {
	mainConfig, source, err := config.LoadMainConfig()
	if err != nil {
		return nil, "", summarizeError(err)
	}

	names := make([]string, 0, len(mainConfig.Storage))
	seen := map[string]struct{}{}
	for _, storageConfig := range mainConfig.Storage {
		name := strings.TrimSpace(storageConfig.Name)
		if name == "" {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	sort.Strings(names)
	return names, source, ""
}

func formatDefaultsSection(catalog helpCatalog) []string {
	lines := []string{
		"Default compiled-in and discovered parameters:",
		fmt.Sprintf("\tAvailable service types: %s", joinValues(catalog.serviceTypes)),
		fmt.Sprintf("\tAvailable container engines: %s", joinValues(catalog.engines)),
	}

	if catalog.storageErr != "" {
		lines = append(lines, fmt.Sprintf("\tAvailable storage names: unavailable (%s)", catalog.storageErr))
		return lines
	}

	if len(catalog.storages) == 0 {
		lines = append(lines, "\tAvailable storage names: none configured")
		return lines
	}

	if catalog.storageSource == "" {
		lines = append(lines, fmt.Sprintf("\tAvailable storage names: %s", joinValues(catalog.storages)))
		return lines
	}

	lines = append(lines, fmt.Sprintf("\tAvailable storage names (from %s): %s", catalog.storageSource, joinValues(catalog.storages)))
	return lines
}

func joinValues(values []string) string {
	if len(values) == 0 {
		return "none"
	}
	return strings.Join(values, ", ")
}

func summarizeError(err error) string {
	message := strings.TrimSpace(err.Error())
	if message == "" {
		return "unknown error"
	}
	if len(message) > 120 {
		return message[:117] + "..."
	}
	return message
}
