"use strict";
const test = require("node:test");
const assert = require("node:assert");
const fs = require("fs");
const os = require("os");
const path = require("path");
const http = require("http");
const L = require("../lib/launcher");

function fakeHost() {
  return new Promise((resolve) => {
    const srv = http.createServer((req, res) => {
      res.setHeader("Content-Type", "application/json");
      res.end(JSON.stringify({ status: "ok" }));
    });
    srv.listen(0, "127.0.0.1", () => resolve({ srv, addr: "127.0.0.1:" + srv.address().port }));
  });
}
const tmp = () => fs.mkdtempSync(path.join(os.tmpdir(), "cal-"));
const announce = (dir, addr) => fs.writeFileSync(path.join(dir, "host.json"), JSON.stringify({ addr, pid: 1, owner: "daemon", secret: "s" }));

test("parseArgs recognises commands and leaves TUI flags alone", () => {
  assert.deepStrictEqual(L.parseArgs(["stop"]), { command: "stop", args: [] });
  assert.deepStrictEqual(L.parseArgs(["status"]), { command: "status", args: [] });
  assert.strictEqual(L.parseArgs(["-v"]).command, "version");
  assert.deepStrictEqual(L.parseArgs(["-c"]), { command: "tui", args: ["-c"] });
});

test("addrFromArgs reads every spelling", () => {
  assert.strictEqual(L.addrFromArgs(["-addr", "10.0.0.2:8787"]), "10.0.0.2:8787");
  assert.strictEqual(L.addrFromArgs(["--addr=10.0.0.2:1"]), "10.0.0.2:1");
  assert.strictEqual(L.addrFromArgs(["-c"]), "");
});

test("isLoopback", () => {
  for (const a of ["127.0.0.1:8787", "localhost:1", "[::1]:1"]) assert.ok(L.isLoopback(a), a);
  for (const a of ["192.168.1.5:8787", "example.com:80", "0.0.0.0:1"]) assert.ok(!L.isLoopback(a), a);
});

test("dataDir agrees with the backend on every platform", () => {
  assert.strictEqual(L.dataDir({ CALABRASS_DATA_DIR: "/x" }, "linux"), "/x");
  assert.strictEqual(L.dataDir({ LOCALAPPDATA: "C:\\L" }, "win32"), path.join("C:\\L", "Calabrass"));
  assert.strictEqual(L.dataDir({ XDG_CONFIG_HOME: "/cfg" }, "linux"), path.join("/cfg", "Calabrass"));
});

test("attaches to a running host and does not start another", async () => {
  const { srv, addr } = await fakeHost();
  const dir = tmp();
  announce(dir, addr);
  let spawned = false;
  const host = await L.ensureHost({ dir, backendBinary: "/nope", spawnFn: () => { spawned = true; } });
  srv.close();
  assert.deepStrictEqual(host, { addr, started: false });
  assert.strictEqual(spawned, false, "a second host was started");
});

test("starts a detached background host when there is none, and waits for it", async () => {
  const dir = tmp();
  const bin = path.join(dir, "fake-backend");
  fs.writeFileSync(bin, "x");
  let opts, argv, srv;
  const spawnFn = (file, args, o) => {
    argv = args; opts = o;
    fakeHost().then((h) => { srv = h.srv; announce(dir, h.addr); });
    return { on() {}, unref() {} };
  };
  const host = await L.ensureHost({ dir, backendBinary: bin, spawnFn, timeoutMs: 5000, explicitAddr: "127.0.0.1:1" });
  srv.close();
  assert.strictEqual(host.started, true);
  assert.deepStrictEqual(argv, ["--daemon"]);
  assert.strictEqual(opts.detached, true, "the host must outlive this terminal");
  assert.strictEqual(opts.stdio[0], "ignore");
});

test("never starts a local host for an address on another machine", async () => {
  let spawned = false;
  await assert.rejects(
    L.ensureHost({ dir: tmp(), backendBinary: "/x", explicitAddr: "203.0.113.9:8787", spawnFn: () => { spawned = true; } }),
    /no Calabrass host answers at 203\.0\.113\.9:8787/,
  );
  assert.strictEqual(spawned, false);
});

test("says so when there is no host and nothing to start one with", async () => {
  await assert.rejects(L.ensureHost({ dir: tmp(), backendBinary: "/does/not/exist", explicitAddr: "127.0.0.1:1" }), /no host binary/);
});

test("the launcher never writes to stdout while starting (stdout belongs to `cal run`)", () => {
  const src = fs.readFileSync(path.join(__dirname, "..", "bin", "cal.js"), "utf8");
  // Everything after the informational commands (--version, help, status, stop).
  const main = src.slice(src.indexOf("if (!fs.existsSync(tuiBinary))"));
  assert.ok(!/console\.log/.test(main), "console.log in the launch path would pollute a piped answer");
});

test("the first-run hint appears only when no model is ready", async () => {
  const dir = tmp();
  fs.writeFileSync(path.join(dir, "host.json"), JSON.stringify({ addr: "127.0.0.1:1", secret: "s" }));
  const reply = (providers) => async (addr, method, p) =>
    p === "/api/auth/local" ? { status: 200, body: { token: "t" } } : { status: 200, body: { providers } };

  const none = await L.modelSetupHint({ addr: "127.0.0.1:8787", dir, request: reply([{ id: "openai", enabled: false, hasKey: false, baseUrl: "https://api.openai.com/v1" }]) });
  assert.match(none, /No model is set up yet/);
  assert.match(none, /\/connect/);
  assert.match(none, /CALABRASS_PROVIDER/);

  assert.strictEqual(await L.modelSetupHint({ addr: "a:1", dir, request: reply([{ id: "groq", enabled: true, hasKey: true, baseUrl: "https://api.groq.com/openai/v1" }]) }), null);
  // A local server needs no key.
  assert.strictEqual(await L.modelSetupHint({ addr: "a:1", dir, request: reply([{ id: "ollama", enabled: true, hasKey: false, baseUrl: "http://localhost:11434/v1" }]) }), null);
  // A provider that is on but has no key is not ready.
  assert.match(await L.modelSetupHint({ addr: "a:1", dir, request: reply([{ id: "openai", enabled: true, hasKey: false, baseUrl: "https://api.openai.com/v1" }]) }), /No model/);
});

test("the first-run hint never breaks a launch", async () => {
  assert.strictEqual(await L.modelSetupHint({ addr: "a:1", dir: tmp() }), null); // no host.json
  const dir = tmp();
  fs.writeFileSync(path.join(dir, "host.json"), JSON.stringify({ secret: "s" }));
  assert.strictEqual(await L.modelSetupHint({ addr: "a:1", dir, request: async () => { throw new Error("boom"); } }), null);
  assert.strictEqual(await L.modelSetupHint({ addr: "a:1", dir, request: async () => ({ status: 500, body: null }) }), null);
});
