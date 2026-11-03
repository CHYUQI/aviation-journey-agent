<script setup lang="ts">
import { computed, nextTick, ref } from 'vue'
import { airportCode, airportOptions, matchingAirports } from '../airportOptions'

const props = defineProps<{ id: string; label: string }>()
const value = defineModel<string>({ required: true })
const open = ref(false)
const browsing = ref(false)
const active = ref(-1)
const selected = computed(() => matchingAirports(value.value)[0])
const choices = computed(() => {
  const query = value.value.trim().toLowerCase()
  if (browsing.value || !query) return airportOptions
  return airportOptions.filter(a => [a.code, a.name, ...a.aliases].some(s => s.toLowerCase().includes(query)))
})
function choose(index: number) {
  const airport = choices.value[index]
  if (!airport) return
  value.value = airport.name
  open.value = false
  active.value = -1
}
function keydown(event: KeyboardEvent) {
  if (event.isComposing) return
  if (event.key === 'Escape') { open.value = false; return }
  if (event.key === 'Enter' && open.value && active.value >= 0) {
    event.preventDefault()
    choose(active.value)
  }
  if (event.key !== 'ArrowDown' && event.key !== 'ArrowUp') return
  event.preventDefault()
  open.value = true
  if (!choices.value.length) return
  active.value = active.value < 0
    ? (event.key === 'ArrowDown' ? 0 : choices.value.length - 1)
    : (active.value + (event.key === 'ArrowDown' ? 1 : -1) + choices.value.length) % choices.value.length
  nextTick(() => document.getElementById(`${props.id}-option-${active.value}`)?.scrollIntoView({ block: 'nearest' }))
}
</script>

<template>
  <div class="airport-field">
    <label :for="id">{{ label }}</label>
    <div class="airport-control">
      <input :id="id" v-model="value" role="combobox" aria-autocomplete="list" :aria-expanded="open"
        :aria-controls="`${id}-options`" :aria-describedby="`${id}-hint`"
        :aria-activedescendant="open && active >= 0 ? `${id}-option-${active}` : undefined"
        placeholder="城市、机场名称或 IATA" autocomplete="off" maxlength="80"
        @focus="open = true; browsing = true; active = -1"
        @click="if (!open) { open = true; browsing = true; active = -1 }"
        @input="open = true; browsing = false; active = -1"
        @blur="open = false" @keydown="keydown" />
      <ul v-if="open" :id="`${id}-options`" class="airport-options" role="listbox" :aria-label="label">
        <li v-for="(airport, index) in choices" :id="`${id}-option-${index}`" :key="airport.code"
          role="option" :aria-selected="airport.code === airportCode(value)" :class="{ active: index === active }"
          @pointerdown.prevent @click="choose(index)" @mouseenter="active = index">
          <span>{{ airport.name }}</span><span class="airport-code">{{ airport.code }}</span>
        </li>
        <li v-if="!choices.length" class="airport-empty" role="presentation">未找到名称，可直接输入三位 IATA</li>
      </ul>
    </div>
    <small :id="`${id}-hint`" class="airport-hint" aria-live="polite">
      <template v-if="selected">{{ matchingAirports(value).length > 1 ? '默认' : '已选' }}：{{ selected.name }} · {{ selected.code }}</template>
      <template v-else-if="airportCode(value)">机场代码：{{ airportCode(value) }}</template>
      <template v-else>支持中文、英文和 IATA</template>
    </small>
  </div>
</template>

<style scoped>
.airport-field { display: grid; align-content: start; gap: 7px; min-width: 0; }
.airport-control { position: relative; min-width: 0; }
.airport-hint { color: var(--text-2); font-size: 12px; line-height: 1.5; min-height: 18px; overflow-wrap: anywhere; }
.airport-options { position: absolute; z-index: 20; top: calc(100% + 6px); left: 0; right: 0; max-height: 240px; overflow-y: auto; margin: 0; padding: 5px; list-style: none; background: #fff; border: 1px solid #d9dde3; border-radius: 12px; box-shadow: 0 8px 24px #00000018; }
.airport-options li { display: flex; justify-content: space-between; align-items: center; gap: 10px; padding: 10px; border-radius: 7px; font-size: 13px; cursor: pointer; }
.airport-options li.active, .airport-options li[aria-selected='true'] { background: #eef5ff; }
.airport-code { color: var(--text-2); flex-shrink: 0; font-size: 12px; }
.airport-options li.airport-empty { color: var(--text-2); cursor: default; }
</style>
