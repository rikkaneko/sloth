package modules

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRegistryResolvesBuiltinModule(t *testing.T) {
	registry := NewRegistry()
	module, err := registry.Resolve("mariadb", "")
	if err != nil {
		t.Fatalf("resolve module: %v", err)
	}

	if module.Type() != "mariadb" {
		t.Fatalf("expected mariadb module, got %s", module.Type())
	}
	if !module.SupportsLocal() {
		t.Fatalf("expected mariadb to support local mode")
	}
}

func TestRegistryAppliesModuleOverride(t *testing.T) {
	directory := t.TempDir()
	overridePath := filepath.Join(directory, "module.yaml")
	content := "modules:\n  directus:\n    backup:\n      command:\n        - npx directus schema snapshot \"{{target_file}}\" --yes\n"
	if err := os.WriteFile(overridePath, []byte(content), 0o600); err != nil {
		t.Fatalf("write override file: %v", err)
	}

	registry := NewRegistry()
	module, err := registry.Resolve("directus", overridePath)
	if err != nil {
		t.Fatalf("resolve module with override: %v", err)
	}

	commandModule, ok := module.(CommandModule)
	if !ok {
		t.Fatalf("expected directus command module")
	}

	if len(commandModule.definition.Backup.Command) != 1 {
		t.Fatalf("expected one override command")
	}
	if commandModule.definition.Backup.Command[0] != "npx directus schema snapshot \"{{target_file}}\" --yes" {
		t.Fatalf("override command not applied")
	}
}

func TestRegistryRejectsUnsupportedType(t *testing.T) {
	registry := NewRegistry()
	_, err := registry.Resolve("unknown", "")
	if err == nil {
		t.Fatalf("expected error for unsupported type")
	}
}

func TestAvailableServiceTypesIncludesVolume(t *testing.T) {
	types, err := AvailableServiceTypes()
	if err != nil {
		t.Fatalf("available service types: %v", err)
	}

	for _, expected := range []string{"directus", "mariadb", "mysql", "pgsql", "rabbitmq", "redis", "volume"} {
		if !contains(types, expected) {
			t.Fatalf("expected %q in available service types: %v", expected, types)
		}
	}
}

func TestBuiltInDatabaseModulesUseOfficialImageEnvVars(t *testing.T) {
	definitions, err := builtInDefinitions()
	if err != nil {
		t.Fatalf("load built-in definitions: %v", err)
	}

	mysqlCommands := strings.Join(append([]string{}, definitions["mysql"].definition.Backup.Command...), "\n") +
		"\n" + strings.Join(definitions["mysql"].definition.Restore.Command, "\n")
	assertContainsAll(t, mysqlCommands, []string{"MYSQL_DATABASE", "MYSQL_USER", "MYSQL_PASSWORD", "MYSQL_ROOT_PASSWORD"})
	assertContainsNone(t, mysqlCommands, []string{"${DB_NAME}", "${DB_USER}", "${DB_PASS}"})

	mariaDBCommands := strings.Join(append([]string{}, definitions["mariadb"].definition.Backup.Command...), "\n") +
		"\n" + strings.Join(definitions["mariadb"].definition.Restore.Command, "\n")
	assertContainsAll(t, mariaDBCommands, []string{"MARIADB_DATABASE", "MARIADB_USER", "MARIADB_PASSWORD", "MARIADB_ROOT_PASSWORD"})
	assertContainsNone(t, mariaDBCommands, []string{"${DB_NAME}", "${DB_USER}", "${DB_PASS}"})

	pgCommands := strings.Join(append([]string{}, definitions["pgsql"].definition.Backup.Command...), "\n") +
		"\n" + strings.Join(definitions["pgsql"].definition.Restore.Command, "\n")
	assertContainsAll(t, pgCommands, []string{"POSTGRES_DB", "POSTGRES_USER", "POSTGRES_PASSWORD"})
	assertContainsNone(t, pgCommands, []string{"${DB_NAME}", "${DB_USER}", "${DB_PASS}"})
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func assertContainsAll(t *testing.T, input string, expected []string) {
	t.Helper()
	for _, value := range expected {
		if !strings.Contains(input, value) {
			t.Fatalf("expected command text to contain %q\n%s", value, input)
		}
	}
}

func assertContainsNone(t *testing.T, input string, forbidden []string) {
	t.Helper()
	for _, value := range forbidden {
		if strings.Contains(input, value) {
			t.Fatalf("expected command text to not contain %q\n%s", value, input)
		}
	}
}
