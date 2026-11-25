<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { appsvc } from '../../wailsjs/go/models'
import { api, parseError } from '../api'
import Modal from './Modal.vue'
import UiIcon from './UiIcon.vue'

const props = defineProps<{ readonly connected: boolean }>()
const { t } = useI18n()
const showing = ref(false)
const picking = ref(false)
const loading = ref(false)
const selected = ref('')
const error = ref('')
const result = ref<appsvc.RoutePreview | null>(null)
const active = computed(() => Boolean(result.value?.engine.running && result.value.engine.capture_alive))
let request = 0

function close() {
  request++
  showing.value = false
  loading.value = false
  selected.value = ''
  error.value = ''
  result.value = null
}

async function choose() {
  if (picking.value || !props.connected) return
  const before = request
  picking.value = true
  let path: string
  try {
    path = await api.pickExeFile()
  } catch (cause) {
    if (before === request) {
      showing.value = true
      result.value = null
      loading.value = false
      request++
      error.value = parseError(cause).msg || t('preview.pick_error')
    }
    return
  } finally {
    picking.value = false
  }
  if (before !== request || !path || !props.connected) return

  const id = ++request
  result.value = null
  selected.value = path
  error.value = ''
  loading.value = true
  showing.value = true
  try {
    const response = await api.previewRoute(path)
    if (id === request) result.value = response
  } catch (cause) {
    if (id === request) {
      error.value = cause instanceof TypeError
        ? t('preview.binding_error')
        : parseError(cause).msg || t('preview.read_error')
    }
  } finally {
    if (id === request) loading.value = false
  }
}

watch(() => props.connected, (connected) => {
  if (connected) return
  request++
  loading.value = false
  result.value = null
  if (showing.value) error.value = t('preview.offline')
})
onBeforeUnmount(() => { request++ })
</script>

<template>
  <button type="button" class="btn-ghost" :disabled="!connected" :aria-disabled="picking || undefined" :aria-busy="picking" @click="choose">
    <UiIcon name="route" />
    {{ t('preview.title') }}
  </button>
  <Modal v-if="showing" :title="t('preview.title')" @close="close">
    <p class="muted">{{ t('preview.scope') }}</p>
    <p v-if="selected" class="mt-2 break-all text-sm">{{ result?.exe_path ?? selected }}</p>
    <div v-if="loading" class="mt-4 rounded-lg bg-slate-100 p-3 dark:bg-slate-800" role="status">{{ t('preview.loading') }}</div>
    <div v-else-if="error" class="mt-4 rounded-lg bg-red-50 p-3 text-sm text-red-900 dark:bg-red-950 dark:text-red-200" role="alert">
      <p class="font-semibold">{{ t('preview.error_title') }}</p>
      <p class="mt-1 break-words">{{ error }}</p>
    </div>
    <template v-else-if="result">
      <p v-if="!active" class="mt-4 rounded-lg bg-amber-50 p-3 text-sm text-amber-900 dark:bg-amber-950 dark:text-amber-100" role="status">{{ t('preview.inactive') }}</p>
      <dl class="mt-4 space-y-3 text-sm">
        <div class="rounded-lg bg-slate-100 p-3 dark:bg-slate-800">
          <dt class="muted">{{ t('preview.saved_route') }}</dt>
          <dd class="text-lg font-semibold">{{ t(`ruleAction.${result.action}`) }}</dd>
        </div>
        <div>
          <dt class="font-semibold">{{ t('preview.winning_match') }}</dt>
          <dd class="muted">{{ t(`preview.reason_${result.reason}`) }}</dd>
          <dd v-if="result.rule" class="mt-1 min-w-0">
            {{ result.rule.name }} · {{ t(`rules.kind_${result.rule.kind}`) }}
            <p class="muted break-all">{{ result.rule.path }}</p>
            <p v-if="result.rule.priority !== undefined" class="muted">{{ t('preview.priority', { priority: result.rule.priority }) }}</p>
          </dd>
          <dd v-if="result.saved_default_action === 'block' && result.action !== 'block'" class="mt-2 rounded-lg bg-amber-50 p-3 text-amber-900 dark:bg-amber-950 dark:text-amber-100">{{ t('preview.default_discrepancy') }}</dd>
        </div>
        <template v-if="result.proxy">
          <div class="border-t border-slate-200 pt-3 dark:border-slate-800">
            <dt class="font-semibold">{{ t('preview.proxy') }}</dt>
            <dd v-if="result.proxy.name" class="mt-1 break-words">{{ result.proxy.name }}</dd>
            <dd class="muted">{{ t(`preview.proxy_${result.proxy.availability}`) }}</dd>
          </div>
          <div>
            <dt class="font-semibold">{{ t('preview.fallback') }}</dt>
            <dd class="muted">{{ t('preview.fallback_value', { action: t(`ruleAction.${result.proxy.fallback_action}`) }) }}</dd>
          </div>
        </template>
      </dl>
      <details class="mt-4 border-t border-slate-200 pt-2 text-sm dark:border-slate-800">
        <summary class="min-h-11 cursor-pointer content-center rounded font-semibold">{{ t('preview.protection') }}</summary>
        <ul class="muted space-y-2 pb-1">
          <li>{{ t(`preview.policy_${result.exe_policy}`) }}</li>
          <li>{{ t(result.kill_switch_global ? 'preview.global_on' : 'preview.global_off') }}</li>
          <li>{{ t(result.lan_access ? 'preview.lan_on' : 'preview.lan_off') }}</li>
          <li>{{ t(result.secure_dns ? 'preview.dns_on' : 'preview.dns_off') }}</li>
          <li>{{ t(result.service_executable ? 'preview.service_exe' : 'preview.exemptions') }}</li>
          <li>{{ t('preview.protection_scope') }}</li>
        </ul>
      </details>
    </template>
    <div class="mt-5 flex flex-wrap justify-end gap-2">
      <button type="button" class="btn-ghost" :disabled="!connected" :aria-disabled="picking || undefined" :aria-busy="picking" @click="choose">{{ t('preview.repick') }}</button>
      <button type="button" class="btn-primary min-w-20" @click="close">{{ t(loading ? 'common.cancel' : 'preview.done') }}</button>
    </div>
  </Modal>
</template>
