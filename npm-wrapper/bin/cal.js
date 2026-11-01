#!/usr/bin/env node
"use strict";

// Calabrass CLI. A guest of the Calabrass host: attaches to the one running on
// this machine, starts it in the background if there is none, and never stops
// it behind your back. See lib/launcher.js.

const fs = require("fs");
const path = require("path");
const { spawn, spawnSync } = require("child_process");
const launcher = require("../lib/launcher");
const remoteLib = require("../lib/remote");

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

Using a host on another machine:
  cal pair                    (on the host) make a one-time code for a new device
  cal connect <addr> <code>   (on the device) pair with a host, e.g. 192.168.1.20:8787
  cal devices                 (on the host) list paired devices
  cal revoke <id|name>        (on the host) remove a device; it is signed out at once
  cal disconnect              (on the device) go back to this machine's own host
  cal --local                 use this machine's host once, even while connected
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

Another machine can use a host only after you pair it: start the host with
CALABRASS_ALLOW_REMOTE=1, run 'cal pair' there, and 'cal connect' on the device.
The connection is plain HTTP, so use it at home, over an SSH tunnel, or on a VPN.

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

const fail = (message) => { console.error("calabrass: " + message); process.exit(1); };

// "--name x", "--name=x" and a bare flag, out of the arguments after the command.
function flagValue(args, name) {
  for (let i = 0; i < args.length; i++) {
    if (args[i] === `--${name}` && args[i + 1]) return args[i + 1];
    if (args[i].startsWith(`--${name}=`)) return args[i].slice(name.length + 3);
  }
  return "";
}

function ago(ms) {
  if (!ms) return "never";
  const s = Math.max(0, Math.round((Date.now() - ms) / 1000));
  if (s < 90) return "just now";
  if (s < 5400) return Math.round(s / 60) + " min ago";
  if (s < 129600) return Math.round(s / 3600) + " h ago";
  return Math.round(s / 86400) + " days ago";
}

async function runPair() {
  try {
    const p = await remoteLib.pair({ dir: launcher.dataDir() });
    console.log(`\nPairing code:  ${p.code}   (works once, for 10 minutes)\n`);
    if (p.reach.length) {
      console.log("On the other machine run:");
      for (const a of p.reach.slice(0, 3)) console.log(`  cal connect ${a} ${p.code}`);
      console.log("\nThat is plain HTTP: fine on a home network; use an SSH tunnel or a VPN for anything else.");
    } else {
      console.log("This host only listens on this machine, so another machine can reach it only through an");
      console.log("SSH tunnel:  ssh -L 8787:127.0.0.1:8787 you@this-machine  , then on that machine:");
      console.log(`  cal connect 127.0.0.1:8787 ${p.code}`);
    }
    console.log("\nSee who is paired with: cal devices");
  } catch (e) {
    fail(e.message);
  }
}

async function runDevices() {
  try {
    const list = await remoteLib.devices({ dir: launcher.dataDir() });
    if (!list.length) { console.log("No devices are paired. Make a code with: cal pair"); return; }
    console.log("ID        NAME                          PAIRED        LAST SEEN");
    for (const d of list) {
      console.log(`${d.id.slice(0, 8).padEnd(9)} ${String(d.name).slice(0, 28).padEnd(29)} ${ago(d.createdAt).padEnd(13)} ${ago(d.lastSeenAt)}`);
    }
    console.log("\nRemove one with: cal revoke <id or name>");
  } catch (e) {
    fail(e.message);
  }
}

async function runRevoke(args) {
  try {
    const d = await remoteLib.revoke({ dir: launcher.dataDir(), which: args.find((a) => !a.startsWith("--")) });
    console.log(`Removed "${d.name}". It is signed out and cannot use this host again unless it is paired anew.`);
  } catch (e) {
    fail(e.message);
  }
}

async function runConnect(args) {
  const positional = args.filter((a, i) => !a.startsWith("--") && !(i > 0 && args[i - 1] === "--name"));
  try {
    const { remote, warning } = await remoteLib.connect({
      addr: positional[0], code: positional[1], name: flagValue(args, "name"),
      dir: launcher.dataDir(), insecure: args.includes("--insecure"),
    });
    console.log(`Paired with ${remote.addr} as "${remote.name}".`);
    if (warning) console.error("\ncalabrass: " + warning + "\n");
    console.log("`cal` and `cal run` now use that host. Go back to this machine's own with: cal disconnect");
  } catch (e) {
    fail(e.message);
  }
}

function runDisconnect() {
  const dir = launcher.dataDir();
  const r = remoteLib.readRemote(dir);
  if (!remoteLib.clearRemote(dir)) { console.log("Not connected to another host."); return; }
  console.log(`Disconnected from ${r ? r.addr : "the remote host"}. \`cal\` uses this machine's own host again.`);
  console.log("(The host still lists this device until you remove it there with: cal revoke)");
}

async function main() {
  const parsed = launcher.parseArgs(process.argv.slice(2));
  const { command } = parsed;
  let args = parsed.args;

  switch (command) {
    case "version": console.log(getVersion()); return;
    case "help": printHelp(); return;
    case "uninstall": printUninstallHelp(); return;
    case "pair": await runPair(); return;
    case "devices": await runDevices(); return;
    case "revoke": await runRevoke(args); return;
    case "connect": await runConnect(args); return;
    case "disconnect": runDisconnect(); return;
    case "status": {
      const remote = remoteLib.readRemote(launcher.dataDir());
      if (remote && !args.includes("--local")) {
        const up = await launcher.healthy(remote.addr, 3000);
        if (!up) {
          console.log(`Paired with ${remote.addr} as "${remote.name}", but that host is NOT reachable.`);
          process.exit(1);
        }
        // Reachable is not the same as welcome: the host may have removed this device.
        const me = await launcher.requestJSON(remote.addr, "GET", "/api/auth/me", { headers: { Authorization: "Bearer " + remote.token } });
        if (me.status === 401 || me.status === 403) {
          console.log(`Reachable, but ${remote.addr} no longer accepts this device (it was removed, or the pairing expired).`);
          console.log("Pair again with `cal connect <address> <code>`, or go back with `cal disconnect`.");
          process.exit(1);
        }
        console.log(`Paired with ${remote.addr} as "${remote.name}": reachable and accepted.`);
        console.log("(Use `cal status --local` for this machine's own host.)");
        process.exit(0);
      }
      process.exit(hostCommand("status"));
    }
    case "stop": process.exit(hostCommand("stop"));
  }

  if (!fs.existsSync(tuiBinary)) {
    console.error(`calabrass: no terminal UI binary for ${process.platform}/${process.arch}.`);
    console.error("Supported platforms: linux, darwin, win32 (x64 or arm64). Try: npm install -g calabrass --force");
    process.exit(1);
  }

  // --local: use this machine's own host once, even while paired with another.
  const useLocal = args.includes("--local");
  args = args.filter((a) => a !== "--local");

  // Paired with a host on another machine: use it, and never start one here.
  const remote = useLocal || launcher.addrFromArgs(args) ? null : remoteLib.readRemote(launcher.dataDir());
  if (remote) {
    if (!(await launcher.healthy(remote.addr, 4000))) {
      fail(`can't reach ${remote.addr}, the host this device is paired with. ` +
        "Is it running and reachable? Use `cal --local` for this machine's own host, or `cal disconnect`.");
    }
    return runUI(args, { addr: remote.addr, started: false }, { CALABRASS_TOKEN: remote.token });
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
  return runUI(args, host, {});
}

// Starts the terminal UI against a host, and exits with its exit code.
async function runUI(args, host, extraEnv) {
  if (host.started) {
    // stderr, not stdout: `cal run` is piped, and the answer is all stdout may carry.
    console.error("calabrass: the host keeps running after you close this terminal, so scheduled tasks still happen. Stop it with: cal stop");
  }

  // First run: tell a person with no model what to do, before the screen opens.
  // Only for the interactive UI; `cal run` reports it through its exit code.
  if (args[0] !== "run" && !extraEnv.CALABRASS_TOKEN && process.stderr.isTTY) {
    const hint = await launcher.modelSetupHint({ addr: host.addr, dir: launcher.dataDir() });
    if (hint) console.error("\ncalabrass: " + hint.split("\n").join("\ncalabrass: ") + "\n");
  }

  const tui = spawn(tuiBinary, args, {
    stdio: "inherit",
    env: {
      ...process.env,
      ...extraEnv,
      CALABRASS_ADDR: host.addr,
      // So the UI can start the host again if it stops while the UI is open, instead
      // of leaving the person to do it. Only set when this install has one.
      // Never for a host on another machine: that one is not this terminal's to start.
      ...(!extraEnv.CALABRASS_TOKEN && fs.existsSync(backendBinary) ? { CALABRASS_BACKEND_BIN: backendBinary } : {}),
    },
  });
  // A signal goes to the terminal UI, never to the host.
  for (const sig of ["SIGINT", "SIGTERM"]) process.on(sig, () => { if (!tui.killed) tui.kill(sig); });
  tui.on("error", (e) => { console.error("calabrass: could not start the terminal UI:", e.message); process.exit(1); });
  tui.on("exit", (code) => process.exit(code === null ? 1 : code));
}

main().catch((e) => { console.error("calabrass:", e.message); process.exit(1); });
