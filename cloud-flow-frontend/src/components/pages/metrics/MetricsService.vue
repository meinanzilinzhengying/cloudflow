<template>
  <div class="space-y-6">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold text-slate-900 dark:text-white">服务指标</h1>
        <p class="text-sm text-slate-500 dark:text-slate-400 mt-1">监控服务性能指标</p>
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
            <p class="text-sm text-slate-500">服务总数</p>
            <p class="text-2xl font-bold text-slate-900 dark:text-white mt-1">{{ totalServices }}</p>
          </div>
          <div class="w-10 h-10 rounded-xl bg-primary-50 dark:bg-primary-500/10 flex items-center justify-center">
            <Layers class="w-5 h-5 text-primary-500" />
          </div>
        </div>
      </div>
      <div class="card p-4">
        <div class="flex items-center justify-between">
          <div>
            <p class="text-sm text-slate-500">实例总数</p>
            <p class="text-2xl font-bold text-primary-500 mt-1">{{ totalInstances }}</p>
          </div>
          <div class="w-10 h-10 rounded-xl bg-accent-50 dark:bg-accent-500/10 flex items-center justify-center">
            <Server class="w-5 h-5 text-accent-500" />
          </div>
        </div>
      </div>
      <div class="card p-4">
        <div class="flex items-center justify-between">
          <div>
            <p class="text-sm text-slate-500">平均QPS</p>
            <p class="text-2xl font-bold text-emerald-500 mt-1">{{ avgQps }}</p>
          </div>
          <div class="w-10 h-10 rounded-xl bg-emerald-50 dark:bg-emerald-500/10 flex items-center justify-center">
            <Zap class="w-5 h-5 text-emerald-500" />
          </div>
        </div>
      </div>
      <div class="card p-4">
        <div class="flex items-center justify-between">
          <div>
            <p class="text-sm text-slate-500">平均错误率</p>
            <p class="text-2xl font-bold text-amber-500 mt-1">{{ avgErrorRate }}%</p>
          </div>
          <div class="w-10 h-10 rounded-xl bg-amber-50 dark:bg-amber-500/10 flex items-center justify-center">
            <AlertTriangle class="w-5 h-5 text-amber-500" />
          </div>
        </div>
      </div>
      <div class="card p-4">
        <div class="flex items-center justify-between">
          <div>
            <p class="text-sm text-slate-500">平均延迟</p>
            <p class="text-2xl font-bold text-violet-500 mt-1">{{ avgLatency }}ms</p>
          </div>
          <div class="w-10 h-10 rounded-xl bg-violet-50 dark:bg-violet-500/10 flex items-center justify-center">
            <Clock class="w-5 h-5 text-violet-500" />
          </div>
        </div>
      </div>
    </div>

    <div class="card p-6">
      <h3 class="text-lg font-semibold text-slate-900 dark:text-white mb-4">服务QPS趋势</h3>
      <div v-if="loading" class="h-64 flex items-center justify-center">
        <p class="text-sm text-slate-400 dark:text-slate-500">加载中...</p>
      </div>
      <div v-else-if="!servicesForChart.length" class="h-64 flex items-center justify-center">
        <p class="text-sm text-slate-400 dark:text-slate-500">暂无数据</p>
      </div>
      <div v-else class="h-64">
        <ECharts :option="serviceOption" class="w-full h-full" />
      </div>
    </div>

    <div class="card p-6">
      <h3 class="text-lg font-semibold text-slate-900 dark:text-white mb-4">服务列表</h3>
      <div v-if="loading" class="space-y-3">
        <div v-for="i in 3" :key="i" class="h-16 bg-slate-100 dark:bg-dark-700 rounded-lg animate-pulse"></div>
      </div>
      <div v-else-if="!services.length" class="h-40 flex items-center justify-center">
        <p class="text-sm text-slate-400 dark:text-slate-500">暂无数据</p>
      </div>
      <div v-else class="overflow-x-auto">
        <table class="w-full text-sm">
          <thead>
            <tr class="text-left text-slate-500 dark:text-slate-400 border-b border-slate-200 dark:border-dark-700">
              <th class="pb-3 pr-4 font-medium">服务名</th>
              <th class="pb-3 pr-4 font-medium">命名空间</th>
              <th class="pb-3 pr-4 font-medium">实例数</th>
              <th class="pb-3 pr-4 font-medium">QPS</th>
              <th class="pb-3 pr-4 font-medium">错误率</th>
              <th class="pb-3 font-medium">平均延迟</th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="s in services"
              :key="s.key"
              class="border-b border-slate-100 dark:border-dark-700 hover:bg-slate-50 dark:hover:bg-dark-700/50"
            >
              <td class="py-3 pr-4 font-medium text-slate-900 dark:text-white">{{ s.name }}</td>
              <td class="py-3 pr-4 text-slate-600 dark:text-slate-300">{{ s.namespace }}</td>
              <td class="py-3 pr-4 text-slate-600 dark:text-slate-300">{{ s.instances }}</td>
              <td class="py-3 pr-4 text-slate-600 dark:text-slate-300">{{ s.qps }}</td>
              <td class="py-3 pr-4">
                <span :class="s.errorRate >= 5 ? 'text-red-500' : s.errorRate >= 1 ? 'text-amber-500' : 'text-emerald-500'">
                  {{ s.errorRate }}%
                </span>
              </td>
              <td class="py-3 text-slate-600 dark:text-slate-300">{{ s.latency }}ms</td>
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
import { LineChart } from 'echarts/charts'
import { TooltipComponent, GridComponent, LegendComponent } from 'echarts/components'
import ECharts from 'vue-echarts'
import { Layers, Server, Zap, AlertTriangle, Clock, RefreshCw } from 'lucide-vue-next'
import { queryService } from '../../../api'

use([CanvasRenderer, LineChart, TooltipComponent, GridComponent, LegendComponent])

const loading = ref(true)
const services = ref([])

const pick = (obj, keys) => {
  if (!obj) return null
  for (const k of keys) {
    if (obj[k] !== undefined && obj[k] !== null) return obj[k]
  }
  return null
}

const totalServices = computed(() => services.value.length)

const totalInstances = computed(() => services.value.reduce((acc, s) => acc + s.instances, 0))

const avgQps = computed(() => {
  if (!services.value.length) return 0
  const sum = services.value.reduce((acc, s) => acc + s.qps, 0)
  return Math.round(sum / services.value.length)
})

const avgErrorRate = computed(() => {
  if (!services.value.length) return 0
  const sum = services.value.reduce((acc, s) => acc + s.errorRate, 0)
  return (sum / services.value.length).toFixed(1)
})

const avgLatency = computed(() => {
  if (!services.value.length) return 0
  const sum = services.value.reduce((acc, s) => acc + s.latency, 0)
  return Math.round(sum / services.value.length)
})

const servicesForChart = computed(() => services.value.slice(0, 5))

const palette = ['#2563eb', '#14b8a6', '#f59e0b', '#22c55e', '#8b5cf6', '#ef4444']

const serviceOption = computed(() => ({
  tooltip: { trigger: 'axis', backgroundColor: 'rgba(255,255,255,0.95)', textStyle: { color: '#1e293b' } },
  legend: { bottom: 0, textStyle: { color: '#64748b' } },
  grid: { left: '3%', right: '4%', bottom: '15%', top: '10%', containLabel: true },
  xAxis: {
    type: 'category',
    data: servicesForChart.value.map((s) => s.name),
    axisLabel: { color: '#64748b', rotate: servicesForChart.value.length > 4 ? 30 : 0, fontSize: 11 },
    axisLine: { lineStyle: { color: '#e2e8f0' } },
  },
  yAxis: {
    type: 'value',
    axisLabel: { color: '#64748b' },
    splitLine: { lineStyle: { color: '#f1f5f9' } },
  },
  series: servicesForChart.value.map((s, idx) => {
    const color = palette[idx % palette.length]
    return {
      name: s.name,
      type: 'line',
      smooth: true,
      lineStyle: { color, width: 2 },
      itemStyle: { color },
      data: s.series && s.series.length ? s.series : [s.qps],
    }
  }),
}))

const parseServicesList = (raw) => {
  if (!raw) return []
  const list = Array.isArray(raw) ? raw : pick(raw, ['services', 'data', 'items', 'results']) || []
  if (!Array.isArray(list)) return []

  return list.map((s, idx) => {
    const seriesRaw = pick(s, ['qps_series', 'series', 'history', 'trend']) || []
    const series = Array.isArray(seriesRaw) ? seriesRaw : []
    return {
      key: pick(s, ['id', 'service_id', 'serviceId', 'name']) || `service-${idx}`,
      name: pick(s, ['service_name', 'service', 'name']) || 'Unknown',
      namespace: pick(s, ['namespace', 'ns']) || 'default',
      instances: Number(pick(s, ['instance_count', 'instanceCount', 'instances', 'replicas']) || 0),
      qps: Math.round(Number(pick(s, ['qps', 'rps', 'request_rate', 'requests_per_second']) || 0)),
      errorRate: Number(pick(s, ['error_rate', 'errorRate', 'error_ratio']) || 0),
      latency: Math.round(Number(pick(s, ['avg_latency', 'avgLatency', 'latency', 'latency_ms', 'response_time']) || 0)),
      series,
    }
  })
}

const fetchData = async () => {
  loading.value = true
  try {
    const [metricsRes, overviewRes] = await Promise.allSettled([
      queryService.getMetrics({ limit: 100, type: 'service' }),
      queryService.getOverview(),
    ])

    const metrics = metricsRes.status === 'fulfilled' ? metricsRes.value : null
    const overview = overviewRes.status === 'fulfilled' ? overviewRes.value : null

    const listFromMetrics = metrics ? parseServicesList(metrics) : []
    const listFromOverview = overview ? parseServicesList(pick(overview, ['services', 'data'])) : []

    if (listFromMetrics.length) {
      services.value = listFromMetrics
    } else if (listFromOverview.length) {
      services.value = listFromOverview
    } else {
      services.value = []
    }
  } catch (err) {
    console.warn('MetricsService fetch error:', err)
    services.value = []
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  fetchData()
})
</script>
