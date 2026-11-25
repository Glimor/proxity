<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '../api'
import { useApp } from '../stores/app'
import ServiceDownCard from '../components/ServiceDownCard.vue'
import Toggle from '../components/Toggle.vue'
import type { ExportProfile, ProxyProfile, Settings } from '../types'

const { t } = useI18n()
const store = useApp()

const st = ref<Settings | null>(null)
const dnsProfiles = ref<ProxyProfile[]>([])
const guiAuto = ref(false)
const updatingGuiAuto = ref(false)
const loadFailed = ref(false)
const saving = ref(false)
const importOverwrite = ref(false)
const exportSecrets = ref(false)
const guiVer = ref('')
const svcVer = ref('')

const defaults: Settings = {
  language: 'en',
  theme: 'system',
  kill_switch_global: false,
  lan_access: true,
  secure_dns: false,
  dns_upstream: '',
  dns_via_proxy_id: '',
  notifications_enabled: true,
  notify_blocked: true,
  notify_proxy_events: true,
  auto_start_service: true,
  auto_start_gui: false,
  start_minimized: false,
  default_action: 'direct',
  log_level: 'info',
}

async function refreshSettings() {
  if (store.connected) {
    loadFailed.value = false
    try {
      st.value = await api.getSettings()
      store.settings = { ...st.value }
    } catch (e) {
      loadFailed.value = !st.value
      store.toastError(e, t('toast.error'))
    }
  }
  try {
    guiAuto.value = await api.guiAutoStart()
  } catch {
    guiAuto.value = store.settings?.auto_start_gui ?? false
  }
  if (store.connected) {
    try {
      dnsProfiles.value = (await api.getProfiles()).filter((p) => p.Enabled)
    } catch {
      /* picker stays on auto */
    }
  }
  try {
    guiVer.value = await api.guiVersion()
  } catch {
    /* dev harness without bindings */
  }
  try {
    svcVer.value = await api.serviceVersion()
  } catch {
    /* service unreachable */
  }
}
onMounted(refreshSettings)
watch(
  () => store.settings,
  (s) => {
    if (s && !saving.value) st.value = { ...s }
  },
)
watch(() => store.connected, (connected) => { if (connected) void refreshSettings() })

// persist pushes the whole settings object to the service (debounced by the
// caller for text fields); GUI-only bits go to gui.json/registry too.
let persistQueue = Promise.resolve()
function persist(silent = false): Promise<void> {
  persistQueue = persistQueue.then(() => persistNow(silent))
  return persistQueue
}
async function persistNow(silent: boolean) {
  if (!st.value) return
  saving.value = true
  try {
    const input = { ...st.value }
    const saved = store.connected ? await api.updateSettings(input) : input
    store.settings = { ...saved }
    await store.savePrefs({
      language: saved.language,
      theme: saved.theme,
      start_minimized: saved.start_minimized,
    })
    if (!silent) store.toast('ok', t('settings.saved'))
  } catch (e) {
    if (store.settings) st.value = { ...store.settings }
    store.toastError(e, t('toast.error'))
  } finally {
    saving.value = false
  }
}

async function onTheme(v: string) {
  if (st.value) {
    st.value.theme = v
    await persist(true)
  }
}

async function onGuiAuto(v: boolean) {
  if (saving.value || updatingGuiAuto.value) return
  updatingGuiAuto.value = true
  try {
    await api.setGuiAutoStart(v)
    guiAuto.value = v
    if (st.value) {
      st.value.auto_start_gui = v
      await persist(true)
    }
  } catch {
    store.toast('err', t('service.autostart_reg_fail'))
  } finally {
    updatingGuiAuto.value = false
  }
}

async function onServiceAuto(v: boolean) {
  if (!st.value || saving.value || updatingGuiAuto.value) return
  st.value.auto_start_service = v
  await persist()
}

async function onKillSwitch(v: boolean) {
  if (!st.value) return
  if (v) {
    const ok = await store.askConfirm(
      t('settings.kill_switch_title'),
      t('settings.kill_switch_body'),
      t('common.enable'),
    )
    if (!ok) return
  }
  st.value.kill_switch_global = v
  await persist()
}

async function onMinimized(v: boolean) {
  if (!st.value || saving.value || updatingGuiAuto.value) return
  st.value.start_minimized = v
  await persist(true)
}

async function restoreDefaults() {
  const ok = await store.askConfirm(
    t('settings.defaults_title'),
    t('settings.defaults_body'),
    t('settings.defaults'),
  )
  if (!ok || !st.value) return
  const { theme, auto_start_service } = st.value
  st.value = { ...defaults, theme, auto_start_service }
  await persist()
}

async function exportProfiles() {
  try {
    const list = await api.exportProfiles(exportSecrets.value)
    const path = await api.pickSaveFile('proxity-profiles.json', '*.json')
    if (!path) return
    await api.writeTextFile(path, JSON.stringify({ profiles: list }, null, 2))
    store.toast('ok', t('settings.export_saved'))
  } catch (e) {
    store.toastError(e, t('toast.error'))
  }
}

async function importProfiles() {
  try {
    const path = await api.pickJSONFile()
    if (!path) return
    const text = await api.readTextFile(path)
    const data = JSON.parse(text)
    const list: ExportProfile[] = Array.isArray(data) ? data : data.profiles ?? []
    if (!list.length) {
      store.toast('err', t('settings.pick_import'))
      return
    }
    const res = await api.importProfiles(list, importOverwrite.value)
    store.toast(
      'ok',
      t('settings.import_done', {
        created: res.created,
        updated: res.updated,
        skipped: res.skipped?.length ? t('settings.import_skipped', { n: res.skipped.length }) : '',
      }),
    )
  } catch (e) {
    store.toastError(e, t('toast.error'))
  }
}

async function quitApp() {
  const ok = await store.askConfirm(t('settings.quit_title'), t('settings.quit_body'), t('settings.quit_app'))
  if (ok) await api.quit()
}
</script>

<template>
  <div class="page max-w-3xl">
    <header class="page-header"><h1 class="page-title">{{ t('settings.title') }}</h1></header>
    <ServiceDownCard v-if="!store.connected" />
    <template v-else-if="st">

    <section class="card space-y-4 p-5 sm:p-6" aria-labelledby="startup-heading">
      <h2 id="startup-heading" class="section-title">{{ t('settings.sec_startup') }}</h2>
      <div>
        <div class="flex items-center justify-between gap-3">
          <span id="service-autostart-label" class="text-sm font-semibold">{{ t('settings.autostart_service') }}</span>
          <Toggle :model-value="st.auto_start_service" :aria-disabled="saving || updatingGuiAuto" aria-labelledby="service-autostart-label" aria-describedby="service-autostart-hint" @update:model-value="onServiceAuto" />
        </div>
        <p id="service-autostart-hint" class="muted max-w-prose">{{ t('settings.autostart_service_hint') }}</p>
      </div>
      <div class="border-t border-slate-200 pt-3 dark:border-slate-800">
        <div class="flex items-center justify-between gap-3">
          <span id="gui-autostart-label" class="text-sm font-semibold">{{ t('settings.autostart_gui') }}</span>
          <Toggle :model-value="guiAuto" aria-labelledby="gui-autostart-label" aria-describedby="gui-autostart-hint" :aria-disabled="saving || updatingGuiAuto" @update:model-value="onGuiAuto" />
        </div>
        <p id="gui-autostart-hint" class="muted">{{ t('settings.autostart_gui_hint') }}</p>
      </div>
      <div>
        <div class="flex items-center justify-between gap-3">
          <span id="minimized-label" class="text-sm">{{ t('settings.start_minimized') }}</span>
          <Toggle :model-value="st.start_minimized" aria-labelledby="minimized-label" aria-describedby="minimized-hint" :aria-disabled="saving || updatingGuiAuto" @update:model-value="onMinimized" />
        </div>
        <p id="minimized-hint" class="muted">{{ t('settings.minimize_hint') }}</p>
      </div>
    </section>

    <div class="card space-y-4 p-5 sm:p-6">
      <h2 class="section-title">{{ t('settings.sec_general') }}</h2>
      <div class="settings-row">
        <label for="settings-theme" class="text-sm">{{ t('settings.theme') }}</label>
        <select id="settings-theme" :value="st.theme" class="input w-36" @change="onTheme(($event.target as HTMLSelectElement).value)">
          <option value="light">{{ t('settings.theme_light') }}</option>
          <option value="dark">{{ t('settings.theme_dark') }}</option>
          <option value="system">{{ t('settings.theme_system') }}</option>
        </select>
      </div>
    </div>
    <!-- security -->
    <div class="card space-y-4">
      <h2 class="section-title">{{ t('settings.sec_security') }}</h2>
      <div>
        <div class="flex items-center justify-between">
          <span class="text-sm font-medium">{{ t('settings.kill_switch') }}</span>
          <Toggle :model-value="st.kill_switch_global" :aria-label="t('settings.kill_switch')" @update:model-value="onKillSwitch" />
        </div>
        <p class="muted">{{ t('settings.kill_switch_hint') }}</p>
      </div>
      <div>
        <div class="flex items-center justify-between">
          <span class="text-sm">{{ t('settings.lan') }}</span>
          <Toggle :model-value="st.lan_access" :aria-label="t('settings.lan')" @update:model-value="st.lan_access = $event; persist(true)" />
        </div>
        <p class="muted">{{ t('settings.lan_hint') }}</p>
      </div>
      <div>
        <div class="flex items-center justify-between">
          <span class="text-sm">{{ t('settings.secure_dns') }}</span>
          <Toggle
            :model-value="st.secure_dns"
            :aria-label="t('settings.secure_dns')"
            :disabled="!store.connected"
            @update:model-value="st.secure_dns = $event; persist()"
          />
        </div>
        <p class="muted">{{ t('settings.secure_dns_hint') }}</p>
      </div>
      <div v-if="st.secure_dns" class="space-y-3 rounded-lg border border-slate-200 p-3 dark:border-slate-800">
        <div class="settings-row">
          <label for="settings-dns-upstream" class="text-sm">{{ t('settings.dns_upstream') }}</label>
          <input
            id="settings-dns-upstream"
            v-model.trim="st.dns_upstream"
            class="input w-52"
            placeholder="1.1.1.1:53"
            :title="t('settings.dns_upstream_hint')"
            @change="persist()"
          />
        </div>
        <div class="settings-row">
          <label for="settings-dns-proxy" class="text-sm">{{ t('settings.dns_proxy') }}</label>
          <select id="settings-dns-proxy" v-model="st.dns_via_proxy_id" class="input w-52" @change="persist()">
            <option value="">{{ t('settings.dns_proxy_auto') }}</option>
            <option v-for="p in dnsProfiles" :key="p.ID" :value="p.ID">{{ p.Name }}</option>
          </select>
        </div>
      </div>
      <div>
        <span class="text-sm">{{ t('settings.default_action') }}</span>
        <p class="muted mt-1">{{ t('settings.default_action_hint') }}</p>
        <div class="mt-1 flex gap-2">
          <button
            v-for="a in (['direct', 'block'] as const)"
            :key="a"
            class="btn-ghost flex-1 !py-1.5 text-xs"
            :aria-pressed="st.default_action === a"
            :class="{ '!bg-brand-50 !text-brand-700 dark:!bg-brand-600/20 dark:!text-brand-100': st.default_action === a }"
            @click="st.default_action = a; persist()"
          >
            {{ t('ruleAction.' + a) }}
          </button>
        </div>
      </div>
    </div>

    <!-- notifications -->
    <div class="card space-y-4">
      <h2 class="section-title">{{ t('settings.sec_notif') }}</h2>
      <div class="flex items-center justify-between">
        <span class="text-sm">{{ t('settings.notif') }}</span>
        <Toggle :model-value="st.notifications_enabled" :aria-label="t('settings.notif')" @update:model-value="st.notifications_enabled = $event; persist(true)" />
      </div>
      <div class="flex items-center justify-between">
        <span class="text-sm">{{ t('settings.notif_blocked') }}</span>
        <Toggle :model-value="st.notify_blocked" :aria-label="t('settings.notif_blocked')" :disabled="!st.notifications_enabled" @update:model-value="st.notify_blocked = $event; persist(true)" />
      </div>
      <div class="flex items-center justify-between">
        <span class="text-sm">{{ t('settings.notif_proxy') }}</span>
        <Toggle :model-value="st.notify_proxy_events" :aria-label="t('settings.notif_proxy')" :disabled="!st.notifications_enabled" @update:model-value="st.notify_proxy_events = $event; persist(true)" />
      </div>
    </div>

    <!-- data -->
    <div class="card space-y-4">
      <h2 class="section-title">{{ t('settings.sec_data') }}</h2>
      <div class="settings-row">
        <span class="text-sm">{{ t('settings.export_profiles') }}</span>
        <div class="flex items-center gap-2">
          <label class="muted flex min-h-11 items-center gap-2">
            <input v-model="exportSecrets" type="checkbox" /> {{ t('settings.export_with_secrets') }}
          </label>
          <button class="btn-ghost !py-1.5 text-xs" :disabled="!store.connected" @click="exportProfiles">{{ t('common.export') }}</button>
        </div>
      </div>
      <div class="settings-row">
        <span class="text-sm">{{ t('settings.import_profiles') }}</span>
        <div class="flex items-center gap-2">
          <label class="muted flex min-h-11 items-center gap-2">
            <input v-model="importOverwrite" type="checkbox" /> {{ t('settings.import_overwrite') }}
          </label>
          <button class="btn-ghost !py-1.5 text-xs" :disabled="!store.connected" @click="importProfiles">{{ t('common.import') }}</button>
        </div>
      </div>
    </div>

    <!-- advanced -->
    <div class="card space-y-4">
      <h2 class="section-title">{{ t('settings.sec_adv') }}</h2>
      <div class="settings-row">
        <label for="settings-log-level" class="text-sm">{{ t('settings.log_level') }}</label>
        <select id="settings-log-level" v-model="st.log_level" class="input w-36" @change="persist()">
          <option value="debug">debug</option>
          <option value="info">info</option>
          <option value="warn">warn</option>
          <option value="error">error</option>
        </select>
      </div>
      <div class="flex items-center justify-between">
        <span class="text-sm">{{ t('settings.defaults') }}</span>
        <button class="btn-ghost text-red-700 dark:text-red-300" @click="restoreDefaults">{{ t('settings.defaults') }}</button>
      </div>
    </div>

    <div class="pb-6">
      <button class="btn-ghost" @click="quitApp">{{ t('settings.quit_app') }}</button>
    </div>

    <!-- about / version -->
    <div class="muted pb-6 text-center">
      <p>
        Proxity v{{ guiVer || 'dev' }}
        <template v-if="svcVer">· {{ t('settings.svc_version', { v: svcVer }) }}</template>
      </p>
      <p v-if="svcVer && guiVer && svcVer !== guiVer" class="mt-1 text-amber-800 dark:text-amber-300">
        {{ t('settings.version_mismatch') }}
      </p>
      <p class="mt-1">GPL-3.0 · WinDivert (LGPL)</p>
    </div>
    </template>
    <div v-else-if="loadFailed" class="card space-y-3">
      <p role="alert">{{ t('settings.load_error') }}</p>
      <button class="btn-ghost" @click="refreshSettings">{{ t('common.retry') }}</button>
    </div>
    <p v-else class="card muted" role="status">{{ t('common.loading') }}</p>
  </div>
</template>
