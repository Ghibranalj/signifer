#!/usr/bin/env bash
set -e

echo "Building signifer..."
go build -o signifer .

echo "Granting CAP_NET_RAW capability..."
sudo setcap cap_net_raw=+ep ./signifer

echo "Done! You can now run ./signifer without sudo"
