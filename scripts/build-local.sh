#!/bin/sh
set -eu

project_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
output_dir=${1:-"$project_dir/dist/local"}
go_cache=${GOCACHE:-"/tmp/zer0-waymarks-go-cache"}

mkdir -p "$output_dir/bin" "$output_dir/firefox-extension"
output_dir=$(CDPATH= cd -- "$output_dir" && pwd)
(
  cd "$project_dir"
  GOCACHE="$go_cache" CGO_ENABLED=0 go build -buildvcs=false -o "$output_dir/bin/zer0-waymarks-helper" ./cmd/zer0-waymarks-helper
  GOCACHE="$go_cache" CGO_ENABLED=0 go build -buildvcs=false -o "$output_dir/bin/zer0-waymarks-native-host" ./cmd/zer0-waymarks-native-host
)
sh "$project_dir/browser-extension/firefox/build.sh" "$output_dir/firefox-extension"
