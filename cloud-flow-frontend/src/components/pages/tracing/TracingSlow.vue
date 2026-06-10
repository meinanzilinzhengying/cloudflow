<template>
  <div class="space-y-6">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold text-slate-900 dark:text-white">慢请求分析</h1>
        <p class="text-sm text-slate-500 dark:text-slate-400 mt-1">识别和分析慢请求</p>
      </div>
      <button @click="fetchData" :disabled="loading" class="btn-secondary">
        <RefreshCw v-if="loading" class="w-4 h-4 animate-spin" />
        <RefreshCw v-else class="w-4 h-4" />
        刷新
      </button>
    </div>

    <div class="grid grid-cols-4 gap-4">
      <div class="card p-4">
        <p class="text-sm text-slate-500">慢请求数</p>
        <p class="text-2xl font-bold text-red-500 mt-1">
          <Loader2 v-if="loading" class="w-5 h-5 animate-spin inline" />
          <span v-else>{{ slowTraces.length }}</span>
        </p>
      </div>
      <div class="card p-4">
        <p class="text-sm text-slate-500">平均延迟</p>
        <p class="text-2xl font-bold text-amber-500 mt-1">
          <Loader2 v-if="loading" class="w-5 h-5 animate-spin inline" />
          <span v-else>{{ stats.avgDuration }} ms</span>
        </p>
      </div>
      <div class="card p-4">
        <p class="text-sm text-slate-500">最大延迟</p>
        <p class="text-2xl font-bold text-red-500 mt-1">
          <Loader2 v-if="loading" class="w-5 h-5 animate-spin inline" />
          <span v-else>{{ stats.maxDuration }} ms</span>
        </p>
      </div>
      <div class="card p-4">
        <p class="text-sm text-slate-500">涉及服务数</p>
        <p class="text-2xl font-bold text-primary-500 mt-1">
          <Loader2 v-if="loading" class="w-5 h-5 animate-spin inline" />
          <span v-else>{{ stats.serviceCount }}</span>
        </p>
      </div>
    </div>

    <div class="card p-6">
      <h3 class="text-lg font-semibold text-slate-900 dark:text-white mb-6">慢请求分布图</h3>
      <div class="h-64 relative">
        <div v-if="loading" class="absolute inset-0 flex items-center justify-center">
          <Loader2 class="w-8 h-8 animate-spin text-slate-400" />
        </div>
        <div v-else-if="slowTraces.length === 0" class="absolute inset-0 flex items-center justify-center text-sm text-slate-400">
          暂无数据
        </div>
        <ECharts v-else :option="slowRequestOption" class="w-full h-full" autoresize />
      </div>
    </div>

    <div class="card">
      <div class="p-6 border-b border-slate-200 dark:border-dark-700">
        <h3 class="text-lg font-semibold text-slate-900 dark:text-white">慢请求 Top 列表</h3>
      </div>
      <div class="overflow-x-auto">
        <table class="w-full">
          <thead>
            <tr class="bg-slate-50 dark:bg-dark-700/50">
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">服务</th>
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">操作</th>
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">延迟(ms)</th>
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">状态</th>
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">时间</th>
            </tr>
          </thead>
          <tbody v-if="loading" class="divide-y divide-slate-200 dark:divide-dark-700">
            <tr v-for="i in 5" :key="i">
              <td colspan="5" class="px-6 py-4">
                <div class="h-4 bg-slate-100 dark:bg-dark-700 rounded animate-pulse"></div>
              </td>
            </tr>
          </tbody>
          <tbody v-else-if="slowTraces.length === 0" class="divide-y divide-slate-200 dark:divide-dark-700">
            <tr>
              <td colspan="5" class="px-6 py-12 text-center text-sm text-slate-400">
                暂无数据
              </td>
            </tr>
          </tbody>
          <tbody v-else class="divide-y divide-slate-200 dark:divide-dark-700">
            <tr
              v-for="(trace, idx) in slowTraces"
              :key="trace.id || idx"
              class="hover:bg-slate-50 dark:hover:bg-dark-700/50"
            >
              <td class="px-6 py-4 text-sm text-slate-900 dark:text-white">{{ trace.service || trace.serviceName || '-' }}</td>
              <td class="px-6 py-4 text-sm text-slate-700 dark:text-slate-200">{{ trace.operation || trace.name || '-' }}</td>
              <td class="px-6 py-4 text-sm font-medium" :class="trace.duration > 1000 ? 'text-red-500' : 'text-amber-500'">{{ trace.duration }}</td>
              <td class="px-6 py-4">
                <span :class="['text-xs px-2 py-1 rounded-full font-medium', statusClass(trace.status)]">
                  {{ trace.status || 'ok' }}
                </span>
              </td>
              <td class="px-6 py-4 text-sm text-slate-500">{{ formatTime(trace.timestamp) }}</td>
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
import { TooltipComponent, GridComponent } from 'echarts/components'
import ECharts from 'vue-echarts'
import { Loader2, RefreshCw } from 'lucide-vue-next'
import { queryService } from '../../../api'

use([CanvasRenderer, BarChart, TooltipComponent, GridComponent])

const loading = ref(false)
const traces = ref([])

const slowTraces = computed(() => {
  return traces.value
    .filter((t) => {
      const dur = t.duration || t.duration_ms || t.latency || 0
      return dur >= 500
    })
    .sort((a, b) => (b.duration || b.duration_ms || b.latency || 0) - (a.duration || a.duration_ms || a.latency || 0))
    .slice(0, 50)
})

const stats = computed(() => {
  if (slowTraces.value.length === 0) {
    return { avgDuration: 0, maxDuration: 0, serviceCount: 0 }
  }
  const durations = slowTraces.value.map((t) => t.duration || t.duration_ms || t.latency || 0)
  const services = new Set(slowTraces.value.map((t) => t.service || t.serviceName || '').filter(Boolean))
  const total = durations.reduce((sum, d) => sum + d, 0)
  return {
    avgDuration: Math.round(total / durations.length),
    maxDuration: Math.max(...durations),
    serviceCount: services.size,
  }
})

const statusClass = (status) => {
  const s = String(status || 'ok').toLowerCase()
  if (s === 'error' || s === 'fail' || s === 'failed') return 'bg-red-100 text-red-600'
  if (s === 'warning' || s === 'warn') return 'bg-amber-100 text-amber-600'
  return 'bg-emerald-100 text-emerald-600'
}

const formatTime = (ts) => {
  if (!ts) return '-'
  const d = new Date(ts)
  if (isNaN(d.getTime())) return String(ts)
  return d.toLocaleString('zh-CN', { hour12: false })
}

const slowRequestOption = computed(() => {
  const topList = slowTraces.value.slice(0, 15)
  if (topList.length === 0) {
    return { xAxis: { data: [] }, yAxis: {}, series: [] }
  }
  const labels = topList.map((t) => (t.service || t.serviceName || 'unknown') + ' - ' + (t.operation || t.name || '?'))
  const data = topList.map((t) => t.duration || t.duration_ms || t.latency || 0)

  return {
    tooltip: {
      trigger: 'axis',
      backgroundColor: 'rgba(255,255,255,0.95)',
      textStyle: { color: '#1e293b' },
      formatter: (params) => {
        const p = params[0]
        return `${p.name}<br/>延迟: ${p.value} ms`
      },
    },
    grid: { left: '3%', right: '4%', bottom: '15%', top: '10%', containLabel: true },
    xAxis: {
      type: 'category',
      data: labels,
      axisLabel: { color: '#64748b', fontSize: 10, rotate: 30 },
    },
    yAxis: {
      type: 'value',
      name: '延迟(ms)',
      axisLabel: { color: '#64748b', fontSize: 11 },
      splitLine: { lineStyle: { color: '#e2e8f0' } },
    },
    series: [{
      type: 'bar',
      data,
      itemStyle: {
        color: (params) => (params.value > 1000 ? '#ef4444' : '#f59e0b'),
        borderRadius: [4, 4, 0, 0],
      },
    }],
  }
})

const fetchData = async () => {
  loading.value = true
  try {
    const res = await queryService.getOTELTraces({ limit: 500 }).catch(() => null)
    const data = res?.data || res?.traces || res || []
    traces.value = Array.isArray(data) ? data : []
  } catch {
    traces.value = []
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  fetchData()
})
</script>
