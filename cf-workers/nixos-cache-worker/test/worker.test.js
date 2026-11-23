import assert from "node:assert/strict";
import { beforeEach, test } from "node:test";
import worker from "../src/worker.js";

const origin = "https://nixos-cache-proxy.elxreno.com";
const narinfo = "/rp1zq1p0rjfdg99rb2cfs14871y6cqaj.narinfo";

let calls;
let stored;
let cached;

function upstream(respond) {
  globalThis.fetch = async (url, init) => {
    calls.push({ url, init });
    return respond();
  };
}

function run(path, init) {
  const ctx = { waitUntil: (promise) => promise };
  return worker.fetch(new Request(origin + path, init), {}, ctx);
}

beforeEach(() => {
  calls = [];
  stored = [];
  cached = undefined;
  globalThis.caches = {
    default: {
      match: async () => cached,
      put: async (key, response) => {
        stored.push({ key, response });
      },
    },
  };
  upstream(() => new Response("unexpected", { status: 500 }));
});

test("rejects methods other than GET and HEAD", async () => {
  const response = await run(narinfo, { method: "POST" });
  assert.equal(response.status, 405);
  assert.equal(calls.length, 0);
});

test("rejects paths outside the binary cache layout", async () => {
  const response = await run("/nar/../secret");
  assert.equal(response.status, 403);
  assert.equal(calls.length, 0);
});

test("proxies a narinfo to cache.nixos.org and marks it immutable", async () => {
  upstream(
    () =>
      new Response("StorePath: /nix/store/x\n", {
        status: 200,
        headers: { server: "AmazonS3", "x-cache": "HIT", etag: '"abc"' },
      }),
  );
  const response = await run(narinfo + "?ignored=1");
  assert.equal(response.status, 200);
  assert.equal(calls[0].url, "https://cache.nixos.org" + narinfo);
  assert.equal(calls[0].init.cf.cacheKey, origin + narinfo);
  assert.match(response.headers.get("cache-control"), /immutable/);
  assert.equal(response.headers.get("server"), null);
  assert.equal(response.headers.get("x-cache"), null);
  assert.equal(response.headers.get("etag"), '"abc"');
  assert.equal(await response.text(), "StorePath: /nix/store/x\n");
});

test("keeps a missing path short lived", async () => {
  upstream(() => new Response("404", { status: 404 }));
  const response = await run(narinfo);
  assert.equal(response.status, 404);
  assert.equal(
    response.headers.get("cache-control"),
    "public, max-age=600, stale-if-error=300",
  );
});

test("passes other upstream statuses through untouched", async () => {
  const failure = new Response("unavailable", { status: 503 });
  upstream(() => failure);
  assert.equal(await run(narinfo), failure);
});

test("answers 502 when the upstream is unreachable", async () => {
  upstream(() => {
    throw new Error("connection reset");
  });
  const response = await run(narinfo);
  assert.equal(response.status, 502);
});

test("drops the body of a HEAD request", async () => {
  upstream(() => new Response(null, { status: 200 }));
  const response = await run(narinfo, { method: "HEAD" });
  assert.equal(response.status, 200);
  assert.equal(calls[0].init.method, "HEAD");
  assert.equal(response.body, null);
});

test("rewrites nix-cache-info to its own host and caches it", async () => {
  upstream(
    () =>
      new Response("StoreDir: /nix/store\nURL: https://cache.nixos.org\n", {
        status: 200,
      }),
  );
  const response = await run("/nix-cache-info");
  const body = await response.text();
  assert.equal(
    body,
    "StoreDir: /nix/store\nURL: https://nixos-cache-proxy.elxreno.com\n",
  );
  assert.equal(response.headers.get("content-length"), String(body.length));
  assert.equal(stored.length, 1);
  assert.equal(stored[0].key.url, origin + "/nix-cache-info");
});

test("serves a cached nix-cache-info without asking the upstream", async () => {
  cached = new Response("cached", { status: 200 });
  const response = await run("/nix-cache-info");
  assert.equal(await response.text(), "cached");
  assert.equal(calls.length, 0);
});
