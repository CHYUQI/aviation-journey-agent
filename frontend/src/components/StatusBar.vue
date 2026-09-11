<script setup lang="ts">
import type { AnalysisStatus, Risk, Stage } from '../types'

defineProps<{
  status: AnalysisStatus
  stage: Stage
  risk: Risk
}>()
</script>

<template>
  <section class="status-bar" :data-risk="risk">
    <div><span class="label">分析状态</span><strong>{{ status }}</strong></div>
    <div><span class="label">当前阶段</span><strong>{{ stage }}</strong></div>
    <div><span class="label">风险等级</span><strong>{{ risk }}</strong></div>
  </section>
</template>
