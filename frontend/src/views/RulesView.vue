<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '../api'
import { exeName } from '../format'
import { useApp } from '../stores/app'
import AppIcon from '../components/AppIcon.vue'
import ServiceDownCard from '../components/ServiceDownCard.vue'
import Modal from '../components/Modal.vue'
import Toggle from '../components/Toggle.vue'
import EmptyState from '../components/EmptyState.vue'
import RoutePreview from '../components/RoutePreview.vue'
import RulesTable from '../components/RulesTable.vue'
import UiIcon from '../components/UiIcon.vue'
import type {
  ProcessInfo,
  ProxyProfile,
  Rule,
  RuleAction,
  RuleInput,
  RuleKind,
  RuleResult,
} from '../types'

const { t } = useI18n()
const store = useApp()

const rules = ref<Rule[]>([])
const profiles = ref<ProxyProfile[]>([])
const loading = ref(true)
const loadFailed = ref(false)

const showModal = ref(false)
const editingID = ref<string | null>(null)
const form = reactive({
  name: '',
  kind: 'exe' as RuleKind,
  path: '',
  action: 'proxy' as RuleAction,
  proxy_id: '',
  enabled: true,
  kill_switch: false,
  include_subdirs: true,
  priority: 0,
})
const formErrors = reactive({ name: '', path: '', proxy: '' })
const saving = ref(false)
const showAdvanced = ref(false)

const showProcModal = ref(false)
const procs = ref<ProcessInfo[]>([])
const procSearch = ref('')
const procsLoading = ref(false)

const proxyName = computed(() => {
  const m: Record<string, string> = {}
  for (const p of profiles.value) m[p.ID] = p.Name
  return m
})

const filteredProcs = computed(() => {
  const q = procSearch.value.trim().toLowerCase()
  const list = procs.value.filter((p) => p.path)
  if (!q) return list
  return list.filter(
    (p) => p.name.toLowerCase().includes(q) || p.path.toLowerCase().includes(q),
  )
})

async function refresh() {
  if (!store.connected) {
    loading.value = false
    return
  }
  loading.value = true
  loadFailed.value = false
  try {
    ;[rules.value, profiles.value] = await Promise.all([api.getRules(), api.getProfiles()])
  } catch (e) {
    loadFailed.value = true
    store.toastError(e, t('toast.error'))
  } finally {
    loading.value = false
  }
}
onMounted(refresh)
watch(() => store.connected, (v) => v && refresh())

// ---------------- add flows ----------------

function resetForm() {
  editingID.value = null
  Object.assign(form, {
    name: '',
    kind: 'exe',
    path: '',
    action: 'proxy',
    proxy_id: profiles.value[0]?.ID ?? '',
    enabled: true,
    kill_switch: false,
    include_subdirs: true,
    priority: 0,
  })
  formErrors.name = formErrors.path = formErrors.proxy = ''
  showAdvanced.value = false
}

function openAdd() {
  resetForm()
  showModal.value = true
}

function openEdit(r: Rule) {
  editingID.value = r.ID
  Object.assign(form, {
    name: r.Name,
    kind: r.Kind,
    path: r.Path,
    action: r.Action,
    proxy_id: r.ProxyID || profiles.value[0]?.ID || '',
    enabled: r.Enabled,
    kill_switch: r.KillSwitch,
    include_subdirs: r.IncludeSubdirs,
    priority: r.Priority,
  })
  formErrors.name = formErrors.path = formErrors.proxy = ''
  showAdvanced.value = false
  showModal.value = true
}

async function pickExe() {
  try {
    const p = await api.pickExeFile()
    if (p) {
      form.path = p
      form.kind = 'exe'
      if (!form.name) form.name = exeName(p).replace(/\.exe$/i, '')
    }
  } catch (e) {
    store.toastError(e, t('toast.error'))
  }
}

async function pickFolder() {
  try {
    const p = await api.pickFolder()
    if (p) {
      form.path = p
      form.kind = 'folder'
      if (!form.name) form.name = exeName(p)
    }
  } catch (e) {
    store.toastError(e, t('toast.error'))
  }
}

async function openProcPicker() {
  showProcModal.value = true
  procsLoading.value = true
  try {
    procs.value = await api.listProcesses()
  } catch (e) {
    store.toastError(e, t('toast.error'))
  } finally {
    procsLoading.value = false
  }
}

function pickProc(p: ProcessInfo) {
  form.path = p.path
  form.kind = 'exe'
  if (!form.name) form.name = p.name.replace(/\.exe$/i, '')
  showProcModal.value = false
}

// drag & drop from the OS (wails file-drop → store.pendingDrops)
watch(
  () => store.pendingDrops,
  async (paths) => {
    if (!paths.length) return
    store.pendingDrops = []
    for (const path of paths) {
      const info = await api.pathInfo(path)
      if (!info.exists) continue
      resetForm()
      if (info.is_dir) {
        form.kind = 'folder'
        form.path = path
        if (!form.name) form.name = info.name
      } else if (info.ext === '.exe') {
        form.kind = 'exe'
        form.path = path
        if (!form.name) form.name = info.name.replace(/\.exe$/i, '')
      } else {
        continue // unsupported file type
      }
      showModal.value = true
      break // handle the first usable drop; one modal at a time
    }
  },
)

// ---------------- save ----------------

function toInput(): RuleInput {
  return {
    name: form.name.trim() || exeName(form.path),
    kind: form.kind,
    path: form.path,
    action: form.action,
    proxy_id: form.action === 'proxy' ? form.proxy_id : '',
    enabled: form.enabled,
    kill_switch: form.kill_switch,
    include_subdirs: form.include_subdirs,
    priority: form.priority,
  }
}

function validate(): boolean {
  formErrors.name = form.name.trim() || form.path ? '' : t('rules.name_required')
  formErrors.path = form.path ? '' : t('rules.path_required')
  formErrors.proxy = form.action === 'proxy' && !form.proxy_id ? t('rules.proxy_required') : ''
  return !formErrors.name && !formErrors.path && !formErrors.proxy
}

async function doSave(): Promise<RuleResult | null> {
  if (editingID.value) return api.updateRule(editingID.value, toInput())
  return api.createRule(toInput())
}

async function save() {
  if (saving.value || !store.connected || !validate()) return
  saving.value = true
  try {
    let res = await doSave()
    if (res?.conflicts?.length) {
      const names = res.conflicts.map((r) => r.Name).join(', ')
      const ok = await store.askConfirm(
        t('rules.conflict_title'),
        t('rules.conflict_body', { name: names }),
        t('common.save'),
        true,
      )
      if (!ok) {
        // The service already persisted it — remove it to honor the cancel.
        if (res.rule?.ID && !editingID.value) await api.deleteRule(res.rule.ID)
        if (editingID.value) {
          const original = rules.value.find((r) => r.ID === editingID.value)
          if (original) await api.updateRule(original.ID, {
            name: original.Name, kind: original.Kind, path: original.Path,
            action: original.Action, proxy_id: original.ProxyID, enabled: original.Enabled,
            kill_switch: original.KillSwitch, include_subdirs: original.IncludeSubdirs,
            priority: original.Priority,
          })
        }
        await refresh()
        saving.value = false
        return
      }
    }
    for (const w of res?.warnings ?? []) {
      store.toast('info', t('rules.warnings_title'), w)
    }
    store.toast('ok', t('rules.saved'))
    showModal.value = false
    await refresh()
  } catch (e) {
    store.toastError(e, t('toast.error'))
  } finally {
    saving.value = false
  }
}

async function del(r: Rule) {
  const ok = await store.askConfirm(
    t('rules.delete_title'),
    t('rules.delete_body', { name: r.Name }),
    t('common.delete'),
  )
  if (!ok) return
  try {
    await api.deleteRule(r.ID)
    store.toast('ok', t('rules.deleted'))
    await refresh()
  } catch (e) {
    store.toastError(e, t('toast.error'))
  }
}

async function toggleEnabled(r: Rule) {
  try {
    await api.setRuleEnabled(r.ID, !r.Enabled)
    r.Enabled = !r.Enabled
  } catch (e) {
    store.toastError(e, t('toast.error'))
  }
}

async function toggleKS(r: Rule) {
  try {
    await api.setRuleKillSwitch(r.ID, !r.KillSwitch)
    r.KillSwitch = !r.KillSwitch
  } catch (e) {
    store.toastError(e, t('toast.error'))
  }
}

</script>

<template>
  <div class="page">
    <header class="page-header">
      <div>
        <h1 class="page-title">{{ t('rules.title') }}</h1>
        <p class="muted mt-2">{{ t('rules.new_conn_note') }}</p>
      </div>
      <div class="flex flex-wrap gap-2">
        <RoutePreview :connected="store.connected" />
        <button v-if="store.connected" class="btn-primary" @click="openAdd">{{ t('rules.add') }}</button>
      </div>
    </header>

    <ServiceDownCard v-if="!store.connected" />
    <p v-else-if="loading" class="card muted" role="status">{{ t('common.loading') }}</p>
    <div v-else-if="loadFailed" class="card space-y-3">
      <p role="alert">{{ t('rules.load_error') }}</p>
      <button class="btn-ghost" @click="refresh">{{ t('common.retry') }}</button>
    </div>
    <EmptyState v-else-if="!rules.length" :title="t('rules.empty_title')" :body="t('rules.empty_body')" />
    <RulesTable v-else :rules="rules" :proxy-names="proxyName" @edit="openEdit" @delete="del" @enabled="toggleEnabled" @kill-switch="toggleKS" />

    <!-- add / edit modal -->
    <Modal v-if="showModal" :title="editingID ? t('rules.edit_title') : t('rules.add_title')" @close="showModal = false">
      <div class="space-y-3">
        <div>
          <label class="label">{{ t('rules.rule_name_label') }}</label>
          <input v-model="form.name" class="input" :placeholder="t('common.optional')" />
        </div>

        <div>
          <label class="label">{{ t('rules.app_label') }} <span class="text-red-500">*</span></label>
          <div v-if="form.path" class="mb-2 flex items-center gap-2 rounded-lg bg-slate-50 p-2 text-sm dark:bg-slate-800/60">
            <AppIcon v-if="form.kind === 'exe'" :path="form.path" :size="20" />
            <UiIcon v-else name="folder" />
            <span class="truncate">{{ form.path }}</span>
          </div>
          <div class="flex flex-wrap gap-2">
            <button class="btn-ghost" @click="pickExe">{{ t('rules.pick_exe') }}</button>
            <button class="btn-ghost" @click="pickFolder">{{ t('rules.pick_folder') }}</button>
            <button class="btn-ghost" @click="openProcPicker">{{ t('rules.pick_running') }}</button>
          </div>
          <p v-if="formErrors.path" class="mt-1 text-xs text-red-500">{{ formErrors.path }}</p>
          <p v-if="form.kind === 'folder' && form.path" class="mt-1 text-xs text-slate-400">{{ t('rules.folder_rule_hint') }}</p>
        </div>

        <div>
          <label class="label">{{ t('rules.action_label') }}</label>
          <div class="flex gap-2">
            <button
              v-for="a in (['proxy', 'direct', 'block'] as const)"
              :key="a"
              class="btn flex-1 border"
              :aria-pressed="form.action === a"
              :class="form.action === a ? 'bg-brand-50 text-brand-700 dark:bg-brand-600/20 dark:text-brand-100' : 'text-slate-600 dark:text-slate-300'"
              @click="form.action = a"
            >
              {{ t('ruleAction.' + a) }}
            </button>
          </div>
        </div>

        <div v-if="form.action === 'proxy'">
          <label class="label">{{ t('rules.proxy_label') }} <span class="text-red-500">*</span></label>
          <select v-model="form.proxy_id" class="input">
            <option v-for="p in profiles" :key="p.ID" :value="p.ID">{{ p.Name }} ({{ p.Host }}:{{ p.Port }})</option>
          </select>
          <p v-if="formErrors.proxy" class="mt-1 text-xs text-red-500">{{ formErrors.proxy }}</p>
        </div>

        <div v-if="form.kind === 'folder'" class="flex items-center gap-2">
          <Toggle v-model="form.include_subdirs" />
          <span class="text-sm">{{ t('rules.subdirs_label') }}</span>
        </div>

        <div v-if="form.action === 'proxy'" class="rounded-lg bg-slate-50 p-3 dark:bg-slate-800/50">
          <div class="flex items-center gap-2">
            <Toggle v-model="form.kill_switch" />
            <span class="text-sm font-medium">{{ t('rules.kill_switch_label') }}</span>
          </div>
          <p class="mt-1 text-xs text-slate-500 dark:text-slate-400">{{ t('rules.kill_switch_hint') }}</p>
        </div>

        <div class="flex items-center gap-2">
          <Toggle v-model="form.enabled" />
          <span class="text-sm">{{ t('rules.enabled_label') }}</span>
        </div>

        <button class="text-xs font-medium text-brand-600 dark:text-brand-100" @click="showAdvanced = !showAdvanced">
          {{ showAdvanced ? '▾' : '▸' }} {{ t('common.advanced') }}
        </button>
        <div v-if="showAdvanced" class="rounded-lg bg-slate-50 p-3 dark:bg-slate-800/50">
          <label class="label">{{ t('rules.priority_label') }}</label>
          <input v-model.number="form.priority" type="number" class="input w-28" />
          <p class="mt-1 text-xs text-slate-400">{{ t('rules.priority_hint') }}</p>
        </div>

        <div class="flex justify-end gap-2 pt-2">
          <button class="btn-ghost" @click="showModal = false">{{ t('common.cancel') }}</button>
          <button class="btn-primary" :disabled="saving" @click="save">{{ saving ? t('common.loading') : t('common.save') }}</button>
        </div>
      </div>
    </Modal>

    <!-- running-process picker -->
    <Modal v-if="showProcModal" :title="t('rules.pick_running')" wide @close="showProcModal = false">
      <input v-model="procSearch" class="input mb-3" :placeholder="t('rules.proc_search_ph')" autofocus />
      <div v-if="procsLoading" class="space-y-2">
        <div v-for="i in 6" :key="i" class="h-8 animate-pulse rounded bg-slate-200/60 dark:bg-slate-800/60" />
      </div>
      <div v-else-if="!filteredProcs.length" class="py-8 text-center text-sm text-slate-400">{{ t('rules.proc_empty') }}</div>
      <div v-else class="max-h-80 divide-y divide-slate-100 overflow-y-auto dark:divide-slate-800">
        <button
          v-for="p in filteredProcs"
          :key="p.pid"
          class="flex w-full items-center gap-3 px-2 py-1.5 text-left hover:bg-slate-50 dark:hover:bg-slate-800/50"
          @click="pickProc(p)"
        >
          <AppIcon :path="p.path" :size="20" />
          <div class="min-w-0">
            <div class="truncate text-sm font-medium">{{ p.name }}</div>
            <div class="truncate text-xs text-slate-400">{{ p.path || t('rules.proc_no_path') }}</div>
          </div>
          <span class="ml-auto text-xs text-slate-400">PID {{ p.pid }}</span>
        </button>
      </div>
    </Modal>
  </div>
</template>
