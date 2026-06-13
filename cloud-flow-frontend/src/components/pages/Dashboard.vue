<template>
  <div class="space-y-6 animate-fade-in">
    <!-- Page Title -->
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold text-slate-900 dark:text-white">运营驾驶舱</h1>
        <p class="text-sm text-slate-500 dark:text-slate-400 mt-1">实时监控云原生网络流量与系统健康状态</p>
      </div>
      <div class="flex items-center gap-3">
        <div :class="[
          'flex items-center gap-2 px-3 py-1.5 rounded-lg border',
          systemHealthy
            ? 'bg-green-50 dark:bg-green-500/10 border-green-200 dark:border-green-500/20'
            : 'bg-red-50 dark:bg-red-500/10 border-red-200 dark:border-red-500/20'
        ]">
          <div :class="['w-2 h-2 rounded-full', systemHealthy ? 'bg-green-500 animate-pulse' : 'bg-red-500']"></div>
          <span :class="['text-xs font-medium', systemHealthy ? 'text-green-600 dark:text-green-400' : 'text-red-600 dark:text-red-400']">
            {{ systemHealthy ? '所有系统正常' : '部分服务异常' }}
          </span>
        </div>
      </div>
    </div>

    <!-- KPI Cards Row -->
    <div class="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-6 gap-4">
      <KPICard
        v-for="(kpi, index) in kpis"
        :key="kpi.id"
        :title="kpi.title"
        :value="kpi.value"
        :unit="kpi.unit"
        :change="kpi.change"
        :icon="kpi.icon"
        :color="kpi.color"
        :loading="loading"
        :style="{ animationDelay: (index * 50) + 'ms' }"
        class="animate-slide-up"
      />
    </div>

    <!-- Second Row: Traffic Trends + Service Top10 -->
    <div class="grid grid-cols-1 lg:grid-cols-3 gap-6">
      <!-- Traffic Trends -->
      <div class="lg:col-span-2 card p-6">
        <div class="flex items-center justify-between mb-6">
          <div>
            <h3 class="text-lg font-semibold text-slate-900 dark:text-white">流量趋势</h3>
            <p class="text-sm text-slate-500 dark:text-slate-400 mt-0.5">入站/出站流量 & PPS</p>
          </div>
          <div class="flex items-center gap-4">
            <div class="flex items-center gap-2">
              <div class="w-3 h-3 rounded-full bg-primary-500"></div>
              <span class="text-xs text-slate-500 dark:text-slate-400">入站</span>
            </div>
            <div class="flex items-center gap-2">
              <div class="w-3 h-3 rounded-full bg-accent-500"></div>
              <span class="text-xs text-slate-500 dark:text-slate-400">出站</span>
            </div>
          </div>
        </div>
        <div class="h-64">
          <div v-if="!loading && !hasTrafficData" class="flex items-center justify-center h-full text-slate-400 dark:text-slate-500">
            <div class="text-center">
              <svg class="w-12 h-12 mx-auto mb-2 opacity-30" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M7 12l3-3 3 3 4-4M8 21l4-4 4 4M3 4h18M4 4h16v12a1 1 0 01-1 1H5a1 1 0 01-1-1V4z"/></svg>
              <p class="text-sm">暂无流量数据</p>
            </div>
          </div>
          <ECharts v-else :option="trafficChartOption" class="w-full h-full" />
        </div>
      </div>

      <!-- Service Top10 -->
      <div class="card p-6">
        <div class="flex items-center justify-between mb-6">
          <div>
            <h3 class="text-lg font-semibold text-slate-900 dark:text-white">服务 Top10</h3>
            <p class="text-sm text-slate-500 dark:text-slate-400 mt-0.5">按请求数排名</p>
          </div>
          <button class="text-xs text-primary-500 hover:text-primary-600 font-medium">查看全部</button>
        </div>
        <div class="space-y-3">
          <div v-if="loading" class="space-y-3">
            <div v-for="i in 5" :key="'ts-skel-' + i" class="animate-pulse">
              <div class="h-4 bg-slate-100 dark:bg-dark-700 rounded w-3/4 mb-1"></div>
              <div class="h-1.5 bg-slate-100 dark:bg-dark-700 rounded w-full"></div>
            </div>
          </div>
          <div v-else-if="topServices.length === 0" class="flex flex-col items-center justify-center py-12 text-slate-400 dark:text-slate-500">
            <svg class="w-10 h-10 mb-2 opacity-30" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10"/></svg>
            <p class="text-sm">暂无服务数据</p>
          </div>
          <template v-else>
            <div v-for="(service, index) in topServices" :key="service.name" class="flex items-center gap-3">
              <span class="w-6 text-xs font-medium text-slate-400">{{ index + 1 }}</span>
              <div class="flex-1 min-w-0">
                <div class="flex items-center justify-between mb-1">
                  <span class="text-sm font-medium text-slate-700 dark:text-slate-200 truncate">{{ service.name }}</span>
                  <span class="text-xs text-slate-500 dark:text-slate-400">{{ service.qps }} QPS</span>
                </div>
                <div class="h-1.5 bg-slate-100 dark:bg-dark-700 rounded-full overflow-hidden">
                  <div class="h-full bg-gradient-to-r from-primary-500 to-accent-500 rounded-full transition-all duration-500" :style="{ width: service.percentage + '%' }"></div>
                </div>
              </div>
            </div>
          </template>
        </div>
      </div>
    </div>

    <!-- Third Row: Alerts + Topology Health + Resource Usage -->
    <div class="grid grid-cols-1 lg:grid-cols-3 gap-6">
      <!-- Alert List -->
      <div class="card p-6">
        <div class="flex items-center justify-between mb-4">
          <div>
            <h3 class="text-lg font-semibold text-slate-900 dark:text-white">实时告警</h3>
            <p class="text-sm text-slate-500 dark:text-slate-400 mt-0.5">最近告警事件</p>
          </div>
          <button class="text-xs text-primary-500 hover:text-primary-600 font-medium">全部告警</button>
        </div>
        <div class="space-y-3 max-h-80 overflow-y-auto">
          <div v-if="loading" class="space-y-3">
            <div v-for="i in 3" :key="'al-skel-' + i" class="animate-pulse flex gap-3 p-3">
              <div class="w-8 h-8 bg-slate-100 dark:bg-dark-700 rounded-lg"></div>
              <div class="flex-1">
                <div class="h-4 bg-slate-100 dark:bg-dark-700 rounded w-2/3 mb-1"></div>
                <div class="h-3 bg-slate-100 dark:bg-dark-700 rounded w-1/2"></div>
              </div>
            </div>
          </div>
          <div v-else-if="alerts.length === 0" class="flex flex-col items-center justify-center py-12 text-slate-400 dark:text-slate-500">
            <svg class="w-10 h-10 mb-2 opacity-30" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M15 17h5l-1.405-1.405A2.032 2.032 0 0118 14.158V11a6.002 6.002 0 00-4-5.659V5a2 2 0 10-4 0v.341C7.67 6.165 6 8.388 6 11v3.159c0 .538-.214 1.055-.595 1.436L4 17h5m6 0v1a3 3 0 11-6 0v-1m6 0H9"/></svg>
            <p class="text-sm">暂无告警</p>
          </div>
          <template v-else>
            <div v-for="alert in alerts" :key="alert.id" class="flex items-start gap-3 p-3 rounded-xl bg-slate-50 dark:bg-dark-700/50 hover:bg-slate-100 dark:hover:bg-dark-700 transition-colors cursor-pointer">
              <div :class="['w-8 h-8 rounded-lg flex items-center justify-center', getAlertSeverityBg(alert.severity)]">
                <AlertTriangle class="w-4 h-4" :class="getAlertSeverityColor(alert.severity)" />
              </div>
              <div class="flex-1 min-w-0">
                <div class="flex items-center gap-2">
                  <span class="text-sm font-medium text-slate-900 dark:text-white truncate">{{ alert.title }}</span>
                  <span :class="['text-[10px] px-1.5 py-0.5 rounded font-medium', getAlertSeverityBadge(alert.severity)]">{{ alert.severity }}</span>
                </div>
                <p class="text-xs text-slate-500 dark:text-slate-400 mt-0.5">{{ alert.service }} · {{ alert.time }}</p>
              </div>
            </div>
          </template>
        </div>
      </div>

      <!-- Topology Health -->
      <div class="card p-6">
        <div class="flex items-center justify-between mb-4">
          <div><h3 class="text-lg font-semibold text-slate-900 dark:text-white">服务拓扑健康度</h3><p class="text-sm text-slate-500 dark:text-slate-400 mt-0.5">服务依赖关系与状态</p></div>
        </div>
        <div class="h-80 flex items-center justify-center">
          <div v-if="!loading && !hasTopologyData" class="flex flex-col items-center justify-center text-slate-400 dark:text-slate-500">
            <svg class="w-12 h-12 mb-2 opacity-30" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M13.828 10.172a4 4 0 00-5.656 0l-4 4a4 4 0 105.656 5.656l1.102-1.101m-.758-4.899a4 4 0 005.656 0l4-4a4 4 0 00-5.656-5.656l-1.1 1.1"/></svg>
            <p class="text-sm">暂无拓扑数据</p>
          </div>
          <ECharts v-else :option="topologyChartOption" class="w-full h-full" />
        </div>
      </div>

      <!-- Resource Usage -->
      <div class="card p-6">
        <div class="flex items-center justify-between mb-4">
          <div><h3 class="text-lg font-semibold text-slate-900 dark:text-white">资源利用率</h3><p class="text-sm text-slate-500 dark:text-slate-400 mt-0.5">CPU / 内存 / 网络 / 磁盘</p></div>
        </div>
        <div class="grid grid-cols-2 gap-4">
          <div v-for="resource in resources" :key="resource.name" class="flex flex-col items-center p-4 rounded-xl bg-slate-50 dark:bg-dark-700/50">
            <div class="relative w-20 h-20">
              <svg class="w-full h-full -rotate-90"><circle cx="40" cy="40" r="36" stroke-width="6" fill="none" class="stroke-slate-200 dark:stroke-dark-600"/><circle cx="40" cy="40" r="36" stroke-width="6" fill="none" :class="resource.color" :stroke-dasharray="(resource.percentage * 2.26) + ' 226'" stroke-linecap="round" class="transition-all duration-1000"/></svg>
              <div class="absolute inset-0 flex items-center justify-center"><span class="text-lg font-bold text-slate-900 dark:text-white">{{ resource.percentage }}%</span></div>
            </div>
            <span class="mt-2 text-sm font-medium text-slate-600 dark:text-slate-300">{{ resource.name }}</span>
          </div>
        </div>
      </div>
    </div>

    <!-- Quick Access -->
    <div class="card p-6">
      <h3 class="text-lg font-semibold text-slate-900 dark:text-white mb-4">快捷入口</h3>
      <div class="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-6 gap-4">
        <QuickAccessCard v-for="shortcut in shortcuts" :key="shortcut.id" :icon="shortcut.icon" :title="shortcut.title" :description="shortcut.description" :color="shortcut.color" />
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import ECharts from 'vue-echarts'
import { use } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { LineChart, PieChart, GraphChart } from 'echarts/charts'
import { TitleComponent, TooltipComponent, LegendComponent, GridComponent } from 'echarts/components'
import { AlertTriangle, TrendingUp, Activity, Server, Layers, Bell } from 'lucide-vue-next'
import KPICard from '../common/KPICard.vue'
import QuickAccessCard from '../common/QuickAccessCard.vue'
import { queryService, alertService, controlPlaneService } from '../../api'

use([CanvasRenderer, LineChart, PieChart, GraphChart, TitleComponent, TooltipComponent, LegendComponent, GridComponent])

const loading = ref(true)
const systemHealthy = ref(true)

// KPIs - all zeroed, NO fake data
const kpis = ref([
  { id: 1, title: '总流量', value: '0', unit: 'GB', change: '0%', icon: TrendingUp, color: 'primary' },
  { id: 2, title: '活跃 Agent', value: '0', unit: '个', change: '0', icon: Server, color: 'accent' },
  { id: 3, title: '活跃服务', value: '0', unit: '个', change: '0', icon: Layers, color: 'success' },
  { id: 4, title: 'Trace 数量', value: '0', unit: 'K', change: '0%', icon: Activity, color: 'warning' },
  { id: 5, title: '告警数量', value: '0', unit: '条', change: '0%', icon: Bell, color: 'danger' },
  { id: 6, title: '平均延迟', value: '0', unit: 'ms', change: '0ms', icon: Activity, color: 'primary' },
])

// All empty by default - populated only from API
const topServices = ref([])
const alerts = ref([])
const resources = ref([
  { name: 'CPU', percentage: 0, color: 'stroke-primary-500' },
  { name: '内存', percentage: 0, color: 'stroke-accent-500' },
  { name: '网络', percentage: 0, color: 'stroke-amber-500' },
  { name: '磁盘', percentage: 0, color: 'stroke-emerald-500' },
])

// Static navigation shortcuts (not data)
const shortcuts = ref([
  { id: 1, title: '流量分析', description: '网络流量监控', icon: TrendingUp, color: 'primary' },
  { id: 2, title: '拓扑分析', description: '服务依赖拓扑', icon: Layers, color: 'accent' },
  { id: 3, title: 'Trace 分析', description: '分布式链路追踪', icon: Activity, color: 'emerald' },
  { id: 4, title: '告警管理', description: '告警规则与事件', icon: Bell, color: 'amber' },
  { id: 5, title: '用户管理', description: '租户与用户管理', icon: Server, color: 'violet' },
  { id: 6, title: 'Agent 管理', description: '节点与服务代理', icon: Layers, color: 'rose' },
])

const hasTrafficData = computed(() => {
  const s = trafficChartOption.value.series
  return s && s[0] && s[0].data && s[0].data.length > 0 && s[0].data.some(function(v) { return v > 0 })
})

const hasTopologyData = computed(() => {
  const s = topologyChartOption.value.series
  return s && s[0] && s[0].data && s[0].data.length > 0
})

// Traffic Chart Option - empty data by default
const trafficChartOption = computed(() => ({
  tooltip: { trigger: 'axis', backgroundColor: 'rgba(255,255,255,0.95)', borderColor: 'rgba(0,0,0,0.1)', textStyle: { color: '#1e293b' } },
  grid: { left: '3%', right: '4%', bottom: '3%', top: '10%', containLabel: true },
  xAxis: { type: 'category', boundaryGap: false, data: [], axisLine: { lineStyle: { color: '#e2e8f0' } }, axisLabel: { color: '#64748b', fontSize: 11 } },
  yAxis: { type: 'value', axisLine: { show: false }, axisTick: { show: false }, splitLine: { lineStyle: { color: '#f1f5f9', type: 'dashed' } }, axisLabel: { color: '#64748b', fontSize: 11 } },
  series: [
    { name: '入站流量', type: 'line', smooth: true, symbol: 'circle', symbolSize: 6, lineStyle: { color: '#2563eb', width: 2 }, itemStyle: { color: '#2563eb' }, areaStyle: { color: { type: 'linear', x: 0, y: 0, x2: 0, y2: 1, colorStops: [{ offset: 0, color: 'rgba(37,99,235,0.15)' }, { offset: 1, color: 'rgba(37,99,235,0)' }] } }, data: [] },
    { name: '出站流量', type: 'line', smooth: true, symbol: 'circle', symbolSize: 6, lineStyle: { color: '#14b8a6', width: 2 }, itemStyle: { color: '#14b8a6' }, areaStyle: { color: { type: 'linear', x: 0, y: 0, x2: 0, y2: 1, colorStops: [{ offset: 0, color: 'rgba(20,184,166,0.15)' }, { offset: 1, color: 'rgba(20,184,166,0)' }] } }, data: [] },
  ],
}))

// Topology Chart - empty by default
const topologyChartOption = computed(() => ({
  tooltip: { trigger: 'item', backgroundColor: 'rgba(255,255,255,0.95)', borderColor: 'rgba(0,0,0,0.1)', textStyle: { color: '#1e293b' } },
  series: [{ type: 'graph', layout: 'force', symbolSize: 40, roam: true, label: { show: true, position: 'bottom', fontSize: 10, color: '#64748b' }, lineStyle: { width: 1, color: '#e2e8f0', curveness: 0.3 }, emphasis: { focus: 'adjacency', lineStyle: { width: 2, color: '#2563eb' } }, data: [], links: [] }],
}))

const getAlertSeverityBg = function(severity) {
  var m = { critical: 'bg-red-100 dark:bg-red-500/20', warning: 'bg-amber-100 dark:bg-amber-500/20', info: 'bg-blue-100 dark:bg-blue-500/20' }
  return m[severity] || m.info
}
const getAlertSeverityColor = function(severity) {
  var m = { critical: 'text-red-500', warning: 'text-amber-500', info: 'text-blue-500' }
  return m[severity] || m.info
}
const getAlertSeverityBadge = function(severity) {
  var m = { critical: 'bg-red-100 text-red-600 dark:bg-red-500/20 dark:text-red-400', warning: 'bg-amber-100 text-amber-600 dark:bg-amber-500/20 dark:text-amber-400', info: 'bg-blue-100 text-blue-600 dark:bg-blue-500/20 dark:text-blue-400' }
  return m[severity] || m.info
}

const fetchData = async function() {
  loading.value = true
  try {
    const overview = await queryService.getOverview().catch(function() { return null })
    if (overview) {
      kpis.value = [
        { id: 1, title: '总流量', value: formatNumber(overview.totalFlows), unit: 'GB', change: overview.flowsChange || '0%', icon: TrendingUp, color: 'primary' },
        { id: 2, title: '活跃 Agent', value: String(overview.activeAgents != null ? overview.activeAgents : 0), unit: '个', change: String(overview.agentsChange != null ? overview.agentsChange : 0), icon: Server, color: 'accent' },
        { id: 3, title: '活跃服务', value: String(overview.activeServices != null ? overview.activeServices : 0), unit: '个', change: String(overview.servicesChange != null ? overview.servicesChange : 0), icon: Layers, color: 'success' },
        { id: 4, title: 'Trace 数量', value: formatNumber(overview.traceCount), unit: 'K', change: overview.tracesChange || '0%', icon: Activity, color: 'warning' },
        { id: 5, title: '告警数量', value: String(overview.alertCount != null ? overview.alertCount : 0), unit: '条', change: overview.alertsChange || '0%', icon: Bell, color: 'danger' },
        { id: 6, title: '平均延迟', value: String(overview.avgLatency != null ? overview.avgLatency : 0), unit: 'ms', change: overview.latencyChange || '0ms', icon: Activity, color: 'primary' },
      ]
      if (overview.trafficLabels && overview.trafficLabels.length) {
        trafficChartOption.value.xAxis.data = overview.trafficLabels
        trafficChartOption.value.series[0].data = overview.trafficInbound || []
        trafficChartOption.value.series[1].data = overview.trafficOutbound || []
      }
      if (Array.isArray(overview.topServices)) topServices.value = overview.topServices
      if (Array.isArray(overview.resources)) resources.value = overview.resources
    }

    const alertData = await alertService.getAlerts({ status: 'active', limit: 5 }).catch(function() { return null })
    if (alertData && alertData.alerts && Array.isArray(alertData.alerts)) {
      alerts.value = alertData.alerts.map(function(a) {
        return { id: a.alert_id || a.id, title: a.title || a.message || '未知告警', severity: (a.severity || a.level || 'info').toLowerCase(), service: a.service || a.resource || 'Unknown', time: formatTime(a.created_at || a.timestamp) }
      })
    }

    const agents = await controlPlaneService.getAgents().catch(function() { return null })
    if (agents && Array.isArray(agents)) kpis.value[1].value = String(agents.length)
  } catch (error) {
    console.warn('[Dashboard] Data fetch error:', error && error.message)
  } finally {
    loading.value = false
  }
}

const formatNumber = function(num) {
  if (!num) return '0'
  if (num >= 1000000) return (num / 1000000).toFixed(1) + 'M'
  if (num >= 1000) return (num / 1000).toFixed(1) + 'K'
  return String(num)
}
const formatTime = function(timestamp) {
  if (!timestamp) return '-'
  const now = Date.now()
  const diff = now - new Date(timestamp).getTime()
  const minutes = Math.floor(diff / 60000)
  if (minutes < 60) return minutes + '分钟前'
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return hours + '小时前'
  return Math.floor(hours / 24) + '天前'
}

onMounted(function() { fetchData() })
</script>
