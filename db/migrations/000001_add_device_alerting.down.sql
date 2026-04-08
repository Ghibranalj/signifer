-- SQLite doesn't support DROP COLUMN directly, need to recreate table
-- This is a limitation of SQLite - in production you'd typically use a different approach
BEGIN;

-- Create new table without failed_pings
CREATE TABLE devices_new (
    id UUID NOT NULL PRIMARY KEY,
    device_name TEXT NOT NULL DEFAULT '',
    hostname TEXT NOT NULL,
    last_ping_latency INTEGER NOT NULL DEFAULT -1,
    is_up BOOLEAN NOT NULL DEFAULT FALSE
);

-- Copy data from old table to new table
INSERT INTO devices_new (id, device_name, hostname, last_ping_latency, is_up)
SELECT id, device_name, hostname, last_ping_latency, is_up FROM devices;

-- Drop old table
DROP TABLE devices;

-- Rename new table to original name
ALTER TABLE devices_new RENAME TO devices;

COMMIT;
