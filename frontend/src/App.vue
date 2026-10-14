<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import SideNav from './components/SideNav.vue'
import ToastHost from './components/ToastHost.vue'
import ConfirmDialog from './components/ConfirmDialog.vue'

const route = useRoute()
const isWizard = computed(() => route.name === 'wizard')
</script>

<template>
  <div class="flex h-full overflow-hidden">
    <SideNav v-if="!isWizard" />
    <main class="min-w-0 flex-1 overflow-y-auto">
      <RouterView />
    </main>
    <ToastHost />
    <ConfirmDialog />
  </div>
</template>
