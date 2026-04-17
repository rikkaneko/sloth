package modules

import (
	"os"
	"path/filepath"
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
