<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useApp } from '../stores/app'

const { t } = useI18n()
const store = useApp()
const expanded = ref<Record<number, boolean>>({})
</script>

<template>
  <div class="pointer-events-none fixed bottom-4 right-4 z-[60] flex w-96 flex-col gap-2">
    <div
      v-for="toast in store.toasts"
      :key="toast.id"
      class="pointer-events-auto card p-3"
      :class="{
        'border-green-500/50': toast.kind === 'ok',
        'border-red-500/60': toast.kind === 'err',
        'border-brand-500/50': toast.kind === 'info',
      }"
    >
      <div class="flex items-start gap-2">
        <span
          class="mt-1 h-2 w-2 shrink-0 rounded-full"
          :class="{
            'bg-green-500': toast.kind === 'ok',
            'bg-red-500': toast.kind === 'err',
            'bg-brand-500': toast.kind === 'info',
          }"
        />
        <div class="min-w-0 flex-1 text-sm">
          <p class="font-medium">{{ toast.msg }}</p>
          <template v-if="toast.detail">
            <button
              class="mt-0.5 text-xs text-slate-400 underline"
              @click="expanded[toast.id] = !expanded[toast.id]"
            >
              {{ expanded[toast.id] ? t('common.hide_details') : t('common.see_details') }}
            </button>
            <pre v-if="expanded[toast.id]" class="mt-1 max-h-32 overflow-auto whitespace-pre-wrap text-xs text-slate-500">{{ toast.detail }}</pre>
          </template>
        </div>
      </div>
    </div>
  </div>
</template>
