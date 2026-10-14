<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '../api'
import { useApp } from '../stores/app'
import Modal from '../components/Modal.vue'
import Toggle from '../components/Toggle.vue'
import StatusDot from '../components/StatusDot.vue'
import EmptyState from '../components/EmptyState.vue'
import type { ProfileInput, ProxyProfile, Rule, TestResult } from '../types'

const { t } = useI18n()
const store = useApp()

const profiles = ref<ProxyProfile[]>([])
const rules = ref<Rule[]>([])
const loading = ref(true)

const showModal = ref(false)
const editingID = ref<string | null>(null)
const form = reactive({
  name: '',
  host: '',
  port: 1080,
  username: '',
  password: '',
  enabled: true,
  timeout_ms: 10000,
  retry_interval_ms: 5000,
  max_retries: 3,
})
const showAuth = ref(false)
const showAdvanced = ref(false)
const saving = ref(false)
const testing = ref<Record<string, boolean>>({})
const testResult = ref<Record<string, TestResult>>({})
const testingInForm = ref(false)
const formTestMsg = ref('')

const errors = reactive({ name: '', host: '', port: '' })

const usedBy = computed(() => {
  const m: Record<string, number> = {}
  for (const r of rules.value) if (r.ProxyID) m[r.ProxyID] = (m[r.ProxyID] ?? 0) + 1
  return m
})

async function refresh() {
  loading.value = true
  try {
    ;[profiles.value, rules.value] = await Promise.all([api.getProfiles(), api.getRules()])
    await store.refreshProxyStatuses()
  } catch (e) {
    store.toastError(e, t('toast.error'))
  } finally {
    loading.value = false
  }
}
onMounted(refresh)
watch(() => store.connected, (v) => v && refresh())

function statusOf(id: string) {
  return store.proxyStatuses[id]?.State ?? 0
}
function detailOf(id: string) {
  return store.proxyStatuses[id]
}

function openAdd() {
  editingID.value = null
  Object.assign(form, {
    name: '',
    host: '',
    port: 1080,
    username: '',
    password: '',
    enabled: true,
    timeout_ms: 10000,
    retry_interval_ms: 5000,
    max_retries: 3,
  })
  showAuth.value = showAdvanced.value = false
  formTestMsg.value = ''
  showModal.value = true
}

function openEdit(p: ProxyProfile) {
  editingID.value = p.ID
  Object.assign(form, {
    name: p.Name,
    host: p.Host,
    port: p.Port,
    username: p.Username,
    password: '',
    enabled: p.Enabled,
    timeout_ms: p.TimeoutMS || 10000,
    retry_interval_ms: p.RetryIntervalMS || 5000,
    max_retries: p.MaxRetries || 3,
  })
  showAuth.value = !!p.Username
  showAdvanced.value = false
  formTestMsg.value = ''
  showModal.value = true
}

function validate(): boolean {
  errors.name = form.name.trim() ? '' : t('wizard.name_required')
  errors.host = form.host.trim() ? '' : t('wizard.host_required')
  errors.port = form.port >= 1 && form.port <= 65535 ? '' : t('wizard.port_required')
  return !errors.name && !errors.host && !errors.port
}

function toInput(): ProfileInput {
  return {
    name: form.name.trim(),
    host: form.host.trim(),
    port: form.port,
    username: form.username.trim(),
    password: form.password ? form.password : null,
    enabled: form.enabled,
    timeout_ms: form.timeout_ms,
    retry_interval_ms: form.retry_interval_ms,
    max_retries: form.max_retries,
  }
}

async function save() {
  if (!validate()) return
  saving.value = true
  try {
    if (editingID.value) await api.updateProfile(editingID.value, toInput())
    else await api.createProfile(toInput())
    store.toast('ok', t('proxies.saved'))
    showModal.value = false
    await refresh()
  } catch (e) {
    store.toastError(e, t('toast.error'))
  } finally {
    saving.value = false
  }
}

async function toggleEnabled(p: ProxyProfile) {
  try {
    await api.setProfileEnabled(p.ID, !p.Enabled)
    p.Enabled = !p.Enabled
  } catch (e) {
    store.toastError(e, t('toast.error'))
  }
}

async function del(p: ProxyProfile) {
  const ok = await store.askConfirm(
    t('proxies.delete_title'),
    t('proxies.delete_body', { name: p.Name }),
    t('common.delete'),
  )
  if (!ok) return
  try {
    await api.deleteProfile(p.ID)
    store.toast('ok', t('proxies.deleted'))
    await refresh()
  } catch (e) {
    store.toastError(e, t('toast.error'))
  }
}

async function test(p: ProxyProfile) {
  testing.value[p.ID] = true
  testResult.value[p.ID] = { latency_ms: 0, public_ip: '', error: '' }
  try {
    testResult.value[p.ID] = await api.testProfile(p.ID)
  } catch (e) {
    const msg = e instanceof Error ? e.message : String(e)
    testResult.value[p.ID] = { latency_ms: 0, public_ip: '', error: msg }
  } finally {
    testing.value[p.ID] = false
  }
}

async function testInForm() {
  if (!validate()) return
  testingInForm.value = true
  formTestMsg.value = ''
  try {
    // Persist first so the service can test the real profile, then test.
    if (editingID.value) {
      await api.updateProfile(editingID.value, toInput())
      const r = await api.testProfile(editingID.value)
      formTestMsg.value = r.error
        ? t('wizard.test_fail', { err: r.error })
        : t('wizard.test_ok', { ms: r.latency_ms, ip: r.public_ip })
    } else {
      const p = await api.createProfile(toInput())
      editingID.value = p.ID
      const r = await api.testProfile(p.ID)
      formTestMsg.value = r.error
        ? t('wizard.test_fail', { err: r.error })
        : t('wizard.test_ok', { ms: r.latency_ms, ip: r.public_ip })
      await refresh()
    }
  } catch (e) {
    formTestMsg.value = t('wizard.test_fail', { err: e instanceof Error ? e.message : String(e) })
  } finally {
    testingInForm.value = false
  }
}
</script>

<template>
  <div class="space-y-4 p-6">
    <div class="flex items-center justify-between">
      <h1 class="text-xl font-bold">{{ t('proxies.title') }}</h1>
      <button class="btn-primary" :disabled="!store.connected" @click="openAdd">+ {{ t('proxies.add') }}</button>
    </div>

    <div v-if="loading" class="space-y-2">
      <div v-for="i in 3" :key="i" class="card h-20 animate-pulse bg-slate-200/60 dark:bg-slate-800/60" />
    </div>

    <EmptyState v-else-if="!profiles.length" :title="t('proxies.empty_title')" :body="t('proxies.empty_body')">
      <button class="btn-primary mt-2" :disabled="!store.connected" @click="openAdd">+ {{ t('proxies.add') }}</button>
    </EmptyState>

    <div v-else class="grid gap-3 lg:grid-cols-2">
      <div v-for="p in profiles" :key="p.ID" class="card flex flex-col gap-3">
        <div class="flex items-start justify-between gap-3">
          <div class="min-w-0">
            <div class="flex items-center gap-2">
              <span class="truncate font-semibold">{{ p.Name }}</span>
              <StatusDot :state="statusOf(p.ID)" />
            </div>
            <div class="mt-0.5 text-sm text-slate-500 dark:text-slate-400">{{ p.Host }}:{{ p.Port }}</div>
            <div v-if="usedBy[p.ID]" class="text-xs text-slate-400">{{ t('proxies.used_by_rules', { n: usedBy[p.ID] }) }}</div>
          </div>
          <Toggle :model-value="p.Enabled" @update:model-value="toggleEnabled(p)" />
        </div>

        <div class="min-h-4 text-xs">
          <template v-if="detailOf(p.ID)?.State === 2">
            <span class="text-green-600 dark:text-green-400">
              {{ t('proxies.test_ok', { ms: detailOf(p.ID)?.LatencyMS }) }}
              <template v-if="detailOf(p.ID)?.PublicIP"> · {{ t('proxies.test_ip', { ip: detailOf(p.ID)?.PublicIP }) }}</template>
            </span>
          </template>
          <span v-else-if="detailOf(p.ID)?.LastError" class="text-red-500">{{ detailOf(p.ID)?.LastError }}</span>
        </div>

        <div v-if="testResult[p.ID]?.latency_ms || testResult[p.ID]?.error" class="text-xs">
          <span v-if="!testResult[p.ID]?.error" class="text-green-600 dark:text-green-400">
            {{ t('proxies.test_ok', { ms: testResult[p.ID]?.latency_ms }) }} · {{ t('proxies.test_ip', { ip: testResult[p.ID]?.public_ip }) }}
          </span>
          <span v-else class="text-red-500">{{ t('proxies.test_fail') }}: {{ testResult[p.ID]?.error }}</span>
        </div>

        <div class="flex gap-2">
          <button class="btn-ghost" :disabled="testing[p.ID]" @click="test(p)">
            {{ testing[p.ID] ? t('wizard.testing') : t('common.test') }}
          </button>
          <button class="btn-ghost" @click="openEdit(p)">{{ t('common.edit') }}</button>
          <button class="btn-ghost ml-auto text-red-500 hover:text-red-600" @click="del(p)">{{ t('common.delete') }}</button>
        </div>
      </div>
    </div>

    <Modal v-if="showModal" :title="editingID ? t('proxies.edit_title') : t('proxies.add_title')" @close="showModal = false">
      <div class="space-y-3">
        <div>
          <label class="label">{{ t('wizard.name_label') }} <span class="text-red-500">*</span></label>
          <input v-model="form.name" class="input" :placeholder="t('wizard.name_ph')" />
          <p v-if="errors.name" class="mt-1 text-xs text-red-500">{{ errors.name }}</p>
        </div>
        <div class="grid grid-cols-3 gap-3">
          <div class="col-span-2">
            <label class="label">{{ t('wizard.host_label') }} <span class="text-red-500">*</span></label>
            <input v-model="form.host" class="input" :placeholder="t('wizard.host_ph')" />
            <p v-if="errors.host" class="mt-1 text-xs text-red-500">{{ errors.host }}</p>
          </div>
          <div>
            <label class="label">{{ t('wizard.port_label') }} <span class="text-red-500">*</span></label>
            <input v-model.number="form.port" type="number" min="1" max="65535" class="input" :placeholder="t('wizard.port_ph')" />
            <p v-if="errors.port" class="mt-1 text-xs text-red-500">{{ errors.port }}</p>
          </div>
        </div>

        <button class="text-xs font-medium text-brand-600 dark:text-brand-100" @click="showAuth = !showAuth">
          {{ showAuth ? '▾' : '▸' }} {{ t('wizard.auth_title') }}
        </button>
        <div v-if="showAuth" class="grid grid-cols-2 gap-3 rounded-lg bg-slate-50 p-3 dark:bg-slate-800/50">
          <div>
            <label class="label">{{ t('wizard.username_label') }}</label>
            <input v-model="form.username" class="input" autocomplete="off" />
          </div>
          <div>
            <label class="label">{{ t('wizard.password_label') }}</label>
            <input v-model="form.password" type="password" class="input" autocomplete="new-password" />
            <p v-if="editingID" class="mt-1 text-xs text-slate-400">{{ t('proxies.password_keep') }}</p>
          </div>
        </div>

        <button class="text-xs font-medium text-brand-600 dark:text-brand-100" @click="showAdvanced = !showAdvanced">
          {{ showAdvanced ? '▾' : '▸' }} {{ t('wizard.advanced_title') }}
        </button>
        <div v-if="showAdvanced" class="grid grid-cols-3 gap-3 rounded-lg bg-slate-50 p-3 dark:bg-slate-800/50">
          <div>
            <label class="label">{{ t('wizard.timeout_label') }}</label>
            <input v-model.number="form.timeout_ms" type="number" min="500" class="input" />
          </div>
          <div>
            <label class="label">{{ t('wizard.retry_label') }}</label>
            <input v-model.number="form.retry_interval_ms" type="number" min="500" class="input" />
          </div>
          <div>
            <label class="label">{{ t('wizard.retries_label') }}</label>
            <input v-model.number="form.max_retries" type="number" min="0" class="input" />
          </div>
        </div>

        <div class="flex items-center gap-2">
          <Toggle v-model="form.enabled" />
          <span class="text-sm">{{ t('common.enabled') }}</span>
        </div>

        <p v-if="formTestMsg" class="text-sm text-slate-600 dark:text-slate-300">{{ formTestMsg }}</p>

        <div class="flex justify-between pt-2">
          <button class="btn-ghost" :disabled="testingInForm || saving" @click="testInForm">
            {{ testingInForm ? t('wizard.testing') : t('common.test') }}
          </button>
          <div class="flex gap-2">
            <button class="btn-ghost" @click="showModal = false">{{ t('common.cancel') }}</button>
            <button class="btn-primary" :disabled="saving" @click="save">{{ saving ? t('common.loading') : t('common.save') }}</button>
          </div>
        </div>
      </div>
    </Modal>
  </div>
</template>
