<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { pushSubscribe, pushUnsubscribe, pushGetSubscription } from '@goleo/bridge'
import type { PushSubscriptionData } from '@goleo/bridge'
import DemoFrame from './DemoFrame.vue'

const serverKey = ref('')
const sub = ref<PushSubscriptionData | null>(null)
const err = ref('')

async function refresh() {
  err.value = ''
  try {
    sub.value = await pushGetSubscription()
  } catch (e) {
    err.value = String(e)
  }
}

async function subscribe() {
  err.value = ''
  try {
    sub.value = await pushSubscribe(serverKey.value || undefined)
  } catch (e) {
    err.value = String(e)
  }
}

async function unsubscribe() {
  err.value = ''
  try {
    await pushUnsubscribe()
    sub.value = null
  } catch (e) {
    err.value = String(e)
  }
}

onMounted(refresh)
</script>

<template>
  <DemoFrame id="push">
    <div class="panel">
      <p class="muted">
        Push needs a push service and a VAPID public key. In PWA/web mode a
        service worker must be registered (it is in <code>public/sw.js</code>).
      </p>
      <div class="row" style="margin-top: 0.5rem">
        <input class="input" style="flex: 1; min-width: 12rem" v-model="serverKey" placeholder="VAPID public key (optional)" />
      </div>
      <div class="row" style="margin-top: 0.5rem">
        <button class="btn btn-primary" @click="subscribe">Subscribe</button>
        <button class="btn" @click="unsubscribe" :disabled="!sub">Unsubscribe</button>
        <button class="btn" @click="refresh">Check subscription</button>
      </div>
      <div class="result" v-if="sub">
        Subscribed.<br />Endpoint: {{ sub.endpoint }}
      </div>
      <div class="result" v-else>No active subscription.</div>
    </div>

    <div class="result result--error" v-if="err">{{ err }}</div>
  </DemoFrame>
</template>
