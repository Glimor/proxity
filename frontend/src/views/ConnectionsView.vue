<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '../api'
import { exeName, fmtBytes, fmtDurationMs } from '../format'
import { useApp } from '../stores/app'
import AppIcon from '../components/AppIcon.vue'
import EmptyState from '../components/EmptyState.vue'
import type { Connection, ProxyProfile, Rule } from '../types'

const { t } = useI18n()
const store = useApp()

const profiles = ref<ProxyProfile[]>([])
const rules = ref<Rule[]>([])
const loading = ref(true)
const search = ref('')
const fApp = ref('')
const fProxy = ref('')
const fState = ref('')

onMounted(async () => {
  loading.value = true
  try {
    ;[profiles.value, rules.value] = await Promise.all([
      api.getProfiles().catch(() => []),
      api.getRules().catch(() => []),
    ])
    if (!store.conns.length) store.conns = await api.getConnections().catch(() => [])
  } finally {
    loading.value = false
  }
})

const proxyName = computed(() => {
  const m: Record<string, string> = {}
  for (const p of profiles.value) m[p.ID] = p.Name
  return m
})
const ruleName = computed(() => {
  const m: Record<string, string> = {}
  for (const r of rules.value) m[r.ID] = r.Name
  return m
})

const appOptions = computed(() => {
  const s = new Set<string>()
  for (const c of store.conns) s.add(exeName(c.ExePath))
  return [...s].sort()
})
const proxyOptions = computed(() => {
  const s = new Set<string>()
  for (const c of store.conns) if (c.ProxyID) s.add(c.ProxyID)
  return [...s]
})

const filtered = computed(() => {
  const q = search.value.trim().toLowerCase()
  return store.conns.filter((c) => {
    if (fApp.value && exeName(c.ExePath) !== fApp.value) return false
    if (fProxy.value && c.ProxyID !== fProxy.value) return false
    if (fState.value && c.Status !== fState.value) return false
    if (!q) return true
    return (
      c.ExePath.toLowerCase().includes(q) ||
      c.DstIP.includes(q) ||
      String(c.DstPort).includes(q) ||
      (proxyName.value[c.ProxyID] ?? '').toLowerCase().includes(q)
    )
  })
})

function dur(c: Connection): string {
  const d = fmtDurationMs(c.Status === 'active' ? Date.now() - new Date(c.StartedAt).getTime() : c.DurationMS)
  return t('fmt.' + d.unit, { n: d.n })
}

const statusBadge: Record<string, string> = {
  active: 'bg-green-600/15 text-green-700 dark:text-green-300',
  closed: 'bg-slate-500/15 text-slate-500 dark:text-slate-400',
  blocked: 'bg-red-600/15 text-red-700 dark:text-red-300',
  failed: 'bg-amber-500/15 text-amber-700 dark:text-amber-300',
}
</script>

<template>
  <div class="space-y-4 p-6">
    <h1 class="text-xl font-bold">{{ t('conns.title') }}</h1>

    <div class="flex flex-wrap items-center gap-2">
      <input v-model="search" class="input w-64" :placeholder="t('conns.search_ph')" />
      <select v-model="fApp" class="input w-44">
        <option value="">{{ t('conns.f_app') }}: {{ t('common.all') }}</option>
        <option v-for="a in appOptions" :key="a" :value="a">{{ a }}</option>
      </select>
      <select v-model="fProxy" class="input w-44">
        <option value="">{{ t('conns.f_proxy') }}: {{ t('common.all') }}</option>
        <option v-for="id in proxyOptions" :key="id" :value="id">{{ proxyName[id] ?? id }}</option>
      </select>
      <select v-model="fState" class="input w-40">
        <option value="">{{ t('conns.f_state') }}: {{ t('common.all') }}</option>
        <option v-for="s in ['active', 'closed', 'blocked', 'failed']" :key="s" :value="s">{{ t('connStatus.' + s) }}</option>
      </select>
    </div>

    <div v-if="loading" class="space-y-2">
      <div v-for="i in 6" :key="i" class="card h-10 animate-pulse bg-slate-200/60 dark:bg-slate-800/60" />
    </div>

    <EmptyState v-else-if="!filtered.length" :title="t('conns.title')" :body="t('conns.empty')" />

    <div v-else class="card overflow-x-auto p-0">
      <table class="w-full min-w-[900px]">
        <thead>
          <tr class="border-b border-slate-200 dark:border-slate-800">
            <th class="th">{{ t('conns.col_app') }}</th>
            <th class="th">{{ t('conns.col_pid') }}</th>
            <th class="th">{{ t('conns.col_dst') }}</th>
            <th class="th">{{ t('conns.col_ver') }}</th>
            <th class="th">{{ t('conns.col_proxy') }}</th>
            <th class="th">{{ t('conns.col_rule') }}</th>
            <th class="th">{{ t('conns.col_state') }}</th>
            <th class="th">{{ t('conns.col_dur') }}</th>
            <th class="th text-right">{{ t('conns.col_down') }}</th>
            <th class="th text-right">{{ t('conns.col_up') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="c in filtered" :key="c.ID" class="border-b border-slate-100 last:border-0 dark:border-slate-800/60">
            <td class="td">
              <div class="flex items-center gap-2">
                <AppIcon :path="c.ExePath" :size="18" />
                <span class="truncate text-sm" :title="c.ExePath">{{ exeName(c.ExePath) || c.ExePath }}</span>
              </div>
            </td>
            <td class="td text-xs text-slate-400">{{ c.PID }}</td>
            <td class="td font-mono text-xs">{{ c.DstIP }}:{{ c.DstPort }}</td>
            <td class="td text-xs">{{ c.IPv6 ? t('conns.v6') : t('conns.v4') }}</td>
            <td class="td text-sm">{{ c.ProxyID ? proxyName[c.ProxyID] ?? '?' : '—' }}</td>
            <td class="td text-sm">{{ c.MatchedRuleID ? ruleName[c.MatchedRuleID] ?? '?' : t('conns.no_rule') }}</td>
            <td class="td"><span class="badge" :class="statusBadge[c.Status]">{{ t('connStatus.' + c.Status) }}</span></td>
            <td class="td text-xs">{{ dur(c) }}</td>
            <td class="td text-right text-xs">{{ fmtBytes(c.BytesDown) }}</td>
            <td class="td text-right text-xs">{{ fmtBytes(c.BytesUp) }}</td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>
