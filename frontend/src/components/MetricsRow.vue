<script setup lang="ts">
import type { Card } from '../types'

defineProps<{ cards: Card[] }>()
</script>

<template>
  <section class="metrics-row">
    <article v-for="card in cards" :key="card.label" class="metric-card">
      <span>{{ card.label }}</span>
      <strong>{{ card.value }}</strong>
    </article>
    <p v-if="cards.length === 0" class="muted">暂无指标。定位或数据到位后会自动出现。</p>
  </section>
</template>
