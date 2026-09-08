<script setup lang="ts">
import type { Advice } from '../types/domain'
defineProps<{ advice: Advice }>()
</script>
<template>
  <section class="status-bar" :data-risk="advice.riskLevel">
    <div><span class="label">当前阶段</span><strong>{{ advice.stage }}</strong></div>
    <div><span class="label">风险等级</span><strong>{{ advice.riskLevel }}</strong></div>
    <div><span class="label">风险分</span><strong>{{ advice.riskScore }}</strong></div>
  </section>
</template>
