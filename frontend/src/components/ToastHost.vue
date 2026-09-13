<script setup lang="ts">
// 契约 7.3：alert 只做"变了就弹"提醒，toast 等级由风险决定（red→error，其余→warning）
export interface ToastItem {
  id: number
  message: string
  type: 'error' | 'warning'
}

defineProps<{ toasts: ToastItem[] }>()
</script>

<template>
  <div class="toast-host" aria-live="polite">
    <div v-for="t in toasts" :key="t.id" class="toast" :data-type="t.type">
      {{ t.message }}
    </div>
  </div>
</template>
