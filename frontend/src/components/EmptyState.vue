<script setup lang="ts">
defineProps<{ title: string; body?: string }>()
</script>

<template>
  <div class="flex flex-col items-center justify-center gap-2 py-14 text-center">
    <div class="text-4xl opacity-40">◌</div>
    <h3 class="text-base font-semibold">{{ title }}</h3>
    <p v-if="body" class="max-w-md text-sm text-slate-500 dark:text-slate-400">{{ body }}</p>
    <slot />
  </div>
</template>
