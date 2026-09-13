<script setup lang="ts">
import { ref } from 'vue'
import { useLocation } from '../stores/location'
import type { ManualStage } from '../types'

// 契约 7.6：定位不可用时的降级入口，选项固定为 6 个 ManualStage
const props = defineProps<{ journeyId: string; disabled: boolean }>()

const stages: { value: ManualStage; label: string }[] = [
  { value: 'en_route', label: '前往机场' },
  { value: 'at_airport', label: '已到机场' },
  { value: 'check_in', label: '值机' },
  { value: 'security', label: '安检' },
  { value: 'waiting', label: '候机' },
  { value: 'boarding', label: '登机' },
]

const { confirmStage } = useLocation()
const pending = ref(false)

async function pick(stage: ManualStage) {
  if (props.disabled || pending.value) return
  pending.value = true
  try {
    await confirmStage(props.journeyId, stage)
  } finally {
    pending.value = false
  }
}
</script>

<template>
  <section class="panel stage-confirm">
    <div class="panel-heading">
      <h2>当前在哪个环节？</h2>
      <span>定位不可用时手动确认</span>
    </div>
    <div class="stage-options">
      <button
        v-for="s in stages"
        :key="s.value"
        type="button"
        :disabled="disabled || pending"
        @click="pick(s.value)"
      >
        {{ s.label }}
      </button>
    </div>
  </section>
</template>
