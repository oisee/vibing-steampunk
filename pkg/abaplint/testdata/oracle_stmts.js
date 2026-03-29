#!/usr/bin/env node
// Oracle script: runs the real abaplint parser on ABAP files and outputs statement-level JSON.
// Usage: node oracle_stmts.js <file.abap> [file2.abap ...]
// Output: JSON array of {file, statements: [{type, tokens: [str...], colon: bool}]}
//
// Requires: npm install @abaplint/core

const {Registry, MemoryFile} = require("@abaplint/core");
const fs = require("fs");
const path = require("path");

const files = process.argv.slice(2);
if (files.length === 0) {
  console.error("Usage: node oracle_stmts.js <file.abap> [file2.abap ...]");
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
  const regName = basename.endsWith(".prog.abap") || basename.endsWith(".clas.abap") ||
                  basename.endsWith(".intf.abap") || basename.endsWith(".fugr.abap")
    ? basename : basename.replace(".abap", ".prog.abap");

  const reg = new Registry();
  reg.addFile(new MemoryFile(regName, source));
  reg.parse();

  let stmts = [];
  for (const o of reg.getObjects()) {
    if (!o.getABAPFiles) continue;
    for (const af of o.getABAPFiles()) {
      for (const s of af.getStatements()) {
        stmts.push({
          type: s.get().constructor.name,
          tokens: s.getTokens().map(t => t.getStr()),
          colon: s.getColon() !== undefined,
        });
      }
    }
  }

  results.push({file: basename, statement_count: stmts.length, statements: stmts});
}

console.log(JSON.stringify(results));
