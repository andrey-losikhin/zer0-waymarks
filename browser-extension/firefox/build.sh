#!/bin/sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
target=${1:-"$script_dir/../dist/firefox"}
mkdir -p "$target"
cp "$script_dir/manifest.json" "$target/manifest.json"
cp "$script_dir/background.js" "$target/background.js"
cp "$script_dir/../shared/native-client.js" "$target/native-client.js"
cp "$script_dir/../shared/bookmark-tree.js" "$target/bookmark-tree.js"
cp "$script_dir/options.html" "$target/options.html"
cp "$script_dir/options.css" "$target/options.css"
cp "$script_dir/options.js" "$target/options.js"
