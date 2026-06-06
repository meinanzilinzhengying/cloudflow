<template>
  <div class="flex h-screen bg-dark-900">
    <Sidebar 
      :modules="enabledModules" 
      :activeModule="activeModule" 
      @select="handleModuleSelect" 
    />
    <div class="flex-1 flex flex-col overflow-hidden">
      <Header :title="activeModuleLabel" />
      <main class="flex-1 overflow-auto p-6 bg-dark-900">
        <component :is="currentModuleComponent" />
      </main>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import Header from './components/layout/Header.vue'
import Sidebar from './components/layout/Sidebar.vue'
import Dashboard from './components/modules/Dashboard.vue'
import ProbeManage from './components/modules/ProbeManage.vue'
import HealthCheck from './components/modules/HealthCheck.vue'
import ConfigManage from './components/modules/ConfigManage.vue'
import SelfAlerts from './components/modules/SelfAlerts.vue'
import ExternalTools from './components/modules/ExternalTools.vue'
import featuresConfig from './config/features.json'

const features = ref(featuresConfig)
const activeModule = ref('dashboard')

const moduleComponents = {
  dashboard: Dashboard,
  probe: ProbeManage,
  health: HealthCheck,
  config: ConfigManage,
  selfAlerts: SelfAlerts,
  externalTools: ExternalTools
}

const enabledModules = computed(() => {
  const mods = features.value.modules
  return Object.entries(mods)
    .filter(([key, val]) => val.enabled)
    .map(([key, val]) => ({
      key,
      label: val.label,
      icon: getModuleIcon(key)
    }))
})

const activeModuleLabel = computed(() => {
  return features.value.modules[activeModule.value]?.label || '平台概览'
})

const currentModuleComponent = computed(() => {
  return moduleComponents[activeModule.value] || Dashboard
})

function getModuleIcon(key) {
  const icons = {
    dashboard: 'LayoutDashboard',
    probe: 'Cpu',
    health: 'HeartPulse',
    config: 'Settings',
    selfAlerts: 'Bell',
    externalTools: 'ExternalLink'
  }
  return icons[key] || 'Circle'
}

function handleModuleSelect(key) {
  if (features.value.modules[key]?.enabled) {
    activeModule.value = key
  }
}

onMounted(() => {
  const firstEnabled = enabledModules.value[0]
  if (firstEnabled) {
    activeModule.value = firstEnabled.key
  }
})
</script>
