#!/bin/sh
set -eu

project_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
go_cache=${GOCACHE:-/tmp/zer0-waymarks-go-cache}

cd "$project_dir"
GOCACHE="$go_cache" go test -buildvcs=false ./...
GOCACHE="$go_cache" go test -buildvcs=false -race ./internal/store ./internal/bridge ./internal/nativehost
GOCACHE="$go_cache" go vet -buildvcs=false ./...

node browser-extension/shared/bookmark-tree.test.js
node browser-extension/shared/native-client.test.js
node browser-extension/firefox/background.test.js
node browser-extension/firefox/options.test.js
node --check browser-extension/firefox/background.js
node --check browser-extension/firefox/options.js
