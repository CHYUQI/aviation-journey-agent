<script setup lang="ts">
import { computed, onUnmounted } from 'vue'
import { useJourneyStore } from '../stores/journey'
import StatusBar from '../components/StatusBar.vue'
import MetricsRow from '../components/MetricsRow.vue'
import Timeline from '../components/Timeline.vue'
import ActionCardList from '../components/ActionCardList.vue'
import FreshnessBar from '../components/FreshnessBar.vue'

const store = useJourneyStore()
const snapshot = computed(() => store.snapshot)

onUnmounted(() => store.stop())
</script>

<template>
  <main class="dashboard">
    <header class="hero">
      <p class="eyebrow">AVIATION JOURNEY AGENT</p>
      <h1>航旅智行</h1>
      <p>把航班、机场和旅客状态转换成下一步可执行行动。</p>
    </header>

    <template v-if="snapshot">
      <StatusBar
        :status="snapshot.status"
        :stage="snapshot.advice.stage"
        :risk="snapshot.advice.risk"
      />
      <FreshnessBar :state="snapshot.state" :connected="store.connected" />

      <p v-if="snapshot.advice.alert" class="alert-banner" :data-risk="snapshot.advice.risk">
        {{ snapshot.advice.alert }}
      </p>

      <MetricsRow :cards="snapshot.advice.cards" />

      <section class="content-grid">
        <Timeline
          :timeline="snapshot.state.timeline"
          :guide="snapshot.state.guide"
          :stage="snapshot.advice.stage"
        />
        <ActionCardList :actions="snapshot.advice.actions" />
      </section>
    </template>

    <section v-else class="empty-state">
      等待行程数据。请先通过后端创建行程，再将 journeyId 接入此页面。
    </section>
  </main>
</template>
