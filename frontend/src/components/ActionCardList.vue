<script setup lang="ts">
import type { AdviceAction } from '../types/domain'
import NavigateButton from './NavigateButton.vue'
defineProps<{ actions: AdviceAction[] }>()
</script>
<template>
  <section class="panel"><div class="panel-heading"><h2>下一步行动</h2><span>建议不会自动执行高影响操作</span></div><article v-for="action in actions" :key="action.id" class="action-card"><div><p class="action-kind">{{ action.kind }}</p><h3>{{ action.title }}</h3><p>{{ action.detail }}</p></div><NavigateButton v-if="action.navigation" :navigation="action.navigation" /></article><p v-if="actions.length === 0" class="muted">当前没有可展示的行动建议。</p></section>
</template>
