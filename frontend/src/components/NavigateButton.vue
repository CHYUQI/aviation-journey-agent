<script setup lang="ts">
import type { Nav } from '../types'

const props = defineProps<{ nav: Nav }>()

// 优先唤起地图应用；应用未安装或用户从桌面访问时回退到网页版。
// 系统只负责跳转，不采集、不保存任何导航轨迹。
function open() {
  const timer = window.setTimeout(() => {
    window.location.href = props.nav.web
  }, 1200)

  // 成功唤起应用时页面会切到后台，此时取消网页兜底
  document.addEventListener(
    'visibilitychange',
    () => window.clearTimeout(timer),
    { once: true },
  )

  window.location.href = props.nav.app
}
</script>

<template>
  <button class="navigate-button" type="button" @click="open">立即导航</button>
</template>
