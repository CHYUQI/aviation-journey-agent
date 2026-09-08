import { ref } from 'vue'
import { updateLocation } from '../api/journey'

export function useLocation() {
  const granted = ref(false)
  const error = ref<string | null>(null)

  async function locate(journeyId: string) {
    if (!navigator.geolocation) {
      error.value = '当前浏览器不支持定位'
      return
    }
    navigator.geolocation.getCurrentPosition(async position => {
      granted.value = true
      error.value = null
      await updateLocation(journeyId, position.coords.latitude, position.coords.longitude)
    }, reason => {
      error.value = reason.message
    })
  }

  return { granted, error, locate }
}
