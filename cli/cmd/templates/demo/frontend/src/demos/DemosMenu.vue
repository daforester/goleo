<script setup lang="ts">
import { navigate } from '../router'
import { demos } from './registry'
</script>

<template>
  <div class="wrap">
    <button class="demo-back" @click="navigate('/')">&larr; Home</button>
    <h1 style="margin: 0.5rem 0 0.25rem; font-size: 1.8rem">Bridge demos</h1>
    <p class="muted" style="margin-bottom: 1.25rem">
      Each demo exercises one Goleo bridge feature. Open one to try it on the
      current platform — support badges show where it works.
    </p>
    <div class="demo-grid">
      <button
        v-for="d in demos"
        :key="d.id"
        class="demo-card"
        @click="navigate('/demos/' + d.id)"
      >
        <span class="icon">{{ d.icon }}</span>
        <span class="title">{{ d.title }}</span>
        <span class="desc">{{ d.description }}</span>
      </button>
    </div>
  </div>
</template>
