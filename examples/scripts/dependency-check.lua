-- dependency-check.lua
-- Check if a class can be transported by analyzing its dependencies
--
-- Usage: vsp -s dev lua examples/scripts/dependency-check.lua

local CLASS = "ZCL_VSP_APC_HANDLER"

print("=== Dependency Check: " .. CLASS .. " ===")
print()

-- Get source with compressed context
local ctx = context("CLAS", CLASS, 10)
if ctx then
    -- Count lines
    local lines = 0
    for _ in ctx:gmatch("\n") do lines = lines + 1 end
    print("Source + context: " .. lines .. " lines")
end
print()

-- Parse the source for structure info
local source = getSource("CLAS", CLASS)
if source then
    local stmts = parse(source)
    local types = {}
    for _, s in ipairs(stmts) do
        types[s.type] = (types[s.type] or 0) + 1
    end
    print("Structure:")
    print("  Statements: " .. #stmts)
    for t, c in pairs(types) do
        if c > 1 then
            print("  " .. t .. ": " .. c)
        end
    end
end
print()

-- Check what this class uses via WBCROSSGT
print("Uses (WBCROSSGT):")
local refs = query(
    "SELECT OTYPE, NAME FROM WBCROSSGT WHERE INCLUDE LIKE '" .. CLASS .. "%'",
    100
)
if refs then
    local custom = {}
    local sap = {}
    local seen = {}
    for _, r in ipairs(refs) do
        local name = r.NAME
        -- Skip component refs and self
        if not name:find("\\") and name ~= CLASS and not seen[name] then
            seen[name] = true
            if name:sub(1,1) == "Z" or name:sub(1,1) == "Y" then
                table.insert(custom, r.OTYPE .. " " .. name)
            else
                table.insert(sap, r.OTYPE .. " " .. name)
            end
        end
    end

    if #custom > 0 then
        print("  Custom (" .. #custom .. "):")
        for _, c in ipairs(custom) do
            print("    " .. c)
        end
    end
    print("  SAP standard: " .. #sap .. " refs")

    if #custom == 0 then
        print()
        print("✓ No external custom dependencies — safe to transport alone")
    else
        print()
        print("⚠ Has " .. #custom .. " custom dependencies — check transport order")
    end
end

-- Lint the source
print()
print("Lint check:")
if source then
    local issues = lint(source)
    if issues and #issues > 0 then
        print("  " .. #issues .. " issues found")
        for _, iss in ipairs(issues) do
            print("  " .. iss.severity:sub(1,1) .. " [" .. iss.key .. "] " .. iss.message)
        end
    else
        print("  ✓ Clean")
    end
end
