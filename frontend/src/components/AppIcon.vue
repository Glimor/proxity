<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { api } from '../api'
import UiIcon from './UiIcon.vue'

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
    aria-hidden="true"
    class="inline-flex shrink-0 items-center justify-center text-slate-600 dark:text-slate-400"
    :style="{ width: (size ?? 20) + 'px', height: (size ?? 20) + 'px' }"
  ><UiIcon name="apps" /></span>
</template>
