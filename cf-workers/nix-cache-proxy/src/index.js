const DAY = 86400;
const NARINFO_TTL = 7 * DAY;
const NAR_TTL = 30 * DAY;
const MISSING_TTL = 300;
const DOWN_TTL = 60;
const DOWN_AFTER = 3;
const LOOKUP_ATTEMPTS = 3;
const LOOKUP_RETRY_MS = 100;
const LOOKUP_TIMEOUT_MS = 4000;
const PRIORITY = 30;

const narinfoPath = /^\/([0-9a-df-np-sv-z]{32})\.narinfo$/;
const narPath = /^\/u\/([a-z0-9-]+)\/(.+)$/;
const narHeaders = [
  "content-type",
  "content-length",
  "content-range",
  "accept-ranges",
  "etag",
  "last-modified",
];

export function upstreamsOf(env) {
  return typeof env.UPSTREAMS === "string"
    ? JSON.parse(env.UPSTREAMS)
    : env.UPSTREAMS;
}

export function rewriteNarinfo(text, name) {
  let url;
  const lines = text.split("\n").map((line) => {
    if (!line.startsWith("URL: ")) {
      return line;
    }
    url = line.slice(5);
    return `URL: u/${name}/${url}`;
  });
  const unsafe =
    !url ||
    url.startsWith("/") ||
    url.includes("://") ||
    url.split(/[/?]/).includes("..");
  return unsafe ? null : lines.join("\n");
}

function text(body, status, headers = {}) {
  return new Response(body, {
    status,
    headers: { "content-type": "text/plain; charset=utf-8", ...headers },
  });
}

function forMethod(request, response) {
  return request.method === "HEAD"
    ? new Response(null, { status: response.status, headers: response.headers })
    : response;
}

function info(request, env) {
  const origin = new URL(request.url).origin;
  const upstreams = upstreamsOf(env);
  return text(
    [
      `substituters = ${origin}`,
      `trusted-public-keys = ${upstreams.map((upstream) => upstream.key).join(" ")}`,
      "",
      ...upstreams.map((upstream) => `${upstream.name} ${upstream.url}`),
      "",
    ].join("\n"),
    200,
    { "cache-control": "public, max-age=3600" },
  );
}

async function ask(upstream, hash, signal) {
  let response;
  try {
    response = await fetch(`${upstream.url}/${hash}.narinfo`, {
      headers: { "accept-encoding": "identity" },
      signal: AbortSignal.any([signal, AbortSignal.timeout(LOOKUP_TIMEOUT_MS)]),
      cf: {
        cacheEverything: true,
        cacheTtlByStatus: {
          "200-299": NARINFO_TTL,
          404: MISSING_TTL,
          "500-599": 0,
        },
      },
    });
  } catch (error) {
    return { state: "failed", reason: String(error) };
  }
  if (response.status !== 200) {
    await response.body?.cancel();
    const missing = response.status === 404 || response.status === 403;
    return missing
      ? { state: "missing" }
      : { state: "failed", reason: `status ${response.status}` };
  }
  const body = rewriteNarinfo(await response.text(), upstream.name);
  return body === null
    ? { state: "failed", reason: "narinfo without a usable URL" }
    : { state: "found", body };
}

async function lookup(upstream, hash, signal) {
  let result;
  for (let attempt = 0; attempt < LOOKUP_ATTEMPTS; attempt++) {
    if (attempt > 0) {
      await new Promise((resolve) =>
        setTimeout(resolve, LOOKUP_RETRY_MS * attempt),
      );
    }
    result = await ask(upstream, hash, signal);
    if (result.state !== "failed" || signal.aborted) {
      break;
    }
  }
  return result;
}

function firstFound(lookups) {
  return new Promise((resolve) => {
    const results = [];
    let pending = lookups.length;
    if (pending === 0) {
      resolve({ results });
    }
    lookups.forEach((lookup, index) =>
      lookup.then((result) => {
        results[index] = result;
        if (result.state === "found") {
          resolve({ index, result });
        } else if (--pending === 0) {
          resolve({ results });
        }
      }),
    );
  });
}

async function failuresOf(cache, key) {
  const stored = await cache.match(key);
  const now = Date.now();
  return Object.fromEntries(
    Object.entries(stored ? await stored.json() : {}).filter(
      ([, failure]) => failure.until > now,
    ),
  );
}

async function narinfo(request, env, ctx, hash) {
  const cache = caches.default;
  const key = new Request(new URL(`/${hash}.narinfo`, request.url));
  const cached = await cache.match(key);
  if (cached) {
    return forMethod(request, cached);
  }

  const failuresKey = new Request(new URL("/.failures", request.url));
  const failures = await failuresOf(cache, failuresKey);
  const live = upstreamsOf(env).filter(
    (upstream) => (failures[upstream.name]?.count ?? 0) < DOWN_AFTER,
  );

  const started = Date.now();
  const controller = new AbortController();
  const outcome = await firstFound(
    live.map((upstream) => lookup(upstream, hash, controller.signal)),
  );

  if (outcome.result) {
    controller.abort();
    const response = new Response(outcome.result.body, {
      status: 200,
      headers: {
        "content-type": "text/x-nix-narinfo",
        "cache-control": `public, max-age=${NARINFO_TTL}`,
        "x-upstream": live[outcome.index].name,
        "server-timing": `lookup;dur=${Date.now() - started}`,
      },
    });
    ctx.waitUntil(cache.put(key, response.clone()));
    return forMethod(request, response);
  }

  const failed = live.filter(
    (_, index) => outcome.results[index].state === "failed",
  );
  if (failed.length > 0) {
    const until = Date.now() + DOWN_TTL * 1000;
    for (const upstream of failed) {
      const count = (failures[upstream.name]?.count ?? 0) + 1;
      failures[upstream.name] = { count, until };
      console.warn(
        `${upstream.name} failed ${count} time(s): ${outcome.results[live.indexOf(upstream)].reason}`,
      );
    }
    ctx.waitUntil(
      cache.put(
        failuresKey,
        new Response(JSON.stringify(failures), {
          headers: {
            "content-type": "application/json",
            "cache-control": `max-age=${DOWN_TTL}`,
          },
        }),
      ),
    );
    return text("upstream lookup failed\n", 503, {
      "retry-after": "1",
      "cache-control": "no-store",
      "x-failed": failed.map((upstream) => upstream.name).join(","),
    });
  }

  const response = text("not found\n", 404, {
    "cache-control": `public, max-age=${MISSING_TTL}`,
  });
  ctx.waitUntil(cache.put(key, response.clone()));
  return forMethod(request, response);
}

async function nar(request, env, name, path, search) {
  const upstream = upstreamsOf(env).find(
    (candidate) => candidate.name === name,
  );
  if (!upstream) {
    return text("unknown upstream\n", 404);
  }

  const headers = new Headers({ "accept-encoding": "identity" });
  if (request.headers.has("range")) {
    headers.set("range", request.headers.get("range"));
  }

  let response;
  try {
    response = await fetch(`${upstream.url}/${path}${search}`, {
      method: request.method,
      headers,
      cf: {
        cacheEverything: true,
        cacheTtlByStatus: { "200-299": NAR_TTL, 404: 60, "500-599": 0 },
      },
    });
  } catch {
    return text("upstream unreachable\n", 502, { "cache-control": "no-store" });
  }

  if (response.status === 404 || response.status === 403) {
    return text("not found\n", 404, { "cache-control": "public, max-age=60" });
  }
  if (response.status !== 200 && response.status !== 206) {
    return text("upstream error\n", 502, { "cache-control": "no-store" });
  }

  const out = new Headers({
    "cache-control": `public, immutable, max-age=${NAR_TTL}`,
    "x-upstream": upstream.name,
  });
  for (const header of narHeaders) {
    if (response.headers.has(header)) {
      out.set(header, response.headers.get(header));
    }
  }
  return new Response(request.method === "HEAD" ? null : response.body, {
    status: response.status,
    headers: out,
  });
}

export default {
  async fetch(request, env, ctx) {
    if (request.method !== "GET" && request.method !== "HEAD") {
      return text("method not allowed\n", 405, { allow: "GET, HEAD" });
    }

    const url = new URL(request.url);

    if (url.pathname === "/nix-cache-info") {
      return forMethod(
        request,
        text(
          `StoreDir: /nix/store\nWantMassQuery: 1\nPriority: ${PRIORITY}\n`,
          200,
          { "cache-control": "public, max-age=3600" },
        ),
      );
    }

    if (url.pathname === "/") {
      return forMethod(request, info(request, env));
    }

    const narinfoMatch = narinfoPath.exec(url.pathname);
    if (narinfoMatch) {
      return narinfo(request, env, ctx, narinfoMatch[1]);
    }

    const narMatch = narPath.exec(url.pathname);
    if (narMatch) {
      return nar(request, env, narMatch[1], narMatch[2], url.search);
    }

    return text("not found\n", 404);
  },
};
