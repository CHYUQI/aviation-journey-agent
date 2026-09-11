<script setup lang="ts">
import type { State } from '../types'

defineProps<{ state: State; connected: boolean }>()

function formatTime(iso: string) {
  const d = new Date(iso)
  return Number.isNaN(d.getTime())
    ? iso
    : d.toLocaleString('zh-CN', { hour12: false })
}
</script>

<template>
  <div class="freshness-bar">
    <span>{{ connected ? 'SSE 已连接' : '等待实时连接' }}</span>
    <span>数据质量：{{ state.quality }}</span>
    <span>更新于：{{ formatTime(state.updatedAt) }}</span>
  </div>
</template>
