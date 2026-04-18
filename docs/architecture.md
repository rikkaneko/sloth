# Sloth Architecture

## Overview
Sloth is a modular Go CLI organized around three runtime contracts:
- `Engine`: execute commands and copy files for `docker`, `podman`, or `local`.
- `Module`: service-specific backup and restore behavior.
- `Provider`: object storage interactions for S3-compatible backends.

## Core Packages
- `internal/cli`: command parsing and user-facing output.
- `internal/orchestrator`: backup/list/restore workflow orchestration.
- `internal/config`: YAML loading, precedence handling, and validation.
- `internal/env`: `.env` parser with `${VAR}` interpolation.
- `internal/container`: engine detection and execution wrappers.
- `internal/modules`: built-in module definitions and optional YAML overrides.
  Built-in command templates are embedded from per-service files in `internal/modules/yaml/*.yaml`.
- `internal/storage/s3`: S3 object operations (`put/get/list/list versions`).
- `internal/versioning`: non-native incrementing version logic.

## Backup Flow
1. Resolve service definition from home service config or fallback local config.
2. Resolve runtime (`--local`, `--engine`, configured engine, or autodetect by container name/service-id).
3. Resolve module and generate backup artifact in temp directory.
4. Resolve storage and compute object key:
   - Native versioning: `<base>/<service>/<artifact>`
   - Non-native versioning: `<base>/<service>/<version>/<artifact>`
5. Upload artifact and persist `last_backup_time` in service config.
6. Log major actions at `info` level; emit external command and storage API detail at `debug` level.

## Restore Flow
- Stage 1 (`restore <service-id> [--version]`): download backup object to CWD.
- Stage 2 (`restore <service-id> --apply <file>`): apply local file using service module.
  For Redis in local mode, restore copies the RDB file to `REDIS_RDB_PATH` (or configured restore path) and requires a service restart.

## Extensibility
- Prefer YAML-first module additions under `internal/modules/yaml/*.yaml` for command-driven services.
- Add a new service type by implementing `modules.Module` in Go only when runtime behavior cannot be represented by command templates alone.
- Override built-in module commands using `module_config` in service config or `--module-config` per command.
- Add new storage backend by implementing `storage.Provider` and extending `DefaultStorageFactory`.

See also:
- `docs/service-modules.md`
- `docs/module-authoring.md`
