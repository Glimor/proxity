<script setup lang="ts">
import { ref, useId, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import { useApp } from '../stores/app'
import UiIcon from './UiIcon.vue'

const { t } = useI18n()
const route = useRoute()
const store = useApp()
const expanded = ref(false)
const menu = ref<HTMLButtonElement | null>(null)
const navId = useId()

const groups = [
  [
    { to: '/', key: 'nav.dashboard', icon: 'dashboard' },
    { to: '/proxies', key: 'nav.proxies', icon: 'proxies' },
    { to: '/rules', key: 'nav.apps', icon: 'apps' },
  ],
  [
    { to: '/connections', key: 'nav.conns', icon: 'connections' },
    { to: '/stats', key: 'nav.stats', icon: 'stats' },
    { to: '/blocked', key: 'nav.blocked', icon: 'blocked' },
    { to: '/settings', key: 'nav.settings', icon: 'settings' },
  ],
] as const

watch(() => route.path, () => { expanded.value = false })

function closeMenu() {
  if (!expanded.value) return
  expanded.value = false
  menu.value?.focus()
}
</script>

<template>
  <aside class="relative z-10 flex shrink-0 flex-col break-words border-b border-slate-200 bg-white dark:border-slate-800 dark:bg-slate-900 md:w-52 md:border-b-0 md:border-r" @keydown.esc="closeMenu">
    <div class="flex min-w-0 items-center justify-between gap-3 px-4 py-2 md:px-5 md:py-6">
      <div class="flex min-w-0 items-center gap-3">
        <img src="/logo.png" alt="" class="size-8 shrink-0 rounded-lg md:size-9" />
        <span class="min-w-0 [overflow-wrap:anywhere] text-lg font-semibold tracking-tight text-slate-950 dark:text-white">{{ t('app.name') }}</span>
      </div>
      <button ref="menu" type="button" class="btn-ghost md:hidden" :aria-expanded="expanded" :aria-controls="navId" @click="expanded = !expanded">
        <UiIcon :name="expanded ? 'close' : 'menu'" />
        <span class="min-w-0 [overflow-wrap:anywhere]">Menu</span>
      </button>
    </div>
    <nav :id="navId" aria-label="Main navigation" class="absolute inset-x-0 top-full max-h-[70dvh] overflow-y-auto border-b border-slate-200 bg-white p-3 dark:border-slate-800 dark:bg-slate-900 md:static md:block md:max-h-none md:flex-1 md:border-0 md:p-3" :class="{ hidden: !expanded }">
      <div v-for="(group, index) in groups" :key="index" class="space-y-1" :class="{ 'mt-4 border-t border-slate-200 pt-4 dark:border-slate-800': index }">
        <RouterLink v-for="item in group" :key="item.to" :to="item.to" class="nav-link">
          <UiIcon :name="item.icon" />
          {{ t(item.key) }}
        </RouterLink>
      </div>
    </nav>
    <div class="hidden border-t border-slate-200 px-5 py-4 text-sm font-medium dark:border-slate-800 md:block">
      <span class="inline-flex items-center gap-2">
        <span aria-hidden="true" class="size-2 rounded-full" :class="store.connected ? 'bg-emerald-600 dark:bg-emerald-400' : 'bg-red-600 dark:bg-red-400'" />
        {{ store.connected ? t('conn.service_ok') : t('conn.service_off') }}
      </span>
    </div>
  </aside>
</template>
