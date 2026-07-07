#!/usr/bin/env node

// Utility script for Goleo npm integration
// This allows running goleo commands through npm

const { execSync } = require('child_process')
const path = require('path')

const args = process.argv.slice(2)
const goleoBin = path.join(__dirname, '..', 'cli', 'goleo')

try {
  const result = execSync(`${goleoBin} ${args.join(' ')}`, { stdio: 'inherit' })
  process.exit(result.status || 0)
} catch (err) {
  process.exit(err.status || 1)
}
