<template>
  <div :class="['min-h-screen transition-colors duration-300', isDark ? 'dark' : '']">
    <div class="flex h-screen bg-slate-50 dark:bg-dark-900">
      <!-- Sidebar -->
      <Sidebar
        :collapsed="sidebarCollapsed"
        :activeMenu="activeMenu"
        :activeSubmenu="activeSubmenu"
        @toggle="toggleSidebar"
        @menu-change="handleMenuChange"
      />

      <!-- Main Content -->
      <div class="flex-1 flex flex-col overflow-hidden">
        <!-- Header -->
        <Header
          :isDark="isDark"
          @toggle-theme="toggleTheme"
          @refresh="handleRefresh"
        />

        <!-- Page Content -->
        <main class="flex-1 overflow-auto p-6 bg-slate-50 dark:bg-dark-900">
          <Dashboard v-if="activeMenu === 'dashboard'" />
          <Traffic v-else-if="activeMenu === 'traffic'" />
          <Topology v-else-if="activeMenu === 'topology'" />
          <Tracing v-else-if="activeMenu === 'tracing'" />
          <Metrics v-else-if="activeMenu === 'metrics'" />
          <Logs v-else-if="activeMenu === 'logs'" />
          <Alerts v-else-if="activeMenu === 'alerts'" />
          <Tenants v-else-if="activeMenu === 'tenants'" />
          <Users v-else-if="activeMenu === 'users'" />
          <Agents v-else-if="activeMenu === 'agents'" />
          <Settings v-else-if="activeMenu === 'settings'" />
          <Dashboard v-else />
        </main>
      </div>
    </div>

    <!-- Loading Overlay -->
    <Transition name="fade">
      <div
        v-if="loading"
        class="fixed inset-0 bg-slate-900/80 backdrop-blur-sm flex items-center justify-center z-50"
      >
        <div class="text-center">
          <div class="relative w-16 h-16 mx-auto mb-4">
            <div class="absolute inset-0 border-4 border-primary-500/20 rounded-full"></div>
            <div class="absolute inset-0 border-4 border-primary-500 rounded-full border-t-transparent animate-spin"></div>
          </div>
          <p class="text-white/80 text-sm font-medium">加载中...</p>
        </div>
      </div>
    </Transition>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import Sidebar from './components/layout/Sidebar.vue'
import Header from './components/layout/Header.vue'
import Dashboard from './components/pages/Dashboard.vue'
import Traffic from './components/pages/Traffic.vue'
import Topology from './components/pages/Topology.vue'
import Tracing from './components/pages/Tracing.vue'
import Metrics from './components/pages/Metrics.vue'
import Logs from './components/pages/Logs.vue'
import Alerts from './components/pages/Alerts.vue'
import Tenants from './components/pages/Tenants.vue'
import Users from './components/pages/Users.vue'
import Agents from './components/pages/Agents.vue'
import Settings from './components/pages/Settings.vue'

const isDark = ref(false)
const loading = ref(false)
const sidebarCollapsed = ref(false)
const activeMenu = ref('dashboard')
const activeSubmenu = ref(null)

onMounted(() => {
  // Check system preference
  if (window.matchMedia('(prefers-color-scheme: dark)').matches) {
    isDark.value = true
  }

  // Check localStorage
  const savedTheme = localStorage.getItem('cloudflow_theme')
  if (savedTheme === 'dark') {
    isDark.value = true
  } else if (savedTheme === 'light') {
    isDark.value = false
  }
})

const toggleTheme = () => {
  isDark.value = !isDark.value
  localStorage.setItem('cloudflow_theme', isDark.value ? 'dark' : 'light')
}

const toggleSidebar = () => {
  sidebarCollapsed.value = !sidebarCollapsed.value
}

const handleMenuChange = (menu, submenu = null) => {
  activeMenu.value = menu
  activeSubmenu.value = submenu
}

const handleRefresh = () => {
  loading.value = true
  setTimeout(() => {
    loading.value = false
  }, 1000)
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
