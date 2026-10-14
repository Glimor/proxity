<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Bar } from 'vue-chartjs'
import {
  BarElement,
  CategoryScale,
  Chart as ChartJS,
  Legend,
  LinearScale,
  Tooltip,
} from 'chart.js'
import { api } from '../api'
import { exeName, fmtBytes, fmtSeconds } from '../format'
import { useApp } from '../stores/app'
import ServiceDownCard from '../components/ServiceDownCard.vue'
import EmptyState from '../components/EmptyState.vue'
import type { ExeDailyStats, ProxyDailyStats, ProxyProfile } from '../types'

ChartJS.register(CategoryScale, LinearScale, BarElement, Legend, Tooltip)

const { t } = useI18n()
const store = useApp()

const range = ref<'today' | 'week' | 'month' | 'all' | 'custom'>('today')
const fromD = ref('')
const toD = ref('')
const exeStats = ref<ExeDailyStats[]>([])
const proxyStats = ref<ProxyDailyStats[]>([])
const profiles = ref<ProxyProfile[]>([])
const loading = ref(false)

function dayStr(d: Date): string {
  const y = d.getFullYear()
  const m = String(d.getMonth() + 1).padStart(2, '0')
  const dd = String(d.getDate()).padStart(2, '0')
  return `${y}-${m}-${dd}`
}

function rangeBounds(): { from: string; to: string } {
  const now = new Date()
  const to = dayStr(now)
  switch (range.value) {
    case 'today':
      return { from: to, to }
    case 'week': {
      const d = new Date(now)
      d.setDate(d.getDate() - 6)
      return { from: dayStr(d), to }
    }
    case 'month': {
      const d = new Date(now)
      d.setDate(d.getDate() - 29)
      return { from: dayStr(d), to }
    }
    case 'all':
      return { from: '1970-01-01', to: '9999-12-31' }
    default:
      return { from: fromD.value || to, to: toD.value || to }
  }
}

async function refresh() {
  if (!store.connected) return
  loading.value = true
  try {
    const { from, to } = rangeBounds()
    ;[exeStats.value, proxyStats.value, profiles.value] = await Promise.all([
      api.getExeStats(from, to).catch(() => []),
      api.getProxyStats(from, to).catch(() => []),
      api.getProfiles().catch(() => []),
    ])
  } finally {
    loading.value = false
  }
}
onMounted(refresh)

const proxyName = computed(() => {
  const m: Record<string, string> = {}
  for (const p of profiles.value) m[p.ID] = p.Name
  return m
})

// aggregate per-exe across days
const exeAgg = computed(() => {
  const m = new Map<string, { down: number; up: number; conns: number; seconds: number }>()
  for (const s of exeStats.value) {
    const e = m.get(s.exe_path) ?? { down: 0, up: 0, conns: 0, seconds: 0 }
    e.down += s.down
    e.up += s.up
    e.conns += s.conns
    e.seconds += s.seconds
    m.set(s.exe_path, e)
  }
  return [...m.entries()]
    .map(([path, v]) => ({ path, ...v }))
    .sort((a, b) => b.down + b.up - (a.down + a.up))
})

const proxyAgg = computed(() => {
  const m = new Map<string, { down: number; up: number; conns: number; seconds: number }>()
  for (const s of proxyStats.value) {
    const e = m.get(s.proxy_id) ?? { down: 0, up: 0, conns: 0, seconds: 0 }
    e.down += s.down
    e.up += s.up
    e.conns += s.conns
    e.seconds += s.seconds
    m.set(s.proxy_id, e)
  }
  return [...m.entries()]
    // Stats survive profile deletion on purpose; unknown IDs render as
    // "(deleted)"/"(silindi)" instead of a raw UUID.
    .map(([id, v]) => ({ id, name: proxyName.value[id] ?? t('stats.proxy_deleted'), ...v }))
    .sort((a, b) => b.down + b.up - (a.down + a.up))
})

const top10 = computed(() => exeAgg.value.slice(0, 10))

const barData = computed(() => ({
  labels: top10.value.map((e) => exeName(e.path)),
  datasets: [
    {
      label: t('stats.col_down'),
      data: top10.value.map((e) => e.down),
      backgroundColor: '#4f46e5',
    },
    {
      label: t('stats.col_up'),
      data: top10.value.map((e) => e.up),
      backgroundColor: '#0ea5e9',
    },
  ],
}))
const barOpts = {
  indexAxis: 'y' as const,
  responsive: true,
  maintainAspectRatio: false,
  scales: { x: { ticks: { callback: (v: any) => fmtBytes(Number(v)) } } },
  plugins: { legend: { labels: { boxWidth: 12 } } },
}

function dur(sec: number): string {
  const d = fmtSeconds(sec)
  return t('fmt.' + d.unit, { n: d.n })
}

async function exportCSV() {
  try {
    const { from, to } = rangeBounds()
    const csv = await api.exportStatsCsv(from, to, 'all')
    const path = await api.pickSaveFile('proxity-stats.csv', '*.csv')
    if (!path) return
    // BOM keeps Excel's UTF-8 detection happy; body itself is UTF-8 CSV
    // with header day,exe_path,proxy_id,down,up,conns,seconds.
    await api.writeTextFile(path, '\uFEFF' + csv)
    store.toast('ok', t('stats.exported'))
  } catch (e) {
    store.toastError(e, t('toast.error'))
  }
}

async function reset() {
  const ok = await store.askConfirm(t('stats.reset_title'), t('stats.reset_body'), t('stats.reset'))
  if (!ok) return
  try {
    await api.resetStats('')
    store.toast('ok', t('stats.reset_done'))
    await refresh()
  } catch (e) {
    store.toastError(e, t('toast.error'))
  }
}
</script>

<template>
  <div class="space-y-4 p-6">
    <ServiceDownCard v-if="!store.connected" />

    <div class="flex flex-wrap items-center justify-between gap-3">
      <h1 class="text-xl font-bold">{{ t('stats.title') }}</h1>
      <div class="flex flex-wrap items-center gap-2">
        <button
          v-for="r in (['today', 'week', 'month', 'all', 'custom'] as const)"
          :key="r"
          class="btn-ghost"
          :class="{ '!border-brand-500 !text-brand-600 dark:!text-brand-100': range === r }"
          @click="range = r; r !== 'custom' && refresh()"
        >
          {{ t('stats.range_' + r) }}
        </button>
      </div>
    </div>

    <div v-if="range === 'custom'" class="card flex flex-wrap items-end gap-3">
      <div>
        <label class="label">{{ t('stats.from') }}</label>
        <input v-model="fromD" type="date" class="input w-40" />
      </div>
      <div>
        <label class="label">{{ t('stats.to') }}</label>
        <input v-model="toD" type="date" class="input w-40" />
      </div>
      <button class="btn-primary" @click="refresh">{{ t('stats.apply') }}</button>
    </div>

    <div v-if="loading" class="card h-40 animate-pulse bg-slate-200/60 dark:bg-slate-800/60" />

    <EmptyState
      v-else-if="!exeAgg.length && !proxyAgg.length"
      :title="t('stats.title')"
      :body="t('stats.empty')"
    />

    <template v-else>
      <div class="card">
        <div class="mb-2 text-sm font-medium text-slate-500 dark:text-slate-400">{{ t('stats.top10') }}</div>
        <div class="h-72"><Bar :data="barData" :options="barOpts" /></div>
      </div>

      <div class="grid gap-4 xl:grid-cols-2">
        <div class="card overflow-x-auto p-0">
          <div class="px-3 pt-3 text-sm font-medium text-slate-500 dark:text-slate-400">{{ t('stats.per_app') }}</div>
          <table class="mt-2 w-full">
            <thead>
              <tr class="border-b border-slate-200 dark:border-slate-800">
                <th class="th">{{ t('stats.col_app') }}</th>
                <th class="th text-right">{{ t('stats.col_down') }}</th>
                <th class="th text-right">{{ t('stats.col_up') }}</th>
                <th class="th text-right">{{ t('stats.col_conns') }}</th>
                <th class="th text-right">{{ t('stats.col_time') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="e in exeAgg" :key="e.path" class="border-b border-slate-100 last:border-0 dark:border-slate-800/60">
                <td class="td max-w-[240px]"><div class="truncate text-sm" :title="e.path">{{ exeName(e.path) }}</div></td>
                <td class="td text-right text-sm">{{ fmtBytes(e.down) }}</td>
                <td class="td text-right text-sm">{{ fmtBytes(e.up) }}</td>
                <td class="td text-right text-sm">{{ e.conns }}</td>
                <td class="td text-right text-sm">{{ dur(e.seconds) }}</td>
              </tr>
            </tbody>
          </table>
        </div>

        <div class="card overflow-x-auto p-0">
          <div class="px-3 pt-3 text-sm font-medium text-slate-500 dark:text-slate-400">{{ t('stats.per_proxy') }}</div>
          <table class="mt-2 w-full">
            <thead>
              <tr class="border-b border-slate-200 dark:border-slate-800">
                <th class="th">{{ t('stats.col_proxy') }}</th>
                <th class="th text-right">{{ t('stats.col_down') }}</th>
                <th class="th text-right">{{ t('stats.col_up') }}</th>
                <th class="th text-right">{{ t('stats.col_conns') }}</th>
                <th class="th text-right">{{ t('stats.col_time') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="p in proxyAgg" :key="p.id" class="border-b border-slate-100 last:border-0 dark:border-slate-800/60">
                <td class="td text-sm">{{ p.name }}</td>
                <td class="td text-right text-sm">{{ fmtBytes(p.down) }}</td>
                <td class="td text-right text-sm">{{ fmtBytes(p.up) }}</td>
                <td class="td text-right text-sm">{{ p.conns }}</td>
                <td class="td text-right text-sm">{{ dur(p.seconds) }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </template>

    <div class="flex gap-2">
      <button class="btn-ghost" :disabled="!store.connected || (!exeStats.length && !proxyStats.length)" @click="exportCSV">
        {{ t('stats.export_csv') }}
      </button>
      <button class="btn-ghost text-red-500" :disabled="!store.connected" @click="reset">{{ t('stats.reset') }}</button>
    </div>
  </div>
</template>
