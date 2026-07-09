<script setup lang="ts">
import { ref } from 'vue'
import { appDataDir, writeTextFile, readTextFile, listDir, deleteFile } from '@goleo/bridge'
import type { FileEntry } from '@goleo/bridge'
import DemoFrame from './DemoFrame.vue'

const dir = ref('')
const content = ref('Hello from Goleo at ' + new Date().toISOString())
const readBack = ref('')
const entries = ref<FileEntry[]>([])
const status = ref('')
const err = ref('')

function filePath() {
  return (dir.value || '.') + '/goleo-demo.txt'
}

async function ensureDir() {
  if (!dir.value) dir.value = await appDataDir('goleo-demo')
  return dir.value
}

async function resolveDir() {
  err.value = ''
  try {
    status.value = 'App data dir: ' + (await ensureDir())
  } catch (e) {
    err.value = String(e)
  }
}

async function write() {
  err.value = ''
  try {
    await ensureDir()
    await writeTextFile(filePath(), content.value)
    status.value = 'Wrote ' + filePath()
  } catch (e) {
    err.value = String(e)
  }
}

async function read() {
  err.value = ''
  try {
    readBack.value = await readTextFile(filePath())
  } catch (e) {
    err.value = String(e)
  }
}

async function list() {
  err.value = ''
  try {
    // A nil Go slice serializes to JSON null, so coalesce to an empty array.
    entries.value = (await listDir(await ensureDir())) ?? []
  } catch (e) {
    err.value = String(e)
  }
}

async function remove() {
  err.value = ''
  try {
    await deleteFile(filePath())
    readBack.value = ''
    status.value = 'Deleted ' + filePath()
  } catch (e) {
    err.value = String(e)
  }
}
</script>

<template>
  <DemoFrame id="filesystem">
    <div class="panel">
      <h2>App data directory</h2>
      <button class="btn" @click="resolveDir">Resolve appDataDir()</button>
      <p class="muted" v-if="status">{{ status }}</p>
    </div>

    <div class="panel">
      <h2>Write &amp; read a file</h2>
      <textarea class="textarea" v-model="content"></textarea>
      <div class="row" style="margin-top: 0.5rem">
        <button class="btn btn-primary" @click="write">Write file</button>
        <button class="btn" @click="read">Read it back</button>
        <button class="btn" @click="remove">Delete</button>
      </div>
      <div class="result" v-if="readBack">{{ readBack }}</div>
    </div>

    <div class="panel">
      <h2>List directory</h2>
      <button class="btn" @click="list">List app data dir</button>
      <ul style="margin-top: 0.5rem; padding-left: 1.1rem" v-if="entries.length">
        <li v-for="e in entries" :key="e.path">
          {{ e.isDir ? '📁' : '📄' }} {{ e.name }} <span class="muted">({{ e.size }} bytes)</span>
        </li>
      </ul>
    </div>

    <div class="result result--error" v-if="err">{{ err }}</div>
  </DemoFrame>
</template>
