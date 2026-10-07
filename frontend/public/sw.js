const OFFLINE_CACHE = "payverge-offline-v2";
const META_CACHE = "payverge-pwa-meta-v1";
const OFFLINE_LOCALE_KEY = "/__pwa_offline_locale__";
const OFFLINE_PAGES = {
  en: "/offline/en.html",
  es: "/offline/es.html",
  "es-AR": "/offline/es-ar.html",
};

function normalizeLocale(locale) {
  if (locale === "es-AR" || locale === "es-ar") return "es-AR";
  if (typeof locale === "string" && locale.toLowerCase().startsWith("es")) return "es";
  return "en";
}

self.addEventListener("install", (event) => {
  event.waitUntil(
    caches
      .open(OFFLINE_CACHE)
      .then((cache) =>
        cache.addAll([
          OFFLINE_PAGES.en,
          OFFLINE_PAGES.es,
          OFFLINE_PAGES["es-AR"],
          "/android-chrome-192x192.png",
        ]),
      )
      .then(() => self.skipWaiting()),
  );
});

self.addEventListener("activate", (event) => {
  event.waitUntil(
    caches
      .keys()
      .then((names) =>
        Promise.all(
          names
            .filter((name) => name.startsWith("payverge-") && name !== OFFLINE_CACHE && name !== META_CACHE)
            .map((name) => caches.delete(name)),
        ),
      )
      .then(() => self.clients.claim()),
  );
});

self.addEventListener("message", (event) => {
  if (event.data?.type !== "SET_OFFLINE_LOCALE") return;

  event.waitUntil(
    caches
      .open(META_CACHE)
      .then((cache) => cache.put(OFFLINE_LOCALE_KEY, new Response(normalizeLocale(event.data.locale)))),
  );
});

async function offlineLocale() {
  const cache = await caches.open(META_CACHE);
  const stored = await cache.match(OFFLINE_LOCALE_KEY);
  return stored ? normalizeLocale(await stored.text()) : "en";
}

async function offlineResponse() {
  const cache = await caches.open(OFFLINE_CACHE);
  const localized = await cache.match(OFFLINE_PAGES[await offlineLocale()]);
  if (localized) return localized;

  const english = await cache.match(OFFLINE_PAGES.en);
  if (english) return english;

  return new Response("Offline", {
    status: 503,
    headers: { "Content-Type": "text/plain; charset=utf-8" },
  });
}

self.addEventListener("fetch", (event) => {
  const { request } = event;
  if (request.mode !== "navigate") return;

  event.respondWith(
    fetch(request).catch(() => offlineResponse()),
  );
});

self.addEventListener("push", (event) => {
  let data = { title: "Payverge", body: "", url: "/app" };
  try {
    if (event.data) data = { ...data, ...event.data.json() };
  } catch {
    // The default notification remains safe when an upstream payload is malformed.
  }

  event.waitUntil(
    self.registration.showNotification(data.title || "Payverge", {
      body: data.body || "",
      icon: "/android-chrome-192x192.png",
      badge: "/favicon-32x32.png",
      data: { url: data.url || "/app" },
    }),
  );
});

self.addEventListener("notificationclick", (event) => {
  event.notification.close();
  event.waitUntil(self.clients.openWindow(event.notification.data?.url || "/app"));
});
