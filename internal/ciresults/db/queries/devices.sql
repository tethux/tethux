-- name: UpsertDevice :one
INSERT INTO
    devices (
        device_key,
        display_name,
        last_os,
        last_os_version,
        last_kernel,
        last_arch,
        last_cpu,
        last_memory_bytes,
        metadata_json,
        first_seen_at,
        last_seen_at
    )
VALUES
    (
        sqlc.arg(device_key),
        sqlc.narg(display_name),
        sqlc.narg(last_os),
        sqlc.narg(last_os_version),
        sqlc.narg(last_kernel),
        sqlc.narg(last_arch),
        sqlc.narg(last_cpu),
        sqlc.narg(last_memory_bytes),
        sqlc.arg(metadata_json),
        sqlc.arg(seen_at),
        sqlc.arg(seen_at)
    ) ON CONFLICT(device_key) DO
UPDATE
SET
    display_name = COALESCE(
        excluded.display_name,
        devices.display_name
    ),
    last_os = COALESCE(
        excluded.last_os,
        devices.last_os
    ),
    last_os_version = COALESCE(
        excluded.last_os_version,
        devices.last_os_version
    ),
    last_kernel = COALESCE(
        excluded.last_kernel,
        devices.last_kernel
    ),
    last_arch = COALESCE(
        excluded.last_arch,
        devices.last_arch
    ),
    last_cpu = COALESCE(
        excluded.last_cpu,
        devices.last_cpu
    ),
    last_memory_bytes = COALESCE(
        excluded.last_memory_bytes,
        devices.last_memory_bytes
    ),
    metadata_json = excluded.metadata_json,
    first_seen_at = COALESCE(
        devices.first_seen_at,
        excluded.first_seen_at
    ),
    last_seen_at = CASE
        WHEN devices.last_seen_at IS NULL
        OR excluded.last_seen_at > devices.last_seen_at THEN excluded.last_seen_at
        ELSE devices.last_seen_at
    END
RETURNING
    *;

-- name: GetDeviceByKey :one
SELECT
    *
FROM
    devices
WHERE
    device_key = sqlc.arg(device_key)
LIMIT
    1;

-- name: ListDevices :many
SELECT
    *
FROM
    devices
ORDER BY
    COALESCE(display_name, device_key);

-- name: ListStaleDevices :many
SELECT
    *
FROM
    devices
WHERE
    last_seen_at IS NULL
    OR last_seen_at < sqlc.arg(cutoff)
ORDER BY
    last_seen_at ASC;
