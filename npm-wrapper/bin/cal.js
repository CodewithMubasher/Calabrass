#!/usr/bin/env node
"use strict";

// Thin shim: finds the prebuilt binary in the platform package npm installed
// (calabrass-<os>-<arch>) and hands the terminal to it. Installed as `cal`.

const { spawnSync } = require("child_process");

const ext = process.platform === "win32" ? ".exe" : "";
const packageName = "calabrass-" + process.platform + "-" + process.arch;

let binary;
try {
  binary = require.resolve(packageName + "/bin/calabrass-tui" + ext);
} catch {
  console.error(
    "calabrass: no binary for " + process.platform + "/" + process.arch + "."
  );
  console.error(
    "Your platform may be unsupported, or npm skipped the optional package."
  );
  console.error("Try: npm install -g calabrass --force");
  process.exit(1);
}

const result = spawnSync(binary, process.argv.slice(2), { stdio: "inherit" });

if (result.error) {
  console.error("calabrass: could not start the TUI: " + result.error.message);
  process.exit(1);
}

process.exit(result.status === null ? 1 : result.status);
