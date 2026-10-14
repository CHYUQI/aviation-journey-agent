import type { CreateJourneyRequest } from '../types'

// 行程本地持久化：只存"重新建一条行程所需的最小信息"，不存快照。
// 快照的唯一来源始终是后端（契约 /state 与 SSE），这里不参与任何状态判断。
const STORAGE_KEY = 'aviation-journey-agent:last-journey'

export interface SavedJourney {
  journeyId: string
  payload: CreateJourneyRequest
  savedAt: string
}

// 写入。隐私模式或配额不足时静默降级，不影响本次使用
export function saveJourney(journeyId: string, payload: CreateJourneyRequest): void {
  try {
    const record: SavedJourney = {
      journeyId,
      payload: {
        flights: payload.flights.map((f) => ({
          number: f.number,
          date: f.date,
          from: f.from,
          to: f.to,
        })),
        hasBaggage: payload.hasBaggage === true,
      },
      savedAt: new Date().toISOString(),
    }
    localStorage.setItem(STORAGE_KEY, JSON.stringify(record))
  } catch {
    // 忽略：本地持久化失败不应影响行程本身
  }
}

// 读取。内容损坏或字段不全时按"没有记录"处理，不抛错
export function loadSavedJourney(): SavedJourney | null {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (!raw) return null
    const parsed = JSON.parse(raw) as Partial<SavedJourney>
    const flight = parsed.payload?.flights?.[0]
    if (!parsed.journeyId) return null
    if (!flight || !flight.number || !flight.date || !flight.from || !flight.to) return null
    return {
      journeyId: parsed.journeyId,
      savedAt: parsed.savedAt ?? '',
      payload: {
        flights: [{
          number: flight.number,
          date: flight.date,
          from: flight.from,
          to: flight.to,
        }],
        hasBaggage: parsed.payload?.hasBaggage === true,
      },
    }
  } catch {
    return null
  }
}

// 删除本机记录。后端行程不落库，随进程退出消失（契约无删除接口）
export function clearSavedJourney(): void {
  try {
    localStorage.removeItem(STORAGE_KEY)
  } catch {
    // 同上
  }
}
