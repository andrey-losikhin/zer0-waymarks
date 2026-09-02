--!nonstrict

local helper = require("./shared.luau")

local query = ""
local addTitle = ""
local addUrl = ""
local addNote = ""
local addTags = ""
local tagSearch = ""
local bookmarks = {}
local selectedIndex = 0
local busy = false
local message = ""
local messageIsError = false
local searchGeneration = 0
local searchInFlight = false
local pendingSearch = false
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
local viewMode = "bookmarks"
local openWithId = nil
local panelSession = 0

local function setMessage(text, isError)
  message = text or ""
  messageIsError = isError == true
end

local openingId = nil

local function defaultBrowserName()
  for _, browser in ipairs(browsers) do
    if browser.id == defaultBrowserId then return browser.name end
  end
  return defaultBrowserId ~= "" and defaultBrowserId or "Not selected"
end

local function clearForm()
  editId = nil
  editRevision = nil
  addTitle = ""
  addUrl = ""
  addNote = ""
  addTags = ""
  tagSearch = ""
  formRevision = formRevision + 1
end

local function selectedTagList()
  local result = {}
  for value in string.gmatch(addTags, "([^,]+)") do
    local tag = noctalia.string.trim(value)
    if tag ~= "" then table.insert(result, tag) end
  end
  return result
end

local function hasSelectedTag(tag)
  local wanted = string.lower(tag)
  for _, selectedTag in ipairs(selectedTagList()) do
    if string.lower(selectedTag) == wanted then return true end
  end
  return false
end

local function openBookmark(id, useSelectedBrowser)
  if openingId == id then return end
  local requestSession = panelSession
  openingId = id
  local selectedBrowser = useSelectedBrowser and browsers[browserIndex + 1] or nil
  local browserId = selectedBrowser and selectedBrowser.id or nil
  local selectedProfile = selectedBrowser and selectedBrowser.profiles and selectedBrowser.profiles[profileIndex] or nil
  local profileId = selectedProfile and selectedProfile.id or ""
  helper.run(helper.openArguments(id, browserId, profileId), function(_data, err)
    if requestSession ~= panelSession then return end
    openingId = nil
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
    table.insert(resultRows, ui.column({
      key = bookmarkId,
      gap = 4,
      padding = 6,
      radius = 8,
      fill = selected and "primary/0.12" or "on_surface/0.06",
      border = selected and "primary" or "on_surface/0.10",
      borderWidth = 1,
      onClick = function() openBookmark(bookmarkId) end,
    }, {
      ui.row({ gap = 6, align = "center" }, {
        ui.label({
          text = rowBookmark.title ~= "" and rowBookmark.title or rowBookmark.url,
          fontWeight = "bold",
          flexGrow = 1,
          maxLines = 1,
        }),
        ui.button({
          glyph = "external-link",
          variant = "ghost",
          tooltip = "Open with default browser",
          onClick = function() openBookmark(bookmarkId) end,
        }),
        ui.button({
          text = "With…",
          variant = openWithId == bookmarkId and "primary" or "ghost",
          tooltip = "Open with another browser",
          enabled = not busy,
          onClick = function() showOpenWith(bookmarkId) end,
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
      ui.row({ gap = 10, align = "center" }, {
        ui.label({ text = rowBookmark.url, color = "on_surface/0.65", maxLines = 1, flexGrow = 1 }),
        ui.label({ text = table.concat(rowBookmark.tags or {}, " · "), color = "primary", maxLines = 1, visible = #(rowBookmark.tags or {}) > 0 }),
      }),
      ui.label({ text = rowBookmark.note or "", color = "on_surface/0.55", maxLines = 2, visible = (rowBookmark.note or "") ~= "" }),
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
    ui.row({ gap = 6 }, {
      ui.button({ text = "1  Bookmarks", glyph = "bookmark", variant = viewMode == "bookmarks" and "primary" or "ghost", flexGrow = 1, onClick = function() selectTab("bookmarks") end }),
      ui.button({ text = "2  Add", glyph = "bookmark-plus", variant = viewMode == "add" and "primary" or "ghost", flexGrow = 1, onClick = function() selectTab("add") end }),
      ui.button({ text = "3  Settings", glyph = "settings", variant = viewMode == "settings" and "primary" or "ghost", flexGrow = 1, onClick = function() selectTab("settings") end }),
    }),
  }

  if viewMode == "bookmarks" then
    table.insert(children, ui.row({ gap = 8 }, {
      ui.input({
        key = "search",
        value = query,
        placeholder = "Type to filter bookmarks…",
        focus = true,
        flexGrow = 1,
        onChange = "onQueryChanged",
        onSubmit = function(value)
          query = value
          if not busy and selectedIndex > 0 and bookmarks[selectedIndex] ~= nil then
            openBookmark(bookmarks[selectedIndex].id)
          end
        end,
      }),
    }))
    table.insert(children, ui.label({ text = "Default browser: " .. defaultBrowserName(), color = "on_surface/0.60", maxLines = 1 }))
    if openWithId ~= nil then
      table.insert(children, ui.column({ gap = 6, padding = 8, radius = 8, fill = "on_surface/0.06" }, {
        ui.row({ gap = 6, align = "center" }, {
          ui.label({ text = "Open selected with", fontWeight = "bold", flexGrow = 1 }),
          ui.button({ glyph = "x", variant = "ghost", tooltip = "Cancel", onClick = "cancelOpenWith" }),
        }),
        ui.row({ gap = 6 }, {
          ui.select({ options = browserOptions, selectedIndex = browserIndex, placeholder = "Browser", enabled = #browserOptions > 0 and not busy, flexGrow = 1, onChange = "onBrowserChanged" }),
          ui.select({ options = profileOptions, selectedIndex = profileIndex, placeholder = "Profile", enabled = #profileOptions > 1 and not busy, flexGrow = 1, onChange = "onProfileChanged" }),
          ui.button({ text = "Open", glyph = "external-link", variant = "primary", enabled = #browserOptions > 0 and not busy, onClick = "openWithSelectedBrowser" }),
        }),
      }))
    end
    table.insert(children, ui.select({
      options = tagOptions,
      selectedIndex = tagIndex,
      placeholder = "Section",
      enabled = not busy,
      onChange = "onTagChanged",
    }))
    table.insert(children, ui.scroll({ flexGrow = 1, gap = 8 }, resultRows))
    table.insert(children, ui.label({
      text = "↑/↓ Select · Enter Open · Ctrl+↑/↓ Section · Ctrl+2 Add · Ctrl+3 Settings",
      color = "on_surface/0.55",
      height = 20,
      maxLines = 1,
    }))
  elseif viewMode == "settings" then
    table.insert(children, ui.label({ text = "Default browser", fontSize = 18, fontWeight = "bold" }))
    table.insert(children, ui.select({
      options = browserOptions,
      selectedIndex = browserIndex,
      placeholder = "Browser",
      enabled = #browserOptions > 0 and not busy,
      onChange = "onBrowserChanged",
    }))
    table.insert(children, ui.select({
      options = profileOptions,
      selectedIndex = profileIndex,
      placeholder = "Profile",
      enabled = #profileOptions > 1 and not busy,
      onChange = "onProfileChanged",
    }))
    table.insert(children, ui.button({
      text = (browsers[browserIndex + 1] and browsers[browserIndex + 1].id == defaultBrowserId) and "Current default" or "Set as default",
      glyph = (browsers[browserIndex + 1] and browsers[browserIndex + 1].id == defaultBrowserId) and "circle-check" or "circle",
      variant = "primary",
      enabled = #browserOptions > 0 and not busy,
      onClick = "setDefaultBrowser",
    }))
    table.insert(children, ui.label({ text = "↑/↓ browser · ←/→ profile · Enter saves default · Ctrl+1 bookmarks", color = "on_surface/0.55", maxLines = 2 }))
    table.insert(children, ui.spacer({ flexGrow = 1 }))
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
      placeholder = "Assigned tags: personal, work, project",
      enabled = not busy,
      onChange = "onFormTagsChanged",
    }))
    table.insert(children, ui.input({
      key = "tag-search-" .. tostring(formRevision),
      value = tagSearch,
      placeholder = "Find an existing tag…",
      enabled = not busy,
      onChange = "onTagSearchChanged",
      onSubmit = "assignSearchedTag",
    }))
    local matchingTags = {}
    local wantedTag = string.lower(noctalia.string.trim(tagSearch))
    for index = 2, #tagOptions do
      local existingTag = tagOptions[index]
      if (wantedTag == "" or string.find(string.lower(existingTag), wantedTag, 1, true) ~= nil) and #matchingTags < 6 then
        local candidateTag = existingTag
        table.insert(matchingTags, ui.button({
          text = (hasSelectedTag(candidateTag) and "✓  " or "+  ") .. candidateTag,
          variant = hasSelectedTag(candidateTag) and "primary" or "ghost",
          enabled = not busy,
          onClick = function() toggleFormTag(candidateTag) end,
        }))
      end
    end
    if #matchingTags > 0 then
      table.insert(children, ui.column({ gap = 4 }, matchingTags))
    elseif wantedTag ~= "" then
      table.insert(children, ui.label({ text = "No existing tags found. Add a new one in Assigned tags.", color = "on_surface/0.55", maxLines = 1 }))
    end
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

  panel.render(ui.column({ padding = 16, gap = 10, fill = "surface", flexGrow = 1, align = "stretch" }, children))
end

function beginEdit(bookmark)
  editId = bookmark.id
  editRevision = bookmark.revision
  deletePendingId = nil
  addTitle = bookmark.title or ""
  addUrl = bookmark.url or ""
  addNote = bookmark.note or ""
  addTags = table.concat(bookmark.tags or {}, ", ")
  tagSearch = ""
  viewMode = "add"
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
  tagSearch = ""
  viewMode = "add"
  focusArea = "form"
  formRevision = formRevision + 1
  setMessage("", false)
  renderPanel()
end

function cancelEdit()
  clearForm()
  viewMode = "bookmarks"
  focusArea = "search"
  setMessage("", false)
  renderPanel()
end

function selectTab(tab)
  if tab ~= "bookmarks" and tab ~= "add" and tab ~= "settings" then return end
  if tab == "add" then
    startAdd()
    return
  end
  if viewMode == "add" then clearForm() end
  openWithId = nil
  viewMode = tab
  focusArea = "search"
  setMessage("", false)
  renderPanel()
end

function toggleFormTag(tag)
  local tags = selectedTagList()
  local wanted = string.lower(noctalia.string.trim(tag))
  if wanted == "" then return end
  local updated = {}
  local removed = false
  for _, selectedTag in ipairs(tags) do
    if string.lower(selectedTag) == wanted then
      removed = true
    else
      table.insert(updated, selectedTag)
    end
  end
  if not removed then table.insert(updated, noctalia.string.trim(tag)) end
  addTags = table.concat(updated, ", ")
  tagSearch = ""
  renderPanel()
end

function onTagSearchChanged(value)
  tagSearch = type(value) == "string" and value or ""
  renderPanel()
end

function onFormTagsChanged(value)
  addTags = type(value) == "string" and value or ""
  renderPanel()
end

function assignSearchedTag(value)
  local candidate = noctalia.string.trim(type(value) == "string" and value or tagSearch)
  if candidate == "" then return end
  for index = 2, #tagOptions do
    if string.lower(tagOptions[index]) == string.lower(candidate) then
      toggleFormTag(tagOptions[index])
      return
    end
  end
  setMessage("Choose an existing tag; add new tags in Assigned tags", true)
  renderPanel()
end

function showOpenWith(bookmarkId)
  if busy then return end
  openWithId = openWithId == bookmarkId and nil or bookmarkId
  renderPanel()
end

function cancelOpenWith()
  openWithId = nil
  renderPanel()
end

function openWithSelectedBrowser()
  if openWithId == nil or browsers[browserIndex + 1] == nil or busy then return end
  for _, bookmark in ipairs(bookmarks) do
    if bookmark.id == openWithId then
      openBookmark(openWithId, true)
      return
    end
  end
  openWithId = nil
  setMessage("Bookmark is no longer in the current results", true)
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
  openWithId = nil
  searchBookmarks()
end

local function cycleBrowser(delta)
  if #browsers == 0 then return end
  browserIndex = (browserIndex + delta) % #browsers
  rebuildProfiles()
  renderPanel()
end

local function cycleProfile(delta)
  if #profileOptions == 0 then return end
  profileIndex = (profileIndex + delta) % #profileOptions
  renderPanel()
end

local function cycleTag(delta)
  if #tagOptions == 0 then return end
  tagIndex = (tagIndex + delta) % #tagOptions
  searchBookmarks()
end

function onQueryChanged(value)
  query = type(value) == "string" and value or ""
  openWithId = nil
  viewMode = "bookmarks"
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
    renderPanel()
  end)
end

function searchBookmarks()
  openWithId = nil
  searchGeneration = searchGeneration + 1
  local requestGeneration = searchGeneration
  busy = true
  bookmarks = {}
  selectedIndex = 0
  setMessage("", false)
  renderPanel()
  if searchInFlight then
    pendingSearch = true
    return
  end
  searchInFlight = true
  local arguments = { "list", "--query", noctalia.string.trim(query), "--limit", "50", "--json" }
  if tagIndex > 0 and tagOptions[tagIndex + 1] ~= nil then
    table.insert(arguments, #arguments, "--tag")
    table.insert(arguments, #arguments, tagOptions[tagIndex + 1])
  end
  helper.run(arguments, function(data, err)
    searchInFlight = false
    if requestGeneration ~= searchGeneration or pendingSearch then
      pendingSearch = false
      searchBookmarks()
      return
    end
    busy = false
    if err ~= nil then
      bookmarks = {}
      setMessage(err, true)
    else
      bookmarks = data.bookmarks or {}
      selectedIndex = #bookmarks > 0 and 1 or 0
      if #bookmarks == 0 then focusArea = "search" end
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
    tagSearch = ""
    editId = nil
    editRevision = nil
    formRevision = formRevision + 1
    setMessage("Bookmark saved", false)
    viewMode = "bookmarks"
    focusArea = "search"
    loadTags(function() searchBookmarks() end)
  end)
end

function onOpen(_context)
  panelSession = panelSession + 1
  viewMode = "bookmarks"
  focusArea = "search"
  openingId = nil
  openWithId = nil
  searchGeneration = searchGeneration + 1
  pendingSearch = false
  loadBrowsers()
  loadTags()
  searchBookmarks()
end

function onClose()
  panelSession = panelSession + 1
  openingId = nil
  openWithId = nil
end

function onKey(chord, pressed)
  if not pressed then return end
  local key = tostring(chord):lower()
  if key == "ctrl+1" or key == "ctrl+f" then
    selectTab("bookmarks")
  elseif key == "ctrl+2" or key == "ctrl+n" then
    selectTab("add")
  elseif key == "ctrl+3" then
    selectTab("settings")
  elseif key == "down" and viewMode == "bookmarks" and #bookmarks > 0 then
    selectedIndex = selectedIndex % #bookmarks + 1; renderPanel()
  elseif key == "up" and viewMode == "bookmarks" and #bookmarks > 0 then
    selectedIndex = (selectedIndex - 2) % #bookmarks + 1; renderPanel()
  elseif key == "ctrl+down" and viewMode == "bookmarks" then
    cycleTag(1)
  elseif key == "ctrl+up" and viewMode == "bookmarks" then
    cycleTag(-1)
  elseif (key == "down" or key == "right") and viewMode == "settings" then
    if key == "down" then cycleBrowser(1) else cycleProfile(1) end
  elseif (key == "up" or key == "left") and viewMode == "settings" then
    if key == "up" then cycleBrowser(-1) else cycleProfile(-1) end
  elseif (key == "return" or key == "enter") and viewMode == "bookmarks" and openWithId ~= nil then
    openWithSelectedBrowser()
  elseif (key == "return" or key == "enter" or key == "ctrl+o") and viewMode == "bookmarks" and not busy and selectedIndex > 0 then
    openBookmark(bookmarks[selectedIndex].id)
  elseif (key == "return" or key == "enter") and viewMode == "settings" then
    setDefaultBrowser()
  elseif (key == "ctrl+return" or key == "ctrl+enter") and viewMode == "add" then
    saveBookmark()
  elseif key == "escape" then
    if viewMode == "bookmarks" then panel.close() else selectTab("bookmarks") end
  end
end
