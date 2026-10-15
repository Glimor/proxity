<script setup lang="ts">
defineProps<{ title: string; wide?: boolean }>()
const emit = defineEmits<{ (e: 'close'): void }>()
</script>

<template>
  <div class="fixed inset-0 z-40 flex items-center justify-center bg-black/50 p-4" @click.self="emit('close')">
    <div
      class="card max-h-[90vh] w-full overflow-y-auto"
      :class="wide ? 'max-w-2xl' : 'max-w-lg'"
      role="dialog"
      aria-modal="true"
    >
      <div class="mb-4 flex items-center justify-between">
        <h3 class="text-lg font-semibold">{{ title }}</h3>
        <button class="text-slate-400 hover:text-slate-600 dark:hover:text-slate-200" @click="emit('close')" aria-label="Close">✕</button>
      </div>
      <slot />
    </div>
  </div>
</template>
