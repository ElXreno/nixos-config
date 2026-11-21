var __defProp = Object.defineProperty;
var __name = (target, value) => __defProp(target, "name", { value, configurable: true });

// src/index.js
var ORIGIN = "https://factory.talos.dev";
var UPSTREAM_DOMAIN_SUFFIX = ".talos.dev";
var PUBLIC_ORIGIN = "https://factory.talos.elxreno.com";
var FRONTEND_CACHE_TTL_SECONDS = 7200;
var STATIC_INSTALLER_CACHE_TTL_SECONDS = 2592e3;
function hasArtifactHash(pathname) {
  return /(?:^|\/)[A-Fa-f0-9]{64}(?=[:/]|$)/.test(pathname);
}
__name(hasArtifactHash, "hasArtifactHash");
function buildUpstreamRequest(request, url, methodOverride) {
  const headers = new Headers(request.headers);
  headers.delete("cache-control");
  headers.delete("pragma");
  const method = methodOverride || request.method;
  return new Request(url.toString(), {
    method,
    headers,
    body: method === "GET" || method === "HEAD" ? null : request.body,
    redirect: "manual"
  });
}
__name(buildUpstreamRequest, "buildUpstreamRequest");
function isRedirectStatus(status) {
  return status >= 300 && status < 400;
}
__name(isRedirectStatus, "isRedirectStatus");
function isRewritableContentType(contentType) {
  return contentType.startsWith("text/") || contentType.startsWith("application/json") || contentType.startsWith("application/javascript") || contentType.startsWith("application/xml");
}
__name(isRewritableContentType, "isRewritableContentType");
function rewriteFactoryLinks(text) {
  const withAbsoluteUrls = text.replace(
    /https?:\/\/(?:[a-z0-9-]+\.)*talos\.dev/gi,
    PUBLIC_ORIGIN
  );
  return withAbsoluteUrls.replace(
    /(^|[^A-Za-z0-9.-])(?:[a-z0-9-]+\.)*talos\.dev(?=[^A-Za-z0-9.-]|$)/g,
    `$1${PUBLIC_ORIGIN}`
  );
}
__name(rewriteFactoryLinks, "rewriteFactoryLinks");
var index_default = {
  async fetch(request, env, ctx) {
    const incomingUrl = new URL(request.url);
    const targetUrl = new URL(`${incomingUrl.pathname}${incomingUrl.search}`, ORIGIN);
    const method = request.method.toUpperCase();
    const isCacheableMethod = method === "GET" || method === "HEAD";
    if (!isCacheableMethod) {
      return fetch(new Request(targetUrl.toString(), request), {
        redirect: "follow"
      });
    }
    const isArtifactRequest = hasArtifactHash(targetUrl.pathname);
    const cacheTtl = isArtifactRequest ? STATIC_INSTALLER_CACHE_TTL_SECONDS : FRONTEND_CACHE_TTL_SECONDS;
    const cache = caches.default;
    const cacheKey = new Request(incomingUrl.toString(), { method: "GET" });
    const cachedResponse = await cache.match(cacheKey);
    if (cachedResponse) {
      if (method === "HEAD") {
        return new Response(null, {
          status: cachedResponse.status,
          statusText: cachedResponse.statusText,
          headers: cachedResponse.headers
        });
      }
      return cachedResponse;
    }
    const isTagManifest = /\/manifests\/(?!sha256:)/.test(targetUrl.pathname);
    const upstreamMethod = method === "HEAD" && isTagManifest ? "GET" : method;
    const originRequest = buildUpstreamRequest(request, targetUrl, upstreamMethod);
    let originResponse = await fetch(originRequest, {
      redirect: "manual",
      cf: {
        cacheEverything: true,
        cacheTtl
      }
    });
    if (isArtifactRequest && isRedirectStatus(originResponse.status)) {
      const location = originResponse.headers.get("location");
      if (location) {
        const redirectedUrl = new URL(location, targetUrl);
        if (redirectedUrl.hostname.endsWith(UPSTREAM_DOMAIN_SUFFIX)) {
          const assetRequest = buildUpstreamRequest(
            request,
            redirectedUrl,
            upstreamMethod
          );
          const cacheKeyUrl = new URL(redirectedUrl.toString());
          cacheKeyUrl.search = "";
          originResponse = await fetch(assetRequest, {
            redirect: "follow",
            cf: {
              cacheEverything: true,
              cacheTtl: STATIC_INSTALLER_CACHE_TTL_SECONDS,
              cacheKey: cacheKeyUrl.toString()
            }
          });
        }
      }
    }
    if (isRedirectStatus(originResponse.status)) {
      const headers = new Headers(originResponse.headers);
      const location = headers.get("location");
      if (location) {
        const rewrittenLocation = location.replace(
          /https?:\/\/(?:[a-z0-9-]+\.)*talos\.dev/gi,
          PUBLIC_ORIGIN
        );
        headers.set("location", rewrittenLocation);
      }
      return new Response(null, {
        status: originResponse.status,
        statusText: originResponse.statusText,
        headers
      });
    }
    const contentType = originResponse.headers.get("content-type") || "";
    let response;
    if (isRewritableContentType(contentType)) {
      const originalBody = await originResponse.text();
      const rewrittenBody = rewriteFactoryLinks(originalBody);
      const headers = new Headers(originResponse.headers);
      headers.delete("content-length");
      response = new Response(rewrittenBody, {
        status: originResponse.status,
        statusText: originResponse.statusText,
        headers
      });
    } else {
      response = new Response(originResponse.body, {
        status: originResponse.status,
        statusText: originResponse.statusText,
        headers: new Headers(originResponse.headers)
      });
    }
    if (response.status === 200) {
      const responseToCache = response.clone();
      const cacheHeaders = new Headers(responseToCache.headers);
      cacheHeaders.set("Cache-Control", `public, max-age=${cacheTtl}`);
      const cacheableResponse = new Response(responseToCache.body, {
        status: responseToCache.status,
        statusText: responseToCache.statusText,
        headers: cacheHeaders
      });
      ctx.waitUntil(cache.put(cacheKey, cacheableResponse));
    }
    if (method === "HEAD") {
      return new Response(null, {
        status: response.status,
        statusText: response.statusText,
        headers: response.headers
      });
    }
    return response;
  }
};
export {
  index_default as default
};
//# sourceMappingURL=index.js.map
