export default {
  async fetch(request, env, ctx) {
    if (request.method !== "GET" && request.method !== "HEAD") {
      return new Response("Method Not Allowed", { status: 405, headers: { Allow: "GET, HEAD" } });
    }

    const url = new URL(request.url);
    
    const allowedPaths = [
      /^\/$/,
      /^\/nix-cache-info$/,
      /^\/[a-z0-9]+\.narinfo$/,
      /^\/nar\/[a-z0-9]+\.nar(\.[a-z0-9]{2,4})?$/,
    ];

    if (!allowedPaths.some((regex) => regex.test(url.pathname))) {
      return new Response("Forbidden", { status: 403 });
    }

    url.search = ""; 
    const normalizedUrlString = url.toString();

    const needsModification = url.pathname === "/" || url.pathname === "/nix-cache-info";
    
    if (needsModification) {
      const cache = caches.default;
      const cacheKey = new Request(normalizedUrlString, {
        method: "GET",
        headers: request.headers
      });
      
      try {
        const cachedResponse = await cache.match(cacheKey);
        if (cachedResponse) {
          if (request.method === "HEAD") {
            return new Response(null, {
              status: cachedResponse.status,
              statusText: cachedResponse.statusText,
              headers: cachedResponse.headers
            });
          }
          return cachedResponse;
        }
      } catch (e) {
        console.error("Cache read error:", e);
      }
    }

    url.protocol = "https:";
    url.host = "cache.nixos.org";
    
    const forwardHeaders = {
      "User-Agent": request.headers.get("User-Agent") || "Nix/2.0 (Cloudflare-Worker)",
    };
    
    const isTextFile = url.pathname.endsWith('.narinfo') || 
                   url.pathname === '/nix-cache-info' ||
                   url.pathname === '/';

    if (isTextFile && request.headers.has("Accept-Encoding")) {
      forwardHeaders["Accept-Encoding"] = request.headers.get("Accept-Encoding");
    }
    
    if (request.headers.has("Range")) {
      forwardHeaders["Range"] = request.headers.get("Range");
    }

    let originResponse;
    try {
      originResponse = await fetch(url.toString(), {
        method: request.method,
        headers: forwardHeaders,
        cf: {
          cacheEverything: true,
          cacheTtl: 31536000, 
          cacheKey: normalizedUrlString,
        }
      });
    } catch (e) {
      console.error("Origin fetch error:", e);
      return new Response("Bad Gateway", { status: 502 });
    }

    if (![200, 206, 404].includes(originResponse.status)) {
        return originResponse;
    }

    let status = originResponse.status;
    let headers = new Headers(originResponse.headers);

    // Единая функция очистки заголовков
    const cleanHeaders = (headers) => {
      const toRemove = [
        "set-cookie", "cf-cache-status", "cf-ray",
        "x-amz-server-side-encryption", "x-amz-request-id", "x-amz-id-2", "x-amz-version-id",
        "via", "x-served-by", "x-cache", "x-cache-hits", "age",
        "server" // скрываем "AmazonS3"
        // ✅ НЕ удаляем "vary" - Cloudflare добавляет его для compressed content
      ];
      toRemove.forEach(key => headers.delete(key));
    };

    if (needsModification && status === 200) {
      const text = await originResponse.text();
      const newBody = text.replace(/cache\.nixos\.org/g, "nixos-cache-proxy.elxreno.com");
      
      headers.set("Content-Length", new TextEncoder().encode(newBody).length.toString());
      cleanHeaders(headers);
      headers.set("Cache-Control", "public, max-age=86400, stale-while-revalidate=3600, stale-if-error=3600");
      
      if (request.method === "GET") {
        const cache = caches.default;
        const cacheKey = new Request(normalizedUrlString, {
          method: "GET",
          headers: request.headers
        });
        const responseToCache = new Response(newBody, { status, headers });
        ctx.waitUntil(cache.put(cacheKey, responseToCache.clone()));
        return responseToCache;
      } else {
        return new Response(null, { status, headers });
      }
    }

    let body = originResponse.body;
    cleanHeaders(headers);

    if (status === 404) {
      headers.set("Cache-Control", "public, max-age=600, stale-if-error=300"); 
    } else if (status === 206) {
      headers.set("Cache-Control", "public, max-age=3600");
    } else if (url.pathname.startsWith('/nar/') || url.pathname.endsWith('.narinfo')) {
      headers.set("Cache-Control", "public, immutable, max-age=31536000, stale-while-revalidate=604800, stale-if-error=86400");
    } else {
      headers.set("Cache-Control", "public, immutable, max-age=31536000");
    }

    if (request.method === "HEAD") {
      body = null;
    }

    return new Response(body, {
      status,
      statusText: originResponse.statusText,
      headers,
    });
  },
};
