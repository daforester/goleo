<script setup lang="ts">
import { ref } from 'vue'
import { clipboardReadText, clipboardWriteText } from '@goleo/bridge'
import DemoFrame from './DemoFrame.vue'

const toWrite = ref('Copied from Goleo 🦁')
const readBack = ref('')
const status = ref('')
const err = ref('')

async function write() {
  err.value = ''
  status.value = ''
  try {
    await clipboardWriteText(toWrite.value)
    status.value = 'Wrote to the clipboard.'
  } catch (e) {
    err.value = String(e)
  }
}

async function read() {
  err.value = ''
  try {
    readBack.value = await clipboardReadText()
  } catch (e) {
    err.value = String(e)
  }
}
</script>

<template>
  <DemoFrame id="clipboard">
    <div class="panel">
      <h2>Write</h2>
      <div class="row">
        <input class="input" style="flex: 1; min-width: 12rem" v-model="toWrite" />
        <button class="btn btn-primary" @click="write">Copy</button>
      </div>
      <p class="muted" v-if="status">{{ status }}</p>
    </div>

    <div class="panel">
      <h2>Read</h2>
      <button class="btn" @click="read">Paste from clipboard</button>
      <div class="result" v-if="readBack">{{ readBack }}</div>
    </div>

    <div class="result result--error" v-if="err">{{ err }}</div>
  </DemoFrame>
</template>
