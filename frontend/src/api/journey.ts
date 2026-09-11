import axios from 'axios'
import type {
  AnalysisAccepted,
  CreateJourneyRequest,
  JourneySnapshot,
  UpdateLocationRequest,
} from '../types'

const http = axios.create({ baseURL: '/api/v1' })

// 创建行程。返回 202，结果通过 SSE 或 getState 获取
export async function createJourney(payload: CreateJourneyRequest) {
  const { data } = await http.post<AnalysisAccepted>('/journey', payload)
  return data
}

// 查询当前快照，前端唯一状态来源
export async function getState(id: string) {
  const { data } = await http.get<JourneySnapshot>(`/journey/${id}/state`)
  return data
}

// 上报定位坐标或手动确认阶段。两者可只给其一
export async function updateLocation(id: string, payload: UpdateLocationRequest) {
  const { data } = await http.post<AnalysisAccepted>(`/journey/${id}/location`, payload)
  return data
}

// 订阅 SSE。事件名固定为 snapshot，heartbeat 由浏览器自行忽略
export function createStream(id: string, onSnapshot: (snapshot: JourneySnapshot) => void) {
  const source = new EventSource(`/api/v1/journey/${id}/stream`)
  source.addEventListener('snapshot', (event) => {
    onSnapshot(JSON.parse((event as MessageEvent).data) as JourneySnapshot)
  })
  return source
}
