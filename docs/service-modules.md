# Service Module Command and Environment Reference

This document maps each built-in service module to:
- Official environment variable names expected by the upstream Docker image or service tooling.
- The exact backup/restore command templates currently embedded in `internal/modules/yaml/*.yaml`.

## mysql
Official image variables:
- `MYSQL_DATABASE`
- `MYSQL_USER`
- `MYSQL_PASSWORD`
- `MYSQL_ROOT_PASSWORD` (fallback)

Embedded backup command:
```bash
if [ -n "${MYSQL_DATABASE}" ]; then mysqldump -u"${MYSQL_USER:-root}" -p"${MYSQL_PASSWORD:-${MYSQL_ROOT_PASSWORD}}" --single-transaction "${MYSQL_DATABASE}" > "{{target_file}}"; else mysqldump -u"${MYSQL_USER:-root}" -p"${MYSQL_PASSWORD:-${MYSQL_ROOT_PASSWORD}}" --single-transaction --all-databases > "{{target_file}}"; fi
```

Embedded restore command:
```bash
if [ -n "${MYSQL_DATABASE}" ]; then mysql -u"${MYSQL_USER:-root}" -p"${MYSQL_PASSWORD:-${MYSQL_ROOT_PASSWORD}}" "${MYSQL_DATABASE}" < "{{backup_file}}"; else mysql -u"${MYSQL_USER:-root}" -p"${MYSQL_PASSWORD:-${MYSQL_ROOT_PASSWORD}}" < "{{backup_file}}"; fi
```

## mariadb
Official image variables:
- `MARIADB_DATABASE`
- `MARIADB_USER`
- `MARIADB_PASSWORD`
- `MARIADB_ROOT_PASSWORD` (fallback)

Embedded backup command:
```bash
if [ -n "${MARIADB_DATABASE}" ]; then mariadb-dump -u"${MARIADB_USER:-root}" -p"${MARIADB_PASSWORD:-${MARIADB_ROOT_PASSWORD}}" --single-transaction "${MARIADB_DATABASE}" > "{{target_file}}"; else mariadb-dump -u"${MARIADB_USER:-root}" -p"${MARIADB_PASSWORD:-${MARIADB_ROOT_PASSWORD}}" --single-transaction --all-databases > "{{target_file}}"; fi
```

Embedded restore command:
```bash
if [ -n "${MARIADB_DATABASE}" ]; then mariadb -u"${MARIADB_USER:-root}" -p"${MARIADB_PASSWORD:-${MARIADB_ROOT_PASSWORD}}" "${MARIADB_DATABASE}" < "{{backup_file}}"; else mariadb -u"${MARIADB_USER:-root}" -p"${MARIADB_PASSWORD:-${MARIADB_ROOT_PASSWORD}}" < "{{backup_file}}"; fi
```

## pgsql (PostgreSQL)
Official image variables:
- `POSTGRES_DB`
- `POSTGRES_USER`
- `POSTGRES_PASSWORD`
- `POSTGRES_PORT` (optional, defaults to `5432`)

Embedded backup command:
```bash
if [ -n "${POSTGRES_DB}" ]; then PGPASSWORD="${POSTGRES_PASSWORD}" pg_dump -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB}" -h localhost -p "${POSTGRES_PORT:-5432}" > "{{target_file}}"; else PGPASSWORD="${POSTGRES_PASSWORD}" pg_dumpall -U "${POSTGRES_USER:-postgres}" -h localhost -p "${POSTGRES_PORT:-5432}" > "{{target_file}}"; fi
```

Embedded restore command:
```bash
if [ -n "${POSTGRES_DB}" ]; then PGPASSWORD="${POSTGRES_PASSWORD}" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB}" -h localhost -p "${POSTGRES_PORT:-5432}" < "{{backup_file}}"; else PGPASSWORD="${POSTGRES_PASSWORD}" psql -U "${POSTGRES_USER:-postgres}" -h localhost -p "${POSTGRES_PORT:-5432}" < "{{backup_file}}"; fi
```

## directus
Embedded backup command:
```bash
npx directus schema snapshot "{{target_file}}"
```

Embedded restore command:
```bash
npx directus schema apply "{{backup_file}}"
```

## rabbitmq

Embedded backup command:
```bash
rabbitmqctl export_definitions "{{target_file}}"
```

Embedded restore command:
```bash
rabbitmqctl import_definitions "{{backup_file}}"
```

## redis
No DB credential variable is required by the built-in module command.

Embedded backup command:
```bash
redis-cli --rdb "{{target_file}}"
```

Restore behavior:
- Container mode copies backup file to `/data/dump.rdb` by default.
- Local mode copies backup file to `REDIS_RDB_PATH` if set, otherwise module/default path.

## volume
No service credential variables are required.

Backup and restore commands are constructed at runtime in `internal/modules/volume_module.go` and run through the selected engine (`docker` or `podman`).
