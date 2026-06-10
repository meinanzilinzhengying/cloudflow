<template>
  <div :class="['min-h-screen transition-colors duration-300', isDark ? 'dark' : '']">
    <div class="flex h-screen bg-slate-50 dark:bg-dark-900">
      <!-- Sidebar -->
      <Sidebar
        :collapsed="sidebarCollapsed"
        :activeMenu="activeMenu"
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
          <!-- Dashboard -->
          <Dashboard v-if="activeMenu === 'dashboard'" />

          <!-- Traffic Analysis -->
          <TrafficOverview v-else-if="activeMenu === 'traffic-overview'" />
          <TrafficSessions v-else-if="activeMenu === 'traffic-sessions'" />
          <TrafficTopN v-else-if="activeMenu === 'traffic-topn'" />
          <TrafficMap v-else-if="activeMenu === 'traffic-map'" />
          <TrafficReplay v-else-if="activeMenu === 'traffic-replay'" />

          <!-- Topology -->
          <TopologyService v-else-if="activeMenu === 'topology-service'" />
          <TopologyPod v-else-if="activeMenu === 'topology-pod'" />
          <TopologyProcess v-else-if="activeMenu === 'topology-process'" />
          <TopologyNamespace v-else-if="activeMenu === 'topology-namespace'" />
          <TopologyDiff v-else-if="activeMenu === 'topology-diff'" />

          <!-- Tracing -->
          <TracingQuery v-else-if="activeMenu === 'tracing-query'" />
          <TracingSlow v-else-if="activeMenu === 'tracing-slow'" />
          <TracingErrors v-else-if="activeMenu === 'tracing-errors'" />
          <TracingCalls v-else-if="activeMenu === 'tracing-calls'" />

          <!-- Metrics -->
          <MetricsHost v-else-if="activeMenu === 'metrics-host'" />
          <MetricsContainer v-else-if="activeMenu === 'metrics-container'" />
          <MetricsService v-else-if="activeMenu === 'metrics-service'" />
          <MetricsCustom v-else-if="activeMenu === 'metrics-custom'" />

          <!-- Logs -->
          <LogsSearch v-else-if="activeMenu === 'logs-search'" />
          <LogsAggregate v-else-if="activeMenu === 'logs-aggregate'" />
          <LogsCorrelation v-else-if="activeMenu === 'logs-correlation'" />

          <!-- Alerts -->
          <AlertCenter v-else-if="activeMenu === 'alerts-events' || activeMenu === 'alerts-rules' || activeMenu === 'alerts-notifications' || activeMenu === 'alerts-stats'" />

          <!-- RCA -->
          <RCAAnalysis v-else-if="activeMenu === 'rca-analysis'" />
          <RCACorrelation v-else-if="activeMenu === 'rca-correlation'" />
          <RCATimeline v-else-if="activeMenu === 'rca-timeline'" />

          <!-- Management -->
          <ManagementAgents v-else-if="activeMenu === 'management-agents'" />
          <ManagementUsers v-else-if="activeMenu === 'management-users'" />
          <ManagementTenants v-else-if="activeMenu === 'management-tenants'" />
          <ManagementAPIKey v-else-if="activeMenu === 'management-apikey'" />
          <ManagementSettings v-else-if="activeMenu === 'management-settings'" />

          <!-- Default -->
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

// Layout Components
import Sidebar from './components/layout/Sidebar.vue'
import Header from './components/layout/Header.vue'

// Dashboard
import Dashboard from './components/pages/Dashboard.vue'

// Traffic Analysis
import TrafficOverview from './components/pages/traffic/TrafficOverview.vue'
import TrafficSessions from './components/pages/traffic/TrafficSessions.vue'
import TrafficTopN from './components/pages/traffic/TrafficTopN.vue'
import TrafficMap from './components/pages/traffic/TrafficMap.vue'
import TrafficReplay from './components/pages/traffic/TrafficReplay.vue'

// Topology
import TopologyService from './components/pages/topology/TopologyService.vue'
import TopologyPod from './components/pages/topology/TopologyPod.vue'
import TopologyProcess from './components/pages/topology/TopologyProcess.vue'
import TopologyNamespace from './components/pages/topology/TopologyNamespace.vue'
import TopologyDiff from './components/pages/topology/TopologyDiff.vue'

// Tracing
import TracingQuery from './components/pages/tracing/TracingOverview.vue'
import TracingSlow from './components/pages/tracing/TracingSlow.vue'
import TracingErrors from './components/pages/tracing/TracingError.vue'
import TracingCalls from './components/pages/tracing/TracingService.vue'

// Metrics
import MetricsHost from './components/pages/metrics/MetricsHost.vue'
import MetricsContainer from './components/pages/metrics/MetricsContainer.vue'
import MetricsService from './components/pages/metrics/MetricsService.vue'
import MetricsCustom from './components/pages/metrics/MetricsCustom.vue'

// Logs
import LogsSearch from './components/pages/logs/LogsSearch.vue'
import LogsAggregate from './components/pages/logs/LogsAggregate.vue'
import LogsCorrelation from './components/pages/logs/LogsCorrelation.vue'

// Alerts
import AlertCenter from './components/pages/alerts/AlertCenter.vue'

// RCA
import RCAAnalysis from './components/pages/rca/RcaAnalysis.vue'
import RCACorrelation from './components/pages/rca/RcaCorrelation.vue'
import RCATimeline from './components/pages/rca/RcaTimeline.vue'

// Management
import ManagementAgents from './components/pages/management/ManagementAgent.vue'
import ManagementUsers from './components/pages/management/ManagementUser.vue'
import ManagementTenants from './components/pages/management/ManagementTenant.vue'
import ManagementAPIKey from './components/pages/management/ManagementApiKey.vue'
import ManagementSettings from './components/pages/management/ManagementSettings.vue'

const isDark = ref(false)
const loading = ref(false)
const sidebarCollapsed = ref(false)
const activeMenu = ref('dashboard')

onMounted(() => {
  if (window.matchMedia('(prefers-color-scheme: dark)').matches) {
    isDark.value = true
  }
  const savedTheme = localStorage.getItem('cloudflow_theme')
  if (savedTheme === 'dark') isDark.value = true
})

const toggleTheme = () => {
  isDark.value = !isDark.value
  localStorage.setItem('cloudflow_theme', isDark.value ? 'dark' : 'light')
}

const toggleSidebar = () => {
  sidebarCollapsed.value = !sidebarCollapsed.value
}

const handleMenuChange = (menu) => {
  activeMenu.value = menu
  loading.value = true
  setTimeout(() => {
    loading.value = false
  }, 300)
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
