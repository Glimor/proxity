<script setup lang="ts">
defineProps<{ title: string; body?: string }>()
</script>

<template>
  <div class="card flex flex-col items-start gap-2 p-6 sm:p-8">
    <h2 class="section-title">{{ title }}</h2>
    <p v-if="body" class="muted max-w-prose">{{ body }}</p>
    <slot />
  </div>
</template>
