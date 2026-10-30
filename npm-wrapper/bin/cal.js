#!/usr/bin/env node
"use strict";

// Calabrass CLI. A guest of the Calabrass host: attaches to the one running on
// this machine, starts it in the background if there is none, and never stops
// it behind your back. See lib/launcher.js.

const fs = require("fs");
const path = require("path");
const { spawn, spawnSync } = require("child_process");
const launcher = require("../lib/launcher");

const ext = process.platform === "win32" ? ".exe" : "";
const platformDir = "calabrass-" + process.platform + "-" + process.arch;
const binDir = path.join(__dirname, "..", "binaries", platformDir);
const tuiBinary = path.join(binDir, "calabrass-tui" + ext);
const backendBinary = path.join(binDir, "calabrass-backend" + ext);
const packageJson = path.join(__dirname, "..", "package.json");

function getVersion() {
  try {
    return JSON.parse(fs.readFileSync(packageJson, "utf8")).version;
  } catch {
    return "unknown";
  }
}

function printHelp() {
  console.log(`calabrass ${getVersion()}

Terminal client for Calabrass — the open source personal AI agent.

Usage:
  cal [flags]                 Open the terminal UI (starts the host if needed)
  cal run "<task>" [flags]    Run one task and exit: for scripts, pipes and CI
                              (see: cal run --help)
  cal status                  Show whether the Calabrass host is running
  cal stop                    Stop the host (scheduled tasks stop with it)
  cal --version, -v           Show the version
  cal --help, -h              Show this help
  cal uninstall               Show uninstall instructions

Flags:
  -addr string    Host address (default: $CALABRASS_ADDR or the running host)
  -c, -continue   Continue the latest session of this folder

The folder you start 'cal' in is the workspace the agent works in.

One host runs per machine. The desktop app, this terminal and the web page are
all guests of it, so they share the same sessions, providers and schedules.
Closing the terminal leaves the host running, so scheduled tasks still happen;
'cal stop' ends it.

The agent asks before it runs commands. Answer in the terminal, or from any
other open Calabrass window. 'cal run' has nobody to ask, so it runs only the
commands you pre-approve with --allow, and exits 3 if it needed more.

Reaching a host on another machine needs its pairing code:
  CALABRASS_PAIRING_CODE=... on the host, and the same code in this terminal's
  environment (see the README).

For more information: https://github.com/CodewithMubasher/Calabrass`);
}

function printUninstallHelp() {
  console.log(`To uninstall the terminal client:

  npm uninstall -g calabrass

Your data is not touched: sessions, providers and workspaces stay in
  Windows: %LOCALAPPDATA%\\Calabrass
  macOS:   ~/Library/Application Support/Calabrass
  Linux:   ~/.config/Calabrass

If the background host is running, stop it first with:  cal stop`);
}

// Runs a subcommand of the host binary (status, stop) and returns its exit code.
function hostCommand(name) {
  if (!fs.existsSync(backendBinary)) {
    console.error("calabrass: this install has no host binary.");
    return 1;
  }
  const res = spawnSync(backendBinary, [name], { stdio: "inherit", env: process.env });
  return res.status === null ? 1 : res.status;
}

async function main() {
  const { command, args } = launcher.parseArgs(process.argv.slice(2));

  switch (command) {
    case "version": console.log(getVersion()); return;
    case "help": printHelp(); return;
    case "uninstall": printUninstallHelp(); return;
    case "status": process.exit(hostCommand("status"));
    case "stop": process.exit(hostCommand("stop"));
  }

  if (!fs.existsSync(tuiBinary)) {
    console.error(`calabrass: no terminal UI binary for ${process.platform}/${process.arch}.`);
    console.error("Supported platforms: linux, darwin, win32 (x64 or arm64). Try: npm install -g calabrass --force");
    process.exit(1);
  }

  let host;
  try {
    host = await launcher.ensureHost({
      explicitAddr: launcher.addrFromArgs(args),
      dir: launcher.dataDir(),
      backendBinary,
      log: (m) => console.error("calabrass: " + m),
    });
  } catch (err) {
    console.error("calabrass: " + err.message);
    process.exit(1);
  }
  if (host.started) {
    // stderr, not stdout: `cal run` is piped, and the answer is all stdout may carry.
    console.error("calabrass: the host keeps running after you close this terminal, so scheduled tasks still happen. Stop it with: cal stop");
  }

  // First run: tell a person with no model what to do, before the screen opens.
  // Only for the interactive UI; `cal run` reports it through its exit code.
  if (args[0] !== "run" && process.stderr.isTTY) {
    const hint = await launcher.modelSetupHint({ addr: host.addr, dir: launcher.dataDir() });
    if (hint) console.error("\ncalabrass: " + hint.split("\n").join("\ncalabrass: ") + "\n");
  }

  const tui = spawn(tuiBinary, args, {
    stdio: "inherit",
    env: { ...process.env, CALABRASS_ADDR: host.addr },
  });
  // A signal goes to the terminal UI, never to the host.
  for (const sig of ["SIGINT", "SIGTERM"]) process.on(sig, () => { if (!tui.killed) tui.kill(sig); });
  tui.on("error", (e) => { console.error("calabrass: could not start the terminal UI:", e.message); process.exit(1); });
  tui.on("exit", (code) => process.exit(code === null ? 1 : code));
}

main().catch((e) => { console.error("calabrass:", e.message); process.exit(1); });
