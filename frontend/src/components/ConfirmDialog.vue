<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { useApp } from '../stores/app'
import Modal from './Modal.vue'

const { t } = useI18n()
const store = useApp()
</script>

<template>
  <Modal
    v-if="store.confirm"
    :title="store.confirm.title"
    alert
    @close="store.answerConfirm(false)"
  >
      <p class="mb-6 whitespace-pre-line text-sm text-slate-600 dark:text-slate-300">{{ store.confirm.body }}</p>
      <div class="flex flex-wrap justify-end gap-2">
        <button class="btn-ghost" autofocus @click="store.answerConfirm(false)">{{ t('common.cancel') }}</button>
        <button
          :class="store.confirm.danger ? 'btn-danger' : 'btn-primary'"
          @click="store.answerConfirm(true)"
        >
          {{ store.confirm.okLabel || t('common.confirm') }}
        </button>
      </div>
  </Modal>
</template>
