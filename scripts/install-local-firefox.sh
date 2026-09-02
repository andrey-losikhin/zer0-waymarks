#!/bin/sh
set -eu

project_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
bundle_dir=${1:-"$project_dir/dist/local"}
data_home=${XDG_DATA_HOME:-"$HOME/.local/share"}
install_dir="$data_home/zer0-waymarks/local"
manifest_dir="$HOME/.mozilla/native-messaging-hosts"

case "$data_home" in
  /*) ;;
  *) echo "XDG_DATA_HOME must be an absolute path" >&2; exit 1 ;;
esac
case "$manifest_dir" in
  /*) ;;
  *) echo "HOME must be an absolute path" >&2; exit 1 ;;
esac

if [ ! -x "$bundle_dir/bin/zer0-waymarks-helper" ] ||
   [ ! -x "$bundle_dir/bin/zer0-waymarks-native-host" ] ||
   [ ! -f "$bundle_dir/firefox-extension/manifest.json" ] ||
   [ ! -f "$bundle_dir/firefox-extension/background.js" ] ||
   [ ! -f "$bundle_dir/firefox-extension/native-client.js" ] ||
   [ ! -f "$bundle_dir/firefox-extension/bookmark-tree.js" ] ||
   [ ! -f "$bundle_dir/firefox-extension/options.html" ] ||
   [ ! -f "$bundle_dir/firefox-extension/options.css" ] ||
   [ ! -f "$bundle_dir/firefox-extension/options.js" ]; then
  echo "local bundle is missing; run scripts/build-local.sh first" >&2
  exit 1
fi

mkdir -p "$install_dir/bin" "$install_dir/firefox-extension" "$manifest_dir"
install_dir=$(CDPATH= cd -- "$install_dir" && pwd -P)
for destination in \
  "$install_dir/bin/zer0-waymarks-helper" \
  "$install_dir/bin/zer0-waymarks-native-host" \
  "$install_dir/firefox-extension/manifest.json" \
  "$install_dir/firefox-extension/background.js" \
  "$install_dir/firefox-extension/native-client.js" \
  "$install_dir/firefox-extension/bookmark-tree.js" \
  "$install_dir/firefox-extension/options.html" \
  "$install_dir/firefox-extension/options.css" \
  "$install_dir/firefox-extension/options.js"; do
  if [ -L "$destination" ]; then
    echo "refusing to overwrite symlink: $destination" >&2
    exit 1
  fi
done
install -m 0755 "$bundle_dir/bin/zer0-waymarks-helper" "$install_dir/bin/zer0-waymarks-helper"
install -m 0755 "$bundle_dir/bin/zer0-waymarks-native-host" "$install_dir/bin/zer0-waymarks-native-host"
install -m 0644 "$bundle_dir/firefox-extension/manifest.json" "$install_dir/firefox-extension/manifest.json"
install -m 0644 "$bundle_dir/firefox-extension/background.js" "$install_dir/firefox-extension/background.js"
install -m 0644 "$bundle_dir/firefox-extension/native-client.js" "$install_dir/firefox-extension/native-client.js"
install -m 0644 "$bundle_dir/firefox-extension/bookmark-tree.js" "$install_dir/firefox-extension/bookmark-tree.js"
install -m 0644 "$bundle_dir/firefox-extension/options.html" "$install_dir/firefox-extension/options.html"
install -m 0644 "$bundle_dir/firefox-extension/options.css" "$install_dir/firefox-extension/options.css"
install -m 0644 "$bundle_dir/firefox-extension/options.js" "$install_dir/firefox-extension/options.js"

host_path="$install_dir/bin/zer0-waymarks-native-host"
if printf '%s' "$host_path" | LC_ALL=C grep -q '[[:cntrl:]]'; then
  echo "install path contains unsupported control characters" >&2
  exit 1
fi
escaped_host=$(printf '%s' "$host_path" | sed 's/\\/\\\\/g; s/"/\\"/g')
manifest_tmp=$(mktemp "$manifest_dir/.zer0.waymarks.XXXXXX")
trap 'rm -f "$manifest_tmp"' EXIT HUP INT TERM
{
  printf '%s\n' '{'
  printf '%s\n' '  "name": "zer0.waymarks",'
  printf '%s\n' '  "description": "Native Messaging host for zer0-waymarks",'
  printf '  "path": "%s",\n' "$escaped_host"
  printf '%s\n' '  "type": "stdio",'
  printf '%s\n' '  "allowed_extensions": ['
  printf '%s\n' '    "waymarks@zer0.local"'
  printf '%s\n' '  ]'
  printf '%s\n' '}'
} > "$manifest_tmp"
chmod 0600 "$manifest_tmp"
mv "$manifest_tmp" "$manifest_dir/zer0.waymarks.json"
trap - EXIT HUP INT TERM

printf '%s\n' "Installed locally."
printf '%s\n' "Firefox extension: $install_dir/firefox-extension"
printf '%s\n' "Open about:debugging -> This Firefox -> Load Temporary Add-on -> manifest.json"
