import { ref } from 'vue'
import { updateLocation } from '../api/journey'
import type { ManualStage } from '../types'

export function useLocation() {
  const granted = ref(false)
  const error = ref<string | null>(null)

  // 上报定位坐标。返回的 Promise 在成功或失败后都会 resolve，
  // 让调用方可以显示"定位中…"并在结束后收起状态。
  function locate(journeyId: string) {
    if (!navigator.geolocation) {
      error.value = '当前浏览器不支持定位'
      return Promise.resolve()
    }
    error.value = null
    return new Promise<void>((resolve) => {
      navigator.geolocation.getCurrentPosition(
        async (position) => {
          try {
            await updateLocation(journeyId, {
              lat: position.coords.latitude,
              lng: position.coords.longitude,
            })
            granted.value = true
            error.value = null
          } catch {
            granted.value = false
            error.value = '定位上报失败，请稍后重试'
          }
          resolve()
        },
        (reason) => {
          error.value = reason.message
          resolve()
        },
      )
    })
  }

  // 定位不可用时的降级入口：旅客手动确认所处阶段
  async function confirmStage(journeyId: string, stage: ManualStage) {
    await updateLocation(journeyId, { stage })
  }

  return { granted, error, locate, confirmStage }
}