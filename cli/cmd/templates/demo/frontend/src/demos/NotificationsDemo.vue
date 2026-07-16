<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { sendNotification, isPermissionGranted, requestPermission } from '@goleo/bridge'
import DemoFrame from './DemoFrame.vue'

const perm = ref('unknown')
const title = ref('Hello from Goleo')
const body = ref('This is a native notification.')
const status = ref('')
const err = ref('')

onMounted(async () => {
  perm.value = (await isPermissionGranted()) ? 'granted' : 'default'
})

async function ask() {
  err.value = ''
  try {
    perm.value = await requestPermission()
  } catch (e) {
    err.value = String(e)
  }
}

async function send() {
  err.value = ''
  status.value = ''
  try {
    if (!(await isPermissionGranted())) {
      const r = await requestPermission()
      perm.value = r
      if (r !== 'granted') {
        status.value = 'Permission was not granted.'
        return
      }
    }
    await sendNotification({ title: title.value, body: body.value })
    status.value = 'Notification sent!'
  } catch (e) {
    err.value = String(e)
  }
}
</script>

<template>
  <DemoFrame id="notifications">
    <div class="panel">
      <p>Permission: <strong>{{ perm }}</strong></p>
      <div class="row" style="margin-top: 0.5rem">
        <button class="btn" @click="ask">Request permission</button>
      </div>
    </div>

    <div class="panel">
      <h2>Send a notification</h2>
      <div class="row">
        <input class="input" style="flex: 1; min-width: 12rem" v-model="title" placeholder="Title" />
      </div>
      <div class="row" style="margin-top: 0.5rem">
        <input class="input" style="flex: 1; min-width: 12rem" v-model="body" placeholder="Body" />
      </div>
      <div class="row" style="margin-top: 0.75rem">
        <button class="btn btn-primary" @click="send">Send notification</button>
      </div>
      <p class="muted" v-if="status">{{ status }}</p>
    </div>

    <div class="result result--error" v-if="err">{{ err }}</div>
  </DemoFrame>
</template>
