import { ref } from 'vue'

// Minimal, dependency-free hash router used by the demo browser. Routes:
//   #/            -> landing page (App.vue hero)
//   #/demos       -> the demo menu
//   #/demos/<id>  -> a single demo (id comes from demos/registry.ts)
//
// If you don't want the demo browser, delete the src/demos folder and this
// file, then render your own root component directly from App.vue.

function currentPath(): string {
  const h = window.location.hash.replace(/^#/, '')
  return h || '/'
}

export const path = ref(currentPath())

window.addEventListener('hashchange', () => {
  path.value = currentPath()
  // Scroll to top on navigation so long demos don't start mid-page.
  window.scrollTo({ top: 0 })
})

export function navigate(to: string): void {
  window.location.hash = to
}
