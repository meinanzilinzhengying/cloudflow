<template>
  <div class="space-y-6">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold text-slate-900 dark:text-white">错误请求分析</h1>
        <p class="text-sm text-slate-500 dark:text-slate-400 mt-1">识别和分析错误请求</p>
      </div>
      <button @click="fetchData" :disabled="loading" class="btn-secondary">
        <RefreshCw v-if="loading" class="w-4 h-4 animate-spin" />
        <RefreshCw v-else class="w-4 h-4" />
        刷新
      </button>
    </div>

    <div class="grid grid-cols-3 gap-4">
      <div class="card p-4">
        <p class="text-sm text-slate-500">总请求数</p>
        <p class="text-2xl font-bold text-slate-900 dark:text-white mt-1">
          <Loader2 v-if="loading" class="w-5 h-5 animate-spin inline" />
          <span v-else>{{ traces.length }}</span>
        </p>
      </div>
      <div class="card p-4">
        <p class="text-sm text-slate-500">错误请求数</p>
        <p class="text-2xl font-bold text-red-500 mt-1">
          <Loader2 v-if="loading" class="w-5 h-5 animate-spin inline" />
          <span v-else>{{ errorTraces.length }}</span>
        </p>
      </div>
      <div class="card p-4">
        <p class="text-sm text-slate-500">错误率</p>
        <p class="text-2xl font-bold text-amber-500 mt-1">
          <Loader2 v-if="loading" class="w-5 h-5 animate-spin inline" />
          <span v-else>{{ errorRate }}%</span>
        </p>
      </div>
    </div>

    <div class="card p-6">
      <h3 class="text-lg font-semibold text-slate-900 dark:text-white mb-6">错误分布</h3>
      <div class="h-64 relative">
        <div v-if="loading" class="absolute inset-0 flex items-center justify-center">
          <Loader2 class="w-8 h-8 animate-spin text-slate-400" />
        </div>
        <div v-else-if="errorTraces.length === 0" class="absolute inset-0 flex items-center justify-center text-sm text-slate-400">
          暂无数据
        </div>
        <ECharts v-else :option="errorOption" class="w-full h-full" autoresize />
      </div>
    </div>

    <div class="card">
      <div class="p-6 border-b border-slate-200 dark:border-dark-700">
        <h3 class="text-lg font-semibold text-slate-900 dark:text-white">错误请求列表</h3>
      </div>
      <div class="overflow-x-auto">
        <table class="w-full">
          <thead>
            <tr class="bg-slate-50 dark:bg-dark-700/50">
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">服务</th>
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">操作</th>
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">状态码</th>
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">延迟(ms)</th>
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
          <tbody v-else-if="errorTraces.length === 0" class="divide-y divide-slate-200 dark:divide-dark-700">
            <tr>
              <td colspan="5" class="px-6 py-12 text-center text-sm text-slate-400">
                暂无数据
              </td>
            </tr>
          </tbody>
          <tbody v-else class="divide-y divide-slate-200 dark:divide-dark-700">
            <tr
              v-for="(trace, idx) in errorTraces"
              :key="trace.id || idx"
              class="hover:bg-slate-50 dark:hover:bg-dark-700/50"
            >
              <td class="px-6 py-4 text-sm text-slate-900 dark:text-white">{{ trace.service || trace.serviceName || '-' }}</td>
              <td class="px-6 py-4 text-sm text-slate-700 dark:text-slate-200">{{ trace.operation || trace.name || '-' }}</td>
              <td class="px-6 py-4">
                <span class="text-xs px-2 py-1 rounded-full font-medium bg-red-100 text-red-600">
                  {{ trace.statusCode || trace.status_code || trace.status || 'error' }}
                </span>
              </td>
              <td class="px-6 py-4 text-sm text-slate-500">{{ trace.duration || trace.duration_ms || 0 }}</td>
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
import { PieChart } from 'echarts/charts'
import { TooltipComponent, LegendComponent } from 'echarts/components'
import ECharts from 'vue-echarts'
import { Loader2, RefreshCw } from 'lucide-vue-next'
import { queryService } from '../../../api'

use([CanvasRenderer, PieChart, TooltipComponent, LegendComponent])

const loading = ref(false)
const traces = ref([])

const isError = (t) => {
  const status = String(t.status || t.statusCode || t.status_code || 'ok').toLowerCase()
  return status === 'error' || status === 'fail' || status === 'failed' || status.startsWith('5') || status.startsWith('4')
}

const errorTraces = computed(() => traces.value.filter(isError).slice(0, 50))

const errorRate = computed(() => {
  if (traces.value.length === 0) return '0.00'
  const rate = (errorTraces.value.length / traces.value.length) * 100
  return rate.toFixed(2)
})

const formatTime = (ts) => {
  if (!ts) return '-'
  const d = new Date(ts)
  if (isNaN(d.getTime())) return String(ts)
  return d.toLocaleString('zh-CN', { hour12: false })
}

const errorOption = computed(() => {
  const statusMap = new Map()
  errorTraces.value.forEach((t) => {
    const key = String(t.statusCode || t.status_code || t.status || 'error')
    statusMap.set(key, (statusMap.get(key) || 0) + 1)
  })

  const data = Array.from(statusMap.entries()).map(([name, value]) => ({ name, value }))
  if (data.length === 0) {
    return { xAxis: { data: [] }, yAxis: {}, series: [] }
  }

  const colors = ['#ef4444', '#f59e0b', '#8b5cf6', '#94a3b8', '#1e40af', '#0ea5e9']
  return {
    tooltip: {
      trigger: 'item',
      backgroundColor: 'rgba(255,255,255,0.95)',
      textStyle: { color: '#1e293b' },
      formatter: (params) => `${params.name}: ${params.value} (${params.percent}%)`,
    },
    legend: { bottom: 0, textStyle: { color: '#64748b' } },
    series: [{
      type: 'pie',
      radius: ['40%', '70%'],
      itemStyle: { borderRadius: 8, borderColor: '#fff', borderWidth: 2 },
      label: { show: false },
      emphasis: { label: { show: true, fontSize: 14, fontWeight: 'bold' } },
      data: data.map((d, i) => ({ ...d, itemStyle: { color: colors[i % colors.length] } })),
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
