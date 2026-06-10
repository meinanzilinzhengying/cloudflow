<template>
  <div class="space-y-6">
    <!-- Page Header -->
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold text-slate-900 dark:text-white">服务地图</h1>
        <p class="text-sm text-slate-500 dark:text-slate-400 mt-1">可视化服务依赖关系与健康状态</p>
      </div>
      <div class="flex items-center gap-3">
        <select class="input w-40">
          <option>所有 Namespace</option>
          <option>default</option>
          <option>kube-system</option>
        </select>
        <button class="btn-secondary">
          <RefreshCw class="w-4 h-4" />
          刷新
        </button>
      </div>
    </div>

    <!-- Toggle Tabs -->
    <div class="flex items-center gap-2 p-1 bg-slate-100 dark:bg-dark-700 rounded-xl w-fit">
      <button
        v-for="tab in tabs"
        :key="tab.id"
        @click="activeTab = tab.id"
        :class="[
          'px-4 py-2 text-sm font-medium rounded-lg transition-all duration-200',
          activeTab === tab.id
            ? 'bg-white dark:bg-dark-600 text-slate-900 dark:text-white shadow-sm'
            : 'text-slate-500 hover:text-slate-700 dark:text-slate-400'
        ]"
      >
        {{ tab.label }}
      </button>
    </div>

    <!-- Stats Cards -->
    <div class="grid grid-cols-3 gap-4">
      <div class="card p-4">
        <div class="flex items-center justify-between">
          <div>
            <p class="text-sm text-slate-500">服务数</p>
            <p class="text-2xl font-bold text-slate-900 dark:text-white mt-1">{{ serviceStats.total }}</p>
          </div>
          <div class="w-10 h-10 rounded-xl bg-primary-50 dark:bg-primary-500/10 flex items-center justify-center">
            <Layers class="w-5 h-5 text-primary-500" />
          </div>
        </div>
      </div>
      <div class="card p-4">
        <div class="flex items-center justify-between">
          <div>
            <p class="text-sm text-slate-500">调用数</p>
            <p class="text-2xl font-bold text-accent-500 mt-1">{{ serviceStats.calls }}K</p>
          </div>
          <div class="w-10 h-10 rounded-xl bg-accent-50 dark:bg-accent-500/10 flex items-center justify-center">
            <Activity class="w-5 h-5 text-accent-500" />
          </div>
        </div>
      </div>
      <div class="card p-4">
        <div class="flex items-center justify-between">
          <div>
            <p class="text-sm text-slate-500">错误率</p>
            <p class="text-2xl font-bold text-emerald-500 mt-1">{{ serviceStats.errorRate }}%</p>
          </div>
          <div class="w-10 h-10 rounded-xl bg-emerald-50 dark:bg-emerald-500/10 flex items-center justify-center">
            <CheckCircle class="w-5 h-5 text-emerald-500" />
          </div>
        </div>
      </div>
    </div>

    <!-- Topology Graph -->
    <div class="card p-6">
      <div class="h-[500px]">
        <ECharts :option="topologyOption" class="w-full h-full" autoresize />
      </div>
    </div>

    <!-- Service Detail Drawer -->
    <Transition name="drawer">
      <div
        v-if="selectedService"
        class="fixed inset-0 z-50 flex justify-end"
        @click.self="selectedService = null"
      >
        <div class="absolute inset-0 bg-slate-900/50"></div>
        <div class="relative w-full max-w-md bg-white dark:bg-dark-800 shadow-2xl overflow-y-auto">
          <div class="sticky top-0 bg-white dark:bg-dark-800 border-b border-slate-200 dark:border-dark-700 px-6 py-4 flex items-center justify-between">
            <h3 class="text-lg font-semibold text-slate-900 dark:text-white">{{ selectedService.name }}</h3>
            <button @click="selectedService = null" class="p-2 hover:bg-slate-100 dark:hover:bg-dark-700 rounded-lg transition-colors">
              <X class="w-5 h-5 text-slate-500" />
            </button>
          </div>
          <div class="p-6 space-y-6">
            <!-- Status Badge -->
            <div class="flex items-center gap-3">
              <div :class="['w-3 h-3 rounded-full', getStatusColor(selectedService.status)]"></div>
              <span class="text-sm font-medium text-slate-700 dark:text-slate-200">{{ getStatusText(selectedService.status) }}</span>
            </div>

            <!-- Metrics -->
            <div class="grid grid-cols-2 gap-4">
              <div class="p-4 bg-slate-50 dark:bg-dark-700 rounded-xl">
                <p class="text-xs text-slate-500 mb-1">QPS</p>
                <p class="text-2xl font-bold text-slate-900 dark:text-white">{{ selectedService.qps }}</p>
              </div>
              <div class="p-4 bg-slate-50 dark:bg-dark-700 rounded-xl">
                <p class="text-xs text-slate-500 mb-1">错误率</p>
                <p class="text-2xl font-bold" :class="selectedService.errorRate > 5 ? 'text-red-500' : 'text-emerald-500'">{{ selectedService.errorRate }}%</p>
              </div>
              <div class="p-4 bg-slate-50 dark:bg-dark-700 rounded-xl">
                <p class="text-xs text-slate-500 mb-1">延迟</p>
                <p class="text-2xl font-bold text-slate-900 dark:text-white">{{ selectedService.latency }}ms</p>
              </div>
              <div class="p-4 bg-slate-50 dark:bg-dark-700 rounded-xl">
                <p class="text-xs text-slate-500 mb-1">P99延迟</p>
                <p class="text-2xl font-bold text-slate-900 dark:text-white">{{ selectedService.p99Latency }}ms</p>
              </div>
            </div>

            <!-- Top Traces -->
            <div>
              <h4 class="text-sm font-semibold text-slate-700 dark:text-slate-200 mb-3">Top Trace</h4>
              <div class="space-y-2">
                <div v-for="trace in selectedService.topTraces" :key="trace.id" class="p-3 bg-slate-50 dark:bg-dark-700 rounded-lg">
                  <div class="flex items-center justify-between mb-1">
                    <span class="text-sm font-medium text-slate-700 dark:text-slate-200">{{ trace.name }}</span>
                    <span :class="['text-xs px-2 py-0.5 rounded-full', trace.status === 'error' ? 'bg-red-100 text-red-600' : 'bg-green-100 text-green-600']">
                      {{ trace.status }}
                    </span>
                  </div>
                  <p class="text-xs text-slate-500">{{ trace.duration }}ms · {{ trace.spans }} spans</p>
                </div>
              </div>
            </div>

            <!-- Recent Alerts -->
            <div>
              <h4 class="text-sm font-semibold text-slate-700 dark:text-slate-200 mb-3">最近告警</h4>
              <div class="space-y-2">
                <div v-for="alert in selectedService.recentAlerts" :key="alert.id" class="p-3 bg-amber-50 dark:bg-amber-500/10 rounded-lg border border-amber-200 dark:border-amber-500/20">
                  <div class="flex items-center gap-2 mb-1">
                    <AlertTriangle class="w-4 h-4 text-amber-500" />
                    <span class="text-sm font-medium text-amber-800 dark:text-amber-300">{{ alert.title }}</span>
                  </div>
                  <p class="text-xs text-amber-600 dark:text-amber-400">{{ alert.time }}</p>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </Transition>
  </div>
</template>

<script setup>
import { ref, computed } from 'vue'
import { use } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { GraphChart } from 'echarts/charts'
import { TooltipComponent, LegendComponent } from 'echarts/components'
import ECharts from 'vue-echarts'
import { Layers, Activity, CheckCircle, X, AlertTriangle } from 'lucide-vue-next'

use([CanvasRenderer, GraphChart, TooltipComponent, LegendComponent])

const activeTab = ref('service')

const tabs = [
  { id: 'service', label: 'Service' },
  { id: 'pod', label: 'Pod' },
  { id: 'process', label: 'Process' },
  { id: 'namespace', label: 'Namespace' },
]

const serviceStats = ref({
  total: 12,
  calls: 156,
  errorRate: 2.3,
})

const selectedService = ref(null)

const topologyOption = computed(() => ({
  tooltip: {
    trigger: 'item',
    backgroundColor: 'rgba(255,255,255,0.95)',
    borderColor: 'rgba(0,0,0,0.1)',
    textStyle: { color: '#1e293b' },
    formatter: (params) => {
      if (params.dataType === 'node') {
        return `<div class="font-medium">${params.name}</div><div class="text-xs text-gray-500">${params.data.status}</div>`
      }
      return ''
    },
  },
  series: [{
    type: 'graph',
    layout: 'force',
    symbolSize: 50,
    roam: true,
    draggable: true,
    label: { show: true, position: 'bottom', fontSize: 12, color: '#64748b' },
    lineStyle: { width: 2, color: '#cbd5e1', curveness: 0.2 },
    emphasis: {
      focus: 'adjacency',
      lineStyle: { width: 3, color: '#2563eb' },
    },
    force: {
      repulsion: 400,
      gravity: 0.1,
      edgeLength: [80, 200],
    },
    data: [
      { name: 'API Gateway', status: '正常', symbol: 'roundRect', itemStyle: { color: '#22c55e' }, category: 0 },
      { name: 'User Service', status: '正常', symbol: 'roundRect', itemStyle: { color: '#22c55e' }, category: 1 },
      { name: 'Order Service', status: '告警', symbol: 'roundRect', itemStyle: { color: '#f59e0b' }, category: 1 },
      { name: 'Payment Service', status: '正常', symbol: 'roundRect', itemStyle: { color: '#22c55e' }, category: 1 },
      { name: 'Product Service', status: '正常', symbol: 'roundRect', itemStyle: { color: '#22c55e' }, category: 1 },
      { name: 'Inventory Service', status: '故障', symbol: 'roundRect', itemStyle: { color: '#ef4444' }, category: 1 },
      { name: 'Database', status: '正常', symbol: 'diamond', itemStyle: { color: '#2563eb' }, category: 2 },
      { name: 'Redis', status: '正常', symbol: 'diamond', itemStyle: { color: '#14b8a6' }, category: 2 },
      { name: 'Kafka', status: '正常', symbol: 'diamond', itemStyle: { color: '#8b5cf6' }, category: 2 },
    ],
    links: [
      { source: 'API Gateway', target: 'User Service' },
      { source: 'API Gateway', target: 'Order Service' },
      { source: 'API Gateway', target: 'Product Service' },
      { source: 'User Service', target: 'Database' },
      { source: 'User Service', target: 'Redis' },
      { source: 'Order Service', target: 'Payment Service' },
      { source: 'Order Service', target: 'Inventory Service' },
      { source: 'Order Service', target: 'Database' },
      { source: 'Order Service', target: 'Kafka' },
      { source: 'Product Service', target: 'Redis' },
      { source: 'Product Service', target: 'Database' },
      { source: 'Payment Service', target: 'Database' },
      { source: 'Inventory Service', target: 'Database' },
    ],
  }],
}))

const getStatusColor = (status) => {
  const colors = { normal: 'bg-green-500', warning: 'bg-amber-500', error: 'bg-red-500', offline: 'bg-gray-400' }
  return colors[status] || colors.normal
}

const getStatusText = (status) => {
  const texts = { normal: '运行正常', warning: '存在告警', error: '服务故障', offline: '离线' }
  return texts[status] || '未知'
}

// Mock selected service data
selectedService.value = null

// When clicking on a node in the chart, we'd set selectedService
const handleNodeClick = (service) => {
  selectedService.value = {
    name: service.name,
    status: 'normal',
    qps: 12500,
    errorRate: 2.3,
    latency: 45,
    p99Latency: 156,
    topTraces: [
      { id: 1, name: 'Checkout', status: 'success', duration: 123, spans: 8 },
      { id: 2, name: 'GetUser', status: 'success', duration: 45, spans: 3 },
      { id: 3, name: 'CreateOrder', status: 'error', duration: 890, spans: 12 },
    ],
    recentAlerts: [
      { id: 1, title: '延迟增加警告', time: '5分钟前' },
    ],
  }
}
</script>

<style scoped>
.drawer-enter-active,
.drawer-leave-active {
  transition: all 0.3s ease;
}
.drawer-enter-from,
.drawer-leave-to {
  opacity: 0;
}
.drawer-enter-from > div:last-child,
.drawer-leave-to > div:last-child {
  transform: translateX(100%);
}
</style>
