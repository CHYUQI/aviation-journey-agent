<script setup lang="ts">
import type { AdviceMetric } from '../types/domain'
defineProps<{ metrics: AdviceMetric[] }>()
</script>
<template>
  <section class="metrics-row"><article v-for="metric in metrics" :key="metric.key" class="metric-card"><span>{{ metric.label }}</span><strong>{{ metric.value }}</strong><small>{{ metric.unit }}</small></article></section>
</template>
