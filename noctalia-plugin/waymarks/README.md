# Waymarks

Waymarks is a keyboard-first Noctalia interface for a local, browser-independent
bookmark collection. Search, organize, and open bookmarks without sending data
to a cloud service or reading browser profile databases.

## Plugin

| Field | Value |
| --- | --- |
| ID | `zer0/waymarks` |
| Entries | Launcher: `search`; panel: `panel` |
| Launcher Prefix | `/wm` |
| License | Apache-2.0 |

## Requirements

Install `zer0-waymarks-helper` on the `PATH` inherited by Noctalia. The helper
and its source are available in the
[zer0-waymarks repository](https://github.com/andrey-losikhin/zer0-waymarks).

## Usage

Open the panel with:

```sh
noctalia msg panel-toggle zer0/waymarks:panel
```

The panel provides three tabs:

- **Bookmarks** searches titles, URLs, optional notes, and tags as you type.
- **Add** creates or edits an HTTP(S) bookmark with optional notes and tags.
  Existing tags can be searched and assigned without retyping them. New tags
  are entered in the **Assigned tags** field.
- **Settings** selects a detected browser and profile and persists the default
  browser used on the next launch.

Use tags as sections such as `personal`, `work`, or `project`. A bookmark may
belong to multiple sections. Deletion requires an explicit second click.

Type `/wm documentation` in the Noctalia launcher to search from anywhere.
Activating a result opens it with the configured browser and optional profile.
The normal open action uses the default browser. Use the small **With…** action
on a bookmark card to choose another browser/profile for that one launch without
changing the default.

## Keyboard shortcuts

| Shortcut | Action |
| --- | --- |
| Type | Filter bookmarks immediately |
| `Up` / `Down` | Select a bookmark |
| `Enter` | Open the selected bookmark |
| `Ctrl+1` | Open Bookmarks |
| `Ctrl+2` or `Ctrl+N` | Open Add |
| `Ctrl+3` | Open Settings |
| `Ctrl+T` | Open the searchable tag picker |
| `Ctrl+Shift+T` | Clear the active tag filter |
| `Ctrl+Up` / `Ctrl+Down` | Change section |
| `Ctrl+Enter` | Save the add/edit form |
| `Escape` | Return to Bookmarks or close the panel |

## Browser settings

Use the panel's **Settings** tab to choose a browser and profile. The default
browser is stored by the helper and is shared with the launcher; no duplicate
Noctalia setting is required.

The helper automatically detects common Linux browsers, including Firefox,
Zen, LibreWolf, Floorp, Waterfox, Chromium, Chrome, Brave, Vivaldi, Opera, Edge,
Thorium, and Helium. Explicit helper configuration takes priority.

## Notes

- Bookmark data remains in the user's XDG data directory.
- Notes and tags are local Waymarks metadata and are not copied into browser
  bookmark titles.
- The plugin invokes the helper with an argument array, never through a shell
  command.
- Only HTTP(S) bookmark URLs are accepted by default.
- This plugin does not require the optional browser synchronization extension.
