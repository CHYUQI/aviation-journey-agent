<script setup lang="ts">
import { computed } from 'vue'
import type { Stage, TimelineNode } from '../types'

const props = defineProps<{
  timeline: TimelineNode[]
  guide: string[]
  stage: Stage
}>()

function formatTime(iso: string) {
  const d = new Date(iso)
  return Number.isNaN(d.getTime())
    ? iso
    : d.toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' })
}

// 高亮下一个尚未到达的节点。纯时间比较，不参与任何风险判断
const activeIndex = computed(() => {
  const now = Date.now()
  const i = props.timeline.findIndex((n) => new Date(n.time).getTime() > now)
  return i === -1 ? props.timeline.length : i
})
</script>

<template>
  <section class="panel">
    <div class="panel-heading">
      <h2>行程时间轴</h2>
      <span>{{ stage }}</span>
    </div>

    <ol v-if="timeline.length" class="timeline">
      <li
        v-for="(node, i) in timeline"
        :key="node.label + node.time"
        :class="{ active: i === activeIndex }"
      >
        <span>{{ formatTime(node.time) }}</span>{{ node.label }}
      </li>
    </ol>
    <p v-else class="muted">航班时间待获取。</p>

    <template v-if="guide.length">
      <div class="panel-heading">
        <h2>机场内指引</h2>
        <span>文字步行指引</span>
      </div>
      <ol class="guide">
        <li v-for="line in guide" :key="line">{{ line }}</li>
      </ol>
    </template>
  </section>
</template>
