--!nonstrict

local helper = require("./shared.luau")
local generation = 0
local visible = {}

local function statusResult(title, glyph)
  return { { id = "__status__", title = title, glyph = glyph or "info-circle" } }
end

function onQuery(text)
  generation = generation + 1
  local requestGeneration = generation
  local query = noctalia.string.trim(text or "")
  launcher.setResults(text, statusResult("Searching bookmarks…", "loader"))

  helper.run({ "list", "--query", query, "--limit", "50", "--json" }, function(data, err)
    if requestGeneration ~= generation then
      return
    end
    if err ~= nil then
      visible = {}
      launcher.setResults(text, statusResult(err, "alert-circle"))
      return
    end

    local results = {}
    visible = {}
    for _, bookmark in ipairs(data.bookmarks or {}) do
      visible[bookmark.id] = true
      table.insert(results, {
        id = bookmark.id,
        title = bookmark.title ~= "" and bookmark.title or bookmark.url,
        subtitle = bookmark.url,
        glyph = "bookmark",
      })
    end
    if #results == 0 then
      results = statusResult(query == "" and "No bookmarks yet" or "No matching bookmarks", "bookmark-off")
    end
    launcher.setResults(text, results)
  end)
end

function onActivate(id)
  if visible[id] ~= true then
    return
  end
  helper.run(helper.openArguments(id), function(_data, err)
    if err ~= nil then
      noctalia.notifyError("Waymarks", err)
    end
  end)
end

