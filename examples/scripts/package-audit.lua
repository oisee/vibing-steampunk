-- package-audit.lua
-- Audit a package: lint all objects, check dependencies, report quality
--
-- Usage: vsp -s dev lua examples/scripts/package-audit.lua

local PKG = "$ZADT_VSP"

print("=== Package Audit: " .. PKG .. " ===")
print()

-- 1. Find all classes in the package
local classes = searchObject(PKG .. "*", "CLAS")
print("Classes found: " .. #classes)
for _, c in ipairs(classes) do
    print("  " .. c.name)
end
print()

-- 2. Lint each class
print("=== Lint Results ===")
local total_issues = 0
for _, cls in ipairs(classes) do
    local source = getSource("CLAS", cls.name)
    if source then
        local issues = lint(source)
        if issues and #issues > 0 then
            print(cls.name .. ": " .. #issues .. " issues")
            for _, iss in ipairs(issues) do
                print("  r" .. iss.row .. " [" .. iss.key .. "] " .. iss.message)
            end
            total_issues = total_issues + #issues
        else
            print(cls.name .. ": clean")
        end
    end
end
print()
print("Total issues: " .. total_issues)

-- 3. Parse statistics
print()
print("=== Code Structure ===")
for _, cls in ipairs(classes) do
    local source = getSource("CLAS", cls.name)
    if source then
        local stmts = parse(source)
        local types = {}
        for _, s in ipairs(stmts) do
            types[s.type] = (types[s.type] or 0) + 1
        end
        print(cls.name .. ": " .. #stmts .. " statements")
        if types["MethodImplementation"] then
            print("  methods: " .. types["MethodImplementation"])
        end
        if types["Data"] then
            print("  data declarations: " .. types["Data"])
        end
    end
end

-- 4. System info
print()
print("=== System ===")
local info = systemInfo()
if info then
    print("System: " .. info.systemId .. " (" .. info.sapRelease .. ")")
end
