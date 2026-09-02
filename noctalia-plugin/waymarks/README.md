# Waymarks

Waymarks searches and adds bookmarks stored by the local `zer0-waymarks-helper`.
It does not read browser profile databases or send bookmark data over the network.

| Field | Value |
| --- | --- |
| Plugin ID | `zer0/waymarks` |
| Launcher | `/wm` |
| Panel | `zer0/waymarks:panel` |

## Requirements

- `zer0-waymarks-helper` installed on `PATH`;
- browser/profile IDs configured in the helper and selected in plugin settings.

## Usage

- Type `/wm documentation` to search by title or URL. Selecting a result opens it
  with the configured browser and optional profile.
- Open the panel with `noctalia msg panel-toggle zer0/waymarks:panel` to search,
  choose a configured browser/profile, add or edit an HTTP/HTTPS bookmark with
  an optional local note, and delete one with an explicit second click. Search
  matches title, URL, note and tags; notes/tags are not copied into browser bookmarks.
- Use tags as sections such as `personal`, `work` and `project`; the section
  dropdown filters results. One bookmark may belong to multiple sections.
- Choose a detected browser and press **Set default** to persist it. Known local
  browsers, including Helium, are discovered automatically.
- The bookmark list is active when the panel opens. Typing filters immediately;
  `Up`/`Down` changes the selected bookmark and `Enter` opens it. There is no
  separate Search button.
- Tabs: `Ctrl+1` bookmarks, `Ctrl+2` (or `Ctrl+N`) add form, `Ctrl+3` settings.
  In bookmarks, `Ctrl+Up`/`Ctrl+Down` changes the section. In settings,
  `Up`/`Down` chooses a browser, `Left`/`Right` chooses its profile, and `Enter`
  saves the default browser. `Ctrl+Enter` saves the form; `Esc` returns to
  bookmarks or closes the panel.

The plugin invokes the helper with an argument array, never through a shell command.
URLs, titles, notes and result IDs remain separate argv values and are validated again by
the helper.
