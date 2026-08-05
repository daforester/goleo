<script setup lang="ts">
import { ref } from 'vue'
import { openFile, saveFile, selectFolder, showMessage, showPrompt } from '@goleo/bridge'
import DemoFrame from './DemoFrame.vue'

const out = ref('')
const err = ref('')

async function run(label: string, fn: () => Promise<unknown>) {
  err.value = ''
  try {
    const r = await fn()
    out.value = label + ' → ' + JSON.stringify(r)
  } catch (e) {
    err.value = String(e)
  }
}
</script>

<template>
  <DemoFrame id="dialogs">
    <div class="panel">
      <h2>Files &amp; folders</h2>
      <div class="row">
        <button class="btn" @click="run('openFile', () => openFile())">Open file</button>
        <button class="btn" @click="run('saveFile', () => saveFile({ title: 'Save as' }))">Save file</button>
        <button class="btn" @click="run('selectFolder', () => selectFolder())">Select folder</button>
      </div>
    </div>

    <div class="panel">
      <h2>Message box &amp; prompt</h2>
      <div class="row">
        <button
          class="btn"
          @click="run('showMessage', () => showMessage({ title: 'Hi', message: 'Hello from Goleo!', buttons: ['OK', 'Cancel'] }))"
        >Message box</button>
        <button
          class="btn"
          @click="run('showPrompt', () => showPrompt({ message: 'What is your name?', defaultValue: 'Goleo' }))"
        >Prompt</button>
      </div>
    </div>

    <div class="result" v-if="out">{{ out }}</div>
    <div class="result result--error" v-if="err">{{ err }}</div>
  </DemoFrame>
</template>
