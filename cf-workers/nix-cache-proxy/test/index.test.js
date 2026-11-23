import assert from "node:assert/strict";
import { beforeEach, test } from "node:test";
import worker, { rewriteNarinfo } from "../src/index.js";

const origin = "https://cache.nixos.elxreno.com";
const hash = "rp1zq1p0rjfdg99rb2cfs14871y6cqaj";
const env = {
  UPSTREAMS: [
    { name: "first", url: "https://first.example", key: "first-1:AAAA" },
    { name: "second", url: "https://second.example", key: "second-1:BBBB" },
    { name: "third", url: "https://third.example", key: "third-1:CCCC" },
  ],
};

const narinfoBody = (url) =>
  `StorePath: /nix/store/${hash}-gcc\nURL: ${url}\nCompression: zstd\nNarSize: 3\nSig: k-1:c2ln\n`;

let routes;
let calls;
let store;

function run(path, init) {
  const pending = [];
  const ctx = { waitUntil: (promise) => pending.push(promise) };
  return worker
    .fetch(new Request(origin + path, init), env, ctx)
    .then(async (response) => {
      await Promise.all(pending);
      return response;
    });
}

beforeEach(() => {
  routes = {};
  calls = [];
  store = new Map();
  globalThis.caches = {
    default: {
      match: async (key) => store.get(key.url)?.clone(),
      put: async (key, response) => {
        store.set(key.url, response);
      },
    },
  };
  globalThis.fetch = async (url, init) => {
    calls.push({ url, init });
    const route = routes[url];
    if (!route) {
      return new Response("missing", { status: 404 });
    }
    return route(init);
  };
});

test("describes itself to Nix", async () => {
  const response = await run("/nix-cache-info");
  assert.equal(
    await response.text(),
    "StoreDir: /nix/store\nWantMassQuery: 1\nPriority: 30\n",
  );
  assert.equal(calls.length, 0);
});

test("lists the keys a client has to trust", async () => {
  const body = await (await run("/")).text();
  assert.match(body, new RegExp(`^substituters = ${origin}$`, "m"));
  assert.match(
    body,
    /^trusted-public-keys = first-1:AAAA second-1:BBBB third-1:CCCC$/m,
  );
});

test("takes a narinfo from the upstream that has it and pins the NAR to it", async () => {
  routes[`https://second.example/${hash}.narinfo`] = () =>
    new Response(narinfoBody("nar/abc.nar.zst"));
  const response = await run(`/${hash}.narinfo`);
  assert.equal(response.status, 200);
  assert.equal(response.headers.get("x-upstream"), "second");
  assert.equal(await response.text(), narinfoBody("u/second/nar/abc.nar.zst"));
  assert.equal(calls.length, 3);
});

test("answers a repeated narinfo from the edge cache", async () => {
  routes[`https://first.example/${hash}.narinfo`] = () =>
    new Response(narinfoBody("nar/abc.nar.zst"));
  await run(`/${hash}.narinfo`);
  const asked = calls.length;
  const response = await run(`/${hash}.narinfo`);
  assert.equal(await response.text(), narinfoBody("u/first/nar/abc.nar.zst"));
  assert.equal(calls.length, asked);
});

test("takes the first upstream that answers with the path", async () => {
  routes[`https://first.example/${hash}.narinfo`] = async () => {
    await new Promise((resolve) => setTimeout(resolve, 50));
    return new Response(narinfoBody("nar/slow.nar.zst"));
  };
  routes[`https://third.example/${hash}.narinfo`] = () =>
    new Response(narinfoBody("nar/fast.nar.zst"));
  const response = await run(`/${hash}.narinfo`);
  assert.equal(response.headers.get("x-upstream"), "third");
});

test("does not wait for a slow upstream once the path is found", async () => {
  routes[`https://first.example/${hash}.narinfo`] = (init) =>
    new Promise((_, reject) =>
      init.signal.addEventListener("abort", () => reject(init.signal.reason)),
    );
  routes[`https://second.example/${hash}.narinfo`] = () =>
    new Response(narinfoBody("nar/abc.nar.zst"));
  const response = await run(`/${hash}.narinfo`);
  assert.equal(response.headers.get("x-upstream"), "second");
});

test("answers 404 only when every upstream said so, and remembers it briefly", async () => {
  const response = await run(`/${hash}.narinfo`);
  assert.equal(response.status, 404);
  assert.equal(response.headers.get("cache-control"), "public, max-age=300");
  const asked = calls.length;
  assert.equal((await run(`/${hash}.narinfo`)).status, 404);
  assert.equal(calls.length, asked);
});

test("answers 503 when an upstream failed and nobody had the path", async () => {
  routes[`https://second.example/${hash}.narinfo`] = () =>
    new Response("boom", { status: 500 });
  const response = await run(`/${hash}.narinfo`);
  assert.equal(response.status, 503);
  assert.equal(response.headers.get("x-failed"), "second");
  assert.equal(response.headers.get("cache-control"), "no-store");
});

test("asks a failing upstream once more before giving up", async () => {
  let asked = 0;
  routes[`https://second.example/${hash}.narinfo`] = () => {
    asked += 1;
    if (asked === 1) {
      throw new Error("tls handshake eof");
    }
    return new Response(narinfoBody("nar/abc.nar.zst"));
  };
  const response = await run(`/${hash}.narinfo`);
  assert.equal(response.status, 200);
  assert.equal(asked, 2);
});

test("leaves an upstream out for a while after it failed three requests in a row", async () => {
  routes[`https://second.example/${hash}.narinfo`] = () => {
    throw new Error("connection reset");
  };
  for (let request = 0; request < 3; request++) {
    assert.equal((await run(`/${hash}.narinfo`)).status, 503);
  }
  calls = [];
  const response = await run(`/${hash}.narinfo`);
  assert.equal(response.status, 404);
  assert.deepEqual(
    calls.map((call) => new URL(call.url).host),
    ["first.example", "third.example"],
  );
});

test("serves a path found elsewhere although another upstream failed", async () => {
  routes[`https://first.example/${hash}.narinfo`] = () =>
    new Response("boom", { status: 502 });
  routes[`https://third.example/${hash}.narinfo`] = () =>
    new Response(narinfoBody("nar/abc.nar.zst"));
  const response = await run(`/${hash}.narinfo`);
  assert.equal(response.status, 200);
  assert.equal(response.headers.get("x-upstream"), "third");
});

test("treats a narinfo pointing outside its cache as a failure", async () => {
  routes[`https://first.example/${hash}.narinfo`] = () =>
    new Response(narinfoBody("https://evil.example/x.nar"));
  assert.equal((await run(`/${hash}.narinfo`)).status, 503);
});

test("answers HEAD for a narinfo without a body", async () => {
  routes[`https://first.example/${hash}.narinfo`] = () =>
    new Response(narinfoBody("nar/abc.nar.zst"));
  const response = await run(`/${hash}.narinfo`, { method: "HEAD" });
  assert.equal(response.status, 200);
  assert.equal(response.body, null);
});

test("fetches a NAR only from the upstream named in its path", async () => {
  routes["https://second.example/nar/abc.nar.zst?hash=x"] = (init) =>
    new Response("nar", {
      status: 206,
      headers: {
        "content-range": "bytes 0-2/3",
        "content-type": "application/x-nix-nar",
        "set-cookie": "a=b",
        via: init.headers.get("range"),
      },
    });
  const response = await run("/u/second/nar/abc.nar.zst?hash=x", {
    headers: { range: "bytes=0-2" },
  });
  assert.equal(response.status, 206);
  assert.equal(await response.text(), "nar");
  assert.equal(response.headers.get("content-range"), "bytes 0-2/3");
  assert.equal(response.headers.get("set-cookie"), null);
  assert.equal(response.headers.get("via"), null);
  assert.match(response.headers.get("cache-control"), /immutable/);
  assert.equal(calls.length, 1);
  assert.equal(calls[0].init.headers.get("range"), "bytes=0-2");
});

test("maps NAR failures to 404 and 502", async () => {
  routes["https://first.example/nar/broken"] = () =>
    new Response("boom", { status: 500 });
  routes["https://first.example/nar/reset"] = () => {
    throw new Error("connection reset");
  };
  assert.equal((await run("/u/first/nar/absent")).status, 404);
  assert.equal((await run("/u/first/nar/broken")).status, 502);
  assert.equal((await run("/u/first/nar/reset")).status, 502);
  assert.equal((await run("/u/unknown/nar/x")).status, 404);
});

test("rejects everything else", async () => {
  assert.equal((await run("/nix-cache-info", { method: "POST" })).status, 405);
  assert.equal((await run("/nar/abc.nar.zst")).status, 404);
  assert.equal((await run("/.failures")).status, 404);
  assert.equal((await run(`/${hash.toUpperCase()}.narinfo`)).status, 404);
  assert.equal(calls.length, 0);
});

test("changes nothing in a narinfo except the URL line", () => {
  assert.equal(
    rewriteNarinfo(narinfoBody("nar/abc.nar.zst?hash=x"), "first"),
    narinfoBody("u/first/nar/abc.nar.zst?hash=x"),
  );
  assert.equal(rewriteNarinfo(narinfoBody("../secret"), "first"), null);
  assert.equal(rewriteNarinfo(narinfoBody("/etc/passwd"), "first"), null);
  assert.equal(rewriteNarinfo("StorePath: /nix/store/x\n", "first"), null);
});
