#!/usr/bin/env node
// Oracle script: runs the real abaplint lexer on ABAP files and outputs JSON.
// Usage: node oracle.js <file.abap> [file2.abap ...]
// Output: JSON array of {file, token_count, tokens: [{str, type, row, col}]}
//
// Requires: npm install @abaplint/core
//
// Deduplicates by basename — last file wins.

const {Registry, MemoryFile} = require("@abaplint/core");
const fs = require("fs");
const path = require("path");

const files = process.argv.slice(2);
if (files.length === 0) {
  console.error("Usage: node oracle.js <file.abap> [file2.abap ...]");
  process.exit(1);
}

// Deduplicate by basename (last wins)
const byName = new Map();
for (const file of files) {
  byName.set(path.basename(file), file);
}

const results = [];

for (const [basename, file] of byName) {
  const source = fs.readFileSync(file, "utf-8");
  // Use proper extension for Registry to recognize object type
  const regName = basename.endsWith(".prog.abap") || basename.endsWith(".clas.abap") ||
                  basename.endsWith(".intf.abap") || basename.endsWith(".fugr.abap")
    ? basename : basename.replace(".abap", ".prog.abap");

  const reg = new Registry();
  const memFile = new MemoryFile(regName, source);
  reg.addFile(memFile);
  reg.parse();

  let tokens = [];
  for (const o of reg.getObjects()) {
    if (!o.getABAPFiles) continue;
    for (const af of o.getABAPFiles()) {
      tokens = af.getTokens().map(t => ({
        str: t.getStr(),
        type: t.constructor.name,
        row: t.getRow(),
        col: t.getCol(),
      }));
    }
  }

  results.push({file: basename, token_count: tokens.length, tokens});
}

console.log(JSON.stringify(results));
