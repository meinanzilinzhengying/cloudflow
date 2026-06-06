<template>
  <div class="min-h-screen bg-dark-900">
    <Sidebar :activeModule="activeModule" @change="handleModuleChange" />
    <div class="ml-64">
      <Header @refresh="handleRefresh" />
      <main class="p-6">
        <transition name="fade" mode="out-in">
          <component 
            :is="currentModule" 
            :key="activeModule" 
          />
        </transition>
      </main>
    </div>
    
    <div v-if="loading" class="fixed inset-0 bg-dark-900/90 flex items-center justify-center z-50">
      <div class="text-center">
        <div class="w-12 h-12 border-4 border-primary-500 border-t-transparent rounded-full animate-spin mx-auto mb-4"></div>
        <p class="text-gray-400">加载中...</p>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed } from 'vue'
import Sidebar from './components/layout/Sidebar.vue'
import Header from './components/layout/Header.vue'
import Overview from './components/modules/Overview.vue'
import Traffic from './components/modules/Traffic.vue'
import Network from './components/modules/Network.vue'
import Alerts from './components/modules/Alerts.vue'
import K8sFilter from './components/modules/K8sFilter.vue'
import Export from './components/modules/Export.vue'
import features from './config/features.json'

const activeModule = ref('overview')
const loading = ref(false)

const modules = {
  overview: Overview,
  traffic: Traffic,
  network: Network,
  alerts: Alerts,
  k8s: K8sFilter,
  export: Export
}

const enabledModules = computed(() => {
  const enabled = {}
  Object.entries(modules).forEach(([key, component]) => {
    const featureKey = key === 'alerts' ? 'alerts' : key
    if (features.modules[featureKey]?.enabled !== false) {
      enabled[key] = component
    }
  })
  return enabled
})

const currentModule = computed(() => {
  if (!enabledModules.value[activeModule.value]) {
    return enabledModules.value[Object.keys(enabledModules.value)[0]] || null
  }
  return enabledModules.value[activeModule.value]
})

const handleModuleChange = (module) => {
  if (enabledModules.value[module]) {
    activeModule.value = module
  }
}

const handleRefresh = () => {
  loading.value = true
  setTimeout(() => {
    loading.value = false
  }, 500)
}
</script>

<style scoped>
.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.2s ease;
}

.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
</style>
