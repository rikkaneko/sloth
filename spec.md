# Sloth
Cloud based deployed service backup tool

# Language
Go (standalone binary)

## Feature
1. Backup and restore data for
- (1) `mariadb`, (2) `mysql`, (3) `pgsql` database;
- Directus schema snapshot;
- RabbitMQ defination;
- Redis Snapshotting
- Docker/Podman volume archive (via temporary container that mounts the volume and pipes its contents into a compressed archive)
depolyed in containerized service or local service
to S3 compatible object storage (garage, minio, blackblaze B2, AWS S3, ...)
2. Version control: Keep previous version of the backup (Unless delete manually or via CLI)
3. Automatic read environment from `.env` or user provided env file.

## Configuration structure

### Service config (`~/.config/sloth/service.yaml`, then `./.sloth.yaml`)

```yaml
service:
  - name: service-id
    container_name: container-name
    type: service-type
    storage: storage-name
    last_backup_time: iso-time-string
```

### Main config (`~/.config/sloth/main.yaml`)

```yaml
storage:
  - name: default
    type: s3
    endpoint: https://your-s3-endpoint.example.com
    region: auto
    backet: your-bucket-name
    access_key_id: your-access-key
    secret_access_key: your-secret-key
    use_native_object_versioning: false
    base_path: /backup
```

## Architecture
Modular architecture to define the shell command (Run by a generic handler) in config file or Go program code for
- Container modules to query running containers, execute command, and push/pull files to/in running containers
  - podman
  - docker
- Backup and restore modules to/from backup data file
  - mariadb
  - mysql
  - pgsql
  - directus
  - rabbitmq
  - redis
  - docker/podman volume
- Storage modules
  - s3

For simple action, i.e., simple commands to backup/restore a database, a config file can be used to define the command:

MariaDB

```yaml
# mariadb.yaml
backup:
  command:
    # The environment variable should be injected into the runtime environment from env file.
    # Execute in container shell
    - mariadb-dump -u"${DB_USER}" -p"${DB_PASS}" --single-transaction --all-databases > ./backup.sql

  # The target file is inside the container
  target_file: ./backup.sql

restore:
  backup_file:
    # The backup data file will be copied into the container before executing the below commands.
    to_container: ./backup.sql
  command:
    # Execute in container shell
    - mariadb -u"${DB_USER}" -p"${DB_PASS}" "${DB_NAME}" < ./backup.sql
```

Directus
```yaml
# directus.yaml
backup:
  command:
    # The environment variable should be injected into the runtime environment from env file.
    # Execute in container shell
    - npx directus schema snapshot ./snapshot.yaml
  target_file: ./snapshot.yaml
restore:
  backup_file:
    # The backup data file will be copied into the container before executing the below commands.
    to_container: ./snapshot.yaml
  command:
    - npx directus schema apply ./snapshot.yaml
```

The design should allow further development to support more software containers, service types and storages.

## Command line

Colorized output
The program will read the service config from `~/.config/sloth/service.yaml`, then `./.sloth.yaml` if the former not exist.

### Backup

CMD backup <service-id> [--type <service-type> --container-name <container-name> --engine <docker|podman> --storage <storage-name|default> --env <file-path>]

If `--engine` not defined, the program will search the container in `podman` then `docker` by exactly `<service-id>` to determine the correct engine.
If `<service-id>` does not exist anywhere, the program will create a `.sloth.yaml` in the current working directory to save service backup config from the argument. In this case, user must provide at least `--type <service-type> --container-name <container-name>` to define a service.

#### Procedure

1. Based on the `<service-type>`, retireve the backup data file / archive from the service container.
2. Depends on whether use native object versioning,
    * If used native object versioning, upload the backup file at `/<base-path>/<service-id>/<backup-data-file>.<original-file-suffix>` to target storage.
    * If not, upload the backup file at `/<base-path>/<service-id>/<version-id>/<backup-data-file>.<original-file-suffix>` where `version-id` is an auto increment integer (number of version + 1) target storage.

### List backup

CMD list [<service-id>]

Use `ListObjectVersions` or `ListObjectsV2` using a prefix `/<base-path>/<service-id>/<version-id>` to show all avilable version of backup data for `<service-id>`.
Show avilable `<service-id>` if not provided.

### Restore

#### Stage 1: Retrieve backup

CMD restore <service-id> [--version <version-id|latest>]

##### Procedure

1. Download the backup data file as `<service-id>-backup-<backup-time>-<version>.<original-file-suffix>` in the current working directory.
2. Notify user to clean up the old container, bind mount directories and volume.

#### Stage 2: Apply backup

CMD restore <service-id> --apply <backup-data-file>

Based on the `<service-type>`, restore the backup to the service container.
