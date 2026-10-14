<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { useApp } from '../stores/app'

const { t } = useI18n()
const store = useApp()
</script>

<template>
  <div
    v-if="store.confirm"
    class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4"
  >
    <div class="card w-full max-w-md" role="alertdialog" aria-modal="true">
      <h3 class="mb-2 text-lg font-semibold" :class="store.confirm.danger ? 'text-red-600 dark:text-red-400' : ''">
        {{ store.confirm.title }}
      </h3>
      <p class="mb-6 whitespace-pre-line text-sm text-slate-600 dark:text-slate-300">{{ store.confirm.body }}</p>
      <div class="flex justify-end gap-2">
        <button class="btn-ghost" @click="store.answerConfirm(false)">{{ t('common.cancel') }}</button>
        <button
          :class="store.confirm.danger ? 'btn-danger' : 'btn-primary'"
          @click="store.answerConfirm(true)"
        >
          {{ store.confirm.okLabel || t('common.confirm') }}
        </button>
      </div>
    </div>
  </div>
</template>
