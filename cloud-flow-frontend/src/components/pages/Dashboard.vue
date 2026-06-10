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
        <div v-if="loading" class="h-64 flex items-center justify-center">
          <p class="text-sm text-slate-400 dark:text-slate-500">加载中...</p>
        </div>
        <div v-else-if="!trafficData.inbound.length" class="h-64 flex items-center justify-center">
          <p class="text-sm text-slate-400 dark:text-slate-500">暂无流量数据</p>
        </div>
        <div v-else class="h-64">
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
        <div v-if="loading" class="space-y-3">
          <div v-for="i in 5" :key="i" class="flex items-center gap-3">
            <div class="w-6 h-3 bg-slate-200 dark:bg-dark-600 rounded animate-pulse"></div>
            <div class="flex-1 space-y-2">
              <div class="h-3 bg-slate-200 dark:bg-dark-600 rounded animate-pulse"></div>
              <div class="h-1.5 bg-slate-200 dark:bg-dark-600 rounded animate-pulse"></div>
            </div>
          </div>
        </div>
        <div v-else-if="!topServices.length" class="h-48 flex items-center justify-center">
          <p class="text-sm text-slate-400 dark:text-slate-500">暂无服务数据</p>
        </div>
        <div v-else class="space-y-3">
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
        <div v-if="loading" class="space-y-3">
          <div v-for="i in 4" :key="i" class="flex items-start gap-3 p-3">
            <div class="w-8 h-8 rounded-lg bg-slate-200 dark:bg-dark-600 animate-pulse"></div>
            <div class="flex-1 space-y-2">
              <div class="h-3 bg-slate-200 dark:bg-dark-600 rounded animate-pulse"></div>
              <div class="h-2 w-24 bg-slate-200 dark:bg-dark-600 rounded animate-pulse"></div>
            </div>
          </div>
        </div>
        <div v-else-if="!alerts.length" class="h-48 flex items-center justify-center">
          <p class="text-sm text-slate-400 dark:text-slate-500">暂无告警数据</p>
        </div>
        <div v-else class="space-y-3 max-h-80 overflow-y-auto">
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
        <div v-if="loading" class="h-80 flex items-center justify-center">
          <p class="text-sm text-slate-400 dark:text-slate-500">加载中...</p>
        </div>
        <div v-else-if="!topologyNodes.length" class="h-80 flex items-center justify-center">
          <p class="text-sm text-slate-400 dark:text-slate-500">暂无拓扑数据</p>
        </div>
        <div v-else class="h-80 flex items-center justify-center">
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
        <div v-if="loading" class="grid grid-cols-2 gap-4">
          <div v-for="i in 4" :key="i" class="flex flex-col items-center p-4 rounded-xl bg-slate-50 dark:bg-dark-700/50">
            <div class="w-20 h-20 rounded-full bg-slate-200 dark:bg-dark-600 animate-pulse"></div>
            <div class="mt-2 w-10 h-3 bg-slate-200 dark:bg-dark-600 rounded animate-pulse"></div>
          </div>
        </div>
        <div v-else-if="!resources.length" class="h-48 flex items-center justify-center">
          <p class="text-sm text-slate-400 dark:text-slate-500">暂无资源数据</p>
        </div>
        <div v-else class="grid grid-cols-2 gap-4">
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

// -----------------------------
// 响应式数据（初始为空值）
// -----------------------------
const kpis = ref([
  { id: 1, title: '总流量', value: '-', unit: '', change: '-', icon: TrendingUp, color: 'primary' },
  { id: 2, title: '活跃 Agent', value: '-', unit: '', change: '-', icon: Server, color: 'accent' },
  { id: 3, title: '活跃服务', value: '-', unit: '', change: '-', icon: Layers, color: 'success' },
  { id: 4, title: 'Trace 数量', value: '-', unit: '', change: '-', icon: Activity, color: 'warning' },
  { id: 5, title: '告警数量', value: '-', unit: '', change: '-', icon: Bell, color: 'danger' },
  { id: 6, title: '平均延迟', value: '-', unit: '', change: '-', icon: Activity, color: 'primary' },
])

const topServices = ref([])
const alerts = ref([])
const resources = ref([])
const trafficData = ref({ labels: [], inbound: [], outbound: [] })
const topologyNodes = ref([])
const topologyLinks = ref([])

const shortcuts = ref([
  { id: 1, title: '流量分析', description: '网络流量监控', icon: TrendingUp, color: 'primary' },
  { id: 2, title: '拓扑分析', description: '服务依赖拓扑', icon: Layers, color: 'accent' },
  { id: 3, title: 'Trace 分析', description: '分布式链路追踪', icon: Activity, color: 'emerald' },
  { id: 4, title: '告警管理', description: '告警规则与事件', icon: Bell, color: 'amber' },
  { id: 5, title: '用户管理', description: '租户与用户管理', icon: Server, color: 'violet' },
  { id: 6, title: 'Agent 管理', description: '节点与服务代理', icon: Layers, color: 'rose' },
])

// -----------------------------
// 图表 Option（computed）
// -----------------------------
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
    data: trafficData.value.labels,
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
      data: trafficData.value.inbound,
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
      data: trafficData.value.outbound,
    },
  ],
}))

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
      data: topologyNodes.value,
      links: topologyLinks.value,
    },
  ],
}))

// -----------------------------
// 辅助函数
// -----------------------------
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

const formatNumber = (num) => {
  if (num === null || num === undefined || num === '') return '0'
  const n = Number(num)
  if (Number.isNaN(n)) return '0'
  if (n >= 1000000) return (n / 1000000).toFixed(1) + 'M'
  if (n >= 1000) return (n / 1000).toFixed(1) + 'K'
  return Math.floor(n).toString()
}

const formatTime = (timestamp) => {
  if (!timestamp) return '未知'
  const now = Date.now()
  const t = typeof timestamp === 'number' ? timestamp : new Date(timestamp).getTime()
  const diff = now - t
  const minutes = Math.floor(diff / 60000)
  if (minutes < 1) return '刚刚'
  if (minutes < 60) return `${minutes}分钟前`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours}小时前`
  return `${Math.floor(hours / 24)}天前`
}

const pick = (obj, keys) => {
  for (const k of keys) {
    if (obj && obj[k] !== undefined && obj[k] !== null) return obj[k]
  }
  return null
}

// -----------------------------
// 数据处理
// -----------------------------
const processOverview = (overview) => {
  if (!overview) return
  kpis.value = [
    {
      id: 1,
      title: '总流量',
      value: formatNumber(pick(overview, ['totalFlows', 'total_flows', 'total_flows_bytes', 'total_bytes'])),
      unit: '',
      change: pick(overview, ['flowsChange', 'flows_change']) || '-',
      icon: TrendingUp,
      color: 'primary',
    },
    {
      id: 2,
      title: '活跃 Agent',
      value: formatNumber(pick(overview, ['activeAgents', 'active_agents', 'agentCount', 'agent_count'])),
      unit: '',
      change: '-',
      icon: Server,
      color: 'accent',
    },
    {
      id: 3,
      title: '活跃服务',
      value: formatNumber(pick(overview, ['activeServices', 'active_services', 'serviceCount', 'service_count'])),
      unit: '',
      change: '-',
      icon: Layers,
      color: 'success',
    },
    {
      id: 4,
      title: 'Trace 数量',
      value: formatNumber(pick(overview, ['traceCount', 'trace_count', 'totalTraces', 'total_traces'])),
      unit: '',
      change: '-',
      icon: Activity,
      color: 'warning',
    },
    {
      id: 5,
      title: '告警数量',
      value: formatNumber(pick(overview, ['alertCount', 'alert_count', 'totalAlerts', 'total_alerts'])),
      unit: '',
      change: '-',
      icon: Bell,
      color: 'danger',
    },
    {
      id: 6,
      title: '平均延迟',
      value: formatNumber(pick(overview, ['avgLatency', 'avg_latency', 'latencyMs', 'latency_ms'])) + ' ms',
      unit: '',
      change: '-',
      icon: Activity,
      color: 'primary',
    },
  ]

  // 从 overview 中提取资源使用率
  const resourceKeys = [
    { key: ['cpu', 'cpuUsage', 'cpu_usage', 'cpuPercent', 'cpu_percent'], name: 'CPU', color: 'stroke-primary-500' },
    { key: ['memory', 'mem', 'memoryUsage', 'memory_usage', 'memPercent', 'mem_percent'], name: '内存', color: 'stroke-accent-500' },
    { key: ['network', 'networkUsage', 'network_usage'], name: '网络', color: 'stroke-amber-500' },
    { key: ['disk', 'diskUsage', 'disk_usage'], name: '磁盘', color: 'stroke-emerald-500' },
  ]
  const parsed = []
  for (const item of resourceKeys) {
    const val = pick(overview, item.key)
    if (val !== null) {
      const pct = Math.min(100, Math.max(0, Math.round(Number(val))))
      parsed.push({ name: item.name, percentage: pct, color: item.color })
    }
  }
  if (parsed.length) resources.value = parsed
}

const processFlows = (flowData) => {
  if (!flowData) return
  // 兼容多种可能结构
  const list = pick(flowData, ['flows', 'data', 'items', 'results']) || flowData
  if (!Array.isArray(list) || !list.length) return

  const labels = []
  const inbound = []
  const outbound = []
  const now = Date.now()
  const step = Math.floor(3600 * 1000 / list.length) || 3600 * 1000
  list.forEach((item, idx) => {
    const ts = pick(item, ['timestamp', 'time', 'ts'])
    if (ts) {
      const d = new Date(typeof ts === 'number' ? ts : ts)
      const h = String(d.getHours()).padStart(2, '0')
      const m = String(d.getMinutes()).padStart(2, '0')
      labels.push(`${h}:${m}`)
    } else {
      const d = new Date(now - (list.length - idx) * step)
      labels.push(`${String(d.getHours()).padStart(2, '0')}:00`)
    }
    const ib = Number(pick(item, ['inbound', 'inbound_bytes', 'in_bytes', 'rx']) || 0)
    const ob = Number(pick(item, ['outbound', 'outbound_bytes', 'out_bytes', 'tx']) || 0)
    inbound.push(ib)
    outbound.push(ob)
  })
  trafficData.value = { labels, inbound, outbound }
}

const processTopology = (topoData) => {
  if (!topoData) return
  const nodesRaw = pick(topoData, ['nodes', 'services', 'data.nodes']) || []
  const linksRaw = pick(topoData, ['links', 'edges', 'data.links']) || []

  if (!Array.isArray(nodesRaw) || !nodesRaw.length) return

  const nodes = nodesRaw.map((n) => {
    const name = pick(n, ['name', 'id', 'serviceName', 'service_name']) || 'Unknown'
    const status = pick(n, ['status', 'state', 'health']) || 'healthy'
    let color = '#22c55e'
    if (typeof status === 'string') {
      const s = status.toLowerCase()
      if (s.includes('warn') || s.includes('degrade')) color = '#f59e0b'
      else if (s.includes('error') || s.includes('critical') || s.includes('down')) color = '#ef4444'
    }
    const type = pick(n, ['type', 'kind'])
    const symbol =
      type && (type.includes('db') || type.includes('database') || type.includes('cache') || type.includes('queue'))
        ? 'diamond'
        : 'roundRect'
    return {
      name,
      symbol,
      itemStyle: { color },
    }
  })

  const links = linksRaw
    .map((l) => {
      const source = pick(l, ['source', 'from', 'source_id'])
      const target = pick(l, ['target', 'to', 'target_id', 'dest'])
      if (!source || !target) return null
      return { source, target }
    })
    .filter(Boolean)

  topologyNodes.value = nodes
  topologyLinks.value = links

  // 从拓扑中提取服务排名
  const serviceCount = new Map()
  links.forEach((l) => {
    serviceCount.set(l.source, (serviceCount.get(l.source) || 0) + 1)
    serviceCount.set(l.target, (serviceCount.get(l.target) || 0) + 1)
  })
  const svcList = [...serviceCount.entries()].sort((a, b) => b[1] - a[1]).slice(0, 10)
  if (svcList.length) {
    const max = svcList[0][1] || 1
    topServices.value = svcList.map(([name, count]) => ({
      name,
      qps: count * 100,
      percentage: Math.round((count / max) * 100),
    }))
  }
}

const processAlerts = (alertData) => {
  if (!alertData) return
  const list = pick(alertData, ['alerts', 'data', 'items', 'results']) || (Array.isArray(alertData) ? alertData : [])
  if (!Array.isArray(list) || !list.length) return
  alerts.value = list.slice(0, 5).map((a) => ({
    id: pick(a, ['id', 'alert_id', 'alertId']) || Math.random().toString(36).slice(2),
    title: pick(a, ['title', 'message', 'summary']) || '未知告警',
    severity: (pick(a, ['severity', 'level']) || 'info').toString().toLowerCase(),
    service: pick(a, ['service', 'service_name', 'source']) || 'Unknown',
    time: formatTime(pick(a, ['created_at', 'createdAt', 'time', 'timestamp'])),
  }))
}

// -----------------------------
// 并行调用所有 API
// -----------------------------
const fetchData = async () => {
  loading.value = true
  try {
    const [overviewRes, flowsRes, topologyRes, alertsRes, agentsRes] = await Promise.allSettled([
      queryService.getOverview(),
      queryService.getFlows({ limit: 24 }),
      queryService.getTopology(),
      alertService.getAlerts({ status: 'active', limit: 5 }),
      controlPlaneService.getAgents(),
    ])

    if (overviewRes.status === 'fulfilled') processOverview(overviewRes.value)
    if (flowsRes.status === 'fulfilled') processFlows(flowsRes.value)
    if (topologyRes.status === 'fulfilled') processTopology(topologyRes.value)
    if (alertsRes.status === 'fulfilled') processAlerts(alertsRes.value)

    // agent 数量会覆盖 overview 中的活跃 agent
    if (agentsRes.status === 'fulfilled' && agentsRes.value) {
      const agents = Array.isArray(agentsRes.value)
        ? agentsRes.value
        : pick(agentsRes.value, ['agents', 'data', 'items']) || []
      if (Array.isArray(agents) && kpis.value[1]) {
        kpis.value[1].value = agents.length.toString()
      }
    }
  } catch (e) {
    console.warn('Dashboard fetch error:', e)
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  fetchData()
})
</script>
