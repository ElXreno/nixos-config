import assert from "node:assert/strict";
import { beforeEach, test } from "node:test";
import worker from "../src/index.js";

const origin = "https://factory.talos.elxreno.com";
const digest = "a".repeat(64);
const artifact = `/image/${digest}/v1.9.0/metal-amd64.iso`;

let calls;
let stored;
let entries;
let pending;

function upstream(respond) {
  globalThis.fetch = async (request, init) => {
    calls.push({ request, init });
    return respond(calls.length, request);
  };
}

async function run(path, init) {
  const ctx = { waitUntil: (promise) => pending.push(promise) };
  const response = await worker.fetch(
    new Request(origin + path, init),
    {},
    ctx,
  );
  await Promise.all(pending.splice(0));
  return response;
}

beforeEach(() => {
  calls = [];
  stored = [];
  entries = new Map();
  pending = [];
  globalThis.caches = {
    default: {
      match: async (key) => {
        const entry = entries.get(key.url);
        return (
          entry &&
          new Response(entry.body, {
            status: entry.status,
            headers: entry.headers,
          })
        );
      },
      put: async (key, response) => {
        stored.push({ key, response });
        const copy = response.clone();
        entries.set(key.url, {
          status: copy.status,
          headers: [...copy.headers],
          body: await copy.arrayBuffer(),
        });
      },
    },
  };
  upstream(() => new Response("unexpected", { status: 500 }));
});

test("forwards writes to the factory without caching", async () => {
  upstream(() => new Response("created", { status: 201 }));
  const response = await run("/schematics", { method: "POST", body: "{}" });
  assert.equal(response.status, 201);
  assert.equal(calls[0].request.url, "https://factory.talos.dev/schematics");
  assert.equal(calls[0].request.method, "POST");
  assert.equal(stored.length, 0);
});

test("rewrites factory links in text responses and caches them", async () => {
  upstream(
    () =>
      new Response(
        '<a href="https://factory.talos.dev/image">x</a> pxe.factory.talos.dev',
        { status: 200, headers: { "content-type": "text/html" } },
      ),
  );
  const response = await run("/");
  assert.equal(
    await response.text(),
    `<a href="${origin}/image">x</a> ${origin}`,
  );
  assert.equal(calls[0].init.cf.cacheTtl, 7200);
  assert.equal(stored.length, 1);
  assert.equal(
    stored[0].response.headers.get("cache-control"),
    "public, max-age=7200",
  );
});

test("keeps binary responses as they are", async () => {
  upstream(
    () =>
      new Response("\u0000talos.dev", {
        status: 200,
        headers: { "content-type": "application/octet-stream" },
      }),
  );
  const response = await run("/image/raw");
  assert.equal(await response.text(), "\u0000talos.dev");
});

test("caches artifacts addressed by a digest for a month", async () => {
  upstream(() => new Response("iso", { status: 200 }));
  await run(artifact);
  assert.equal(calls[0].init.cf.cacheTtl, 2592000);
  assert.equal(
    stored[0].response.headers.get("cache-control"),
    "public, max-age=2592000",
  );
});

test("follows an artifact redirect that stays inside talos.dev", async () => {
  upstream((call) =>
    call === 1
      ? new Response(null, {
          status: 302,
          headers: { location: "https://cdn.talos.dev/blob?sig=1" },
        })
      : new Response("blob", { status: 200 }),
  );
  const response = await run(artifact);
  assert.equal(await response.text(), "blob");
  assert.equal(calls[1].request.url, "https://cdn.talos.dev/blob?sig=1");
  assert.equal(calls[1].init.cf.cacheKey, "https://cdn.talos.dev/blob");
});

test("rewrites the location of other redirects", async () => {
  upstream(
    () =>
      new Response(null, {
        status: 301,
        headers: { location: "https://factory.talos.dev/new" },
      }),
  );
  const response = await run("/old");
  assert.equal(response.status, 301);
  assert.equal(response.headers.get("location"), `${origin}/new`);
  assert.equal(stored.length, 0);
});

test("asks for a tag manifest with GET when the client sent HEAD", async () => {
  upstream(() => new Response("{}", { status: 200 }));
  const response = await run("/v2/installer/manifests/v1.9.0", {
    method: "HEAD",
  });
  assert.equal(calls[0].request.method, "GET");
  assert.equal(response.status, 200);
  assert.equal(response.body, null);
  assert.equal(stored.length, 0);
});

test("serves a cached response without asking the factory", async () => {
  upstream(
    () =>
      new Response("front", {
        status: 200,
        headers: { "content-type": "text/html" },
      }),
  );
  await (await run("/")).text();
  const response = await run("/");
  assert.equal(await response.text(), "front");
  assert.equal(calls.length, 1);
});

test("keeps a HEAD response out of the cache", async () => {
  upstream(() => new Response(null, { status: 200 }));
  const response = await run(artifact, { method: "HEAD" });
  assert.equal(response.status, 200);
  assert.equal(calls[0].request.method, "HEAD");
  assert.equal(stored.length, 0);
});

test("answers a GET after a HEAD with the full body", async () => {
  upstream((_, request) =>
    request.method === "HEAD"
      ? new Response(null, { status: 200 })
      : new Response("iso", { status: 200 }),
  );
  await run(artifact, { method: "HEAD" });
  const response = await run(artifact);
  assert.equal(await response.text(), "iso");
  assert.equal(calls.length, 2);
  assert.equal(calls[1].request.method, "GET");
});

test("answers a HEAD after a GET from the cache", async () => {
  upstream(() => new Response("iso", { status: 200 }));
  await (await run(artifact)).text();
  const response = await run(artifact, { method: "HEAD" });
  assert.equal(response.status, 200);
  assert.equal(response.body, null);
  assert.equal(calls.length, 1);
});

test("ignores entries stored under the unversioned key", async () => {
  entries.set(origin + artifact, {
    status: 200,
    headers: [],
    body: new ArrayBuffer(0),
  });
  upstream(() => new Response("iso", { status: 200 }));
  const response = await run(artifact);
  assert.equal(await response.text(), "iso");
  assert.equal(calls.length, 1);
  assert.notEqual(stored[0].key.url, origin + artifact);
});
