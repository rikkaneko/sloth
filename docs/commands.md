# Sloth Commands

## Help
```bash
sloth --help
sloth help
sloth help backup
sloth backup --help
sloth restore --help
sloth list --help
```

Behavior:
- Help output uses sectioned command descriptions and includes dynamic available values for `--type`, `--engine`, and `--storage`.
- `--type` values come from embedded module YAML definitions plus `volume`.
- `--storage` values are discovered from `~/.config/sloth/main.yaml`; if unavailable, help still prints with a graceful notice.

## backup
```bash
sloth backup <service-id> [-t|--type <service-type>] [-c|--container-name <container-name>] [-E|--engine <docker|podman>] [-l|--local] [-s|--storage <storage-name>] [-e|--env <env-file>] [-m|--module-config <yaml>] [-n|--volume-name <name>] [-N|--volume-names <n1,n2>] [--force] [--use-checksum] [--use-file-size-check] [-d|--debug]
```

Behavior:
- If service is missing and `--type` is provided, sloth writes `./.sloth.yaml` with the new service entry.
- If `--container-name` is omitted, sloth probes containers by `<service-id>`.
- If `--engine` is omitted, sloth checks `podman` then `docker`.
- Supported `--engine` values: `docker`, `podman`.
- Local mode is explicit via `--local` (do not use `--engine local`).
- `--debug` shows external command output and S3 request/response summaries.
- After backup upload completes, output prints the same backup table format as `sloth list <service-id>`.
- Delta check mode:
  - Default: checksum.
  - Config override: `common.file_delta_check: checksum|file_size`.
  - Command overrides: `--use-checksum`, `--use-file-size-check`.
  - `--force` bypasses all delta checks and always uploads a new backup version.
  - Upload is skipped when any enabled check matches latest backup (`[info] Backup file is already up-to-date. Skipped.`).

## list
```bash
sloth list [<service-id>] [--show-object-key] [-d|--debug]
```

Behavior:
- Without `<service-id>`: lists configured services using columns `service`, `type`, `storage`, `last_backup`.
- Empty service storage values are rendered as `default`.
- With `<service-id>`: lists backup objects/versions for that service using the same solid-border table style.
- Backup object `size` is rendered in human-readable format.
- `object_key` is hidden by default; include `--show-object-key` to show it.
- `--debug` shows storage API call details.

## restore stage 1 (retrieve)
```bash
sloth restore <service-id> [-v|--version <version-id|latest>] [-t|--type <type>] [-c|--container-name <name>] [-E|--engine <docker|podman>] [-l|--local] [-s|--storage <storage-name>] [-e|--env <env-file>] [-m|--module-config <yaml>] [-d|--debug]
```

Behavior:
- Downloads backup artifact to current directory.
- File naming format: `<service-id>-backup-<backup-time>-<version>.<suffix>`.
- Prints operator guidance for container and volume cleanup before apply.
- Local mode is explicit via `--local`.
- `--debug` shows storage API call details.

## restore stage 2 (apply)
```bash
sloth restore <service-id> -a|--apply <backup-data-file> [-t|--type <type>] [-c|--container-name <name>] [-E|--engine <docker|podman>] [-l|--local] [-s|--storage <storage-name>] [-e|--env <env-file>] [-m|--module-config <yaml>] [-d|--debug]
```

Behavior:
- Applies local file to the target service using the module restore flow.
- Redis restore is guided/manual: dump is copied to `/data/dump.rdb`; restart is required.
- For local Redis restore, set `REDIS_RDB_PATH` (or service meta `redis_rdb_path`) to control destination path before restart.

## Related Docs
- Service environment variable and command mapping: `docs/service-modules.md`
- New module and override workflow: `docs/module-authoring.md`
