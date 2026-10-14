import { ref } from 'vue'
import { updateLocation } from '../api/journey'
import type { ManualStage } from '../types'

// 浏览器定位失败时的中文提示（GeolocationPositionError.code：1 拒绝 / 2 不可用 / 3 超时）
const GEO_ERRORS: Record<number, string> = {
  1: '定位权限被拒绝，请在浏览器地址栏允许本站定位，或直接手动确认所处环节',
  2: '系统位置服务不可用，请检查系统是否开启位置服务，或直接手动确认所处环节',
  3: '定位超时，请重试，或直接手动确认所处环节',
}

export function useLocation() {
  const granted = ref(false)
  const error = ref<string | null>(null)

  // 上报定位坐标。拿到坐标后后端会自动解除手动确认的阶段。
  // 返回的 Promise 在「坐标已上报」或「定位失败」之后 resolve，供按钮控制 loading 态。
  function locate(journeyId: string): Promise<void> {
    if (!navigator.geolocation) {
      error.value = '当前浏览器不支持定位'
      return Promise.resolve()
    }
    return new Promise<void>((resolve) => {
      navigator.geolocation.getCurrentPosition(
        async (position) => {
          granted.value = true
          error.value = null
          try {
            await updateLocation(journeyId, {
              lat: position.coords.latitude,
              lng: position.coords.longitude,
            })
          } catch {
            error.value = '坐标上报失败，请稍后重试'
          }
          resolve()
        },
        (reason) => {
          error.value = GEO_ERRORS[reason.code] ?? reason.message
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
