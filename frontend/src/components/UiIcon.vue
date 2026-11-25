<script setup lang="ts">
const paths = {
  dashboard: 'M3 3h7v9H3z M14 3h7v5h-7z M14 12h7v9h-7z M3 16h7v5H3z',
  proxies: 'M3 7h17m-4-4 4 4-4 4M21 17H4m4-4-4 4 4 4',
  apps: 'M8 3H5a2 2 0 0 0-2 2v3m13-5h3a2 2 0 0 1 2 2v3M3 16v3a2 2 0 0 0 2 2h3m13-5v3a2 2 0 0 1-2 2h-3M8 8h8v8H8z',
  connections: 'M10 13a5 5 0 0 0 7 0l3-3a5 5 0 0 0-7-7l-2 2m3 6a5 5 0 0 0-7 0l-3 3a5 5 0 0 0 7 7l2-2',
  stats: 'M3 3v18h18M8 16v-3m5 3V8m5 8v-5',
  blocked: 'M22 12a10 10 0 1 1-20 0 10 10 0 0 1 20 0zM5 5l14 14',
  settings: 'M4 6h16M4 12h16M4 18h16M9 4v4m6 2v4m-6 2v4',
  menu: 'M4 6h16M4 12h16M4 18h16',
  close: 'm6 6 12 12M18 6 6 18',
  folder: 'M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z',
  route: 'M5 6a2 2 0 1 0 0-4 2 2 0 0 0 0 4Zm14 16a2 2 0 1 0 0-4 2 2 0 0 0 0 4ZM5 6v3a3 3 0 0 0 3 3h8a3 3 0 0 1 3 3v3',
} as const

defineProps<{ readonly name: keyof typeof paths }>()
</script>

<template>
  <svg aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" class="size-5 shrink-0">
    <path :d="paths[name]" />
  </svg>
</template>
