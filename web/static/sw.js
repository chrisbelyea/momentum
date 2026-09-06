/*
 * Momentum's offline shell cache.
 *
 * This worker deliberately caches static assets only. The application uses
 * cookie-authenticated requests, so pages, API responses, CalDAV resources,
 * and authentication endpoints must always go to the network and must never
 * be persisted in a browser cache by this worker.
 */
const STATIC_CACHE = "momentum-static-v1";
const PRECACHE = [
  "/static/manifest.json",
  "/static/icon-192.png",
  "/static/icon-512.png"
];

self.addEventListener("install", (event) => {
  event.waitUntil(
    caches.open(STATIC_CACHE)
      .then((cache) => cache.addAll(PRECACHE))
      .then(() => self.skipWaiting())
  );
});

self.addEventListener("activate", (event) => {
  event.waitUntil(
    caches.keys()
      .then((keys) => Promise.all(
        keys
          .filter((key) => key.startsWith("momentum-static-") && key !== STATIC_CACHE)
          .map((key) => caches.delete(key))
      ))
      .then(() => self.clients.claim())
  );
});

self.addEventListener("fetch", (event) => {
  const request = event.request;
  const url = new URL(request.url);

  // Only same-origin, idempotent static assets are eligible for caching.
  // In particular, do not add a navigation fallback or cache API/auth data.
  if (request.method !== "GET" || url.origin !== self.location.origin ||
      !url.pathname.startsWith("/static/") || url.pathname === "/static/sw.js") {
    return;
  }

  event.respondWith(
    caches.match(request).then((cached) => cached || fetch(request).then((response) => {
      if (response.ok) {
        const copy = response.clone();
        caches.open(STATIC_CACHE).then((cache) => cache.put(request, copy));
      }
      return response;
    }))
  );
});
