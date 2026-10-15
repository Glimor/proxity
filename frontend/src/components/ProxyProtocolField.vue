<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { ProxyProtocol } from '../types'

defineProps<{ modelValue: ProxyProtocol }>()
defineEmits<{ 'update:modelValue': [value: ProxyProtocol] }>()
const { t } = useI18n()
const protocols: ProxyProtocol[] = ['http', 'https', 'socks4', 'socks5']
</script>

<template>
  <div>
    <label class="label" for="proxy-protocol">{{ t('proxies.protocol_label') }}</label>
    <select id="proxy-protocol" class="input" :value="modelValue"
      @change="$emit('update:modelValue', ($event.target as HTMLSelectElement).value as ProxyProtocol)">
      <option v-for="protocol in protocols" :key="protocol" :value="protocol">{{ protocol.toUpperCase() }}</option>
    </select>
    <p class="mt-1 text-xs text-slate-400">{{ t(`proxies.protocol_help.${modelValue}`) }}</p>
  </div>
</template>
