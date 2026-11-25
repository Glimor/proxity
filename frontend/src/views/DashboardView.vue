<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '../api'
import { useApp } from '../stores/app'
import type { LastSession } from '../types'
import ServiceDownCard from '../components/ServiceDownCard.vue'
import TrafficSummary from '../components/TrafficSummary.vue'

const { t } = useI18n()
const store = useApp()
const inventory = ref<{ readonly profiles: number; readonly rules: number } | null>(null)
const loadingInventory = ref(false)
const inventoryError = ref(false)
const changePending = ref(false)
const lastSession = ref<LastSession | null>(null)
let request = 0

async function refresh() {
  const id = ++request
  inventory.value = null
  lastSession.value = null
  inventoryError.value = false
  loadingInventory.value = store.connected
  if (!store.connected) return
  const [profiles, rules, session] = await Promise.allSettled([
    api.getProfiles(), api.getRules(), api.getLastSession(), store.refreshEngine(),
  ])
  if (id !== request) return
  loadingInventory.value = false
  if (session.status === 'fulfilled') lastSession.value = session.value
  if (profiles.status === 'fulfilled' && rules.status === 'fulfilled') {
    inventory.value = {
      profiles: profiles.value.filter((profile) => profile.Enabled).length,
      rules: rules.value.filter((rule) => rule.Enabled).length,
    }
  } else {
    inventoryError.value = true
  }
}

watch(() => store.connected, refresh, { immediate: true })
const active = computed(() => store.engine.running && store.engine.capture_alive)
const routingState = computed(() => {
  if (active.value) return 'active'
  return store.engine.running ? 'capture_unavailable' : 'paused'
})
const destination = computed(() => {
  if (inventory.value?.profiles === 0) return { path: '/proxies', label: t('dash.create_proxy') }
  if (inventory.value?.rules === 0) return { path: '/rules', label: t('dash.create_rule') }
  if (active.value) return { path: '/connections', label: t('dash.open_connections') }
  return null
})
const sessionTime = computed(() => {
  if (!lastSession.value) return ''
  const date = new Date(lastSession.value.ended_at)
  return Number.isNaN(date.getTime()) ? '' : date.toLocaleString()
})

async function toggleEngine() {
  if (changePending.value || !store.connected) return
  changePending.value = true
  try {
    if (store.engine.running) {
      const ok = await store.askConfirm(t('dash.stop_engine_title'), t('dash.stop_engine_body'), t('dash.stop_engine'))
      if (!ok || !store.connected) return
      store.engine = await api.stopEngine()
      store.toast('info', t('dash.engine_stopped_toast'))
    } else {
      store.engine = await api.startEngine()
      store.toast('ok', t('dash.engine_started'))
    }
  } catch (error) {
    store.toastError(error, t('toast.error'))
  } finally {
    changePending.value = false
  }
}
</script>

<template>
  <div class="page">
    <header class="page-header"><h1 class="page-title">{{ t('dash.title') }}</h1></header>
    <ServiceDownCard v-if="!store.connected" />

    <template v-else>
      <section class="card space-y-5 p-5 sm:p-6" aria-labelledby="routing-status">
        <div class="flex flex-wrap items-start justify-between gap-x-6 gap-y-4">
          <div class="max-w-prose">
            <h2 id="routing-status" class="section-title flex items-center gap-3">
              <span aria-hidden="true" class="size-2 shrink-0 rounded-full" :class="active ? 'bg-emerald-600 dark:bg-emerald-400' : 'bg-amber-600 dark:bg-amber-400'" />
              {{ t(`dash.${routingState}_title`) }}
            </h2>
            <p class="muted mt-2">{{ t(`dash.${routingState}_body`) }}</p>
            <p v-if="inventory?.profiles === 0" class="muted mt-1">{{ t('dash.no_enabled_profiles') }}</p>
            <p v-else-if="inventory?.rules === 0" class="muted mt-1">{{ t('dash.no_enabled_rules') }}</p>
          </div>
          <div class="flex flex-wrap gap-2">
            <RouterLink v-if="destination" class="btn-primary" :to="destination.path">{{ destination.label }}</RouterLink>
            <button v-else-if="!loadingInventory && !inventoryError && !store.engine.running" class="btn-primary" :disabled="changePending" :aria-busy="changePending" @click="toggleEngine">{{ t('dash.start_engine') }}</button>
            <button v-if="store.engine.running" class="btn-ghost" :disabled="changePending" :aria-busy="changePending" @click="toggleEngine">{{ t('dash.stop_engine') }}</button>
          </div>
        </div>
        <p v-if="loadingInventory" class="muted" role="status">{{ t('common.loading') }}</p>
        <div v-else-if="inventoryError" class="flex flex-wrap items-center gap-2">
          <p class="muted" role="alert">{{ t('dash.inventory_error') }}</p>
          <button class="btn-ghost" @click="refresh">{{ t('common.retry') }}</button>
        </div>
        <dl v-else-if="inventory" class="flex flex-wrap gap-x-8 gap-y-3 border-t border-slate-200 pt-4 text-sm dark:border-slate-800">
          <div><dt class="muted">{{ t('dash.enabled_profiles') }}</dt><dd class="font-semibold tabular-nums">{{ inventory.profiles }}</dd></div>
          <div><dt class="muted">{{ t('dash.enabled_rules') }}</dt><dd class="font-semibold tabular-nums">{{ inventory.rules }}</dd></div>
          <div v-if="store.settings"><dt class="muted">{{ t('dash.global_kill_switch') }}</dt><dd class="font-semibold">{{ t(store.killSwitchOn ? 'common.enabled' : 'common.disabled') }}</dd></div>
        </dl>
      </section>

      <TrafficSummary v-if="active" />

      <section v-if="lastSession && active" class="card p-5 sm:p-6">
        <h2 class="section-title">{{ t('dash.last_session') }}</h2>
        <p v-if="sessionTime" class="muted mt-2">{{ t('dash.last_session_ended', { t: sessionTime }) }}</p>
        <ul class="mt-3 flex flex-wrap gap-x-6 gap-y-2 text-sm tabular-nums">
          <li>{{ t('dash.last_session_succeeded', { n: lastSession.counters.succeeded }) }}</li>
          <li>{{ t('dash.last_session_blocked', { n: lastSession.counters.blocked }) }}</li>
          <li>{{ t('dash.last_session_failed', { n: lastSession.counters.failed }) }}</li>
        </ul>
      </section>
    </template>
  </div>
</template>
