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
    class="relative inline-flex h-5 w-9 shrink-0 items-center rounded-full transition-colors disabled:opacity-50"
    :class="modelValue ? 'bg-brand-600' : 'bg-slate-300 dark:bg-slate-600'"
    @click.stop="flip"
  >
    <span
      class="inline-block h-4 w-4 transform rounded-full bg-white shadow transition-transform"
      :class="modelValue ? 'translate-x-4' : 'translate-x-0.5'"
    />
  </button>
</template>
