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
- Main config: `<config-home>/main.yaml`
- Service config: `<config-home>/service.yaml`
- Environment file: `.env` by default, or `--env <path>`

Default config home is `~/.config/sloth`. Override it globally with `--config-home <dir>` (or `-C <dir>`).
Global options (`--config-home`, `--sudo`, `--sudo-program`) can be used before or after subcommands.

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
  - Relational databases (MariaDB, MySQL and PostgreSQL)
  - Directus schema snapshot
  - RabbitMQ definitions
  - Redis snapshot backup and guided restore
  - Docker/Podman volume archive
- Local mode support for: `mariadb`, `mysql`, `pgsql`, `directus`, `rabbitmq`, `redis`
- S3-compatible storage backend (AWS S3, MinIO, Garage, Backblaze B2, etc.)
- Native object versioning mode and sloth-managed incremental versioning mode
- Container engine auto-detection on `container_name` or `<service-id>`.
- Automatic environment variable loading with `${VAR}` interpolation
- List remote service backups
- Backup delta-check strategies: checksum (default) or file-size
- Backup keep/dry-run options: `-k|--keep`, `--dry-run`

## Usage Examples
### Create a backup for a MySQL database deployed in a container

```bash
sloth backup app-db -t mysql
```

This command will also create a service config entry in `<config-home>/service.yaml`.

### Create a backup on existing service config:

```bash
sloth backup app-db
```

### Backup with explicit file-size delta check:

```bash
sloth backup app-db --use-file-size-check
```

### Backup and keep generated artifact in current directory

```bash
sloth backup app-db --keep
```

### Dry run backup upload (skip final put/upload call)

```bash
sloth backup app-db --dry-run
```

### Use a custom config home globally

```bash
sloth -C /tmp/sloth-config backup app-db -t mysql
```

### Run backup/restore-apply container commands with privilege elevation

```bash
sloth -S backup app-db
sloth -S --sudo-program doas restore app-db --apply ./app-db-backup.sql
```

### Create a backup for a MySQL database running in the host

```bash
sloth backup app-db --type mysql --local
```

### List available service config in local

```bash
sloth list
```

### List available backup version for a service

```bash
sloth list app-db
```

### List available service config in remote storage

```bash
sloth list --remote
```

### List available backup version for a service (remote only)

```bash
sloth list --remote app-db
```

### Restore a backup (Stage 1)

Retrieve the latest version of the backup from remote storage

```bash
sloth restore app-db
```

### Restore a backup (Stage 2)

> [!WARNING]
> Before restoring a backup, you need to stop and remove the targeted containers. Then, recreate the container with its dependencies.

Apply the backup to the target service

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
