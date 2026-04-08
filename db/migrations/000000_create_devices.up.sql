CREATE TABLE devices (
    id UUID NOT NULL PRIMARY KEY,
    device_name TEXT NOT NULL DEFAULT '',
    hostname TEXT NOT NULL,
    last_ping_latency INTEGER NOT NULL DEFAULT -1,
    is_up BOOLEAN NOT NULL DEFAULT FALSE
);
