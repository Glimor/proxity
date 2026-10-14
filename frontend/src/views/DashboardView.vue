<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Line } from 'vue-chartjs'
import {
  CategoryScale,
  Chart as ChartJS,
  Filler,
  Legend,
  LineElement,
  LinearScale,
  PointElement,
  Tooltip,
} from 'chart.js'
import { api } from '../api'
import { fmtBytes } from '../format'
import { useApp } from '../stores/app'
import type { LastSession } from '../types'
import ServiceDownCard from '../components/ServiceDownCard.vue'

ChartJS.register(CategoryScale, LinearScale, PointElement, LineElement, Legend, Tooltip, Filler)

const { t } = useI18n()
const store = useApp()

const profiles = ref<{ ID: string; Enabled: boolean }[]>([])
// Snapshot of the previous session, shown right after the engine starts so
// the user can see what happened while the app was closed.
const lastSession = ref<LastSession | null>(null)

function fmtTime(iso: string): string {
  const d = new Date(iso)
  return isNaN(d.getTime()) ? '' : d.toLocaleString()
}

const connectedCount = computed(
  () => Object.values(store.proxyStatuses).filter((s) => s.State === 2).length,
)

async function refresh() {
  if (store.connected) {
    try {
      profiles.value = await api.getProfiles()
    } catch {
      /* offline */
    }
    try {
      lastSession.value = await api.getLastSession()
    } catch {
      lastSession.value = null
    }
    await store.refreshEngine()
  }
}
onMounted(refresh)
watch(() => store.connected, (connected) => connected && refresh())

const chartData = computed(() => {
  const labels = store.history.map((_, i) => i)
  return {
    labels,
    datasets: [
      {
        label: t('dash.down'),
        data: store.history.map((s) => s.total_down_bps),
        borderColor: '#4f46e5',
        backgroundColor: 'rgba(79,70,229,0.15)',
        fill: true,
        pointRadius: 0,
        tension: 0.3,
      },
      {
        label: t('dash.up'),
        data: store.history.map((s) => s.total_up_bps),
        borderColor: '#0ea5e9',
        backgroundColor: 'rgba(14,165,233,0.10)',
        fill: true,
        pointRadius: 0,
        tension: 0.3,
      },
    ],
  }
})
const chartOpts = {
  responsive: true,
  maintainAspectRatio: false,
  animation: false as const,
  scales: {
    x: { display: false },
    y: { ticks: { callback: (v: any) => fmtBytes(Number(v)) }, beginAtZero: true },
  },
  plugins: { legend: { labels: { boxWidth: 12 } } },
}

async function toggleEngine() {
  if (store.engine.running) {
    const ok = await store.askConfirm(
      t('dash.stop_engine_title'),
      t('dash.stop_engine_body'),
      t('dash.stop_engine'),
    )
    if (!ok) return
    try {
      store.engine = await api.stopEngine()
      store.toast('info', t('dash.engine_stopped_toast'))
    } catch (e) {
      store.toastError(e, t('toast.error'))
    }
  } else {
    try {
      store.engine = await api.startEngine()
      store.toast('ok', t('dash.engine_started'))
    } catch (e) {
      store.toastError(e, t('toast.error'))
    }
  }
}

</script>

<template>
  <div class="space-y-4 p-6">
    <h1 class="text-xl font-bold">{{ t('dash.title') }}</h1>

    <ServiceDownCard v-if="!store.connected" />

    <template v-else>
      <!-- engine card -->
      <div class="card flex flex-wrap items-center justify-between gap-4">
        <div class="flex items-center gap-4">
          <span
            class="flex h-12 w-12 items-center justify-center rounded-xl text-xl"
            :class="store.engine.running ? 'bg-green-500/15 text-green-500' : 'bg-slate-300/40 text-slate-400 dark:bg-slate-700/50'"
            >⚡</span
          >
          <div>
            <div class="text-sm text-slate-500 dark:text-slate-400">{{ t('dash.engine') }}</div>
            <div class="text-lg font-bold">
              {{ store.engine.running ? t('dash.engine_running') : t('dash.engine_stopped') }}
            </div>
            <div class="text-xs text-slate-400">
              {{ store.engine.capture_alive ? t('dash.capture_ok') : t('dash.capture_dead') }}
            </div>
          </div>
        </div>
        <div class="flex items-center gap-3">
          <span class="badge" :class="store.killSwitchOn ? 'bg-red-500/15 text-red-500' : 'bg-slate-400/15 text-slate-500 dark:text-slate-400'">
            {{ store.killSwitchOn ? t('dash.kill_switch_on') : t('dash.kill_switch_off') }}
          </span>
          <button
            :class="store.engine.running ? 'btn-danger' : 'btn-primary'"
            @click="toggleEngine"
          >
            {{ store.engine.running ? t('dash.stop_engine') : t('dash.start_engine') }}
          </button>
        </div>
      </div>

      <!-- counters -->
      <div class="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-6">
        <div class="card py-3">
          <div class="text-xs text-slate-400">{{ t('dash.down') }}</div>
          <div class="text-lg font-bold">{{ fmtBytes(store.snapshot?.total_down_bps ?? 0) }}/s</div>
        </div>
        <div class="card py-3">
          <div class="text-xs text-slate-400">{{ t('dash.up') }}</div>
          <div class="text-lg font-bold">{{ fmtBytes(store.snapshot?.total_up_bps ?? 0) }}/s</div>
        </div>
        <div class="card py-3">
          <div class="text-xs text-slate-400">{{ t('dash.active_conns') }}</div>
          <div class="text-lg font-bold">{{ store.snapshot?.active_conns ?? 0 }}</div>
        </div>
        <div class="card py-3">
          <div class="text-xs text-slate-400">{{ t('dash.protected_apps') }}</div>
          <div class="text-lg font-bold">{{ store.protectedApps }}</div>
        </div>
        <div class="card py-3">
          <div class="text-xs text-slate-400">{{ t('dash.active_proxies') }}</div>
          <div class="text-lg font-bold">{{ connectedCount }}</div>
        </div>
        <div class="card py-3">
          <div class="text-xs text-slate-400">{{ t('dash.blocked_today') }}</div>
          <div class="text-lg font-bold">{{ store.snapshot?.blocked_count ?? 0 }}</div>
        </div>
      </div>

      <!-- live chart -->
      <div class="card">
        <div class="mb-2 text-sm font-medium text-slate-500 dark:text-slate-400">{{ t('dash.chart_title') }}</div>
        <div class="h-48">
          <Line :data="chartData" :options="chartOpts" />
        </div>
      </div>

      <!-- previous session summary, only while this session is fresh -->
      <div v-if="lastSession && store.engine.running" class="card flex flex-wrap items-center gap-x-6 gap-y-1 py-3 text-sm">
        <span class="font-medium">{{ t('dash.last_session') }}</span>
        <span class="text-xs text-slate-400">{{ t('dash.last_session_ended', { t: fmtTime(lastSession.ended_at) }) }}</span>
        <span>{{ t('dash.last_session_succeeded', { n: lastSession.counters.succeeded }) }}</span>
        <span>{{ t('dash.last_session_blocked', { n: lastSession.counters.blocked }) }}</span>
        <span>{{ t('dash.last_session_failed', { n: lastSession.counters.failed }) }}</span>
      </div>
    </template>
  </div>
</template>
