<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import { useApp } from '../stores/app'

const { t } = useI18n()
const route = useRoute()
const store = useApp()

const items = [
  { to: '/', key: 'nav.dashboard', icon: '▤' },
  { to: '/proxies', key: 'nav.proxies', icon: '⇄' },
  { to: '/rules', key: 'nav.apps', icon: '▣' },
  { to: '/connections', key: 'nav.conns', icon: '⟷' },
  { to: '/stats', key: 'nav.stats', icon: '▥' },
  { to: '/blocked', key: 'nav.blocked', icon: '⊘' },
  { to: '/settings', key: 'nav.settings', icon: '⚙' },
]
</script>

<template>
  <aside class="flex w-56 shrink-0 flex-col border-r border-slate-200 bg-white dark:border-slate-800 dark:bg-slate-900">
    <div class="flex items-center gap-2 px-4 py-5">
      <div class="flex h-8 w-8 items-center justify-center rounded-lg bg-brand-600 font-bold text-white">P</div>
      <div>
        <div class="text-sm font-bold leading-tight">{{ t('app.name') }}</div>
        <div class="text-[10px] leading-tight text-slate-400">{{ t('app.tagline') }}</div>
      </div>
    </div>
    <nav class="flex-1 space-y-1 px-2">
      <RouterLink
        v-for="it in items"
        :key="it.to"
        :to="it.to"
        class="flex items-center gap-3 rounded-lg px-3 py-2 text-sm font-medium transition-colors"
        :class="
          route.path === it.to
            ? 'bg-brand-600/10 text-brand-700 dark:bg-brand-600/20 dark:text-brand-100'
            : 'text-slate-600 hover:bg-slate-100 dark:text-slate-300 dark:hover:bg-slate-800'
        "
      >
        <span class="w-4 text-center">{{ it.icon }}</span>
        {{ t(it.key) }}
      </RouterLink>
    </nav>
    <div class="border-t border-slate-200 px-4 py-3 text-xs dark:border-slate-800">
      <span class="inline-flex items-center gap-1.5">
        <span class="h-2 w-2 rounded-full" :class="store.connected ? 'bg-green-500' : 'bg-red-500'" />
        {{ store.connected ? t('conn.service_ok') : t('conn.service_off') }}
      </span>
    </div>
  </aside>
</template>
