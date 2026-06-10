<template>
  <div class="space-y-6">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold text-slate-900 dark:text-white">容器指标</h1>
        <p class="text-sm text-slate-500 dark:text-slate-400 mt-1">监控容器性能指标</p>
      </div>
      <button class="btn-secondary" @click="fetchData" :disabled="loading">
        <RefreshCw class="w-4 h-4" :class="{ 'animate-spin': loading }" />
        刷新
      </button>
    </div>

    <div class="grid grid-cols-2 md:grid-cols-5 gap-4">
      <div class="card p-4">
        <div class="flex items-center justify-between">
          <div>
            <p class="text-sm text-slate-500">容器总数</p>
            <p class="text-2xl font-bold text-slate-900 dark:text-white mt-1">{{ totalContainers }}</p>
          </div>
          <div class="w-10 h-10 rounded-xl bg-primary-50 dark:bg-primary-500/10 flex items-center justify-center">
            <Box class="w-5 h-5 text-primary-500" />
          </div>
          </div>
      </div>
      <div class="card p-4">
        <div class="flex items-center justify-between">
          <div>
            <p class="text-sm text-slate-500">平均CPU</p>
            <p class="text-2xl font-bold text-primary-500 mt-1">{{ avgCpu }}%</p>
          </div>
          <div class="w-10 h-10 rounded-xl bg-accent-50 dark:bg-accent-500/10 flex items-center justify-center">
            <Cpu class="w-5 h-5 text-accent-500" />
          </div>
        </div>
      </div>
      <div class="card p-4">
        <div class="flex items-center justify-between">
          <div>
            <p class="text-sm text-slate-500">平均内存</p>
            <p class="text-2xl font-bold text-accent-500 mt-1">{{ avgMemory }}%</p>
          </div>
          <div class="w-10 h-10 rounded-xl bg-emerald-50 dark:bg-emerald-500/10 flex items-center justify-center">
            <Database class="w-5 h-5 text-emerald-500" />
          </div>
          </div>
      </div>
      <div class="card p-4">
        <div class="flex items-center justify-between">
          <div>
            <p class="text-sm text-slate-500">总重启次数</p>
            <p class="text-2xl font-bold text-amber-500 mt-1">{{ totalRestarts }}</p>
          </div>
          <div class="w-10 h-10 rounded-xl bg-amber-50 dark:bg-amber-500/10 flex items-center justify-center">
            <RotateCcw class="w-5 h-5 text-amber-500" />
          </div>
          </div>
      </div>
      <div class="card p-4">
        <div class="flex items-center justify-between">
          <div>
            <p class="text-sm text-slate-500">异常容器</p>
            <p class="text-2xl font-bold text-red-500 mt-1">{{ unhealthyCount }}</p>
          </div>
          <div class="w-10 h-10 rounded-xl bg-red-50 dark:bg-red-500/10 flex items-center justify-center">
            <AlertTriangle class="w-5 h-5 text-red-500" />
          </div>
        </div>
      </div>
    </div>

    <div class="card p-6">
      <h3 class="text-lg font-semibold text-slate-900 dark:text-white mb-4">容器资源使用</h3>
      <div v-if="loading" class="h-64 flex items-center justify-center">
        <p class="text-sm text-slate-400 dark:text-slate-500">加载中...</p>
      </div>
      <div v-else-if="!containersForChart.length" class="h-64 flex items-center justify-center">
        <p class="text-sm text-slate-400 dark:text-slate-500">暂无数据</p>
      </div>
      <div v-else class="h-64">
        <ECharts :option="containerOption" class="w-full h-full" />
      </div>
    </div>

    <div class="card p-6">
      <h3 class="text-lg font-semibold text-slate-900 dark:text-white mb-4">容器列表</h3>
      <div v-if="loading" class="space-y-3">
        <div v-for="i in 3" :key="i" class="h-16 bg-slate-100 dark:bg-dark-700 rounded-lg animate-pulse"></div>
      </div>
      <div v-else-if="!containers.length" class="h-40 flex items-center justify-center">
        <p class="text-sm text-slate-400 dark:text-slate-500">暂无数据</p>
      </div>
      <div v-else class="overflow-x-auto">
        <table class="w-full text-sm">
          <thead>
            <tr class="text-left text-slate-500 dark:text-slate-400 border-b border-slate-200 dark:border-dark-700">
              <th class="pb-3 pr-4 font-medium">容器名</th>
              <th class="pb-3 pr-4 font-medium">所属Pod</th>
              <th class="pb-3 pr-4 font-medium">所属服务</th>
              <th class="pb-3 pr-4 font-medium">CPU%</th>
              <th class="pb-3 pr-4 font-medium">内存%</th>
              <th class="pb-3 pr-4 font-medium">重启次数</th>
              <th class="pb-3 font-medium">状态</th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="c in containers"
              :key="c.key"
              class="border-b border-slate-100 dark:border-dark-700 hover:bg-slate-50 dark:hover:bg-dark-700/50"
            >
              <td class="py-3 pr-4 font-medium text-slate-900 dark:text-white">{{ c.name }}</td>
              <td class="py-3 pr-4 text-slate-600 dark:text-slate-300">{{ c.pod }}</td>
              <td class="py-3 pr-4 text-slate-600 dark:text-slate-300">{{ c.service }}</td>
              <td class="py-3 pr-4">
                <span :class="c.cpu >= 80 ? 'text-red-500' : c.cpu >= 60 ? 'text-amber-500' : 'text-emerald-500'">
                  {{ c.cpu }}%
                </span>
              </td>
              <td class="py-3 pr-4">
                <span :class="c.memory >= 80 ? 'text-red-500' : c.memory >= 60 ? 'text-amber-500' : 'text-emerald-500'">
                  {{ c.memory }}%
                </span>
              </td>
              <td class="py-3 pr-4 text-slate-600 dark:text-slate-300">{{ c.restarts }}</td>
              <td class="py-3">
                <span
                  :class="[
                    'text-xs px-2 py-0.5 rounded-full font-medium',
                    c.status === 'running' ? 'bg-emerald-100 text-emerald-600 dark:bg-emerald-500/20 dark:text-emerald-400' :
                    c.status === 'warning' ? 'bg-amber-100 text-amber-600 dark:bg-amber-500/20 dark:text-amber-400' :
                    c.status === 'error' ? 'bg-red-100 text-red-600 dark:bg-red-500/20 dark:text-red-400' :
                    'bg-slate-100 text-slate-600 dark:bg-slate-500/20 dark:text-slate-400'
                  ]"
                >
                  {{ c.statusText }}
                </span>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { use } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { BarChart } from 'echarts/charts'
import { TooltipComponent, GridComponent, LegendComponent } from 'echarts/components'
import ECharts from 'vue-echarts'
import { Box, Cpu, Database, RotateCcw, AlertTriangle, RefreshCw } from 'lucide-vue-next'
import { queryService } from '../../../api'

use([CanvasRenderer, BarChart, TooltipComponent, GridComponent, LegendComponent])

const loading = ref(true)
const containers = ref([])

const pick = (obj, keys) => {
  if (!obj) return null
  for (const k of keys) {
    if (obj[k] !== undefined && obj[k] !== null) return obj[k]
  }
  return null
}

const normalizeStatus = (s) => {
  if (!s) return 'running'
  const str = String(s).toLowerCase()
  if (str.includes('error') || str.includes('fail') || str.includes('crash') || str.includes('故障')) return 'error'
  if (str.includes('warn') || str.includes('restart') || str.includes('告警')) return 'warning'
  if (str.includes('run') || str.includes('running') || str.includes('正常') || str.includes('运行')) return 'running'
  return str
}

const totalContainers = computed(() => containers.value.length)

const avgCpu = computed(() => {
  if (!containers.value.length) return 0
  const sum = containers.value.reduce((acc, c) => acc + c.cpu, 0)
  return Math.round(sum / containers.value.length)
})

const avgMemory = computed(() => {
  if (!containers.value.length) return 0
  const sum = containers.value.reduce((acc, c) => acc + c.memory, 0)
  return Math.round(sum / containers.value.length)
})

const totalRestarts = computed(() => containers.value.reduce((acc, c) => acc + c.restarts, 0))

const unhealthyCount = computed(() => containers.value.filter((c) => c.status !== 'running').length)

const containersForChart = computed(() => containers.value.slice(0, 10))

const containerOption = computed(() => ({
  tooltip: { trigger: 'axis', backgroundColor: 'rgba(255,255,255,0.95)', textStyle: { color: '#1e293b' } },
  legend: { bottom: 0, textStyle: { color: '#64748b' } },
  grid: { left: '3%', right: '4%', bottom: '15%', top: '10%', containLabel: true },
  xAxis: {
    type: 'category',
    data: containersForChart.value.map((c) => c.name),
    axisLabel: { color: '#64748b', rotate: containersForChart.value.length > 6 ? 30 : 0, fontSize: 11 },
    axisLine: { lineStyle: { color: '#e2e8f0' } },
  },
  yAxis: {
    type: 'value',
    axisLabel: { color: '#64748b' },
    splitLine: { lineStyle: { color: '#f1f5f9' } },
  },
  series: [
    { name: 'CPU%', type: 'bar', itemStyle: { color: '#2563eb' }, data: containersForChart.value.map((c) => c.cpu) },
    { name: '内存%', type: 'bar', itemStyle: { color: '#14b8a6' }, data: containersForChart.value.map((c) => c.memory) },
  ],
}))

const parseContainersList = (raw) => {
  if (!raw) return []
  const list = Array.isArray(raw) ? raw : pick(raw, ['containers', 'data', 'items', 'results', 'pods']) || []
  if (!Array.isArray(list)) return []

  return list.map((c, idx) => {
    const rawStatus = pick(c, ['status', 'state', 'phase', 'health']) || 'running'
    const status = normalizeStatus(rawStatus)
    const statusText = status === 'running' ? '运行中' : status === 'warning' ? '告警' : status === 'error' ? '异常' : '未知'
    return {
      key: pick(c, ['id', 'container_id', 'containerId', 'name']) || `container-${idx}`,
      name: pick(c, ['container_name', 'container', 'name']) || 'Unknown',
      pod: pick(c, ['pod_name', 'pod', 'podName']) || '-',
      service: pick(c, ['service_name', 'service', 'serviceName']) || '-',
      cpu: Math.round(Number(pick(c, ['cpu_usage', 'cpuUsage', 'cpu', 'cpu_percent']) || 0)),
      memory: Math.round(Number(pick(c, ['memory_usage', 'memoryUsage', 'mem', 'memory_percent']) || 0)),
      restarts: Number(pick(c, ['restart_count', 'restartCount', 'restarts', 'restart']) || 0),
      status,
      statusText,
    }
  })
}

const fetchData = async () => {
  loading.value = true
  try {
    const [metricsRes, overviewRes] = await Promise.allSettled([
      queryService.getMetrics({ limit: 100, type: 'container' }),
      queryService.getOverview(),
    ])

    const metrics = metricsRes.status === 'fulfilled' ? metricsRes.value : null
    const overview = overviewRes.status === 'fulfilled' ? overviewRes.value : null

    const listFromMetrics = metrics ? parseContainersList(metrics) : []
    const listFromOverview = overview ? parseContainersList(pick(overview, ['containers', 'pods', 'data'])) : []

    if (listFromMetrics.length) {
      containers.value = listFromMetrics
    } else if (listFromOverview.length) {
      containers.value = listFromOverview
    } else {
      containers.value = []
    }
  } catch (err) {
    console.warn('MetricsContainer fetch error:', err)
    containers.value = []
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  fetchData()
})
</script>
