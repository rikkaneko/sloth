package cli

import (
	"fmt"
	"sort"
	"strings"

	"sloth/internal/config"
	"sloth/internal/modules"
)

var supportedEngines = []string{"docker", "podman"}

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
		"    sloth [global-options] <command> [<args>]",
		"    sloth <command> [<args>]",
		"    sloth help [backup|restore|list]",
		"",
		"COMMANDS",
		"    backup   create and upload a backup artifact",
		"    restore  retrieve or apply a backup artifact",
		"    list     list configured services or backup versions",
		"    help     show help for root or subcommand",
		"",
		"GLOBAL OPTIONS",
		"    -h, --help   show this help message",
		"    -C, --config-home <dir>  config directory (default: ~/.config/sloth)",
		"    -S, --sudo               prepend privileged program for container runtime commands",
		"    --sudo-program <cmd>     privileged program name (default: sudo)",
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
		"    sloth [global-options] backup <service-id> [options]",
		"    sloth backup <service-id> [options]",
		"",
		"GLOBAL OPTIONS",
		"    -h, --help   show this help message",
		"    -C, --config-home <dir>  config directory (default: ~/.config/sloth)",
		"    -S, --sudo               prepend privileged program for container runtime commands",
		"    --sudo-program <cmd>     privileged program name (default: sudo)",
		"",
		"OPTIONS",
		fmt.Sprintf("    -t, --type <service-type>        service type (available: %s)", serviceTypes),
		"    -c, --container-name <name>      target container name",
		fmt.Sprintf("    -E, --engine <container-engine>  engine name (available: %s)", engines),
		"    -l, --local                      run in local mode",
		fmt.Sprintf("    -s, --storage <storage-name>     storage name (available: %s)", storages),
		"    -e, --env <env-file>             env file path (default: .env)",
		"    -m, --module-config <yaml-path>  module override yaml path",
		"    -n, --volume-name <volume-name>  single volume name for type=volume",
		"    -N, --volume-names <n1,n2>       comma-separated volume names for type=volume",
		"    -k, --keep                       keep generated backup file in current directory",
		"    --force                          force upload regardless of delta checks",
		"    --dry-run                        dry run upload and skip final put call",
		"    --use-checksum                   enable checksum delta check",
		"    --use-file-size-check            enable file-size delta check",
		"    -d, --debug                      show debug logs",
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
		"    sloth [global-options] restore <service-id> [--version <version|latest>] [options]",
		"    sloth [global-options] restore <service-id> --apply <backup-file> [options]",
		"    sloth restore <service-id> [--version <version|latest>] [options]",
		"    sloth restore <service-id> --apply <backup-file> [options]",
		"",
		"GLOBAL OPTIONS",
		"    -h, --help   show this help message",
		"    -C, --config-home <dir>  config directory (default: ~/.config/sloth)",
		"    -S, --sudo               prepend privileged program for container runtime commands",
		"    --sudo-program <cmd>     privileged program name (default: sudo)",
		"",
		"OPTIONS",
		"    -v, --version <version|latest>   backup version to retrieve (default: latest)",
		"    -a, --apply <backup-file>        apply a downloaded backup file",
		fmt.Sprintf("    -t, --type <service-type>        service type (available: %s)", serviceTypes),
		"    -c, --container-name <name>      target container name",
		fmt.Sprintf("    -E, --engine <container-engine>  engine name (available: %s)", engines),
		"    -l, --local                      run in local mode",
		fmt.Sprintf("    -s, --storage <storage-name>     storage name (available: %s)", storages),
		"    -e, --env <env-file>             env file path (default: .env)",
		"    -m, --module-config <yaml-path>  module override yaml path",
		"    -d, --debug                      show debug logs",
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
		"    sloth [global-options] list [--remote] [<service-id>]",
		"    sloth list [--remote] [<service-id>]",
		"",
		"GLOBAL OPTIONS",
		"    -h, --help   show this help message",
		"    -C, --config-home <dir>  config directory (default: ~/.config/sloth)",
		"    -S, --sudo               prepend privileged program for container runtime commands",
		"    --sudo-program <cmd>     privileged program name (default: sudo)",
		"",
		"OPTIONS",
		"    -d, --debug  show debug logs",
		"    --remote  list from remote storage instead of local service config",
		"    --show-object-key  show object_key column for service backup list",
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
