<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { Rule, RuleAction } from '../types'
import AppIcon from './AppIcon.vue'
import Toggle from './Toggle.vue'
import UiIcon from './UiIcon.vue'

defineProps<{ readonly rules: readonly Rule[]; readonly proxyNames: Readonly<Record<string, string>> }>()
const emit = defineEmits<{ edit: [Rule]; delete: [Rule]; enabled: [Rule]; killSwitch: [Rule] }>()
const { t } = useI18n()
const actionStyle = {
  proxy: 'bg-brand-50 text-brand-700 dark:bg-brand-600/20 dark:text-brand-100',
  direct: 'bg-slate-100 text-slate-700 dark:bg-slate-800 dark:text-slate-200',
  block: 'bg-red-50 text-red-800 dark:bg-red-950 dark:text-red-200',
} as const satisfies Record<RuleAction, string>
</script>

<template>
  <div class="table-wrap">
    <table role="table" class="w-full max-lg:block" :aria-label="t('rules.saved_rules')">
      <thead role="rowgroup" class="bg-slate-50 dark:bg-slate-800/60 max-lg:sr-only">
        <tr role="row">
          <th role="columnheader" scope="col" class="th">{{ t('rules.app_label') }}</th>
          <th role="columnheader" scope="col" class="th">{{ t('common.path') }}</th>
          <th role="columnheader" scope="col" class="th">{{ t('rules.action_label') }}</th>
          <th role="columnheader" scope="col" class="th">{{ t('rules.proxy_label') }}</th>
          <th role="columnheader" scope="col" class="th whitespace-nowrap">{{ t('rules.kill_switch_label') }}</th>
          <th role="columnheader" scope="col" class="th">{{ t('common.enabled') }}</th>
          <th role="columnheader" scope="col" class="th">{{ t('common.actions') }}</th>
        </tr>
      </thead>
      <tbody role="rowgroup" class="max-lg:block">
        <tr v-for="r in rules" :key="r.ID" role="row" class="border-b border-slate-200 last:border-0 dark:border-slate-800 max-lg:grid max-lg:grid-cols-2 max-lg:gap-x-3 max-lg:gap-y-2 max-lg:p-4">
          <td role="cell" class="rule-cell max-lg:col-span-full">
            <div class="flex min-w-0 items-center gap-2">
              <AppIcon v-if="r.Kind === 'exe'" :path="r.Path" :size="20" />
              <UiIcon v-else name="folder" />
              <div class="min-w-0">
                <div class="break-words font-semibold">{{ r.Name }}</div>
                <div class="muted">{{ t(`rules.kind_${r.Kind}`) }}</div>
              </div>
            </div>
          </td>
          <td role="cell" class="rule-cell min-w-0 max-lg:col-span-full">
            <span aria-hidden="true" class="rule-label">{{ t('common.path') }}</span>
            <div class="muted break-all lg:max-w-56">{{ r.Path }}</div>
          </td>
          <td role="cell" class="rule-cell">
            <span aria-hidden="true" class="rule-label">{{ t('rules.action_label') }}</span>
            <span class="badge" :class="actionStyle[r.Action]">{{ t(`ruleAction.${r.Action}`) }}</span>
          </td>
          <td role="cell" class="rule-cell break-words">
            <span aria-hidden="true" class="rule-label">{{ t('rules.proxy_label') }}</span>
            {{ r.Action === 'proxy' ? proxyNames[r.ProxyID] ?? t('preview.proxy_missing') : t('common.none') }}
          </td>
          <td role="cell" class="rule-cell">
            <span aria-hidden="true" class="rule-label">{{ t('rules.kill_switch_label') }}</span>
            <Toggle v-if="r.Action === 'proxy'" :model-value="r.KillSwitch" :aria-label="t('rules.kill_switch_for', { name: r.Name })" @update:model-value="emit('killSwitch', r)" />
            <span v-else class="muted">{{ t('common.none') }}</span>
          </td>
          <td role="cell" class="rule-cell">
            <span aria-hidden="true" class="rule-label">{{ t('common.enabled') }}</span>
            <Toggle :model-value="r.Enabled" :aria-label="t('rules.enabled_for', { name: r.Name })" @update:model-value="emit('enabled', r)" />
          </td>
          <td role="cell" class="rule-cell max-lg:col-span-full">
            <div class="flex flex-wrap gap-2 lg:flex-nowrap">
              <button class="list-action text-brand-700 dark:text-brand-100" @click="emit('edit', r)">{{ t('common.edit') }}</button>
              <button class="list-action text-red-700 dark:text-red-300" @click="emit('delete', r)">{{ t('common.delete') }}</button>
            </div>
          </td>
        </tr>
      </tbody>
    </table>
  </div>
</template>
