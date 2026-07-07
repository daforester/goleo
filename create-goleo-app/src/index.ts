#!/usr/bin/env node

import { createApp } from './create-app.js'

const projectName = process.argv[2]

if (!projectName) {
  console.error('Usage: npm create goleo-app@latest <project-name>')
  process.exit(1)
}

createApp(projectName).catch((err) => {
  console.error('Failed to create project:', err.message)
  process.exit(1)
})
