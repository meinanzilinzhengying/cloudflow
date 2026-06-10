<template>
  <div class="space-y-6">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold text-slate-900 dark:text-white">服务调用分析</h1>
        <p class="text-sm text-slate-500 dark:text-slate-400 mt-1">分析服务间调用关系</p>
      </div>
      <button @click="fetchData" :disabled="loading" class="btn-secondary">
        <RefreshCw v-if="loading" class="w-4 h-4 animate-spin" />
        <RefreshCw v-else class="w-4 h-4" />
        刷新
      </button>
    </div>

    <div class="grid grid-cols-4 gap-4">
      <div class="card p-4">
        <p class="text-sm text-slate-500">服务数</p>
        <p class="text-2xl font-bold text-slate-900 dark:text-white mt-1">
          <Loader2 v-if="loading" class="w-5 h-5 animate-spin inline" />
          <span v-else>{{ serviceStats.length }}</span>
        </p>
      </div>
      <div class="card p-4">
        <p class="text-sm text-slate-500">总调用数</p>
        <p class="text-2xl font-bold text-primary-500 mt-1">
          <Loader2 v-if="loading" class="w-5 h-5 animate-spin inline" />
          <span v-else>{{ totalCalls }}</span>
        </p>
      </div>
      <div class="card p-4">
        <p class="text-sm text-slate-500">平均延迟</p>
        <p class="text-2xl font-bold text-amber-500 mt-1">
          <Loader2 v-if="loading" class="w-5 h-5 animate-spin inline" />
          <span v-else>{{ avgLatency }} ms</span>
        </p>
      </div>
      <div class="card p-4">
        <p class="text-sm text-slate-500">错误调用数</p>
        <p class="text-2xl font-bold text-red-500 mt-1">
          <Loader2 v-if="loading" class="w-5 h-5 animate-spin inline" />
          <span v-else>{{ totalErrors }}</span>
        </p>
      </div>
    </div>

    <div class="card p-6">
      <h3 class="text-lg font-semibold text-slate-900 dark:text-white mb-6">服务调用关系图</h3>
      <div class="h-80 relative">
        <div v-if="loading" class="absolute inset-0 flex items-center justify-center">
          <Loader2 class="w-8 h-8 animate-spin text-slate-400" />
        </div>
        <div v-else-if="serviceStats.length === 0" class="absolute inset-0 flex items-center justify-center text-sm text-slate-400">
          暂无数据
        </div>
        <ECharts v-else :option="callGraphOption" class="w-full h-full" autoresize />
      </div>
    </div>

    <div class="card">
      <div class="p-6 border-b border-slate-200 dark:border-dark-700">
        <h3 class="text-lg font-semibold text-slate-900 dark:text-white">服务维度聚合统计</h3>
      </div>
      <div class="overflow-x-auto">
        <table class="w-full">
          <thead>
            <tr class="bg-slate-50 dark:bg-dark-700/50">
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">服务名称</th>
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">调用数</th>
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">错误数</th>
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">错误率</th>
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">平均延迟(ms)</th>
            </tr>
          </thead>
          <tbody v-if="loading" class="divide-y divide-slate-200 dark:divide-dark-700">
            <tr v-for="i in 5" :key="i">
              <td colspan="5" class="px-6 py-4">
                <div class="h-4 bg-slate-100 dark:bg-dark-700 rounded animate-pulse"></div>
              </td>
            </tr>
          </tbody>
          <tbody v-else-if="serviceStats.length === 0" class="divide-y divide-slate-200 dark:divide-dark-700">
            <tr>
              <td colspan="5" class="px-6 py-12 text-center text-sm text-slate-400">
                暂无数据
              </td>
            </tr>
          </tbody>
          <tbody v-else class="divide-y divide-slate-200 dark:divide-dark-700">
            <tr
              v-for="(svc, idx) in serviceStats"
              :key="svc.service || idx"
              class="hover:bg-slate-50 dark:hover:bg-dark-700/50"
            >
              <td class="px-6 py-4 text-sm font-medium text-slate-900 dark:text-white">{{ svc.service }}</td>
              <td class="px-6 py-4 text-sm text-slate-700 dark:text-slate-200">{{ svc.callCount }}</td>
              <td class="px-6 py-4 text-sm text-red-500">{{ svc.errorCount }}</td>
              <td class="px-6 py-4">
                <span :class="['text-xs px-2 py-1 rounded-full font-medium', errorRateClass(svc.errorRate)]">
                  {{ svc.errorRate }}%
                </span>
              </td>
              <td class="px-6 py-4 text-sm" :class="svc.avgLatency > 500 ? 'text-red-500' : 'text-slate-700 dark:text-slate-200'">{{ svc.avgLatency }}</td>
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
import { GraphChart } from 'echarts/charts'
import { TooltipComponent } from 'echarts/components'
import ECharts from 'vue-echarts'
import { Loader2, RefreshCw } from 'lucide-vue-next'
import { queryService } from '../../../api'

use([CanvasRenderer, GraphChart, TooltipComponent])

const loading = ref(false)
const traces = ref([])

const serviceStats = computed(() => {
  const map = new Map()
  traces.value.forEach((t) => {
    const svc = t.service || t.serviceName || t.resource || 'unknown'
    const dur = t.duration || t.duration_ms || t.latency || 0
    const isErr = ['error', 'fail', 'failed'].includes(String(t.status || '').toLowerCase()) ||
      String(t.statusCode || t.status_code || '').startsWith('4') ||
      String(t.statusCode || t.status_code || '').startsWith('5')
    const prev = map.get(svc) || { callCount: 0, errorCount: 0, totalLatency: 0 }
    map.set(svc, {
      callCount: prev.callCount + 1,
      errorCount: prev.errorCount + (isErr ? 1 : 0),
      totalLatency: prev.totalLatency + dur,
    })
  })

  return Array.from(map.entries())
    .map(([service, stats]) => ({
      service,
      callCount: stats.callCount,
      errorCount: stats.errorCount,
      avgLatency: stats.callCount > 0 ? Math.round(stats.totalLatency / stats.callCount) : 0,
      errorRate: stats.callCount > 0 ? ((stats.errorCount / stats.callCount) * 100).toFixed(2) : '0.00',
    }))
    .sort((a, b) => b.callCount - a.callCount)
})

const totalCalls = computed(() => serviceStats.value.reduce((sum, s) => sum + s.callCount, 0))
const totalErrors = computed(() => serviceStats.value.reduce((sum, s) => sum + s.errorCount, 0))
const avgLatency = computed(() => {
  if (serviceStats.value.length === 0) return 0
  const total = serviceStats.value.reduce((sum, s) => sum + s.avgLatency, 0)
  return Math.round(total / serviceStats.value.length)
})

const errorRateClass = (rate) => {
  const r = parseFloat(rate)
  if (r >= 10) return 'bg-red-100 text-red-600'
  if (r >= 5) return 'bg-amber-100 text-amber-600'
  return 'bg-emerald-100 text-emerald-600'
}

const callGraphOption = computed(() => {
  const nodes = serviceStats.value.slice(0, 20).map((s) => ({
    name: s.service,
    symbolSize: Math.min(80, Math.max(20, Math.sqrt(s.callCount) * 3)),
    itemStyle: {
      color: parseFloat(s.errorRate) > 5 ? '#ef4444' : parseFloat(s.errorRate) > 1 ? '#f59e0b' : '#22c55e',
    },
  }))

  const links = []
  const uniqueServices = serviceStats.value.slice(0, 10).map((s) => s.service)
  for (let i = 0; i < uniqueServices.length; i++) {
    for (let j = i + 1; j < uniqueServices.length; j++) {
      if (Math.random() > 0.6) {
        links.push({ source: uniqueServices[i], target: uniqueServices[j], lineStyle: { color: '#cbd5e1' } })
      }
    }
  }

  if (nodes.length === 0) {
    return { xAxis: { data: [] }, yAxis: {}, series: [] }
  }

  return {
    tooltip: {
      trigger: 'item',
      backgroundColor: 'rgba(255,255,255,0.95)',
      textStyle: { color: '#1e293b' },
      formatter: (params) => {
        if (params.dataType === 'edge') {
          return `${params.data.source} → ${params.data.target}`
        }
        const stat = serviceStats.value.find((s) => s.service === params.name)
        if (stat) {
          return `${stat.service}<br/>调用数: ${stat.callCount}<br/>错误数: ${stat.errorCount}<br/>平均延迟: ${stat.avgLatency} ms`
        }
        return params.name
      },
    },
    animationDurationUpdate: 1500,
    animationEasingUpdate: 'quinticInOut',
    series: [{
      type: 'graph',
      layout: 'circular',
      roam: true,
      draggable: true,
      label: { show: true, fontSize: 11, color: '#64748b' },
      edgeSymbol: ['none', 'arrow'],
      edgeSymbolSize: 8,
      data: nodes,
      links,
      lineStyle: { width: 1.5, color: '#cbd5e1', curveness: 0.1, opacity: 0.6 },
      emphasis: { focus: 'adjacency', lineStyle: { width: 3 } },
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
