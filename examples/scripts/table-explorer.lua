-- table-explorer.lua
-- Interactive table explorer — query SAP tables from Lua
--
-- Usage: vsp -s dev lua examples/scripts/table-explorer.lua

print("=== SAP Table Explorer ===")
print()

-- Query T000 (clients)
print("--- Clients ---")
local clients = query("SELECT MANDT, MTEXT, ORT01 FROM T000")
if clients then
    for _, row in ipairs(clients) do
        print("  " .. row.MANDT .. "  " .. row.MTEXT .. "  " .. row.ORT01)
    end
end
print()

-- Query custom objects in $TMP
print("--- Custom Classes in $TMP ---")
local objects = query("SELECT OBJ_NAME FROM TADIR WHERE DEVCLASS = '$TMP' AND OBJECT = 'CLAS'", 10)
if objects then
    print("  " .. #objects .. " classes")
    for i, row in ipairs(objects) do
        if i <= 5 then
            print("  " .. row.OBJ_NAME)
        end
    end
    if #objects > 5 then print("  ... +" .. (#objects - 5) .. " more") end
end
print()

-- Query data dictionary for a table
print("--- MARA Table Structure (first 10 fields) ---")
local fields = query("SELECT FIELDNAME, ROLLNAME, DATATYPE, LENG FROM DD03L WHERE TABNAME = 'MARA'", 10)
if fields then
    for _, f in ipairs(fields) do
        print("  " .. f.FIELDNAME .. "  " .. f.ROLLNAME .. "  " .. f.DATATYPE .. "(" .. f.LENG .. ")")
    end
end
