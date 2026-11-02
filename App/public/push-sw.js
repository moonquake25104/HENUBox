/* Notifications only: no fetch handler and no cache of private application data. */
self.addEventListener("install", () => self.skipWaiting());
self.addEventListener("activate", event => event.waitUntil(self.clients.claim()));
self.addEventListener("push", event => {
  let data = {};
  try { data = event.data?.json() || {}; } catch { /* Always display a safe visible notification. */ }
  event.waitUntil(self.registration.showNotification("HENUBox · 手机测试", {
    body: typeof data.body === "string" ? data.body.slice(0, 200) : "收到一条测试提醒，请打开HENUBox查看。",
    icon: "/favicon.svg",
    tag: typeof data.tag === "string" ? data.tag.slice(0, 80) : "xique-test",
    data: { url: "/#/settings" },
  }));
});
self.addEventListener("notificationclick", event => {
  event.notification.close();
  event.waitUntil((async () => {
    const windows = await self.clients.matchAll({ type: "window", includeUncontrolled: true });
    for (const client of windows) {
      if (new URL(client.url).origin === self.location.origin) {
        await client.navigate("/#/settings");
        return client.focus();
      }
    }
    return self.clients.openWindow("/#/settings");
  })());
});
