<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { api, parseError } from '../api'
import { useApp } from '../stores/app'
import Toggle from '../components/Toggle.vue'
import type { ExportProfile, ProxyProfile, Settings } from '../types'

const { t } = useI18n()
const store = useApp()

const st = ref<Settings | null>(null)
const dnsProfiles = ref<ProxyProfile[]>([])
const guiAuto = ref(false)
const saving = ref(false)
const importOverwrite = ref(false)
const exportSecrets = ref(false)
const guiVer = ref('')
const svcVer = ref('')

const defaults: Settings = {
  language: 'tr',
  theme: 'system',
  kill_switch_global: false,
  lan_access: true,
  secure_dns: false,
  dns_upstream: '',
  dns_via_proxy_id: '',
  notifications_enabled: true,
  notify_blocked: true,
  notify_proxy_events: true,
  auto_start_service: false,
  auto_start_gui: false,
  start_minimized: false,
  default_action: 'direct',
  log_level: 'info',
}

onMounted(async () => {
  if (store.connected) {
    try {
      st.value = await api.getSettings()
    } catch {
      st.value = { ...defaults, language: store.prefs.language as 'tr' | 'en', theme: store.prefs.theme }
    }
  } else {
    st.value = { ...defaults, language: store.prefs.language as 'tr' | 'en', theme: store.prefs.theme, start_minimized: store.prefs.start_minimized }
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
})
watch(
  () => store.settings,
  (s) => {
    if (s && !st.value) st.value = s
  },
)

// persist pushes the whole settings object to the service (debounced by the
// caller for text fields); GUI-only bits go to gui.json/registry too.
async function persist(silent = false) {
  if (!st.value) return
  saving.value = true
  try {
    if (store.connected) st.value = await api.updateSettings(st.value)
    store.settings = st.value
    await store.savePrefs({
      language: st.value.language,
      theme: st.value.theme,
      start_minimized: st.value.start_minimized,
    })
    if (!silent) store.toast('ok', t('settings.saved'))
  } catch (e) {
    store.toastError(e, t('toast.error'))
  } finally {
    saving.value = false
  }
}

async function onLang(v: string) {
  await store.setLanguage(v as 'tr' | 'en')
  if (st.value) st.value.language = v as 'tr' | 'en'
}
async function onTheme(v: string) {
  await store.setTheme(v)
  if (st.value) st.value.theme = v
}

async function onGuiAuto(v: boolean) {
  try {
    await api.setGuiAutoStart(v)
    guiAuto.value = v
    if (st.value) {
      st.value.auto_start_gui = v
      await persist(true)
    }
  } catch {
    store.toast('err', t('service.autostart_reg_fail'))
  }
}

async function onServiceAuto(v: boolean) {
  if (!st.value) return
  if (v) {
    // Enabling = ensure the service is installed (auto-start by default).
    const s = await api.serviceStatus().catch(() => null)
    if (s && !s.installed) {
      try {
        await api.installService()
        store.toast('info', t('service.starting'))
      } catch (e) {
        if (parseError(e).code === 'uac.cancelled') store.toast('err', t('service.uac_cancelled'))
        else store.toastError(e, t('service.install_failed'))
        return
      }
    }
    st.value.auto_start_service = true
  } else {
    const ok = await store.askConfirm(t('settings.uninstall_title'), t('settings.uninstall_body'), t('settings.uninstall_service'))
    if (!ok) return
    try {
      await api.uninstallService()
      store.toast('ok', t('service.uninstalled'))
    } catch (e) {
      if (parseError(e).code === 'uac.cancelled') store.toast('err', t('service.uac_cancelled'))
      else store.toastError(e, t('toast.error'))
      return
    }
    st.value.auto_start_service = false
  }
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
  if (!st.value) return
  st.value.start_minimized = v
  await store.savePrefs({ start_minimized: v })
  await persist(true)
}

async function restoreDefaults() {
  const ok = await store.askConfirm(
    t('settings.defaults_title'),
    t('settings.defaults_body'),
    t('settings.defaults'),
  )
  if (!ok || !st.value) return
  const lang = st.value.language
  const theme = st.value.theme
  st.value = { ...defaults, language: lang, theme }
  await persist()
  store.toast('ok', t('settings.defaults_done'))
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
  <div v-if="st" class="mx-auto max-w-2xl space-y-4 p-6">
    <h1 class="text-xl font-bold">{{ t('settings.title') }}</h1>

    <!-- general -->
    <div class="card space-y-4">
      <h2 class="text-sm font-semibold uppercase tracking-wide text-slate-400">{{ t('settings.sec_general') }}</h2>
      <div class="flex items-center justify-between">
        <span class="text-sm">{{ t('settings.language') }}</span>
        <div class="flex gap-1">
          <button class="btn-ghost !px-3 !py-1 text-xs" :class="{ '!border-brand-500 !text-brand-600 dark:!text-brand-100': st.language === 'tr' }" @click="onLang('tr')">Türkçe</button>
          <button class="btn-ghost !px-3 !py-1 text-xs" :class="{ '!border-brand-500 !text-brand-600 dark:!text-brand-100': st.language === 'en' }" @click="onLang('en')">English</button>
        </div>
      </div>
      <div class="flex items-center justify-between">
        <span class="text-sm">{{ t('settings.theme') }}</span>
        <select :value="st.theme" class="input w-36" @change="onTheme(($event.target as HTMLSelectElement).value)">
          <option value="light">{{ t('settings.theme_light') }}</option>
          <option value="dark">{{ t('settings.theme_dark') }}</option>
          <option value="system">{{ t('settings.theme_system') }}</option>
        </select>
      </div>
    </div>

    <!-- startup -->
    <div class="card space-y-4">
      <h2 class="text-sm font-semibold uppercase tracking-wide text-slate-400">{{ t('settings.sec_startup') }}</h2>
      <div>
        <div class="flex items-center justify-between">
          <span class="text-sm">{{ t('settings.autostart_service') }}</span>
          <Toggle :model-value="st.auto_start_service" @update:model-value="onServiceAuto" />
        </div>
        <p class="mt-0.5 text-xs text-slate-400">{{ t('settings.autostart_service_hint') }}</p>
      </div>
      <div class="flex items-center justify-between">
        <span class="text-sm">{{ t('settings.autostart_gui') }}</span>
        <Toggle :model-value="guiAuto" @update:model-value="onGuiAuto" />
      </div>
      <div class="flex items-center justify-between">
        <span class="text-sm">{{ t('settings.start_minimized') }}</span>
        <Toggle :model-value="st.start_minimized" @update:model-value="onMinimized" />
      </div>
      <p class="text-xs text-slate-400">{{ t('settings.minimize_hint') }}</p>
    </div>

    <!-- security -->
    <div class="card space-y-4">
      <h2 class="text-sm font-semibold uppercase tracking-wide text-slate-400">{{ t('settings.sec_security') }}</h2>
      <div>
        <div class="flex items-center justify-between">
          <span class="text-sm font-medium">{{ t('settings.kill_switch') }}</span>
          <Toggle :model-value="st.kill_switch_global" @update:model-value="onKillSwitch" />
        </div>
        <p class="mt-0.5 text-xs text-slate-400">{{ t('settings.kill_switch_hint') }}</p>
      </div>
      <div>
        <div class="flex items-center justify-between">
          <span class="text-sm">{{ t('settings.lan') }}</span>
          <Toggle :model-value="st.lan_access" @update:model-value="st.lan_access = $event; persist(true)" />
        </div>
        <p class="mt-0.5 text-xs text-slate-400">{{ t('settings.lan_hint') }}</p>
      </div>
      <div>
        <div class="flex items-center justify-between">
          <span class="text-sm">{{ t('settings.secure_dns') }}</span>
          <Toggle
            :model-value="st.secure_dns"
            :disabled="!store.connected"
            @update:model-value="st.secure_dns = $event; persist()"
          />
        </div>
        <p class="mt-0.5 text-xs text-slate-400">{{ t('settings.secure_dns_hint') }}</p>
      </div>
      <div v-if="st.secure_dns" class="space-y-3 rounded-lg border border-slate-200 p-3 dark:border-slate-800">
        <div class="flex items-center justify-between gap-3">
          <span class="text-sm">{{ t('settings.dns_upstream') }}</span>
          <input
            v-model.trim="st.dns_upstream"
            class="input w-52"
            placeholder="1.1.1.1:53"
            :title="t('settings.dns_upstream_hint')"
            @change="persist()"
          />
        </div>
        <div class="flex items-center justify-between gap-3">
          <span class="text-sm">{{ t('settings.dns_proxy') }}</span>
          <select v-model="st.dns_via_proxy_id" class="input w-52" @change="persist()">
            <option value="">{{ t('settings.dns_proxy_auto') }}</option>
            <option v-for="p in dnsProfiles" :key="p.ID" :value="p.ID">{{ p.Name }}</option>
          </select>
        </div>
      </div>
      <div>
        <span class="text-sm">{{ t('settings.default_action') }}</span>
        <div class="mt-1 flex gap-2">
          <button
            v-for="a in (['direct', 'block'] as const)"
            :key="a"
            class="btn-ghost flex-1 !py-1.5 text-xs"
            :class="{ '!border-brand-500 !text-brand-600 dark:!text-brand-100': st.default_action === a }"
            @click="st.default_action = a; persist()"
          >
            {{ t('ruleAction.' + a) }}
          </button>
        </div>
      </div>
    </div>

    <!-- notifications -->
    <div class="card space-y-4">
      <h2 class="text-sm font-semibold uppercase tracking-wide text-slate-400">{{ t('settings.sec_notif') }}</h2>
      <div class="flex items-center justify-between">
        <span class="text-sm">{{ t('settings.notif') }}</span>
        <Toggle :model-value="st.notifications_enabled" @update:model-value="st.notifications_enabled = $event; persist(true)" />
      </div>
      <div class="flex items-center justify-between" :class="{ 'opacity-40': !st.notifications_enabled }">
        <span class="text-sm">{{ t('settings.notif_blocked') }}</span>
        <Toggle :model-value="st.notify_blocked" :disabled="!st.notifications_enabled" @update:model-value="st.notify_blocked = $event; persist(true)" />
      </div>
      <div class="flex items-center justify-between" :class="{ 'opacity-40': !st.notifications_enabled }">
        <span class="text-sm">{{ t('settings.notif_proxy') }}</span>
        <Toggle :model-value="st.notify_proxy_events" :disabled="!st.notifications_enabled" @update:model-value="st.notify_proxy_events = $event; persist(true)" />
      </div>
    </div>

    <!-- data -->
    <div class="card space-y-4">
      <h2 class="text-sm font-semibold uppercase tracking-wide text-slate-400">{{ t('settings.sec_data') }}</h2>
      <div class="flex items-center justify-between">
        <span class="text-sm">{{ t('settings.export_profiles') }}</span>
        <div class="flex items-center gap-2">
          <label class="flex items-center gap-1 text-xs text-slate-400">
            <input v-model="exportSecrets" type="checkbox" /> {{ t('settings.export_with_secrets') }}
          </label>
          <button class="btn-ghost !py-1.5 text-xs" :disabled="!store.connected" @click="exportProfiles">{{ t('common.export') }}</button>
        </div>
      </div>
      <div class="flex items-center justify-between">
        <span class="text-sm">{{ t('settings.import_profiles') }}</span>
        <div class="flex items-center gap-2">
          <label class="flex items-center gap-1 text-xs text-slate-400">
            <input v-model="importOverwrite" type="checkbox" /> {{ t('settings.import_overwrite') }}
          </label>
          <button class="btn-ghost !py-1.5 text-xs" :disabled="!store.connected" @click="importProfiles">{{ t('common.import') }}</button>
        </div>
      </div>
    </div>

    <!-- advanced -->
    <div class="card space-y-4">
      <h2 class="text-sm font-semibold uppercase tracking-wide text-slate-400">{{ t('settings.sec_adv') }}</h2>
      <div class="flex items-center justify-between">
        <span class="text-sm">{{ t('settings.log_level') }}</span>
        <select v-model="st.log_level" class="input w-36" @change="persist()">
          <option value="debug">debug</option>
          <option value="info">info</option>
          <option value="warn">warn</option>
          <option value="error">error</option>
        </select>
      </div>
      <div class="flex items-center justify-between">
        <span class="text-sm text-red-500">{{ t('settings.defaults') }}</span>
        <button class="btn-ghost !py-1.5 text-xs text-red-500" @click="restoreDefaults">{{ t('settings.defaults') }}</button>
      </div>
    </div>

    <div class="pb-6">
      <button class="btn-ghost text-xs text-slate-400" @click="quitApp">{{ t('settings.quit_app') }}</button>
    </div>

    <!-- about / version -->
    <div class="pb-6 text-center text-xs text-slate-400">
      <p>
        Proxity v{{ guiVer || 'dev' }}
        <template v-if="svcVer">· {{ t('settings.svc_version', { v: svcVer }) }}</template>
      </p>
      <p v-if="svcVer && guiVer && svcVer !== guiVer" class="mt-1 text-amber-500">
        {{ t('settings.version_mismatch') }}
      </p>
      <p class="mt-1">GPL-3.0 · WinDivert (LGPL)</p>
    </div>
  </div>
</template>
