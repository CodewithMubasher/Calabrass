#!/usr/bin/env node
"use strict";

// Turns the binaries the release workflow downloaded into npm packages:
// one `calabrass-<os>-<arch>` package per platform, plus the main package's
// version and optionalDependencies.
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
const platformsDir = path.join(root, "platforms");

const targets = {
  "linux-x64": { os: ["linux"], cpu: ["x64"] },
  "linux-arm64": { os: ["linux"], cpu: ["arm64"] },
  "darwin-x64": { os: ["darwin"], cpu: ["x64"] },
  "darwin-arm64": { os: ["darwin"], cpu: ["arm64"] },
  "win32-x64": { os: ["win32"], cpu: ["x64"] },
  "win32-arm64": { os: ["win32"], cpu: ["arm64"] },
};

const repository = {
  type: "git",
  url: "git+https://github.com/CodewithMubasher/Calabrass.git",
  directory: "TUI",
};

const optionalDependencies = {};

for (const [suffix, target] of Object.entries(targets)) {
  const name = "calabrass-" + suffix;
  const dir = path.join(platformsDir, name);
  const binDir = path.join(dir, "bin");

  if (!fs.existsSync(binDir) || fs.readdirSync(binDir).length === 0) {
    console.error("prepare: missing binary for " + name + " (expected in " + binDir + ")");
    process.exit(1);
  }

  const pkg = {
    name,
    version,
    description: "Calabrass TUI binary for " + suffix,
    os: target.os,
    cpu: target.cpu,
    files: ["bin"],
    repository,
    license: "MIT",
  };
  fs.writeFileSync(path.join(dir, "package.json"), JSON.stringify(pkg, null, 2) + "\n");
  optionalDependencies[name] = version;
}

const mainPath = path.join(root, "package.json");
const main = JSON.parse(fs.readFileSync(mainPath, "utf8"));
main.version = version;
main.optionalDependencies = optionalDependencies;
fs.writeFileSync(mainPath, JSON.stringify(main, null, 2) + "\n");

console.log("prepare: " + Object.keys(targets).length + " platform packages at " + version);
