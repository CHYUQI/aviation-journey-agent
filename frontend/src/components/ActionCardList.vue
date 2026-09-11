<script setup lang="ts">
import type { Action } from '../types'
import NavigateButton from './NavigateButton.vue'

defineProps<{ actions: Action[] }>()
</script>

<template>
  <section class="panel">
    <div class="panel-heading">
      <h2>下一步行动</h2>
      <span>建议不会自动执行高影响操作</span>
    </div>
    <article v-for="action in actions" :key="action.id" class="action-card">
      <div>
        <h3>{{ action.title }}</h3>
        <p>{{ action.detail }}</p>
      </div>
      <NavigateButton v-if="action.nav" :nav="action.nav" />
    </article>
    <p v-if="actions.length === 0" class="muted">当前没有可展示的行动建议。</p>
  </section>
</template>
