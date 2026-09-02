--!nonstrict

local M = {}
local operationCounter = 0

local function errorMessage(response, fallback)
  if type(response) == "table" and type(response.error) == "table" and type(response.error.message) == "string" then
    return response.error.message
  end
  return fallback
end

function M.operationId(scope)
  operationCounter = operationCounter + 1
  return scope .. "-" .. tostring(noctalia.nowMs()) .. "-" .. tostring(operationCounter)
end

function M.run(arguments, callback)
  if not noctalia.commandExists("zer0-waymarks-helper") then
    callback(nil, "zer0-waymarks-helper is not installed or is not on PATH")
    return
  end

  local argv = { "zer0-waymarks-helper" }
  for _, argument in ipairs(arguments) do
    table.insert(argv, argument)
  end

  local accepted = noctalia.runAsync(argv, function(result)
    if result.stdoutTruncated then
      callback(nil, "helper response was truncated")
      return
    end

    local response, decodeError = noctalia.json.decode(result.stdout or "")
    if response == nil then
      callback(nil, "helper returned invalid JSON: " .. tostring(decodeError))
      return
    end
    if result.exitCode ~= 0 or response.ok ~= true then
      callback(nil, errorMessage(response, "helper command failed"))
      return
    end
    callback(response.data, nil)
  end)

  if not accepted then
    callback(nil, "Noctalia could not start zer0-waymarks-helper")
  end
end

function M.openArguments(nodeId, browserId, profileId)
  local arguments = { "open", nodeId, "--json" }
  if type(browserId) == "string" and browserId ~= "" then
    table.insert(arguments, #arguments, "--browser")
    table.insert(arguments, #arguments, browserId)
  end
  local selectedProfile = profileId
  if type(selectedProfile) == "string" and selectedProfile ~= "" then
    table.insert(arguments, #arguments, "--profile")
    table.insert(arguments, #arguments, selectedProfile)
  end
  return arguments
end

return M
