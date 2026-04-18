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
sloth backup <service-id> [--type <service-type> --container-name <container-name> --engine <docker|podman|local> --storage <storage-name> --env <env-file> --module-config <yaml> --volume-name <name> --volume-names <n1,n2>]
```

Behavior:
- If service is missing and `--type` + `--container-name` are provided, sloth writes `./.sloth.yaml` with the new service entry.
- If `--engine` is omitted, sloth checks `podman` then `docker` by `container_name`.
- Supported engines: `docker`, `podman`, `local`.

## list
```bash
sloth list [<service-id>]
```

Behavior:
- Without `<service-id>`: lists configured services.
- With `<service-id>`: lists backup objects/versions for that service.

## restore stage 1 (retrieve)
```bash
sloth restore <service-id> [--version <version-id|latest>] [--type <type> --container-name <name> --engine <engine> --storage <storage-name> --env <env-file> --module-config <yaml>]
```

Behavior:
- Downloads backup artifact to current directory.
- File naming format: `<service-id>-backup-<backup-time>-<version>.<suffix>`.
- Prints operator guidance for container and volume cleanup before apply.

## restore stage 2 (apply)
```bash
sloth restore <service-id> --apply <backup-data-file> [--type <type> --container-name <name> --engine <engine> --storage <storage-name> --env <env-file> --module-config <yaml>]
```

Behavior:
- Applies local file to the target service using the module restore flow.
- Redis restore is guided/manual: dump is copied to `/data/dump.rdb`; restart is required.
- For local Redis restore, set `REDIS_RDB_PATH` (or service meta `redis_rdb_path`) to control destination path before restart.

## Related Docs
- Service environment variable and command mapping: `docs/service-modules.md`
- New module and override workflow: `docs/module-authoring.md`
