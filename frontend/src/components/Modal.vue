<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, useId } from 'vue'
import { useI18n } from 'vue-i18n'
import UiIcon from './UiIcon.vue'

defineProps<{ readonly title: string; readonly wide?: boolean; readonly alert?: boolean }>()
const emit = defineEmits<{ close: [] }>()
const { t } = useI18n()
const dialog = ref<HTMLDialogElement | null>(null)
const titleId = useId()
let origin: HTMLElement | null = null

onMounted(() => {
  if (document.activeElement instanceof HTMLElement) origin = document.activeElement
  dialog.value?.showModal()
})

onBeforeUnmount(() => {
  dialog.value?.close()
  if (origin?.isConnected) origin.focus({ preventScroll: true })
})
</script>

<template>
  <Teleport to="body">
    <dialog
      ref="dialog"
      class="dialog-panel"
      :class="{ 'max-w-2xl': wide }"
      :role="alert ? 'alertdialog' : undefined"
      :aria-labelledby="titleId"
      @cancel.prevent="emit('close')"
    >
      <div class="mb-4 flex items-start justify-between gap-3">
        <h2 :id="titleId" class="section-title self-center">{{ title }}</h2>
        <button type="button" class="list-action -mr-2" :aria-label="t('common.close')" :title="t('common.close')" @click="emit('close')"><UiIcon name="close" /></button>
      </div>
      <slot />
    </dialog>
  </Teleport>
</template>
