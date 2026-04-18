# Sloth
Cloud backup tool for containerized and local services, built as a standalone Go CLI.

## Installation
1. Ensure Go is installed.
2. Clone the repository.
3. Install dependencies:
```bash
go mod tidy
```
4. Build the binary:
```bash
go build -o bin/sloth ./cmd/sloth
```

Set build-time version (optional):
```bash
go build -ldflags "-X main.Version=1.0.0" -o bin/sloth ./cmd/sloth
```

## Environment Configuration
Sloth reads configuration from:
- Main config: `~/.config/sloth/main.yaml`
- Service config: `~/.config/sloth/service.yaml`
- Fallback service config: `./.sloth.yaml` (used only when home service config does not exist)
- Environment file: `.env` by default, or `--env <path>`

Main config example:
```yaml
storage:
  - name: default
    type: s3
    endpoint: https://s3-endpoint.example.com
    region: auto
    bucket: backup-bucket
    access_key_id: your-key
    secret_access_key: your-secret
    use_native_object_versioning: false
    base_path: /backup
common:
  file_delta_check: checksum # checksum | file_size
```

Service config example:
```yaml
service:
  - name: app-db
    container_name: app-db-container
    type: mysql
    storage: default
    engine: docker
    env_file: .env
```

## Features
- Backup and restore modules for:
  - `mariadb`, `mysql`, `pgsql`
  - Directus schema snapshot
  - RabbitMQ definitions
  - Redis snapshot backup and guided restore apply
  - Docker/Podman volume archive backup/restore
- Local mode support for: `mariadb`, `mysql`, `pgsql`, `directus`, `rabbitmq`, `redis`
- S3-compatible storage backend (AWS S3, MinIO, Garage, Backblaze B2, etc.)
- Native object versioning mode and sloth-managed incremental versioning mode
- Engine auto-detection (`podman` then `docker`) by `container_name`, or by `<service-id>` when container name is omitted
- Automatic env loading with `${VAR}` interpolation
- Sectioned `--help` output for root and subcommands with dynamic available values for `--type`, `--engine`, and `--storage`
- Short and long flag pairs for backup/restore/list (`-t/-c/-E/-l/-s/-e/-m/-n/-N/-v/-a/-d`)
- Unified info/debug logging (`--debug`) including external command output and S3 API call summaries
- Backup delta-check strategies: checksum (default) or file-size (`common.file_delta_check` and backup flags)
- `backup --force` to always upload a new backup version regardless of delta-check match
- Built-in module templates embedded from per-service YAML files under `internal/modules/yaml/*.yaml`
- Colorized command output and solid-border table-formatted backup/service listings

## Usage Examples
Backup a service:
```bash
sloth backup app-db
```

Backup with explicit file-size delta check:
```bash
sloth backup app-db --use-file-size-check
```

Force upload regardless of delta checks:
```bash
sloth backup app-db --force
```

Create a new local service entry and backup immediately:
```bash
sloth backup app-db --type mysql --container-name app-db-container --engine docker
```

Backup in local mode:
```bash
sloth backup app-db --type mysql --local
```

List configured services:
```bash
sloth list
```

List backups for a service:
```bash
sloth list app-db
```

List backups with object key column:
```bash
sloth list app-db --show-object-key
```

Restore stage 1 (retrieve backup):
```bash
sloth restore app-db --version latest
```

Restore stage 2 (apply local backup file):
```bash
sloth restore app-db --apply ./app-db-backup-20260417-120000-3.sql
```

For Redis local restore target path override:
```bash
REDIS_RDB_PATH=/var/lib/redis/dump.rdb sloth restore redis-service --local --apply ./redis-service-backup-20260417-120000-latest.rdb
```

## Documentation
- Commands and help usage: `docs/commands.md`
- Service env variables and embedded module commands: `docs/service-modules.md`
- Module authoring and YAML override guide: `docs/module-authoring.md`
- Architecture overview: `docs/architecture.md`

## License
MIT
