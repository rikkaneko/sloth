# Module Authoring and Override Guide

## Add a New Service Module (YAML-First)
1. Create `internal/modules/yaml/<service-type>.yaml`.
2. Define required fields:
   - `artifact_ext`
   - `supports_local`
   - `backup.command`
   - `backup.target_file`
   - `restore.backup_file.to_container` (optional but recommended)
   - `restore.command`
3. Use `{{target_file}}` and `{{backup_file}}` placeholders where appropriate.
4. Keep command steps shell-safe and deterministic.

Minimal template:
```yaml
artifact_ext: sql
supports_local: true
backup:
  command:
    - your-backup-command > "{{target_file}}"
  target_file: /tmp/sloth-your-service-backup.sql
restore:
  backup_file:
    to_container: /tmp/sloth-your-service-restore.sql
  command:
    - your-restore-command < "{{backup_file}}"
```

## When to Implement a Go Module Instead of YAML
Use a custom Go module when restore or backup requires non-trivial behavior:
- Multi-step host/container file choreography beyond simple command execution.
- Conditional runtime behavior that depends on engine mode.
- Custom operator guidance or path resolution rules.

Example:
- `redis` uses a custom Go module (`internal/modules/redis_module.go`) to support local restore copy logic and guidance messages.

## Override Built-in Module Commands
You can override built-in YAML module definitions with a module override file.

Override file example (`./module-override.yaml`):
```yaml
modules:
  mysql:
    backup:
      command:
        - mysqldump -u"${MYSQL_USER}" -p"${MYSQL_PASSWORD}" "${MYSQL_DATABASE}" > "{{target_file}}"
```

Apply override via service config:
```yaml
service:
  - name: app-db
    type: mysql
    container_name: app-db
    module_config: ./module-override.yaml
```

Or apply override at runtime:
```bash
sloth backup app-db --module-config ./module-override.yaml
sloth restore app-db --module-config ./module-override.yaml --version latest
```

Merge behavior:
- Only fields present in the override are replaced.
- Omitted fields keep built-in defaults.

## Local Service Config Behavior
Service config path is at `<config-home>/service.yaml`

Default config home is `~/.config/sloth`; override with global CLI option `--config-home` (or `-C`).

## New Module Checklist
- Add YAML in `internal/modules/yaml/`.
- Ensure command env vars match official service image/tool names.
- Run `go test ./...`.
- Update:
  - `docs/service-modules.md`
  - `docs/commands.md` (if CLI surface changed)
  - `README.md` (if user-facing capability changed)
