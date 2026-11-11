-- hd500_bridge.lua — development bridge between REAPER and the editor's
-- /dev/helix page (Helix Native used as the visual reference for the UI).
--
-- Run once per REAPER session: Actions › Show action list › New action ›
-- Load ReaScript… › this file › Run. It keeps running in the background
-- (Actions list shows it as running; run it again to stop it).
--
-- It talks to the editor through files in <REAPER resource path>/hd500-bridge:
--   state.json  written here: every FX on every track with its parameters
--               (name, normalized value, value as the plugin formats it)
--   cmd.txt     written by the editor, one command per line:
--                 set <track> <fx> <param> <normalized 0..1>
--                 show <track> <fx>      (open the FX in a floating window)
--                 hide <track> <fx>
--               tracks are 0-based, -1 = master.

local dir = reaper.GetResourcePath() .. "/hd500-bridge"
reaper.RecursiveCreateDirectory(dir, 0)
local stateFile, cmdFile = dir .. "/state.json", dir .. "/cmd.txt"

local function track(i)
  if i < 0 then return reaper.GetMasterTrack(0) end
  return reaper.GetTrack(0, i)
end

local function esc(s)
  return (s:gsub('[%c"\\]', function(c)
    if c == '"' then return '\\"' elseif c == "\\" then return "\\\\" end
    return string.format("\\u%04x", c:byte())
  end))
end

local function runCommands()
  -- take the file away first (atomic), so lines the editor appends meanwhile aren't lost
  local work = cmdFile .. ".work"
  if not os.rename(cmdFile, work) then return end
  local f = io.open(work, "r")
  if not f then return end
  local text = f:read("a")
  f:close()
  os.remove(work)
  for line in text:gmatch("[^\n]+") do
    local op, a, b, c, d = line:match("^(%a+)%s+(%-?%d+)%s+(%d+)%s*(%d*)%s*([%d%.eE%-]*)")
    local tr = op and track(tonumber(a))
    if tr then
      local fx = tonumber(b)
      if op == "set" and c ~= "" and d ~= "" then
        reaper.TrackFX_SetParamNormalized(tr, fx, tonumber(c), tonumber(d))
      elseif op == "show" then
        reaper.TrackFX_Show(tr, fx, 3)
      elseif op == "hide" then
        reaper.TrackFX_Show(tr, fx, 2)
      end
    end
  end
end

local function stateJSON()
  local out = {}
  for t = -1, reaper.CountTracks(0) - 1 do
    local tr = track(t)
    local _, tname = reaper.GetTrackName(tr)
    for fx = 0, reaper.TrackFX_GetCount(tr) - 1 do
      local _, fxname = reaper.TrackFX_GetFXName(tr, fx, "")
      local params = {}
      for p = 0, reaper.TrackFX_GetNumParams(tr, fx) - 1 do
        local _, pname = reaper.TrackFX_GetParamName(tr, fx, p, "")
        local _, shown = reaper.TrackFX_GetFormattedParamValue(tr, fx, p, "")
        params[#params + 1] = string.format('{"i":%d,"name":"%s","value":%.6f,"shown":"%s"}',
          p, esc(pname), reaper.TrackFX_GetParamNormalized(tr, fx, p), esc(shown))
      end
      out[#out + 1] = string.format('{"track":%d,"trackName":"%s","fx":%d,"name":"%s","open":%s,"params":[%s]}',
        t, esc(tname), fx, esc(fxname), tostring(reaper.TrackFX_GetFloatingWindow(tr, fx) ~= nil),
        table.concat(params, ","))
    end
  end
  return '{"fx":[' .. table.concat(out, ",") .. ']}'
end

local last, lastTime = "", 0
local function loop()
  runCommands()
  local now = reaper.time_precise()
  if now - lastTime > 0.3 then
    lastTime = now
    local s = stateJSON()
    if s ~= last then
      local f = io.open(stateFile .. ".tmp", "w")
      if f then
        f:write(s)
        f:close()
        os.rename(stateFile .. ".tmp", stateFile)
        last = s
      end
    end
  end
  reaper.defer(loop)
end

reaper.atexit(function() os.remove(stateFile) end)
loop()
