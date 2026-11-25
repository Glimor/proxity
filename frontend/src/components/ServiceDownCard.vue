<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { api, parseError } from '../api'
import { useApp } from '../stores/app'
import type { ServiceStatus } from '../types'

const { t } = useI18n()
const store = useApp()
const status = ref<ServiceStatus | null>(null)
const installing = ref(false)
let refreshTimer: ReturnType<typeof setInterval> | undefined

async function refreshStatus() {
  try {
    status.value = await api.serviceStatus()
  } catch {
    status.value = null
  }
}

async function installService() {
  installing.value = true
  try {
    await api.installService()
    store.toast('info', t('service.starting'))
  } catch (e) {
    if (parseError(e).code === 'uac.cancelled') store.toast('err', t('service.uac_cancelled'))
    else store.toastError(e, t('service.install_failed'))
  } finally {
    installing.value = false
    await refreshStatus()
  }
}

onMounted(() => {
  void refreshStatus()
  refreshTimer = setInterval(() => {
    void refreshStatus()
  }, 3000)
})

onUnmounted(() => {
  if (refreshTimer !== undefined) clearInterval(refreshTimer)
})
</script>

<template>
  <div class="card space-y-3 p-5 sm:p-6">
    <h2 class="section-title">
      {{ t('dash.service_down_title') }}
    </h2>
    <p class="muted max-w-prose">
      {{ t('dash.service_down_body') }}
    </p>
    <p v-if="status && !status.binary_found" class="mb-3 text-sm text-red-600 dark:text-red-400">
      {{ t('dash.service_binary_missing') }}
    </p>
    <p v-else-if="status && status.binary_found && !status.installed" class="muted break-all">
      {{ t('dash.service_not_installed') }} — {{ status.binary_path }}
    </p>
    <div class="flex flex-wrap items-center gap-3">
      <button
        class="btn-primary"
        :disabled="installing || !status?.binary_found"
        @click="installService"
      >
        {{
          installing
            ? t('common.loading')
            : status?.installed && (status.disabled || !status.running)
              ? t('dash.start_service_button')
              : t('dash.install_service_button')
        }}
      </button>
      <span class="muted">{{ t('dash.install_service_hint') }}</span>
    </div>
  </div>
</template>
