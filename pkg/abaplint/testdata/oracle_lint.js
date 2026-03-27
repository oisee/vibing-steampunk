#!/usr/bin/env node
// Oracle: runs abaplint rules on ABAP files, outputs JSON issues.
// Usage: node oracle_lint.js <file.abap> [file2.abap ...]
// Uses rules that match our Go linter: empty_statement, obsolete_statement,
// preferred_compare_operator, line_length, max_one_statement, local_variable_names

const {Registry, MemoryFile, Config} = require("@abaplint/core");
const fs = require("fs");
const path = require("path");

const files = process.argv.slice(2);
if (files.length === 0) {
  console.error("Usage: node oracle_lint.js <file.abap> [file2.abap ...]");
  process.exit(1);
}

const byName = new Map();
for (const file of files) {
  byName.set(path.basename(file), file);
}

const ruleConf = {
  global: {files: "/src/**/*.*"},
  syntax: {version: "v702", errorNamespace: ""},
  rules: {
    empty_statement: true,
    obsolete_statement: {
      compute: true, add: true, subtract: true,
      multiply: true, divide: true, move: true, refresh: true,
    },
    preferred_compare_operator: {
      badOperators: ["EQ", "><", "NE", "GE", "GT", "LT", "LE"],
    },
    line_length: {length: 120},
    max_one_statement: true,
    // local_variable_names needs structure parsing — skip for now
  },
};

const results = [];

for (const [basename, file] of byName) {
  const source = fs.readFileSync(file, "utf-8");
  const regName = basename.endsWith(".prog.abap") || basename.endsWith(".clas.abap") ||
                  basename.endsWith(".intf.abap") || basename.endsWith(".fugr.abap")
    ? basename : basename.replace(".abap", ".prog.abap");

  const reg = new Registry(new Config(JSON.stringify(ruleConf)));
  reg.addFile(new MemoryFile(regName, source));
  reg.parse();
  const issues = reg.findIssues();

  const mapped = issues.map(i => ({
    key: i.getKey(),
    message: i.getMessage(),
    row: i.getStart().getRow(),
    col: i.getStart().getCol(),
  }));

  results.push({file: basename, issue_count: mapped.length, issues: mapped});
}

console.log(JSON.stringify(results));
