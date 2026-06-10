<template>
  <div class="space-y-6 animate-fade-in">
    <!-- Page Title -->
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold text-slate-900 dark:text-white">运营驾驶舱</h1>
        <p class="text-sm text-slate-500 dark:text-slate-400 mt-1">实时监控云原生网络流量与系统健康状态</p>
      </div>
      <div class="flex items-center gap-3">
        <div class="flex items-center gap-2 px-3 py-1.5 rounded-lg bg-green-50 dark:bg-green-500/10 border border-green-200 dark:border-green-500/20">
          <div class="w-2 h-2 rounded-full bg-green-500 animate-pulse"></div>
          <span class="text-xs font-medium text-green-600 dark:text-green-400">所有系统正常</span>
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
        :style="{ animationDelay: `${index * 50}ms` }"
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
          <ECharts :option="trafficChartOption" class="w-full h-full" />
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
          <div
            v-for="(service, index) in topServices"
            :key="service.name"
            class="flex items-center gap-3"
          >
            <span class="w-6 text-xs font-medium text-slate-400">{{ index + 1 }}</span>
            <div class="flex-1 min-w-0">
              <div class="flex items-center justify-between mb-1">
                <span class="text-sm font-medium text-slate-700 dark:text-slate-200 truncate">{{ service.name }}</span>
                <span class="text-xs text-slate-500 dark:text-slate-400">{{ service.qps }} QPS</span>
              </div>
              <div class="h-1.5 bg-slate-100 dark:bg-dark-700 rounded-full overflow-hidden">
                <div
                  class="h-full bg-gradient-to-r from-primary-500 to-accent-500 rounded-full transition-all duration-500"
                  :style="{ width: `${service.percentage}%` }"
                ></div>
              </div>
            </div>
          </div>
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
          <div
            v-for="alert in alerts"
            :key="alert.id"
            class="flex items-start gap-3 p-3 rounded-xl bg-slate-50 dark:bg-dark-700/50 hover:bg-slate-100 dark:hover:bg-dark-700 transition-colors cursor-pointer"
          >
            <div :class="['w-8 h-8 rounded-lg flex items-center justify-center', getAlertSeverityBg(alert.severity)]">
              <AlertTriangle class="w-4 h-4" :class="getAlertSeverityColor(alert.severity)" />
            </div>
            <div class="flex-1 min-w-0">
              <div class="flex items-center gap-2">
                <span class="text-sm font-medium text-slate-900 dark:text-white truncate">{{ alert.title }}</span>
                <span :class="['text-[10px] px-1.5 py-0.5 rounded font-medium', getAlertSeverityBadge(alert.severity)]">
                  {{ alert.severity }}
                </span>
              </div>
              <p class="text-xs text-slate-500 dark:text-slate-400 mt-0.5">{{ alert.service }} · {{ alert.time }}</p>
            </div>
          </div>
        </div>
      </div>

      <!-- Topology Health -->
      <div class="card p-6">
        <div class="flex items-center justify-between mb-4">
          <div>
            <h3 class="text-lg font-semibold text-slate-900 dark:text-white">服务拓扑健康度</h3>
            <p class="text-sm text-slate-500 dark:text-slate-400 mt-0.5">服务依赖关系与状态</p>
          </div>
        </div>
        <div class="h-80 flex items-center justify-center">
          <ECharts :option="topologyChartOption" class="w-full h-full" />
        </div>
      </div>

      <!-- Resource Usage -->
      <div class="card p-6">
        <div class="flex items-center justify-between mb-4">
          <div>
            <h3 class="text-lg font-semibold text-slate-900 dark:text-white">资源利用率</h3>
            <p class="text-sm text-slate-500 dark:text-slate-400 mt-0.5">CPU / 内存 / 网络 / 磁盘</p>
          </div>
        </div>
        <div class="grid grid-cols-2 gap-4">
          <div
            v-for="resource in resources"
            :key="resource.name"
            class="flex flex-col items-center p-4 rounded-xl bg-slate-50 dark:bg-dark-700/50"
          >
            <div class="relative w-20 h-20">
              <svg class="w-full h-full -rotate-90">
                <circle
                  cx="40" cy="40" r="36"
                  stroke-width="6"
                  fill="none"
                  class="stroke-slate-200 dark:stroke-dark-600"
                />
                <circle
                  cx="40" cy="40" r="36"
                  stroke-width="6"
                  fill="none"
                  :class="resource.color"
                  :stroke-dasharray="`${resource.percentage * 2.26} 226`"
                  stroke-linecap="round"
                  class="transition-all duration-1000"
                />
              </svg>
              <div class="absolute inset-0 flex items-center justify-center">
                <span class="text-lg font-bold text-slate-900 dark:text-white">{{ resource.percentage }}%</span>
              </div>
            </div>
            <span class="mt-2 text-sm font-medium text-slate-600 dark:text-slate-300">{{ resource.name }}</span>
          </div>
        </div>
      </div>
    </div>

    <!-- Fourth Row: Quick Access -->
    <div class="card p-6">
      <h3 class="text-lg font-semibold text-slate-900 dark:text-white mb-4">快捷入口</h3>
      <div class="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-6 gap-4">
        <QuickAccessCard
          v-for="shortcut in shortcuts"
          :key="shortcut.id"
          :icon="shortcut.icon"
          :title="shortcut.title"
          :description="shortcut.description"
          :color="shortcut.color"
        />
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
import {
  TitleComponent,
  TooltipComponent,
  LegendComponent,
  GridComponent,
} from 'echarts/components'
import { AlertTriangle, TrendingUp, Activity, Server, Layers, Bell } from 'lucide-vue-next'
import KPICard from '../common/KPICard.vue'
import QuickAccessCard from '../common/QuickAccessCard.vue'
import { queryService, alertService, controlPlaneService } from '../../api'

// Register ECharts components
use([
  CanvasRenderer,
  LineChart,
  PieChart,
  GraphChart,
  TitleComponent,
  TooltipComponent,
  LegendComponent,
  GridComponent,
])

const loading = ref(true)

// KPIs
const kpis = ref([
  { id: 1, title: '总流量', value: '0', unit: 'GB', change: '+12.5%', icon: TrendingUp, color: 'primary' },
  { id: 2, title: '活跃 Agent', value: '0', unit: '个', change: '+5', icon: Server, color: 'accent' },
  { id: 3, title: '活跃服务', value: '0', unit: '个', change: '+3', icon: Layers, color: 'success' },
  { id: 4, title: 'Trace 数量', value: '0', unit: 'K', change: '+8.2%', icon: Activity, color: 'warning' },
  { id: 5, title: '告警数量', value: '0', unit: '条', change: '-15%', icon: Bell, color: 'danger' },
  { id: 6, title: '平均延迟', value: '0', unit: 'ms', change: '-3ms', icon: Activity, color: 'primary' },
])

// Top Services
const topServices = ref([
  { name: 'api-gateway', qps: 12500, percentage: 100 },
  { name: 'user-service', qps: 8200, percentage: 66 },
  { name: 'order-service', qps: 6100, percentage: 49 },
  { name: 'payment-service', qps: 4800, percentage: 38 },
  { name: 'product-service', qps: 4200, percentage: 34 },
  { name: 'inventory-service', qps: 3100, percentage: 25 },
  { name: 'notification-service', qps: 2800, percentage: 22 },
  { name: 'analytics-service', qps: 2100, percentage: 17 },
  { name: 'search-service', qps: 1800, percentage: 14 },
  { name: 'cache-service', qps: 1500, percentage: 12 },
])

// Alerts
const alerts = ref([
  { id: 1, title: 'CPU 使用率超过 80%', severity: 'warning', service: 'api-gateway', time: '2分钟前' },
  { id: 2, title: '响应延迟增加', severity: 'info', service: 'user-service', time: '5分钟前' },
  { id: 3, title: '内存使用率接近阈值', severity: 'warning', service: 'order-service', time: '8分钟前' },
  { id: 4, title: 'P99 延迟超标', severity: 'critical', service: 'payment-service', time: '12分钟前' },
  { id: 5, title: '错误率上升', severity: 'info', service: 'product-service', time: '15分钟前' },
])

// Resources
const resources = ref([
  { name: 'CPU', percentage: 45, color: 'stroke-primary-500' },
  { name: '内存', percentage: 62, color: 'stroke-accent-500' },
  { name: '网络', percentage: 38, color: 'stroke-amber-500' },
  { name: '磁盘', percentage: 55, color: 'stroke-emerald-500' },
])

// Shortcuts
const shortcuts = ref([
  { id: 1, title: '流量分析', description: '网络流量监控', icon: TrendingUp, color: 'primary' },
  { id: 2, title: '拓扑分析', description: '服务依赖拓扑', icon: Layers, color: 'accent' },
  { id: 3, title: 'Trace 分析', description: '分布式链路追踪', icon: Activity, color: 'emerald' },
  { id: 4, title: '告警管理', description: '告警规则与事件', icon: Bell, color: 'amber' },
  { id: 5, title: '用户管理', description: '租户与用户管理', icon: Server, color: 'violet' },
  { id: 6, title: 'Agent 管理', description: '节点与服务代理', icon: Layers, color: 'rose' },
])

// Traffic Chart Option
const trafficChartOption = computed(() => ({
  tooltip: {
    trigger: 'axis',
    backgroundColor: 'rgba(255, 255, 255, 0.95)',
    borderColor: 'rgba(0, 0, 0, 0.1)',
    textStyle: { color: '#1e293b' },
  },
  grid: { left: '3%', right: '4%', bottom: '3%', top: '10%', containLabel: true },
  xAxis: {
    type: 'category',
    boundaryGap: false,
    data: ['00:00', '02:00', '04:00', '06:00', '08:00', '10:00', '12:00', '14:00', '16:00', '18:00', '20:00', '22:00'],
    axisLine: { lineStyle: { color: '#e2e8f0' } },
    axisLabel: { color: '#64748b', fontSize: 11 },
  },
  yAxis: {
    type: 'value',
    axisLine: { show: false },
    axisTick: { show: false },
    splitLine: { lineStyle: { color: '#f1f5f9', type: 'dashed' } },
    axisLabel: { color: '#64748b', fontSize: 11 },
  },
  series: [
    {
      name: '入站流量',
      type: 'line',
      smooth: true,
      symbol: 'circle',
      symbolSize: 6,
      lineStyle: { color: '#2563eb', width: 2 },
      itemStyle: { color: '#2563eb' },
      areaStyle: {
        color: {
          type: 'linear',
          x: 0, y: 0, x2: 0, y2: 1,
          colorStops: [
            { offset: 0, color: 'rgba(37, 99, 235, 0.15)' },
            { offset: 1, color: 'rgba(37, 99, 235, 0)' },
          ],
        },
      },
      data: [320, 450, 380, 520, 680, 850, 920, 880, 750, 620, 580, 420],
    },
    {
      name: '出站流量',
      type: 'line',
      smooth: true,
      symbol: 'circle',
      symbolSize: 6,
      lineStyle: { color: '#14b8a6', width: 2 },
      itemStyle: { color: '#14b8a6' },
      areaStyle: {
        color: {
          type: 'linear',
          x: 0, y: 0, x2: 0, y2: 1,
          colorStops: [
            { offset: 0, color: 'rgba(20, 184, 166, 0.15)' },
            { offset: 1, color: 'rgba(20, 184, 166, 0)' },
          ],
        },
      },
      data: [280, 380, 320, 450, 580, 720, 780, 740, 630, 520, 480, 350],
    },
  ],
}))

// Topology Chart Option
const topologyChartOption = computed(() => ({
  tooltip: {
    trigger: 'item',
    backgroundColor: 'rgba(255, 255, 255, 0.95)',
    borderColor: 'rgba(0, 0, 0, 0.1)',
    textStyle: { color: '#1e293b' },
  },
  series: [
    {
      type: 'graph',
      layout: 'force',
      symbolSize: 40,
      roam: true,
      label: { show: true, position: 'bottom', fontSize: 10, color: '#64748b' },
      lineStyle: { width: 1, color: '#e2e8f0', curveness: 0.3 },
      emphasis: { focus: 'adjacency', lineStyle: { width: 2, color: '#2563eb' } },
      data: [
        { name: 'API Gateway', symbol: 'roundRect', itemStyle: { color: '#22c55e' } },
        { name: 'User Svc', symbol: 'roundRect', itemStyle: { color: '#22c55e' } },
        { name: 'Order Svc', symbol: 'roundRect', itemStyle: { color: '#f59e0b' } },
        { name: 'Payment Svc', symbol: 'roundRect', itemStyle: { color: '#22c55e' } },
        { name: 'Product Svc', symbol: 'roundRect', itemStyle: { color: '#22c55e' } },
        { name: 'DB', symbol: 'diamond', itemStyle: { color: '#2563eb' } },
        { name: 'Cache', symbol: 'diamond', itemStyle: { color: '#14b8a6' } },
        { name: 'Queue', symbol: 'diamond', itemStyle: { color: '#8b5cf6' } },
      ],
      links: [
        { source: 'API Gateway', target: 'User Svc' },
        { source: 'API Gateway', target: 'Order Svc' },
        { source: 'API Gateway', target: 'Product Svc' },
        { source: 'User Svc', target: 'DB' },
        { source: 'Order Svc', target: 'Payment Svc' },
        { source: 'Order Svc', target: 'DB' },
        { source: 'Order Svc', target: 'Queue' },
        { source: 'Payment Svc', target: 'DB' },
        { source: 'Product Svc', target: 'Cache' },
        { source: 'Product Svc', target: 'DB' },
        { source: 'User Svc', target: 'Cache' },
      ],
    },
  ],
}))

// Helper functions
const getAlertSeverityBg = (severity) => {
  const classes = {
    critical: 'bg-red-100 dark:bg-red-500/20',
    warning: 'bg-amber-100 dark:bg-amber-500/20',
    info: 'bg-blue-100 dark:bg-blue-500/20',
  }
  return classes[severity] || classes.info
}

const getAlertSeverityColor = (severity) => {
  const classes = {
    critical: 'text-red-500',
    warning: 'text-amber-500',
    info: 'text-blue-500',
  }
  return classes[severity] || classes.info
}

const getAlertSeverityBadge = (severity) => {
  const classes = {
    critical: 'bg-red-100 text-red-600 dark:bg-red-500/20 dark:text-red-400',
    warning: 'bg-amber-100 text-amber-600 dark:bg-amber-500/20 dark:text-amber-400',
    info: 'bg-blue-100 text-blue-600 dark:bg-blue-500/20 dark:text-blue-400',
  }
  return classes[severity] || classes.info
}

// Fetch data from APIs
const fetchData = async () => {
  loading.value = true
  try {
    // Fetch overview data
    const overview = await queryService.getOverview()
    if (overview) {
      kpis.value[0].value = formatNumber(overview.totalFlows)
      kpis.value[1].value = overview.activeAgents?.toString() || '0'
      kpis.value[2].value = overview.activeServices?.toString() || '0'
      kpis.value[3].value = formatNumber(overview.traceCount)
      kpis.value[4].value = overview.alertCount?.toString() || '0'
      kpis.value[5].value = overview.avgLatency?.toString() || '0'
    }

    // Fetch alerts
    const alertData = await alertService.getAlerts({ status: 'active', limit: 5 })
    if (alertData?.alerts) {
      alerts.value = alertData.alerts.map((a) => ({
        id: a.alert_id,
        title: a.title,
        severity: a.severity?.toLowerCase() || 'info',
        service: a.service || 'Unknown',
        time: formatTime(a.created_at),
      }))
    }

    // Fetch agents
    const agents = await controlPlaneService.getAgents()
    if (agents) {
      kpis.value[1].value = (agents.length || 0).toString()
    }
  } catch (error) {
    console.warn('API call failed, using placeholder data:', error)
    // Data will remain with placeholder values
  } finally {
    loading.value = false
  }
}

const formatNumber = (num) => {
  if (!num) return '0'
  if (num >= 1000000) return (num / 1000000).toFixed(1) + 'M'
  if (num >= 1000) return (num / 1000).toFixed(1) + 'K'
  return num.toString()
}

const formatTime = (timestamp) => {
  if (!timestamp) return '未知'
  const now = Date.now()
  const diff = now - new Date(timestamp).getTime()
  const minutes = Math.floor(diff / 60000)
  if (minutes < 60) return `${minutes}分钟前`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours}小时前`
  return `${Math.floor(hours / 24)}天前`
}

onMounted(() => {
  fetchData()
})
</script>
