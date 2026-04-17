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
- Engine auto-detection (`podman` then `docker`) by `container_name` when engine is omitted
- Automatic env loading with `${VAR}` interpolation
- Built-in module templates embedded from per-service YAML files under `internal/modules/yaml/*.yaml`
- Colorized command output and table-formatted backup/service listings

## Usage Examples
Backup a service:
```bash
sloth backup app-db
```

Create a new local service entry and backup immediately:
```bash
sloth backup app-db --type mysql --container-name app-db-container --engine docker
```

List configured services:
```bash
sloth list
```

List backups for a service:
```bash
sloth list app-db
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
REDIS_RDB_PATH=/var/lib/redis/dump.rdb sloth restore redis-service --engine local --apply ./redis-service-backup-20260417-120000-latest.rdb
```

## License
MIT
