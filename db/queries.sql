-- name: GetDevices :many
SELECT * FROM devices;


-- name: CreateDevices :one
INSERT INTO devices(
  id, device_name, hostname
)
VALUES (?, ?, ?)
RETURNING *; 


-- name: UpdateDevices :one
UPDATE devices
SET device_name = ?, hostname = ?
WHERE id = ?
RETURNING *;

-- name: SetDeviceStateAndLatency :one
UPDATE devices
SET is_up = ?, last_ping_latency = ?
WHERE id = ?
RETURNING *;


-- name: DeleteDevice :execrows
DELETE FROM devices
WHERE id = ?;
