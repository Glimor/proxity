<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import SideNav from './components/SideNav.vue'
import ToastHost from './components/ToastHost.vue'
import ConfirmDialog from './components/ConfirmDialog.vue'

const route = useRoute()
const isWizard = computed(() => route.name === 'wizard')
const main = ref<HTMLElement | null>(null)

watch(() => route.path, async (path) => {
  await nextTick()
  if (route.path !== path) return
  main.value?.scrollTo(0, 0)
  main.value?.focus({ preventScroll: true })
})
</script>

<template>
  <div class="flex h-full min-w-0 flex-col overflow-hidden md:flex-row">
    <a href="#main-content" class="btn-primary sr-only z-20 focus:not-sr-only focus:absolute focus:left-3 focus:top-3" @click.prevent="main?.focus()">Skip to content</a>
    <SideNav v-if="!isWizard" />
    <main id="main-content" ref="main" tabindex="-1" class="app-main">
      <RouterView />
    </main>
    <ToastHost />
    <ConfirmDialog />
  </div>
</template>
