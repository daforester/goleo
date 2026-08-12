import { defineConfig } from 'vitest/config'

export default defineConfig({
  test: {
    // happy-dom, not node: most of bridge/src is browser-API fallback logic
    // (navigator.clipboard, localStorage, Notification, navigator.bluetooth), and
    // the branches that pick between the Go backend and those fallbacks are exactly
    // where the bugs have been. Testing them under a bare node global would mean
    // hand-rolling every one of those APIs.
    environment: 'happy-dom',
    include: ['src/**/*.test.ts'],
    // No coverage thresholds on purpose: this suite starts as regression tests for
    // specific known-wrong behaviours, so a percentage would be noise.
  },
})
