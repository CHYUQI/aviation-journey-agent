<script setup lang="ts">
import type { JourneySnapshot } from '../types/domain'
defineProps<{ snapshot: JourneySnapshot; connected: boolean }>()
</script>
<template><div class="freshness-bar"><span>{{ connected ? 'SSE 已连接' : '等待实时连接' }}</span><span>数据质量：{{ snapshot.state.dataQuality?.status ?? 'unknown' }}</span><span>更新于：{{ snapshot.generatedAt }}</span></div></template>
