#!/usr/bin/env node
"use strict";

// Thin shim: runs the prebuilt binary bundled for this platform and hands it
// the terminal. Installed as `cal`.

const fs = require("fs");
const path = require("path");
const { spawnSync } = require("child_process");

const ext = process.platform === "win32" ? ".exe" : "";
const platformDir = "calabrass-" + process.platform + "-" + process.arch;
const binary = path.join(__dirname, "..", "binaries", platformDir, "calabrass-tui" + ext);

if (!fs.existsSync(binary)) {
  console.error(
    "calabrass: no binary for " + process.platform + "/" + process.arch + "."
  );
  console.error("Supported platforms: linux, darwin, win32 (x64 or arm64).");
  console.error("This install may be incomplete; try: npm install -g calabrass --force");
  process.exit(1);
}

const result = spawnSync(binary, process.argv.slice(2), { stdio: "inherit" });

if (result.error) {
  console.error("calabrass: could not start the TUI: " + result.error.message);
  process.exit(1);
}

process.exit(result.status === null ? 1 : result.status);
