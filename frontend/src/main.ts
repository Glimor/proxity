import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import { i18n } from './i18n'
import { router } from './router'
import { useApp } from './stores/app'
import { api } from './api'
import './style.css'

const app = createApp(App)
const pinia = createPinia()
app.use(pinia)
app.use(i18n)
app.use(router)

const store = useApp()
store.init().then(async () => {
  // First-run: land on the wizard when it was never finished and there are
  // no profiles yet.
  if (!store.prefs.wizard_done) {
    let hasProfiles = false
    if (store.connected) {
      try {
        hasProfiles = (await api.getProfiles()).length > 0
      } catch {
        hasProfiles = true // can't tell; don't force the wizard
      }
    }
    if (!hasProfiles) router.push('/wizard')
  }
})

app.mount('#app')
