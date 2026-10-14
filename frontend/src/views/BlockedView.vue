<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { api, onEvent } from '../api'
import { exeName, fmtTime } from '../format'
import { useApp } from '../stores/app'
import AppIcon from '../components/AppIcon.vue'
import EmptyState from '../components/EmptyState.vue'
import type { BlockedEntry } from '../types'

const { t } = useI18n()
const store = useApp()

const list = ref<BlockedEntry[]>([])
const loading = ref(true)

async function refresh() {
  if (!store.connected) {
    loading.value = false
    return
  }
  loading.value = true
  try {
    list.value = (await api.getBlocked(200)) ?? []
  } catch (e) {
    store.toastError(e, t('toast.error'))
  } finally {
    loading.value = false
  }
}
onMounted(() => {
  refresh()
  onEvent('svc:blocked', refresh)
})
watch(() => store.connected, (v) => v && refresh())

async function clear() {
  const ok = await store.askConfirm(
    t('blocked.clear_title'),
    t('blocked.clear_body'),
    t('blocked.clear'),
  )
  if (!ok) return
  try {
    await api.clearBlocked()
    store.toast('ok', t('blocked.cleared'))
    await refresh()
  } catch (e) {
    store.toastError(e, t('toast.error'))
  }
}
</script>

<template>
  <div class="space-y-4 p-6">
    <div class="flex items-center justify-between">
      <h1 class="text-xl font-bold">{{ t('blocked.title') }}</h1>
      <button class="btn-ghost text-red-500" :disabled="!list.length" @click="clear">
        {{ t('blocked.clear') }}
      </button>
    </div>

    <div v-if="loading" class="space-y-2">
      <div v-for="i in 5" :key="i" class="card h-10 animate-pulse bg-slate-200/60 dark:bg-slate-800/60" />
    </div>

    <EmptyState v-else-if="!list.length" :title="t('blocked.title')" :body="t('blocked.empty')" />

    <div v-else class="card overflow-x-auto p-0">
      <table class="w-full min-w-[720px]">
        <thead>
          <tr class="border-b border-slate-200 dark:border-slate-800">
            <th class="th">{{ t('blocked.col_time') }}</th>
            <th class="th">{{ t('blocked.col_app') }}</th>
            <th class="th">{{ t('blocked.col_dst') }}</th>
            <th class="th">{{ t('blocked.col_proto') }}</th>
            <th class="th">{{ t('blocked.col_reason') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="b in list" :key="b.ID" class="border-b border-slate-100 last:border-0 dark:border-slate-800/60">
            <td class="td whitespace-nowrap text-xs text-slate-400">{{ fmtTime(b.TS) }}</td>
            <td class="td">
              <div class="flex items-center gap-2">
                <AppIcon :path="b.ExePath" :size="18" />
                <div class="min-w-0">
                  <div class="truncate text-sm">{{ exeName(b.ExePath) || b.ExePath }}</div>
                  <div class="text-xs text-slate-400">PID {{ b.PID }}</div>
                </div>
              </div>
            </td>
            <td class="td font-mono text-xs">{{ b.Dst }}:{{ b.Port }}</td>
            <td class="td text-xs">{{ b.Proto }}</td>
            <td class="td text-xs">{{ b.Reason }}</td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>
