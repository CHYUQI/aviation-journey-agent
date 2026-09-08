<script setup lang="ts">
defineProps<{ state: Record<string, unknown>; stage: string }>()
</script>
<template>
  <section class="panel"><div class="panel-heading"><h2>行程时间轴</h2><span>机场内使用文字指引</span></div><ol class="timeline"><li :class="{ active: stage === 'en_route' }"><span>01</span>前往机场</li><li :class="{ active: stage === 'at_airport' }"><span>02</span>值机 / 托运</li><li :class="{ active: stage === 'security' }"><span>03</span>通过安检</li><li :class="{ active: stage === 'boarding' }"><span>04</span>前往登机口</li></ol></section>
</template>
