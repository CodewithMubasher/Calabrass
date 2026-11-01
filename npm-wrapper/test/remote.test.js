"use strict";
const test = require("node:test");
const assert = require("node:assert");
const fs = require("fs");
const os = require("os");
const path = require("path");
const R = require("../lib/remote");

const tmp = () => fs.mkdtempSync(path.join(os.tmpdir(), "calr-"));
const hostFile = (dir, addr = "127.0.0.1:8787") =>
  fs.writeFileSync(path.join(dir, "host.json"), JSON.stringify({ addr, pid: 1, owner: "daemon", secret: "s3cret" }));

// A scripted host: answers by method + path.
const host = (routes, seen = []) => async (addr, method, p, opts = {}) => {
  seen.push({ addr, method, path: p, body: opts.body, headers: opts.headers });
  const fn = routes[`${method} ${p}`] || routes[`${method} *`];
  return fn ? fn(opts) : { status: 404, body: null };
};

test("addresses are judged by how safe plain HTTP is to them", () => {
  const level = (a) => R.assessAddr(a).level;
  for (const a of ["127.0.0.1:8787", "localhost:1", "[::1]:1", "100.64.1.2:8787", "100.127.0.1:1", "box.tail1234.ts.net:8787"]) assert.strictEqual(level(a), "ok", a);
  for (const a of ["192.168.1.20:8787", "10.0.0.5:1", "172.16.0.1:1", "172.31.9.9:1", "169.254.1.1:1", "mybox.local:8787", "nas:8787"]) assert.strictEqual(level(a), "lan", a);
  for (const a of ["8.8.8.8:8787", "203.0.113.9:1", "172.32.0.1:1", "example.com:8787", "100.63.0.1:1"]) assert.strictEqual(level(a), "public", a);
});

test("connect pairs, remembers the device privately, and warns on a LAN address", async () => {
  const dir = tmp();
  const seen = [];
  const request = host({ "POST /api/pairing/claim": () => ({ status: 200, body: { token: "dev-tok", device: { id: "d1", name: "Laptop" } } }) }, seen);
  const { remote, warning } = await R.connect({ addr: "http://192.168.1.20:8787/", code: "abcd-efgh", name: "Laptop", dir, request });

  assert.strictEqual(remote.addr, "192.168.1.20:8787");
  assert.match(warning, /plain HTTP/);
  assert.deepStrictEqual(seen[0].body, { code: "abcd-efgh", name: "Laptop" });
  const saved = R.readRemote(dir);
  assert.strictEqual(saved.token, "dev-tok");
  if (process.platform !== "win32") assert.strictEqual(fs.statSync(path.join(dir, "remote.json")).mode & 0o077, 0, "remote.json must be owner-only");
});

test("connect refuses the open internet unless told it is understood", async () => {
  const dir = tmp();
  let called = false;
  const request = host({ "POST /api/pairing/claim": () => { called = true; return { status: 200, body: { token: "t", device: { name: "x" } } }; } });
  await assert.rejects(R.connect({ addr: "203.0.113.9:8787", code: "ABCD-EFGH", dir, request }), /open internet/);
  assert.strictEqual(called, false, "the code was sent over the open internet");
  assert.strictEqual(R.readRemote(dir), null);
  const { warning } = await R.connect({ addr: "203.0.113.9:8787", code: "ABCD-EFGH", dir, request, insecure: true });
  assert.strictEqual(warning, "");
  assert.ok(R.readRemote(dir));
});

test("connect explains each way it can fail and saves nothing", async () => {
  const dir = tmp();
  const status = (n, body = null) => host({ "POST /api/pairing/claim": () => ({ status: n, body }) });
  await assert.rejects(R.connect({ addr: "192.168.1.2:8787", code: "X", dir, request: status(0) }), /can't reach a Calabrass host/);
  await assert.rejects(R.connect({ addr: "192.168.1.2:8787", code: "X", dir, request: status(404) }), /does not accept other devices/);
  await assert.rejects(R.connect({ addr: "192.168.1.2:8787", code: "X", dir, request: status(429) }), /too many attempts/);
  await assert.rejects(R.connect({ addr: "192.168.1.2:8787", code: "X", dir, request: status(403, { error: "x" }) }), /not valid, or it has expired/);
  await assert.rejects(R.connect({ addr: "not an address", code: "X", dir }), /should look like/);
  await assert.rejects(R.connect({ addr: "192.168.1.2:8787", code: " ", dir }), /pairing code/);
  assert.strictEqual(R.readRemote(dir), null);
});

test("a damaged or half-written remote.json is no connection at all", () => {
  const dir = tmp();
  fs.writeFileSync(path.join(dir, "remote.json"), "{ nope");
  assert.strictEqual(R.readRemote(dir), null);
  fs.writeFileSync(path.join(dir, "remote.json"), JSON.stringify({ addr: "a:1" }));
  assert.strictEqual(R.readRemote(dir), null, "no token means not connected");
  assert.strictEqual(R.clearRemote(dir), true);
  assert.strictEqual(R.clearRemote(dir), false);
});

test("pair signs in with the host secret and returns the code with the addresses to use", async () => {
  const dir = tmp();
  hostFile(dir, "0.0.0.0:8787");
  const seen = [];
  const request = host({
    "POST /api/auth/local": () => ({ status: 200, body: { token: "owner-tok" } }),
    "POST /api/pairing/start": () => ({ status: 200, body: { code: "K7QD-M2XH", expiresAt: "soon" } }),
  }, seen);
  const interfaces = { eth0: [{ family: "IPv4", internal: false, address: "192.168.1.20" }], lo: [{ family: "IPv4", internal: true, address: "127.0.0.1" }] };
  const p = await R.pair({ dir, request, interfaces });

  assert.strictEqual(p.code, "K7QD-M2XH");
  assert.deepStrictEqual(p.reach, ["192.168.1.20:8787"]);
  assert.strictEqual(seen[0].body.secret, "s3cret");
  assert.strictEqual(seen[0].body.create, false);
  assert.strictEqual(seen[1].headers.Authorization, "Bearer owner-tok");
  assert.strictEqual(seen[0].addr, "127.0.0.1:8787", "a 0.0.0.0 listen address is dialled as loopback");
});

test("pair says how to turn remote access on when it is off", async () => {
  const dir = tmp();
  hostFile(dir);
  const request = host({
    "POST /api/auth/local": () => ({ status: 200, body: { token: "t" } }),
    "POST /api/pairing/start": () => ({ status: 409, body: { code: "remote_off", error: "Start it with CALABRASS_ALLOW_REMOTE=1" } }),
  });
  await assert.rejects(R.pair({ dir, request }), (e) => e.remoteOff === true && /CALABRASS_ALLOW_REMOTE/.test(e.message));
});

test("commands that need the host say so plainly when there is none", async () => {
  await assert.rejects(R.pair({ dir: tmp() }), /no Calabrass host is running on this machine/);
  const dir = tmp();
  hostFile(dir);
  await assert.rejects(R.devices({ dir, request: host({ "POST /api/auth/local": () => ({ status: 409, body: {} }) }) }), /nobody has an account/);
});

test("revoke finds a device by id, a start of its id, or its name, and refuses ambiguity", async () => {
  const dir = tmp();
  hostFile(dir);
  const list = [{ id: "aaaa1111", name: "Laptop" }, { id: "aaaa2222", name: "Phone" }, { id: "bbbb3333", name: "Laptop" }];
  const seen = [];
  const request = host({
    "POST /api/auth/local": () => ({ status: 200, body: { token: "t" } }),
    "GET /api/devices": () => ({ status: 200, body: { devices: list } }),
    "DELETE *": () => ({ status: 200, body: { removed: true } }),
  }, seen);

  assert.strictEqual((await R.revoke({ dir, which: "bbbb", request })).id, "bbbb3333");
  assert.strictEqual((await R.revoke({ dir, which: "phone", request })).id, "aaaa2222");
  assert.strictEqual(seen.filter((s) => s.method === "DELETE").pop().path, "/api/devices/aaaa2222");
  await assert.rejects(R.revoke({ dir, which: "aaaa", request }), /matches 2 devices/);
  await assert.rejects(R.revoke({ dir, which: "Laptop", request }), /matches 2 devices/);
  await assert.rejects(R.revoke({ dir, which: "zzz", request }), /no paired device matches/);
  await assert.rejects(R.revoke({ dir, which: "", request }), /say which device/);
});

test("reachableAddrs lists what another machine could dial", () => {
  const ifaces = { eth0: [{ family: "IPv4", internal: false, address: "10.0.0.7" }], wl: [{ family: "IPv4", internal: false, address: "192.168.1.9" }, { family: "IPv6", internal: false, address: "fe80::1" }] };
  assert.deepStrictEqual(R.reachableAddrs("0.0.0.0:8787", ifaces), ["10.0.0.7:8787", "192.168.1.9:8787"]);
  assert.deepStrictEqual(R.reachableAddrs("192.168.1.9:9000", ifaces), ["192.168.1.9:9000"]);
  assert.deepStrictEqual(R.reachableAddrs("127.0.0.1:8787", ifaces), []);
});
