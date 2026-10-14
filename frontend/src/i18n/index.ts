import { createI18n } from 'vue-i18n'
import tr from './tr.json'
import en from './en.json'

export const i18n = createI18n({
  legacy: false,
  locale: 'tr',
  fallbackLocale: 'en',
  messages: { tr, en },
})

export function setLocale(lang: string) {
  i18n.global.locale.value = lang === 'en' ? 'en' : 'tr'
  document.documentElement.lang = lang === 'en' ? 'en' : 'tr'
}
