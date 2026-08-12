const CACHE_NAME = 'goleo-pwa-v1'

self.addEventListener('install', (event) => {
  event.waitUntil(
    caches.open(CACHE_NAME).then((cache) => {
      return cache.addAll([
        '/',
        '/index.html',
      ])
    })
  )
})

self.addEventListener('activate', (event) => {
  event.waitUntil(
    caches.keys().then((cacheNames) => {
      return Promise.all(
        cacheNames
          .filter((name) => name !== CACHE_NAME)
          .map((name) => caches.delete(name))
      )
    })
  )
})

self.addEventListener('fetch', (event) => {
  event.respondWith(
    caches.match(event.request).then((response) => {
      return response || fetch(event.request)
    })
  )
})

// Show notifications pushed from a push service (PWA push demo).
self.addEventListener('push', (event) => {
  const payload = (() => {
    try {
      return event.data ? event.data.json() : {}
    } catch {
      return { title: 'Goleo', body: event.data ? event.data.text() : '' }
    }
  })()
  event.waitUntil(
    self.registration.showNotification(payload.title || 'Goleo', {
      body: payload.body || '',
    })
  )
})
