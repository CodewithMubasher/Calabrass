"use strict";

// The `cal` launcher. Calabrass runs as one host per machine, and this is a
// guest of it: it looks for the host that is already running, starts a
// background one when there is none, and leaves it running when the terminal
// closes, so scheduled tasks keep happening. `cal stop` ends it.

const fs = require("fs");
const os = require("os");
const path = require("path");
const http = require("http");
const { spawn } = require("child_process");

const DEFAULT_ADDR = "127.0.0.1:8787";
const START_TIMEOUT_MS = 20000;

const COMMANDS = {
  version: ["--version", "-v", "version"],
  help: ["--help", "-h", "help"],
  uninstall: ["uninstall"],
  stop: ["stop"],
  status: ["status"],
};

// Where Calabrass keeps its data. It must agree with the backend's DataDir.
function dataDir(env = process.env, platform = process.platform) {
  if (env.CALABRASS_DATA_DIR && env.CALABRASS_DATA_DIR.trim()) return env.CALABRASS_DATA_DIR.trim();
  if (platform === "win32" && env.LOCALAPPDATA) return path.join(env.LOCALAPPDATA, "Calabrass");
  if (platform === "darwin") return path.join(os.homedir(), "Library", "Application Support", "Calabrass");
  const base = env.XDG_CONFIG_HOME && env.XDG_CONFIG_HOME.trim() ? env.XDG_CONFIG_HOME : path.join(os.homedir(), ".config");
  return path.join(base, "Calabrass");
}

function readHostFile(dir) {
  try {
    return JSON.parse(fs.readFileSync(path.join(dir, "host.json"), "utf8"));
  } catch {
    return null;
  }
}

function isLoopback(addr) {
  const host = String(addr).replace(/:\d+$/, "").replace(/^\[|\]$/g, "").toLowerCase();
  return host === "localhost" || host === "::1" || /^127\./.test(host);
}

// -addr, --addr, -addr=x, --addr=x anywhere in the arguments.
function addrFromArgs(args) {
  for (let i = 0; i < args.length; i++) {
    const a = args[i];
    const m = /^--?addr=(.+)$/.exec(a);
    if (m) return m[1];
    if ((a === "-addr" || a === "--addr") && args[i + 1]) return args[i + 1];
  }
  return "";
}

function parseArgs(argv) {
  const args = argv.slice();
  for (const [command, aliases] of Object.entries(COMMANDS)) {
    if (aliases.includes(args[0])) return { command, args: args.slice(1) };
  }
  return { command: "tui", args };
}

function healthy(addr, timeoutMs = 1500) {
  return new Promise((resolve) => {
    const req = http.get({ host: addr.replace(/:\d+$/, "").replace(/^\[|\]$/g, ""), port: Number(addr.split(":").pop()), path: "/api/health", timeout: timeoutMs }, (res) => {
      let body = "";
      res.on("data", (d) => (body += d));
      res.on("end", () => {
        try {
          resolve(res.statusCode === 200 && JSON.parse(body).status === "ok");
        } catch {
          resolve(false);
        }
      });
    });
    req.on("timeout", () => { req.destroy(); resolve(false); });
    req.on("error", () => resolve(false));
  });
}

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

// Finds the host this terminal should talk to, starting a background one only
// when there is none and the address is this machine's.
//
// Returns { addr, started }. Throws an Error whose message is for the person.
async function ensureHost({ explicitAddr = "", dir, backendBinary, spawnFn = spawn, log = () => {}, timeoutMs = START_TIMEOUT_MS }) {
  const wanted = (explicitAddr || process.env.CALABRASS_ADDR || "").trim();

  // A host on another machine is not ours to start or stop.
  if (wanted && !isLoopback(wanted)) {
    if (await healthy(wanted, 3000)) return { addr: wanted, started: false };
    throw new Error(`no Calabrass host answers at ${wanted}. It must be listening with a pairing code, and this terminal must be given it.`);
  }

  const info = readHostFile(dir);
  if (info && info.addr && (await healthy(info.addr))) return { addr: info.addr, started: false };
  // A host from before hosts announced themselves.
  const local = wanted || DEFAULT_ADDR;
  if (await healthy(local)) return { addr: local, started: false };

  if (!backendBinary || !fs.existsSync(backendBinary)) {
    throw new Error("the Calabrass host is not running and this install has no host binary to start. Try: npm install -g calabrass --force");
  }

  log("starting the Calabrass host in the background...");
  fs.mkdirSync(dir, { recursive: true });
  const out = fs.openSync(path.join(dir, "host.log"), "a");
  const child = spawnFn(backendBinary, ["--daemon"], {
    detached: true,
    stdio: ["ignore", out, out],
    windowsHide: true,
  });
  child.on && child.on("error", () => {});
  child.unref && child.unref();
  fs.closeSync(out);

  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    const now = readHostFile(dir);
    if (now && now.addr && (await healthy(now.addr))) return { addr: now.addr, started: true };
    await sleep(150);
  }
  throw new Error(`the Calabrass host did not start within ${Math.round(timeoutMs / 1000)}s. See ${path.join(dir, "host.log")}`);
}

module.exports = { DEFAULT_ADDR, COMMANDS, dataDir, readHostFile, isLoopback, addrFromArgs, parseArgs, healthy, ensureHost, spawn };
