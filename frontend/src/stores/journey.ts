import { defineStore } from 'pinia'
import { ref } from 'vue'
import { createStream, getState } from '../api/journey'
import type { JourneySnapshot } from '../types/domain'
export const useJourneyStore = defineStore('journey', () => {
  const snapshot = ref<JourneySnapshot | null>(null); const connected = ref(false); let stream: EventSource | null = null
  async function load(id: string) { snapshot.value = await getState(id); stream?.close(); stream = createStream(id, next => { snapshot.value = next; connected.value = true }); stream.onerror = () => { connected.value = false } }
  function stop() { stream?.close(); stream = null; connected.value = false }
  return { snapshot, connected, load, stop }
})
