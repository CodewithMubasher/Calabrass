"use strict";

// Reaching a Calabrass host on another machine.
//
//   on the machine the host runs on:   cal pair            -> a one-time code
//   on the other machine:              cal connect <addr> <code>
//
// `connect` trades the code for a device session and remembers it in remote.json,
// so `cal` and `cal run` there talk to that host instead of starting their own.
// `cal devices` lists the paired machines and `cal revoke` removes one, which
// signs it out at once. `cal disconnect` goes back to this machine's own host.

const fs = require("fs");
const os = require("os");
const path = require("path");
const { requestJSON, readHostFile, isLoopback } = require("./launcher");

const REMOTE_FILE = "remote.json";

function remotePath(dir) { return path.join(dir, REMOTE_FILE); }

function readRemote(dir) {
  try {
    const r = JSON.parse(fs.readFileSync(remotePath(dir), "utf8"));
    if (r && typeof r.addr === "string" && typeof r.token === "string" && r.addr && r.token) return r;
  } catch { /* none, or not readable */ }
  return null;
}

function writeRemote(dir, remote) {
  fs.mkdirSync(dir, { recursive: true });
  const tmp = remotePath(dir) + ".tmp";
  fs.writeFileSync(tmp, JSON.stringify(remote, null, 2), { mode: 0o600 });
  fs.renameSync(tmp, remotePath(dir));
}

function clearRemote(dir) {
  try { fs.unlinkSync(remotePath(dir)); return true; } catch { return false; }
}

// Plain HTTP carries the device's token and everything the agent reads. On this
// machine, or through an encrypted tunnel, that is fine. On a network of your own,
// it can be read by anyone on it. Across the internet it must not be done.
function assessAddr(addr) {
  const host = String(addr).replace(/:\d+$/, "").replace(/^\[|\]$/g, "").toLowerCase();
  if (isLoopback(addr)) return { level: "ok" };

  const v4 = /^(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})$/.exec(host);
  if (v4) {
    const [a, b] = [Number(v4[1]), Number(v4[2])];
    if (a === 100 && b >= 64 && b <= 127) return { level: "ok" }; // Tailscale / CGNAT: WireGuard already encrypts it
    if (a === 10 || (a === 172 && b >= 16 && b <= 31) || (a === 192 && b === 168) || (a === 169 && b === 254)) {
      return { level: "lan" };
    }
    return { level: "public" };
  }
  if (/^(fc|fd)[0-9a-f]{2}:/.test(host) || host.startsWith("fe80:")) return { level: "lan" };
  if (host.endsWith(".ts.net")) return { level: "ok" };
  if (host.endsWith(".local") || host.endsWith(".lan") || host.endsWith(".home.arpa") || !host.includes(".")) return { level: "lan" };
  return { level: "public" };
}

const LAN_WARNING =
  "This address is on a local network, over plain HTTP: anyone on that network can read what is sent, " +
  "including what the agent reads from your files. Fine at home; for anything shared, use an SSH tunnel " +
  "(ssh -L 8787:127.0.0.1:8787 you@host, then connect to 127.0.0.1:8787) or a VPN such as Tailscale.";

const PUBLIC_REFUSAL =
  "That address is on the open internet, and this connection is plain HTTP: your device token and everything " +
  "the agent reads would travel where anyone can read it. Use an SSH tunnel (ssh -L 8787:127.0.0.1:8787 you@host, " +
  "then connect to 127.0.0.1:8787) or a VPN such as Tailscale. If you understand the risk, add --insecure.";

function bodyError(res, fallback) {
  return (res.body && typeof res.body.error === "string" && res.body.error.slice(0, 400)) || fallback;
}

// Trade a one-time code for a device session and remember it.
async function connect({ addr, code, name, dir, insecure = false, request = requestJSON }) {
  addr = String(addr || "").trim().replace(/^https?:\/\//, "").replace(/\/+$/, "");
  if (!/^[^\s/:]+:\d{1,5}$|^\[[0-9a-f:]+\]:\d{1,5}$/i.test(addr)) {
    throw new Error("the address should look like 192.168.1.20:8787");
  }
  if (!String(code || "").trim()) throw new Error("give the pairing code that `cal pair` printed on the other machine");

  const assessed = assessAddr(addr);
  if (assessed.level === "public" && !insecure) throw new Error(PUBLIC_REFUSAL);

  const deviceName = (name || os.hostname() || "device").slice(0, 60);
  const res = await request(addr, "POST", "/api/pairing/claim", { body: { code, name: deviceName }, timeoutMs: 8000 });
  if (res.status === 0) throw new Error(`can't reach a Calabrass host at ${addr}. Is it running with CALABRASS_ALLOW_REMOTE=1, and is that address reachable from here?`);
  if (res.status === 404) throw new Error("that host does not accept other devices (CALABRASS_ALLOW_REMOTE is off)");
  if (res.status === 429) throw new Error("too many attempts; wait a minute and try again");
  if (res.status === 403) throw new Error("that pairing code is not valid, or it has expired. Run `cal pair` again on the host.");
  if (res.status !== 200 || !res.body || !res.body.token) throw new Error(bodyError(res, `the host answered ${res.status}`));

  const remote = {
    addr, token: res.body.token, name: res.body.device && res.body.device.name,
    deviceId: res.body.device && res.body.device.id, pairedAt: new Date().toISOString(),
  };
  writeRemote(dir, remote);
  return { remote, warning: assessed.level === "lan" ? LAN_WARNING : "" };
}

// Sign in as the machine's owner with the host's secret, for pair/devices/revoke.
async function ownerToken({ dir, request = requestJSON }) {
  const info = readHostFile(dir);
  if (!info || !info.addr || !info.secret) {
    throw new Error("no Calabrass host is running on this machine. Start it with `cal` (or open the desktop app), then try again.");
  }
  const res = await request(info.addr.replace(/^0\.0\.0\.0|^\[::\]|^:/, "127.0.0.1"), "POST", "/api/auth/local",
    { body: { secret: info.secret, create: false } });
  if (res.status === 409) throw new Error("nobody has an account on this host yet. Open Calabrass once (the app or `cal`) and set it up first.");
  if (res.status !== 200 || !res.body || !res.body.token) throw new Error(bodyError(res, "could not sign in to the local host"));
  return { token: res.body.token, addr: info.addr.replace(/^0\.0\.0\.0|^\[::\]|^:/, "127.0.0.1") };
}

// Addresses another machine could use to reach this host.
function reachableAddrs(listenAddr, interfaces = os.networkInterfaces()) {
  const port = String(listenAddr).split(":").pop();
  const host = String(listenAddr).replace(/:\d+$/, "").replace(/^\[|\]$/g, "");
  if (isLoopback(listenAddr)) return [];
  if (host && host !== "0.0.0.0" && host !== "::") return [`${host}:${port}`];
  const out = [];
  for (const list of Object.values(interfaces)) {
    for (const i of list || []) if (i.family === "IPv4" && !i.internal) out.push(`${i.address}:${port}`);
  }
  return out;
}

async function pair({ dir, request = requestJSON, interfaces }) {
  const { token, addr } = await ownerToken({ dir, request });
  const res = await request(addr, "POST", "/api/pairing/start", { headers: { Authorization: "Bearer " + token } });
  if (res.status === 409 && res.body && res.body.code === "remote_off") {
    const err = new Error(bodyError(res, "this host does not accept other devices")); err.remoteOff = true; throw err;
  }
  if (res.status !== 200 || !res.body || !res.body.code) throw new Error(bodyError(res, `the host answered ${res.status}`));
  const info = readHostFile(dir) || {};
  return { code: res.body.code, expiresAt: res.body.expiresAt, reach: reachableAddrs(info.addr || res.body.addr || "", interfaces) };
}

async function devices({ dir, request = requestJSON }) {
  const { token, addr } = await ownerToken({ dir, request });
  const res = await request(addr, "GET", "/api/devices", { headers: { Authorization: "Bearer " + token } });
  if (res.status !== 200 || !res.body) throw new Error(bodyError(res, `the host answered ${res.status}`));
  return res.body.devices || [];
}

// Remove a device named by its id, an unambiguous start of its id, or its name.
async function revoke({ dir, which, request = requestJSON }) {
  const list = await devices({ dir, request });
  const w = String(which || "").trim().toLowerCase();
  if (!w) throw new Error("say which device: `cal revoke <id or name>` (see `cal devices`)");
  let matches = list.filter((d) => d.id.toLowerCase() === w);
  if (!matches.length) matches = list.filter((d) => d.id.toLowerCase().startsWith(w));
  if (!matches.length) matches = list.filter((d) => String(d.name).toLowerCase() === w);
  if (matches.length === 0) throw new Error(`no paired device matches "${which}"`);
  if (matches.length > 1) throw new Error(`"${which}" matches ${matches.length} devices; use more of its id`);

  const { token, addr } = await ownerToken({ dir, request });
  const res = await request(addr, "DELETE", "/api/devices/" + encodeURIComponent(matches[0].id), { headers: { Authorization: "Bearer " + token } });
  if (res.status !== 200) throw new Error(bodyError(res, `the host answered ${res.status}`));
  return matches[0];
}

module.exports = { REMOTE_FILE, readRemote, writeRemote, clearRemote, assessAddr, connect, pair, devices, revoke, reachableAddrs, LAN_WARNING, PUBLIC_REFUSAL };
