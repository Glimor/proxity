<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Line } from 'vue-chartjs'
import {
  CategoryScale, Chart as ChartJS, Legend, LineElement, LinearScale, PointElement, Tooltip,
} from 'chart.js'
import type { ChartOptions } from 'chart.js'
import { fmtBytes } from '../format'
import { useApp } from '../stores/app'

ChartJS.register(CategoryScale, LinearScale, PointElement, LineElement, Legend, Tooltip)
const { t } = useI18n()
const store = useApp()
function readPalette() {
  const ink = getComputedStyle(document.documentElement)
  return {
    down: ink.getPropertyValue('--chart-download'),
    up: ink.getPropertyValue('--chart-upload'),
    labels: ink.getPropertyValue('--chart-label'),
    grid: ink.getPropertyValue('--chart-grid'),
  }
}
const palette = ref(readPalette())
const themeObserver = new MutationObserver(() => { palette.value = readPalette() })
onMounted(() => {
  themeObserver.observe(document.documentElement, { attributes: true, attributeFilter: ['class'] })
})
onUnmounted(() => themeObserver.disconnect())
const chartData = computed(() => {
  const colors = palette.value
  return {
    labels: store.history.map((_, index) => index),
    datasets: [
      { label: t('dash.down'), data: store.history.map((s) => s.total_down_bps), borderColor: colors.down, pointRadius: 0, tension: 0.3 },
      { label: t('dash.up'), data: store.history.map((s) => s.total_up_bps), borderColor: colors.up, borderDash: [5, 4], pointRadius: 0, tension: 0.3 },
    ],
  }
})
const chartOptions = computed<ChartOptions<'line'>>(() => ({
  responsive: true,
  maintainAspectRatio: false,
  animation: false,
  scales: {
    x: { display: false },
    y: { ticks: { callback: (value) => fmtBytes(Number(value)), color: palette.value.labels }, grid: { color: palette.value.grid }, beginAtZero: true },
  },
  plugins: { legend: { labels: { color: palette.value.labels, boxWidth: 16 } } },
}))
</script>

<template>
  <section class="card space-y-4 p-5 sm:p-6" aria-labelledby="traffic-title">
    <h2 id="traffic-title" class="section-title">{{ t('dash.live_activity') }}</h2>
    <dl v-if="store.snapshot" class="grid grid-cols-2 gap-4 text-sm tabular-nums sm:grid-cols-4">
      <div><dt class="muted">{{ t('dash.down') }}</dt><dd class="font-semibold">{{ fmtBytes(store.snapshot.total_down_bps) }}/s</dd></div>
      <div><dt class="muted">{{ t('dash.up') }}</dt><dd class="font-semibold">{{ fmtBytes(store.snapshot.total_up_bps) }}/s</dd></div>
      <div><dt class="muted">{{ t('dash.active_conns') }}</dt><dd class="font-semibold">{{ store.snapshot.active_conns }}</dd></div>
      <div><dt class="muted">{{ t('dash.blocked_today') }}</dt><dd class="font-semibold">{{ store.snapshot.blocked_count }}</dd></div>
    </dl>
    <p v-else class="muted" role="status">{{ t('dash.no_traffic_sample') }}</p>
    <div v-if="store.history.length > 1" class="relative h-44 min-w-0 overflow-hidden sm:h-52">
      <Line :data="chartData" :options="chartOptions" :aria-label="t('dash.traffic_chart_label')" />
    </div>
  </section>
</template>
