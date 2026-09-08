import axios from 'axios'
import type { JourneySnapshot } from '../types/domain'
const http = axios.create({ baseURL: '/api/v1' })
export async function getState(id: string) { const response = await http.get<JourneySnapshot>(`/journey/${id}/state`); return response.data }
export async function updateLocation(id: string, lat: number, lng: number) { const response = await http.post<JourneySnapshot>(`/journey/${id}/location`, { lat, lng, source: 'gps' }); return response.data }
export function createStream(id: string, onSnapshot: (snapshot: JourneySnapshot) => void) {
  const source = new EventSource(`/api/v1/journey/${id}/stream`)
  source.addEventListener('snapshot', event => onSnapshot(JSON.parse((event as MessageEvent).data) as JourneySnapshot))
  return source
}
