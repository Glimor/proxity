import { createRouter, createWebHashHistory } from 'vue-router'
import DashboardView from '../views/DashboardView.vue'
import ProxiesView from '../views/ProxiesView.vue'
import RulesView from '../views/RulesView.vue'
import ConnectionsView from '../views/ConnectionsView.vue'
import StatsView from '../views/StatsView.vue'
import BlockedView from '../views/BlockedView.vue'
import SettingsView from '../views/SettingsView.vue'
import WizardView from '../views/WizardView.vue'

export const router = createRouter({
  history: createWebHashHistory(),
  routes: [
    { path: '/', name: 'dashboard', component: DashboardView },
    { path: '/proxies', name: 'proxies', component: ProxiesView },
    { path: '/rules', name: 'rules', component: RulesView },
    { path: '/connections', name: 'connections', component: ConnectionsView },
    { path: '/stats', name: 'stats', component: StatsView },
    { path: '/blocked', name: 'blocked', component: BlockedView },
    { path: '/settings', name: 'settings', component: SettingsView },
    { path: '/wizard', name: 'wizard', component: WizardView },
    { path: '/:pathMatch(.*)*', redirect: '/' },
  ],
})
