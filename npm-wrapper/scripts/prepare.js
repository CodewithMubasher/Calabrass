#!/usr/bin/env node
"use strict";

// Stamps the main package with the release version and checks that every
// platform binary the release workflow downloaded is present.
//
// Run by CI before publishing, but safe to run locally:
//   node scripts/prepare.js 1.0.5

const fs = require("fs");
const path = require("path");

const version = process.argv[2];
if (!version || !/^\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$/.test(version)) {
  console.error("usage: node scripts/prepare.js <version>   (e.g. 1.0.5)");
  process.exit(1);
}

const root = path.join(__dirname, "..");
const binariesDir = path.join(root, "binaries");

const targets = [
  "calabrass-linux-x64",
  "calabrass-linux-arm64",
  "calabrass-darwin-x64",
  "calabrass-darwin-arm64",
  "calabrass-win32-x64",
  "calabrass-win32-arm64",
];

const missing = [];
for (const target of targets) {
  const dir = path.join(binariesDir, target);
  const exe = path.join(dir, "calabrass-tui" + (target.includes("win32") ? ".exe" : ""));
  if (!fs.existsSync(exe)) missing.push(path.relative(root, exe));
}
if (missing.length) {
  console.error("prepare: missing binaries:\n  " + missing.join("\n  "));
  process.exit(1);
}

const mainPath = path.join(root, "package.json");
const main = JSON.parse(fs.readFileSync(mainPath, "utf8"));
main.version = version;
delete main.optionalDependencies;
fs.writeFileSync(mainPath, JSON.stringify(main, null, 2) + "\n");

console.log("prepare: " + targets.length + " binaries bundled, version " + version);
