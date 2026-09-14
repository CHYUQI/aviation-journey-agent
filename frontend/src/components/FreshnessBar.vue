<script setup lang="ts">
import type { State } from '../types'

defineProps<{
  state: State
  connected: boolean
  fallbackActive: boolean
  reasons: string[]
}>()

function formatTime(iso: string) {
  const d = new Date(iso)
  return Number.isNaN(d.getTime())
    ? iso
    : d.toLocaleString('zh-CN', { hour12: false })
}
</script>

<template>
  <div class="freshness-bar">
    <span v-if="connected" class="conn-ok">SSE 已连接</span>
    <span v-else-if="fallbackActive" class="conn-fallback">实时连接已断开，已切换轮询</span>
    <span v-else class="conn-waiting">等待实时连接</span>
    <span>数据质量：{{ state.quality }}</span>
    <span>更新于：{{ formatTime(state.updatedAt) }}</span>
  </div>

  <!-- 契约 7.5：quality != complete 时必须展示"数据可能不完整"提示及数据相关说明 -->
  <div v-if="state.quality !== 'complete'" class="quality-warning">
    <p class="quality-warning-title">数据可能不完整</p>
    <ul v-if="reasons.length" class="quality-reasons">
      <li v-for="reason in reasons" :key="reason">{{ reason }}</li>
    </ul>
  </div>
</template>
