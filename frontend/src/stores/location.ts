import { ref } from 'vue'
import { updateLocation } from '../api/journey'
import type { ManualStage } from '../types'

export function useLocation() {
  const granted = ref(false)
  const error = ref<string | null>(null)

  // 上报定位坐标。拿到坐标后后端会自动解除手动确认的阶段
  function locate(journeyId: string) {
    if (!navigator.geolocation) {
      error.value = '当前浏览器不支持定位'
      return
    }
    navigator.geolocation.getCurrentPosition(
      async (position) => {
        granted.value = true
        error.value = null
        await updateLocation(journeyId, {
          lat: position.coords.latitude,
          lng: position.coords.longitude,
        })
      },
      (reason) => {
        error.value = reason.message
      },
    )
  }

  // 定位不可用时的降级入口：旅客手动确认所处阶段
  async function confirmStage(journeyId: string, stage: ManualStage) {
    await updateLocation(journeyId, { stage })
  }

  return { granted, error, locate, confirmStage }
}
