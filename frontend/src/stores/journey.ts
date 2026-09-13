import { defineStore } from 'pinia'
import { ref } from 'vue'
import { createStream, getState } from '../api/journey'
import type { JourneySnapshot } from '../types'

// 契约 3.4：断开超过 20 秒未恢复时，回退到 15 秒轮询 /state
const FALLBACK_AFTER_MS = 20_000
const POLL_INTERVAL_MS = 15_000

export const useJourneyStore = defineStore('journey', () => {
  const snapshot = ref<JourneySnapshot | null>(null)
  // failed 快照会把 state/advice 清空，这里保留最后一次 ready，供界面"保留已有内容"
  const lastReady = ref<JourneySnapshot | null>(null)
  const connected = ref(false)
  const fallbackActive = ref(false)

  let stream: EventSource | null = null
  let fallbackTimer: ReturnType<typeof setTimeout> | null = null
  let pollTimer: ReturnType<typeof setInterval> | null = null
  let currentId: string | null = null

  function clearFallback() {
    if (fallbackTimer !== null) {
      clearTimeout(fallbackTimer)
      fallbackTimer = null
    }
    if (pollTimer !== null) {
      clearInterval(pollTimer)
      pollTimer = null
    }
    fallbackActive.value = false
  }

  async function pollState() {
    if (!currentId) return
    try {
      const next = await getState(currentId)
      snapshot.value = next
      if (next.status === 'ready') lastReady.value = next
    } catch {
      // 轮询失败保持现状，等下一个周期
    }
  }

  function apply(next: JourneySnapshot) {
    snapshot.value = next
    connected.value = true
    fallbackActive.value = false
    if (next.status === 'ready') lastReady.value = next
    // 连接恢复或快照到达，取消轮询回退
    clearFallback()
  }

  async function load(id: string) {
    currentId = id
    const next = await getState(id)
    if (next.status === 'ready') lastReady.value = next
    snapshot.value = next

    stream?.close()
    clearFallback()
    connected.value = false

    stream = createStream(id, apply)
    stream.onopen = () => {
      connected.value = true
      fallbackActive.value = false
      clearFallback()
    }
    stream.onerror = () => {
      // EventSource 自身按 retry 自动重连；超时未恢复才启用轮询兜底
      connected.value = false
      if (fallbackTimer === null && pollTimer === null) {
        fallbackTimer = setTimeout(() => {
          fallbackTimer = null
          fallbackActive.value = true
          pollTimer = setInterval(pollState, POLL_INTERVAL_MS)
        }, FALLBACK_AFTER_MS)
      }
    }
  }

  function stop() {
    stream?.close()
    stream = null
    clearFallback()
    connected.value = false
    fallbackActive.value = false
    currentId = null
  }

  return { snapshot, lastReady, connected, fallbackActive, load, stop }
})
