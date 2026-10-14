<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { api } from '../api'

const props = defineProps<{ path: string; size?: number }>()
const src = ref('')

async function load() {
  if (!props.path) {
    src.value = ''
    return
  }
  try {
    src.value = await api.getExeIcon(props.path)
  } catch {
    src.value = ''
  }
}
onMounted(load)
watch(() => props.path, load)
</script>

<template>
  <img
    v-if="src"
    :src="src"
    :width="size ?? 20"
    :height="size ?? 20"
    class="inline-block shrink-0 rounded"
    alt=""
  />
  <span
    v-else
    class="inline-flex shrink-0 items-center justify-center rounded bg-slate-300 text-[10px] font-bold text-slate-600 dark:bg-slate-700 dark:text-slate-300"
    :style="{ width: (size ?? 20) + 'px', height: (size ?? 20) + 'px' }"
    >?</span
  >
</template>
