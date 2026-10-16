<script setup lang="ts">
import type { AnalysisStatus, Risk, Stage } from '../types'
import { AnalysisStatusLabel, RiskLabel, StageLabel } from '../types'

defineProps<{
  status: AnalysisStatus
  stage: Stage
  risk: Risk
}>()
</script>

<template>
  <section class="status-bar" :data-risk="risk">
    <div><span class="label">分析状态</span><strong>{{ AnalysisStatusLabel[status] }}</strong></div>
    <div><span class="label">当前阶段</span><strong>{{ StageLabel[stage] }}</strong></div>
    <div><span class="label">风险等级</span><strong>{{ RiskLabel[risk] }}</strong></div>
  </section>
</template>
