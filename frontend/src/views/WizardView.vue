<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { api, parseError } from '../api'
import { exeName } from '../format'
import { useApp } from '../stores/app'
import Toggle from '../components/Toggle.vue'
import Modal from '../components/Modal.vue'
import AppIcon from '../components/AppIcon.vue'
import ServiceDownCard from '../components/ServiceDownCard.vue'
import ProxyProtocolField from '../components/ProxyProtocolField.vue'
import type { ProxyProtocol, ProcessInfo, ProfileInput, ProxyProfile } from '../types'

const { t } = useI18n()
const router = useRouter()
const store = useApp()

const step = ref(1)
const busy = ref(false)

// ---- step 2: proxy ---------------------------------------------------------
const p = reactive({
  protocol: 'socks5' as ProxyProtocol,
  name: '',
  host: '',
  port: 1080,
  username: '',
  password: '',
  timeout_ms: 10000,
  retry_interval_ms: 5000,
  max_retries: 3,
})
const pErr = reactive({ name: '', host: '', port: '' })
watch(() => p.protocol, (protocol) => {
  if (protocol === 'socks4') p.password = ''
})
const showAuth = ref(true)
const showAdv = ref(false)
const created = ref<ProxyProfile | null>(null)
const profiles = ref<ProxyProfile[]>([])
const testMsg = ref('')
const testOk = ref(false)
let savedInput = ''

function validateProxy(): boolean {
  pErr.name = '' // A display name is optional; derive one from the endpoint.
  pErr.host = p.host.trim() ? '' : t('wizard.host_required')
  pErr.port = Number.isInteger(p.port) && p.port >= 1 && p.port <= 65535 ? '' : t('wizard.port_required')
  return !pErr.name && !pErr.host && !pErr.port
}

function profileInput(): ProfileInput {
  return {
    protocol: p.protocol,
    name: p.name.trim() || `${p.host.trim()}:${p.port}`,
    host: p.host.trim(),
    port: p.port,
    username: p.username.trim(),
    password: p.protocol === 'socks4' ? '' : (p.password || null),
    enabled: true,
    timeout_ms: p.timeout_ms,
    retry_interval_ms: p.retry_interval_ms,
    max_retries: p.max_retries,
  }
}

async function ensureProfile(): Promise<ProxyProfile | null> {
  const input = profileInput()
  const key = JSON.stringify(input)
  if (created.value && key === savedInput) return created.value
  try {
    const isNew = !created.value
    const profile = created.value
      ? await api.updateProfile(created.value.ID, input)
      : await api.createProfile(input)
    created.value = profile
    savedInput = key
    profiles.value = [...profiles.value.filter((p) => p.ID !== profile.ID), profile]
    proxyID.value = profile.ID
    if (isNew) store.toast('ok', t('wizard.proxy_created'))
    return created.value
  } catch (e) {
    store.toastError(e, t('toast.error'))
    return null
  }
}

watch(p, () => {
  testMsg.value = ''
  testOk.value = false
})

async function test() {
  if (busy.value || !store.connected || !validateProxy()) return
  busy.value = true
  testMsg.value = t('wizard.testing')
  testOk.value = false
  try {
    const prof = await ensureProfile()
    if (!prof) return
    const r = await api.testProfile(prof.ID)
    if (r.error) {
      testMsg.value = t('wizard.test_fail', { err: r.error })
    } else {
      testMsg.value = t('wizard.test_ok', { ms: r.latency_ms, ip: r.public_ip })
      testOk.value = true
    }
  } catch (e) {
    testMsg.value = t('wizard.test_fail', { err: parseError(e).msg })
  } finally {
    busy.value = false
  }
}

async function toStep3() {
  if (busy.value || !store.connected || !validateProxy()) return
  busy.value = true
  try {
    if (await ensureProfile()) step.value = 3
  } finally {
    busy.value = false
  }
}

// ---- step 3: app + proxy ---------------------------------------------------
const appPath = ref('')
const appIsDir = ref(false)
const proxyID = ref('')
const showProc = ref(false)
const procs = ref<ProcessInfo[]>([])
const procSearch = ref('')
const procsLoading = ref(false)

const filteredProcs = computed(() => {
  const q = procSearch.value.trim().toLowerCase()
  const list = procs.value.filter((x) => x.path)
  return q ? list.filter((x) => x.name.toLowerCase().includes(q) || x.path.toLowerCase().includes(q)) : list
})

async function loadExistingProfile() {
  if (!store.connected || created.value || busy.value || step.value !== 1) return
  // If profiles already exist (returning user), skip straight to step 3.
  try {
    const list = (await api.getProfiles()).filter((p) => p.Enabled)
    profiles.value = list
    if (list.length && !created.value) {
      created.value = list[0]
      proxyID.value = list[0].ID
      Object.assign(p, {
        protocol: list[0].Protocol || 'socks5', name: list[0].Name, host: list[0].Host, port: list[0].Port,
        username: list[0].Username, password: '', timeout_ms: list[0].TimeoutMS,
        retry_interval_ms: list[0].RetryIntervalMS, max_retries: list[0].MaxRetries,
      })
      savedInput = JSON.stringify(profileInput())
      step.value = 3
    }
  } catch {
    /* service may be down */
  }
}
onMounted(loadExistingProfile)
watch(() => store.connected, (connected) => connected && loadExistingProfile())

watch(
  () => store.pendingDrops,
  async (paths) => {
    if (!paths.length || step.value !== 3) return
    store.pendingDrops = []
    for (const path of paths) {
      const info = await api.pathInfo(path)
      if (info.exists && (info.is_dir || info.ext === '.exe')) {
        appPath.value = path
        appIsDir.value = info.is_dir
        break
      }
    }
  },
)

async function pickFile() {
  try {
    const f = await api.pickExeFile()
    if (f) {
      appPath.value = f
      appIsDir.value = false
    }
  } catch (e) {
    store.toastError(e, t('toast.error'))
  }
}

async function openProc() {
  showProc.value = true
  procsLoading.value = true
  try {
    procs.value = await api.listProcesses()
  } catch (e) {
    store.toastError(e, t('toast.error'))
  } finally {
    procsLoading.value = false
  }
}

function pickProc(pi: ProcessInfo) {
  appPath.value = pi.path
  appIsDir.value = false
  showProc.value = false
}

async function finish() {
  if (busy.value || !store.connected || !proxyID.value) return
  if (!appPath.value) {
    // Allow finishing without an app — the proxy alone is progress.
    await done()
    return
  }
  busy.value = true
  try {
    await api.createRule({
      name: exeName(appPath.value).replace(/\.exe$/i, '') || appPath.value,
      kind: appIsDir.value ? 'folder' : 'exe',
      path: appPath.value,
      action: 'proxy',
      proxy_id: proxyID.value,
      enabled: true,
      kill_switch: false,
      include_subdirs: true,
      priority: 0,
    })
    store.toast('ok', t('wizard.rule_created'))
    await store.refreshRulesCount()
    step.value = 4 // only show success after the rule was actually saved
  } catch (e) {
    store.toastError(e, t('toast.error'))
  } finally {
    busy.value = false
  }
}

async function done() {
  try {
    await store.savePrefs({ wizard_done: true })
    await router.push('/')
  } catch (e) {
    store.toastError(e, t('toast.error'))
  }
}

async function skip() {
  const ok = await store.askConfirm(
    t('wizard.skip_confirm_title'),
    t('wizard.skip_confirm_body'),
    t('common.skip'),
    false,
  )
  if (ok) await done()
}
</script>

<template>
  <div class="flex h-full items-center justify-center p-6">
    <div class="card w-full max-w-xl">
      <div class="mb-1 flex items-center justify-between">
        <h1 class="text-lg font-bold">{{ t('wizard.title') }}</h1>
        <span class="text-xs text-slate-400">{{ t('wizard.step_of', { n: Math.min(step, 3) }) }}</span>
      </div>
      <div class="mb-5 h-1.5 overflow-hidden rounded-full bg-slate-200 dark:bg-slate-800">
        <div class="h-full bg-brand-600 transition-all" :style="{ width: (Math.min(step, 3) / 3) * 100 + '%' }" />
      </div>

      <!-- step 1 -->
      <div v-if="step === 1" class="space-y-4">
        <img src="/logo.png" alt="Proxity" class="h-16 w-16 rounded-2xl" />
        <h2 class="text-2xl font-bold">{{ t('wizard.welcome_title') }}</h2>
        <p class="text-sm leading-6 text-slate-600 dark:text-slate-300">{{ t('wizard.welcome_body') }}</p>
        <div class="flex justify-between pt-2">
          <button class="text-xs text-slate-400 hover:underline" @click="skip">{{ t('common.skip') }}</button>
          <button class="btn-primary" :disabled="!store.connected" @click="step = 2">{{ t('common.next') }}</button>
        </div>
        <ServiceDownCard v-if="!store.connected" />
      </div>

      <!-- step 2 -->
      <fieldset v-else-if="step === 2" class="space-y-3" :disabled="busy">
        <h2 class="text-base font-semibold">{{ t('wizard.proxy_step_title') }}</h2>
        <p class="text-xs text-slate-400">{{ t('wizard.proxy_help') }}</p>

        <div>
          <label class="label">{{ t('wizard.name_label') }} ({{ t('common.optional') }})</label>
          <input v-model="p.name" class="input" :placeholder="t('wizard.name_ph')" />
          <p v-if="pErr.name" class="mt-1 text-xs text-red-500">{{ pErr.name }}</p>
        </div>
        <ProxyProtocolField v-model="p.protocol" />
        <div class="grid grid-cols-3 gap-3">
          <div class="col-span-2">
            <label class="label">{{ t('wizard.host_label') }}</label>
            <input v-model="p.host" class="input" :placeholder="t('wizard.host_ph')" />
            <p v-if="pErr.host" class="mt-1 text-xs text-red-500">{{ pErr.host }}</p>
          </div>
          <div>
            <label class="label">{{ t('wizard.port_label') }}</label>
            <input v-model.number="p.port" type="number" min="1" max="65535" class="input" :placeholder="t('wizard.port_ph')" />
            <p v-if="pErr.port" class="mt-1 text-xs text-red-500">{{ pErr.port }}</p>
          </div>
        </div>

        <button class="text-xs font-medium text-brand-600 dark:text-brand-100" @click="showAuth = !showAuth">
          {{ showAuth ? '▾' : '▸' }} {{ t('wizard.auth_title') }}
        </button>
        <div v-if="showAuth" class="grid grid-cols-2 gap-3 rounded-lg bg-slate-50 p-3 dark:bg-slate-800/50">
          <div>
            <label class="label">{{ t(p.protocol === 'socks4' ? 'proxies.userid_label' : 'wizard.username_label') }}</label>
            <input v-model="p.username" class="input" autocomplete="off" />
          </div>
          <div v-if="p.protocol !== 'socks4'">
            <label class="label">{{ t('wizard.password_label') }}</label>
            <input v-model="p.password" type="password" class="input" autocomplete="new-password" />
          </div>
        </div>

        <button class="text-xs font-medium text-brand-600 dark:text-brand-100" @click="showAdv = !showAdv">
          {{ showAdv ? '▾' : '▸' }} {{ t('wizard.advanced_title') }}
        </button>
        <div v-if="showAdv" class="grid grid-cols-3 gap-3 rounded-lg bg-slate-50 p-3 dark:bg-slate-800/50">
          <div>
            <label class="label">{{ t('wizard.timeout_label') }}</label>
            <input v-model.number="p.timeout_ms" type="number" min="500" class="input" />
          </div>
          <div>
            <label class="label">{{ t('wizard.retry_label') }}</label>
            <input v-model.number="p.retry_interval_ms" type="number" min="500" class="input" />
          </div>
          <div>
            <label class="label">{{ t('wizard.retries_label') }}</label>
            <input v-model.number="p.max_retries" type="number" min="0" class="input" />
          </div>
        </div>

        <p v-if="testMsg" class="text-sm" :class="testOk ? 'text-green-600 dark:text-green-400' : 'text-slate-600 dark:text-slate-300'">{{ testMsg }}</p>

        <div class="flex justify-between pt-2">
          <button class="btn-ghost" :disabled="busy" @click="step = 1">{{ t('common.back') }}</button>
          <div class="flex gap-2">
            <button class="btn-ghost" :disabled="busy || !store.connected" @click="test">{{ busy ? t('common.loading') : t('common.test') }}</button>
            <button class="btn-primary" :disabled="busy || !store.connected" @click="toStep3">{{ t('common.next') }}</button>
          </div>
        </div>
      </fieldset>

      <!-- step 3 -->
      <div v-else-if="step === 3" class="space-y-4">
        <h2 class="text-base font-semibold">{{ t('wizard.app_step_title') }}</h2>
        <p class="text-xs text-slate-400">{{ t('wizard.app_help') }} {{ t('wizard.drop_hint') }}</p>

        <div v-if="appPath" class="flex items-center gap-2 rounded-lg bg-slate-50 p-2 text-sm dark:bg-slate-800/60">
          <AppIcon v-if="!appIsDir" :path="appPath" :size="20" />
          <span v-else>📁</span>
          <span class="truncate">{{ appPath }}</span>
        </div>
        <div class="flex gap-2">
          <button class="btn-ghost" @click="openProc">{{ t('wizard.pick_running') }}</button>
          <button class="btn-ghost" @click="pickFile">{{ t('wizard.pick_file') }}</button>
        </div>

        <div>
          <label class="label">{{ t('wizard.choose_proxy') }}</label>
          <select v-model="proxyID" class="input">
            <option v-for="profile in profiles" :key="profile.ID" :value="profile.ID">{{ profile.Name }} ({{ (profile.Protocol || 'socks5').toUpperCase() }} · {{ profile.Host }}:{{ profile.Port }})</option>
          </select>
        </div>

        <div class="flex justify-between pt-2">
          <button class="btn-ghost" :disabled="busy" @click="step = 2">{{ t('common.back') }}</button>
          <button class="btn-primary" :disabled="busy || !proxyID || !store.connected" @click="finish">
            {{ busy ? t('common.loading') : t('common.finish') }}
          </button>
        </div>
      </div>

      <!-- done -->
      <div v-else class="space-y-4 text-center">
        <div class="text-5xl">✓</div>
        <h2 class="text-xl font-bold text-green-600 dark:text-green-400">{{ t('wizard.done_title') }}</h2>
        <p class="text-sm text-slate-600 dark:text-slate-300">{{ t('wizard.done_body') }}</p>
        <button class="btn-primary mx-auto" @click="done">{{ t('common.finish') }}</button>
      </div>
    </div>

    <Modal v-if="showProc" :title="t('wizard.pick_running')" wide @close="showProc = false">
      <input v-model="procSearch" class="input mb-3" :placeholder="t('rules.proc_search_ph')" autofocus />
      <div v-if="procsLoading" class="space-y-2">
        <div v-for="i in 6" :key="i" class="h-8 animate-pulse rounded bg-slate-200/60 dark:bg-slate-800/60" />
      </div>
      <div v-else-if="!filteredProcs.length" class="py-8 text-center text-sm text-slate-400">{{ t('rules.proc_empty') }}</div>
      <div v-else class="max-h-80 divide-y divide-slate-100 overflow-y-auto dark:divide-slate-800">
        <button
          v-for="pi in filteredProcs"
          :key="pi.pid"
          class="flex w-full items-center gap-3 px-2 py-1.5 text-left hover:bg-slate-50 dark:hover:bg-slate-800/50"
          @click="pickProc(pi)"
        >
          <AppIcon :path="pi.path" :size="20" />
          <div class="min-w-0">
            <div class="truncate text-sm font-medium">{{ pi.name }}</div>
            <div class="truncate text-xs text-slate-400">{{ pi.path }}</div>
          </div>
          <span class="ml-auto text-xs text-slate-400">PID {{ pi.pid }}</span>
        </button>
      </div>
    </Modal>
  </div>
</template>
