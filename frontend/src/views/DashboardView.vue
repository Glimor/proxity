<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
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
import { api, parseError } from '../api'
import { fmtBytes } from '../format'
import { useApp } from '../stores/app'
import type { LastSession, ServiceStatus } from '../types'

ChartJS.register(CategoryScale, LinearScale, PointElement, LineElement, Legend, Tooltip, Filler)

const { t } = useI18n()
const store = useApp()

const svc = ref<ServiceStatus | null>(null)
const installing = ref(false)
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
  try {
    svc.value = await api.serviceStatus()
  } catch {
    svc.value = null
  }
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

async function installService() {
  installing.value = true
  try {
    await api.installService()
    store.toast('info', t('service.starting'))
  } catch (e) {
    if (parseError(e).code === 'uac.cancelled') store.toast('err', t('service.uac_cancelled'))
    else store.toastError(e, t('service.install_failed'))
  } finally {
    installing.value = false
  }
}
</script>

<template>
  <div class="space-y-4 p-6">
    <h1 class="text-xl font-bold">{{ t('dash.title') }}</h1>

    <!-- service unreachable state -->
    <div v-if="!store.connected" class="card border-amber-400/60 dark:border-amber-500/40">
      <h2 class="mb-1 text-base font-semibold text-amber-600 dark:text-amber-400">
        {{ t('dash.service_down_title') }}
      </h2>
      <p class="mb-3 max-w-2xl text-sm text-slate-600 dark:text-slate-300">{{ t('dash.service_down_body') }}</p>
      <p v-if="svc && !svc.binary_found" class="mb-3 text-sm text-red-600 dark:text-red-400">
        {{ t('dash.service_binary_missing') }}
      </p>
      <p v-else-if="svc && svc.binary_found && !svc.installed" class="mb-3 text-xs text-slate-400">
        {{ t('dash.service_not_installed') }} · {{ svc.binary_path }}
      </p>
      <div class="flex items-center gap-3">
        <button class="btn-primary" :disabled="installing || !svc?.binary_found" @click="installService">
          {{ installing ? t('common.loading') : t('dash.install_service') }}
        </button>
        <span class="text-xs text-slate-400">{{ t('dash.install_service_hint') }}</span>
      </div>
    </div>

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
