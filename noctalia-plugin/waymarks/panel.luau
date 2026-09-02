--!nonstrict

local helper = require("./shared.luau")

local query = ""
local addTitle = ""
local addUrl = ""
local addNote = ""
local addTags = ""
local bookmarks = {}
local selectedIndex = 0
local busy = false
local message = ""
local messageIsError = false
local searchGeneration = 0
local formRevision = 0
local editId = nil
local editRevision = nil
local deletePendingId = nil
local browsers = {}
local browserOptions = {}
local browserIndex = 0
local defaultBrowserId = ""
local profileOptions = { "Default profile" }
local profileIndex = 0
local tagOptions = { "All sections" }
local tagIndex = 0
local focusArea = "search"
local viewMode = "search"

local function setMessage(text, isError)
  message = text or ""
  messageIsError = isError == true
end

local function openBookmark(id)
  local selectedBrowser = browsers[browserIndex + 1]
  local browserId = selectedBrowser and selectedBrowser.id or nil
  local selectedProfile = selectedBrowser and selectedBrowser.profiles and selectedBrowser.profiles[profileIndex] or nil
  local profileId = selectedProfile and selectedProfile.id or ""
  helper.run(helper.openArguments(id, browserId, profileId), function(_data, err)
    if err ~= nil then
      setMessage(err, true)
      renderPanel()
      return
    end
    panel.close()
  end)
end

function renderPanel()
  local resultRows = {}
  for _, bookmark in ipairs(bookmarks) do
    local rowBookmark = bookmark
    local bookmarkId = rowBookmark.id
    local rowIndex = #resultRows + 1
    local selected = rowIndex == selectedIndex
    table.insert(resultRows, ui.column({ key = bookmarkId, gap = 4, padding = 6, radius = 8, fill = selected and "primary/0.10" or "surface" }, {
      ui.row({ gap = 6 }, {
        ui.button({
          text = rowBookmark.title ~= "" and rowBookmark.title or rowBookmark.url,
          glyph = "external-link",
          contentAlign = "start",
          flexGrow = 1,
          onClick = function() openBookmark(bookmarkId) end,
        }),
        ui.button({
          glyph = "edit",
          variant = "ghost",
          tooltip = "Edit bookmark",
          enabled = not busy,
          onClick = function() beginEdit(rowBookmark) end,
        }),
        ui.button({
          glyph = "trash",
          variant = deletePendingId == bookmarkId and "destructive" or "ghost",
          tooltip = deletePendingId == bookmarkId and "Press again to delete" or "Delete bookmark",
          enabled = not busy,
          onClick = function() requestDelete(rowBookmark) end,
        }),
      }),
      ui.label({ text = rowBookmark.url, color = "on_surface/0.65", maxLines = 1 }),
      ui.label({ text = rowBookmark.note or "", color = "on_surface/0.55", maxLines = 2, visible = (rowBookmark.note or "") ~= "" }),
      ui.label({ text = table.concat(rowBookmark.tags or {}, " · "), color = "primary", maxLines = 1, visible = #(rowBookmark.tags or {}) > 0 }),
    }))
  end
  if #resultRows == 0 then
    table.insert(resultRows, ui.label({ text = busy and "Searching…" or "No bookmarks found", color = "on_surface/0.65" }))
  end

  local children = {
    ui.label({ text = "Waymarks", fontSize = 24, fontWeight = "bold" }),
    ui.label({
      key = "status",
      text = message ~= "" and message or " ",
      color = messageIsError and "error" or "primary",
      height = 20,
      maxLines = 1,
    }),
  }

  if viewMode == "search" then
    table.insert(children, ui.row({ gap = 8 }, {
      ui.input({
        key = "search",
        value = query,
        placeholder = "Search title, URL or note",
        focus = focusArea == "search",
        flexGrow = 1,
        onChange = function(value) query = value end,
        onSubmit = function(value) query = value; searchBookmarks() end,
      }),
      ui.button({ text = "Search", glyph = "search", enabled = not busy, onClick = "searchBookmarks" }),
    }))
    table.insert(children, ui.row({ gap = 8 }, {
      ui.select({
        options = browserOptions,
        selectedIndex = browserIndex,
        placeholder = "Browser",
        enabled = #browserOptions > 0 and not busy,
        flexGrow = 1,
        onChange = "onBrowserChanged",
      }),
      ui.select({
        options = profileOptions,
        selectedIndex = profileIndex,
        placeholder = "Profile",
        enabled = #profileOptions > 1 and not busy,
        flexGrow = 1,
        onChange = "onProfileChanged",
      }),
      ui.button({
        text = (browsers[browserIndex + 1] and browsers[browserIndex + 1].id == defaultBrowserId) and "Default" or "Set default",
        glyph = (browsers[browserIndex + 1] and browsers[browserIndex + 1].id == defaultBrowserId) and "circle-check" or "circle",
        enabled = #browserOptions > 0 and not busy,
        onClick = "setDefaultBrowser",
      }),
    }))
    table.insert(children, ui.select({
      options = tagOptions,
      selectedIndex = tagIndex,
      placeholder = "Section",
      enabled = not busy,
      onChange = "onTagChanged",
    }))
    table.insert(children, ui.scroll({ flexGrow = 1, gap = 8 }, resultRows))
    table.insert(children, ui.label({ text = "Ctrl+F search · Ctrl+N add · ↑/↓ select · Ctrl+O open · Esc close", color = "on_surface/0.55", maxLines = 2 }))
    table.insert(children, ui.button({
      text = "Add bookmark",
      glyph = "bookmark-plus",
      variant = "primary",
      enabled = not busy,
      onClick = "startAdd",
    }))
  else
    table.insert(children, ui.row({ align = "center", gap = 8 }, {
      ui.label({ text = editId == nil and "Add bookmark" or "Edit bookmark", fontSize = 18, fontWeight = "bold", flexGrow = 1 }),
      ui.button({ text = "Back", glyph = "arrow-left", variant = "ghost", enabled = not busy, onClick = "cancelEdit" }),
    }))
    table.insert(children, ui.input({
      key = "title-" .. tostring(formRevision),
      value = addTitle,
      placeholder = "Title (optional)",
      enabled = not busy,
      focus = focusArea == "form",
      onChange = function(value) addTitle = value end,
    }))
    table.insert(children, ui.input({
      key = "url-" .. tostring(formRevision),
      value = addUrl,
      placeholder = "https://example.test",
      enabled = not busy,
      onChange = function(value) addUrl = value end,
      onSubmit = function(value) addUrl = value; saveBookmark() end,
    }))
    table.insert(children, ui.input({
      key = "note-" .. tostring(formRevision),
      value = addNote,
      placeholder = "Note (optional)",
      enabled = not busy,
      onChange = function(value) addNote = value end,
    }))
    table.insert(children, ui.input({
      key = "tags-" .. tostring(formRevision),
      value = addTags,
      placeholder = "Sections/tags: personal, work, project",
      enabled = not busy,
      onChange = function(value) addTags = value end,
    }))
    table.insert(children, ui.label({ text = "Tab changes fields · Ctrl+Enter saves · Esc returns to search", color = "on_surface/0.55", maxLines = 2 }))
    table.insert(children, ui.button({
      text = busy and "Working…" or (editId == nil and "Add bookmark" or "Save changes"),
      glyph = editId == nil and "bookmark-plus" or "device-floppy",
      variant = "primary",
      enabled = not busy,
      onClick = "saveBookmark",
    }))
    table.insert(children, ui.spacer({ flexGrow = 1 }))
  end

  panel.render(ui.column({ padding = 16, gap = 10, fill = "surface" }, children))
end

function beginEdit(bookmark)
  editId = bookmark.id
  editRevision = bookmark.revision
  deletePendingId = nil
  addTitle = bookmark.title or ""
  addUrl = bookmark.url or ""
  addNote = bookmark.note or ""
  addTags = table.concat(bookmark.tags or {}, ", ")
  viewMode = "form"
  focusArea = "form"
  formRevision = formRevision + 1
  setMessage("", false)
  renderPanel()
end

function startAdd()
  editId = nil
  editRevision = nil
  addTitle = ""
  addUrl = ""
  addNote = ""
  addTags = ""
  viewMode = "form"
  focusArea = "form"
  formRevision = formRevision + 1
  setMessage("", false)
  renderPanel()
end

function cancelEdit()
  editId = nil
  editRevision = nil
  addTitle = ""
  addUrl = ""
  addNote = ""
  addTags = ""
  viewMode = "search"
  focusArea = "search"
  formRevision = formRevision + 1
  setMessage("", false)
  renderPanel()
end

function requestDelete(bookmark)
  if busy then return end
  if deletePendingId ~= bookmark.id then
    deletePendingId = bookmark.id
    setMessage("Press the highlighted delete button again to confirm", true)
    renderPanel()
    return
  end
  deletePendingId = nil
  busy = true
  setMessage("", false)
  renderPanel()
  helper.run({
    "remove", bookmark.id,
    "--base-revision", tostring(bookmark.revision),
    "--operation-id", helper.operationId("panel-remove"),
    "--json",
  }, function(_data, err)
    busy = false
    if err ~= nil then
      setMessage(err, true)
      renderPanel()
      return
    end
    if editId == bookmark.id then cancelEdit() end
    setMessage("Bookmark deleted", false)
    searchBookmarks()
  end)
end

local function rebuildProfiles()
  profileOptions = { "Default profile" }
  local selectedBrowser = browsers[browserIndex + 1]
  if selectedBrowser ~= nil then
    for _, profile in ipairs(selectedBrowser.profiles or {}) do
      table.insert(profileOptions, profile.name)
    end
  end
  profileIndex = 0
end

function onBrowserChanged(value)
  browserIndex = tonumber(value) or 0
  rebuildProfiles()
  renderPanel()
end

function onProfileChanged(value)
  profileIndex = tonumber(value) or 0
  renderPanel()
end

function onTagChanged(value)
  tagIndex = tonumber(value) or 0
  searchBookmarks()
end

function setDefaultBrowser()
  local selected = browsers[browserIndex + 1]
  if selected == nil or busy then return end
  busy = true
  renderPanel()
  helper.run({ "set-default-browser", selected.id, "--json" }, function(data, err)
    busy = false
    if err ~= nil then
      setMessage(err, true)
    else
      defaultBrowserId = data.default_browser_id or selected.id
      setMessage("Default browser saved", false)
    end
    renderPanel()
  end)
end

local function loadTags(afterLoad)
  helper.run({ "tags", "--json" }, function(data, err)
    local previousTagIndex = tagIndex
    tagOptions = { "All sections" }
    if err == nil then
      for _, tag in ipairs(data.tags or {}) do table.insert(tagOptions, tag) end
    end
    if tagIndex >= #tagOptions then tagIndex = 0 end
    renderPanel()
    if type(afterLoad) == "function" then
      afterLoad()
    elseif previousTagIndex ~= tagIndex then
      searchBookmarks()
    end
  end)
end

local function loadBrowsers()
  helper.run({ "browsers", "--json" }, function(data, err)
    if err ~= nil then
      browsers = {}
      browserOptions = {}
      rebuildProfiles()
      setMessage(err, true)
      renderPanel()
      return
    end
    browsers = data.browsers or {}
    defaultBrowserId = data.default_browser_id or ""
    browserOptions = {}
    local configuredBrowser = defaultBrowserId
    browserIndex = 0
    for index, browser in ipairs(browsers) do
      table.insert(browserOptions, browser.name)
      if browser.id == configuredBrowser then browserIndex = index - 1 end
    end
    rebuildProfiles()
    local configuredProfile = noctalia.getConfig("profile_id")
    local selectedBrowser = browsers[browserIndex + 1]
    if type(configuredProfile) == "string" and configuredProfile ~= "" and selectedBrowser ~= nil then
      for index, profile in ipairs(selectedBrowser.profiles or {}) do
        if profile.id == configuredProfile then profileIndex = index end
      end
    end
    renderPanel()
  end)
end

function searchBookmarks()
  searchGeneration = searchGeneration + 1
  local requestGeneration = searchGeneration
  busy = true
  setMessage("", false)
  renderPanel()
  local arguments = { "list", "--query", noctalia.string.trim(query), "--limit", "50", "--json" }
  if tagIndex > 0 and tagOptions[tagIndex + 1] ~= nil then
    table.insert(arguments, #arguments, "--tag")
    table.insert(arguments, #arguments, tagOptions[tagIndex + 1])
  end
  helper.run(arguments, function(data, err)
    if requestGeneration ~= searchGeneration then return end
    busy = false
    if err ~= nil then
      bookmarks = {}
      setMessage(err, true)
    else
      bookmarks = data.bookmarks or {}
      selectedIndex = #bookmarks > 0 and 1 or 0
    end
    renderPanel()
  end)
end

function saveBookmark()
  if busy then return end
  local url = noctalia.string.trim(addUrl)
  local title = noctalia.string.trim(addTitle)
  local note = addNote
  local tags = noctalia.string.trim(addTags)
  if url == "" then
    setMessage("URL is required", true)
    renderPanel()
    return
  end

  busy = true
  setMessage("", false)
  renderPanel()
  local arguments
  if editId == nil then
    arguments = { "add", "--url", url, "--operation-id", helper.operationId("panel-add"), "--json" }
    if title ~= "" then
      table.insert(arguments, #arguments, "--title")
      table.insert(arguments, #arguments, title)
    end
    if note ~= "" then
      table.insert(arguments, #arguments, "--note")
      table.insert(arguments, #arguments, note)
    end
    if tags ~= "" then
      table.insert(arguments, #arguments, "--tags")
      table.insert(arguments, #arguments, tags)
    end
  else
    arguments = {
      "update", editId,
      "--url", url,
      "--title", title,
      "--note", note,
      "--tags", tags,
      "--base-revision", tostring(editRevision),
      "--operation-id", helper.operationId("panel-update"),
      "--json",
    }
  end
  helper.run(arguments, function(_data, err)
    busy = false
    if err ~= nil then
      setMessage(err, true)
      renderPanel()
      return
    end
    addTitle = ""
    addUrl = ""
    addNote = ""
    addTags = ""
    editId = nil
    editRevision = nil
    formRevision = formRevision + 1
    setMessage("Bookmark saved", false)
    viewMode = "search"
    focusArea = "search"
    loadTags(function() searchBookmarks() end)
  end)
end

function onOpen(_context)
  viewMode = "search"
  focusArea = "search"
  loadBrowsers()
  loadTags()
  searchBookmarks()
end

function onKey(chord, pressed)
  if not pressed then return end
  local key = tostring(chord):lower()
  if key == "ctrl+f" then
    if viewMode == "form" then
      cancelEdit()
    else
      focusArea = "search"; formRevision = formRevision + 1; renderPanel()
    end
  elseif key == "ctrl+n" then
    startAdd()
  elseif key == "down" and #bookmarks > 0 then
    selectedIndex = selectedIndex % #bookmarks + 1; renderPanel()
  elseif key == "up" and #bookmarks > 0 then
    selectedIndex = (selectedIndex - 2) % #bookmarks + 1; renderPanel()
  elseif key == "ctrl+o" and selectedIndex > 0 and focusArea == "search" then
    openBookmark(bookmarks[selectedIndex].id)
  elseif key == "ctrl+return" or key == "ctrl+enter" then
    saveBookmark()
  elseif key == "escape" then
    if viewMode == "form" then cancelEdit() else panel.close() end
  end
end
