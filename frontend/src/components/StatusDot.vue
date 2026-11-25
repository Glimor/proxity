<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

// state: 0 disconnected, 1 connecting, 2 connected, 3 error
const props = defineProps<{ state: number }>()
const { t } = useI18n()

const cls = computed(() => {
  switch (props.state) {
    case 2:
      return 'bg-green-500'
    case 1:
      return 'bg-amber-400 animate-pulse'
    case 3:
      return 'bg-red-500'
    default:
      return 'bg-slate-400'
  }
})
const label = computed(() => {
  switch (props.state) {
    case 2:
      return t('proxyState.connected')
    case 1:
      return t('proxyState.connecting')
    case 3:
      return t('proxyState.error')
    default:
      return t('proxyState.disconnected')
  }
})
</script>

<template>
  <span class="muted inline-flex items-center gap-2">
    <span aria-hidden="true" class="size-2 rounded-full" :class="cls" />
    <span>{{ label }}</span>
  </span>
</template>
