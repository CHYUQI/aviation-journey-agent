<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue'
import { useJourneyStore } from '../stores/journey'
import { createJourney } from '../api/journey'
import type { CreateJourneyRequest } from '../types'
import StatusBar from '../components/StatusBar.vue'
import MetricsRow from '../components/MetricsRow.vue'
import Timeline from '../components/Timeline.vue'
import ActionCardList from '../components/ActionCardList.vue'
import FreshnessBar from '../components/FreshnessBar.vue'
import StageConfirm from '../components/StageConfirm.vue'
import ToastHost from '../components/ToastHost.vue'
import type { ToastItem } from '../components/ToastHost.vue'

const store = useJourneyStore()
const snapshot = computed(() => store.snapshot)

// ---- 创建行程（契约：POST /journey 返回 202，随后 load 订阅）----
const journeyId = ref<string | null>(null)
const creating = ref(false)
const formError = ref<string | null>(null)
const form = ref<CreateJourneyRequest>({
  flights: [{ number: '', date: today(), from: '', to: '' }],
  hasBaggage: false,
})

function today() {
  const d = new Date()
  const m = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  return `${d.getFullYear()}-${m}-${day}`
}

function buildPayload(): CreateJourneyRequest {
  const f = form.value.flights[0]
  return {
    flights: [{
      number: f.number.trim().toUpperCase(),
      date: f.date,
      from: f.from.trim().toUpperCase(),
      to: f.to.trim().toUpperCase(),
    }],
    hasBaggage: form.value.hasBaggage,
  }
}

function validate() {
  const f = form.value.flights[0]
  if (!f.number.trim() || !f.date || !f.from.trim() || !f.to.trim()) {
    formError.value = '请填写完整航班信息'
    return false
  }
  if (!/^[A-Z]{3}$/.test(f.from.trim().toUpperCase()) || !/^[A-Z]{3}$/.test(f.to.trim().toUpperCase())) {
    formError.value = '机场代码需为 3 位大写字母（如 PVG）'
    return false
  }
  formError.value = null
  return true
}

async function submit() {
  if (!validate()) return
  creating.value = true
  try {
    const accepted = await createJourney(buildPayload())
    journeyId.value = accepted.journeyId
    await store.load(accepted.journeyId)
  } catch {
    formError.value = '创建行程失败，请稍后重试'
  } finally {
    creating.value = false
  }
}

// ---- failed 重新分析：用当前航班信息重新创建行程，触发一次新分析 ----
async function retry() {
  if (!validate()) return
  creating.value = true
  try {
    const accepted = await createJourney(buildPayload())
    journeyId.value = accepted.journeyId
    await store.load(accepted.journeyId)
  } catch {
    formError.value = '重新分析失败，请稍后重试'
  } finally {
    creating.value = false
  }
}

// ---- alert 提醒（契约 7.3）：变了才 toast；red→error，其余→warning ----
const toasts = ref<ToastItem[]>([])
let lastAlert: string | null = null
let toastSeq = 0

watch(() => snapshot.value?.advice.alert, (alert) => {
  if (alert && alert !== lastAlert) {
    const id = ++toastSeq
    toasts.value.push({
      id,
      message: alert,
      type: snapshot.value?.advice.risk === 'red' ? 'error' : 'warning',
    })
    window.setTimeout(() => {
      toasts.value = toasts.value.filter((t) => t.id !== id)
    }, 5000)
  }
  lastAlert = alert ?? null
})

// ---- failed 保留已有内容：用最后一次 ready 快照渲染，状态与错误取当前快照 ----
const display = computed(() => {
  const s = snapshot.value
  if (!s) return null
  if (s.status === 'failed' && store.lastReady) {
    return { ...store.lastReady, status: 'failed' as const, error: s.error }
  }
  return s
})

onUnmounted(() => store.stop())
</script>

<template>
  <main class="dashboard">
    <header class="hero">
      <p class="eyebrow">AVIATION JOURNEY AGENT</p>
      <h1>航旅智行</h1>
      <p>把航班、机场和旅客状态转换成下一步可执行行动。</p>
    </header>

    <!-- 无行程：创建表单 -->
    <section v-if="!journeyId" class="panel journey-form">
      <div class="panel-heading"><h2>创建行程</h2><span>填写首个航段信息</span></div>
      <form @submit.prevent="submit">
        <div class="form-grid">
          <label>航班号<input v-model.trim="form.flights[0].number" placeholder="CA1234" maxlength="8" /></label>
          <label>日期<input v-model="form.flights[0].date" type="date" /></label>
          <label>出发机场<input v-model.trim="form.flights[0].from" placeholder="PVG" maxlength="3" /></label>
          <label>到达机场<input v-model.trim="form.flights[0].to" placeholder="PEK" maxlength="3" /></label>
        </div>
        <label class="baggage-check"><input v-model="form.hasBaggage" type="checkbox" /> 携带托运行李</label>
        <p v-if="formError" class="form-error">{{ formError }}</p>
        <button class="primary" type="submit" :disabled="creating">
          {{ creating ? '正在分析…' : '开始分析' }}
        </button>
      </form>
    </section>

    <!-- 有行程：三态渲染 -->
    <template v-else-if="display">
      <StatusBar
        :status="display.status"
        :stage="display.advice.stage"
        :risk="display.advice.risk"
      />
      <FreshnessBar
        :state="display.state"
        :connected="store.connected"
        :fallback-active="store.fallbackActive"
        :reasons="display.advice.reasons"
      />

      <!-- failed 横幅 -->
      <section v-if="display.status === 'failed'" class="error-banner">
        <p>{{ display.error || '分析失败，请重新尝试' }}</p>
        <button type="button" class="primary" :disabled="creating" @click="retry">重新分析</button>
      </section>

      <!-- processing：骨架屏，不显示风险颜色、不渲染行动卡片 -->
      <section v-if="display.status === 'processing'" class="skeleton" aria-label="正在分析行程">
        <div v-for="i in 3" :key="i" class="skeleton-card"></div>
        <p class="muted">正在分析行程…</p>
      </section>

      <!-- ready / failed：渲染内容（failed 已用 lastReady 保留） -->
      <template v-else>
        <p v-if="display.advice.alert" class="alert-banner" :data-risk="display.advice.risk">
          {{ display.advice.alert }}
        </p>
        <MetricsRow :cards="display.advice.cards" />
        <section class="content-grid">
          <Timeline
            :timeline="display.state.timeline"
            :guide="display.state.guide"
            :stage="display.advice.stage"
            :updated-at="display.state.updatedAt"
          />
          <div class="side-stack">
            <ActionCardList :actions="display.advice.actions" />
            <StageConfirm :journey-id="journeyId" :disabled="creating" />
          </div>
        </section>
      </template>
    </template>

    <ToastHost :toasts="toasts" />
  </main>
</template>
