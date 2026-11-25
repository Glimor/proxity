<script setup lang="ts">
const props = defineProps<{ modelValue: boolean; disabled?: boolean }>()
const emit = defineEmits<{ (e: 'update:modelValue', v: boolean): void }>()

function flip() {
  if (!props.disabled) emit('update:modelValue', !props.modelValue)
}
</script>

<template>
  <button
    type="button"
    role="switch"
    :aria-checked="modelValue"
    :disabled="disabled"
    class="inline-flex size-11 shrink-0 items-center justify-center rounded-lg hover:bg-slate-100 active:bg-slate-200 disabled:cursor-not-allowed disabled:opacity-60 dark:hover:bg-slate-800 dark:active:bg-slate-700"
    @click.stop="flip"
  >
    <span
      aria-hidden="true"
      class="flex h-5 w-9 items-center rounded-full transition-colors"
      :class="modelValue ? 'bg-brand-600 dark:bg-brand-500' : 'bg-slate-500 dark:bg-slate-600'"
    >
      <span class="size-4 rounded-full bg-white transition-transform" :class="modelValue ? 'translate-x-[1.125rem]' : 'translate-x-0.5'" />
    </span>
  </button>
</template>
