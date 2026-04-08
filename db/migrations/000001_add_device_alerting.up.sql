-- Add failed_pings column to devices table
ALTER TABLE devices ADD COLUMN failed_pings INTEGER NOT NULL DEFAULT 0;
ALTER TABLE devices ADD COLUMN alerted_down BOOLEAN NOT NULL DEFAULT FALSE;
