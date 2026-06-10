<template>
  <div class="space-y-6">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold text-slate-900 dark:text-white">日志聚合</h1>
        <p class="text-sm text-slate-500 dark:text-slate-400 mt-1">聚合分析日志数据</p>
      </div>
    </div>

    <div v-if="loading" class="card p-6">
      <div class="flex items-center justify-center py-20">
        <div class="text-center">
          <RefreshCw class="w-8 h-8 text-primary-500 animate-spin mx-auto mb-3" />
          <p class="text-slate-500 dark:text-slate-400">加载中...</p>
        </div>
      </div>
    </div>

    <template v-else>
      <div class="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <div class="card p-6">
          <h3 class="text-lg font-semibold text-slate-900 dark:text-white mb-4">服务日志分布</h3>
          <div v-if="serviceData.length === 0" class="h-64 flex items-center justify-center">
            <p class="text-slate-500 dark:text-slate-400">暂无日志数据</p>
          </div>
          <div v-else class="h-64">
            <ECharts :option="servicePieOption" class="w-full h-full" />
          </div>
        </div>

        <div class="card p-6">
          <h3 class="text-lg font-semibold text-slate-900 dark:text-white mb-4">日志级别分布</h3>
          <div v-if="levelData.length === 0" class="h-64 flex items-center justify-center">
            <p class="text-slate-500 dark:text-slate-400">暂无日志数据</p>
          </div>
          <div v-else class="h-64">
            <ECharts :option="levelPieOption" class="w-full h-full" />
          </div>
        </div>
      </div>

      <div class="card p-6">
        <h3 class="text-lg font-semibold text-slate-900 dark:text-white mb-4">各服务日志量柱状图</h3>
        <div v-if="serviceData.length === 0" class="h-64 flex items-center justify-center">
          <p class="text-slate-500 dark:text-slate-400">暂无日志数据</p>
        </div>
        <div v-else class="h-64">
          <ECharts :option="serviceBarOption" class="w-full h-full" />
        </div>
      </div>

      <div class="card p-6">
        <h3 class="text-lg font-semibold text-slate-900 dark:text-white mb-4">聚合统计表</h3>
        <div v-if="aggregateStats.length === 0" class="py-10 text-center">
          <p class="text-slate-500 dark:text-slate-400">暂无日志数据</p>
        </div>
        <div v-else class="overflow-x-auto">
          <table class="w-full text-sm">
            <thead>
              <tr class="border-b border-slate-200 dark:border-slate-700">
                <th class="px-4 py-3 text-left font-medium text-slate-500 dark:text-slate-400">服务</th>
                <th class="px-4 py-3 text-right font-medium text-slate-500 dark:text-slate-400">日志数</th>
                <th class="px-4 py-3 text-right font-medium text-slate-500 dark:text-slate-400">错误数</th>
                <th class="px-4 py-3 text-right font-medium text-slate-500 dark:text-slate-400">错误率</th>
                <th class="px-4 py-3 text-left font-medium text-slate-500 dark:text-slate-400">状态</th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="item in aggregateStats"
                :key="item.service"
                class="border-b border-slate-100 dark:border-slate-800 hover:bg-slate-50 dark:hover:bg-slate-800/50"
              >
                <td class="px-4 py-3 text-slate-900 dark:text-white font-medium">{{ item.service }}</td>
                <td class="px-4 py-3 text-right text-slate-700 dark:text-slate-300">{{ item.total }}</td>
                <td class="px-4 py-3 text-right text-red-600 dark:text-red-400">{{ item.errors }}</td>
                <td class="px-4 py-3 text-right text-slate-700 dark:text-slate-300">{{ item.errorRate }}%</td>
                <td class="px-4 py-3">
                  <span
                    :class="[
                      'px-2 py-1 text-xs rounded-full',
                      item.errorRate >= 10
                        ? 'bg-red-100 text-red-700 dark:bg-red-500/20 dark:text-red-400'
                        : item.errorRate >= 3
                          ? 'bg-amber-100 text-amber-700 dark:bg-amber-500/20 dark:text-amber-400'
                          : 'bg-green-100 text-green-700 dark:bg-green-500/20 dark:text-green-400'
                    ]"
                  >
                    {{ item.errorRate >= 10 ? '高风险' : item.errorRate >= 3 ? '警告' : '正常' }}
                  </span>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </template>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { RefreshCw } from 'lucide-vue-next'
import { use } from 'echarts/core'
import { PieChart, BarChart } from 'echarts/charts'
import { TooltipComponent, LegendComponent, GridComponent, TitleComponent } from 'echarts/components'
import ECharts from 'vue-echarts'
import { queryService } from '../../../api'

use([PieChart, BarChart, TooltipComponent, LegendComponent, GridComponent, TitleComponent])

const loading = ref(false)
const rawLogs = ref([])

const SERVICE_COLORS = ['#3b82f6', '#10b981', '#8b5cf6', '#f59e0b', '#ec4899', '#06b6d4', '#ef4444', '#6b7280']

const LEVEL_COLORS = {
  ERROR: '#ef4444',
  WARN: '#f59e0b',
  WARNING: '#f59e0b',
  INFO: '#22c55e',
  DEBUG: '#3b82f6',
}

const normalizeLog = (log) => {
  const message = log.message || log.content || log.log || log.body || ''
  const levelRaw = log.level || log.severity || 'INFO'
  const level = typeof levelRaw === 'string' ? levelRaw.toUpperCase() : String(levelRaw).toUpperCase()
  const service = log.service || log.service_name || log.hostname || 'unknown'
  const timestamp = log.timestamp || log.time || log.created_at || new Date().toISOString()
  return { message, level, service, timestamp }
}

const fetchData = async () => {
  loading.value = true
  try {
    const response = await queryService.getOTELLogs({ limit: 200 })
    let logs = []
    if (Array.isArray(response)) {
      logs = response
    } else if (Array.isArray(response?.data)) {
      logs = response.data
    } else if (Array.isArray(response?.logs)) {
      logs = response.logs
    } else if (Array.isArray(response?.result)) {
      logs = response.result
    }
    rawLogs.value = logs.map(normalizeLog)
  } catch (error) {
    console.error('Failed to fetch logs:', error)
    rawLogs.value = []
  } finally {
    loading.value = false
  }
}

const serviceData = computed(() => {
  const map = new Map()
  rawLogs.value.forEach(log => {
    map.set(log.service, (map.get(log.service) || 0) + 1)
  })
  return Array.from(map.entries())
    .map(([name, value]) => ({ name, value }))
    .sort((a, b) => b.value - a.value)
})

const levelData = computed(() => {
  const map = new Map()
  rawLogs.value.forEach(log => {
    map.set(log.level, (map.get(log.level) || 0) + 1)
  })
  return Array.from(map.entries()).map(([name, value]) => ({
    name,
    value,
    itemStyle: { color: LEVEL_COLORS[name] || '#6b7280' }
  }))
})

const aggregateStats = computed(() => {
  const stats = {}
  rawLogs.value.forEach(log => {
    if (!stats[log.service]) {
      stats[log.service] = { service: log.service, total: 0, errors: 0 }
    }
    stats[log.service].total += 1
    if (log.level === 'ERROR') {
      stats[log.service].errors += 1
    }
  })
  return Object.values(stats)
    .map(s => ({
      ...s,
      errorRate: s.total === 0 ? 0 : Number(((s.errors / s.total) * 100).toFixed(2))
    }))
    .sort((a, b) => b.total - a.total)
})

const servicePieOption = computed(() => ({
  tooltip: { trigger: 'item', backgroundColor: 'rgba(255,255,255,0.95)', textStyle: { color: '#1e293b' } },
  legend: { bottom: 0, textStyle: { color: '#94a3b8' }, type: 'scroll' },
  series: [{
    type: 'pie',
    radius: ['40%', '70%'],
    itemStyle: { borderRadius: 8, borderColor: '#fff', borderWidth: 2 },
    label: { show: false },
    emphasis: { label: { show: true, fontSize: 14, fontWeight: 'bold' } },
    data: serviceData.value.map((item, idx) => ({
      ...item,
      itemStyle: { color: SERVICE_COLORS[idx % SERVICE_COLORS.length] }
    })),
  }],
}))

const levelPieOption = computed(() => ({
  tooltip: { trigger: 'item', backgroundColor: 'rgba(255,255,255,0.95)', textStyle: { color: '#1e293b' } },
  legend: { bottom: 0, textStyle: { color: '#94a3b8' } },
  series: [{
    type: 'pie',
    radius: ['40%', '70%'],
    itemStyle: { borderRadius: 8, borderColor: '#fff', borderWidth: 2 },
    label: { show: false },
    emphasis: { label: { show: true, fontSize: 14, fontWeight: 'bold' } },
    data: levelData.value,
  }],
}))

const serviceBarOption = computed(() => ({
  tooltip: { trigger: 'axis', backgroundColor: 'rgba(255,255,255,0.95)', textStyle: { color: '#1e293b' } },
  grid: { left: 40, right: 20, top: 30, bottom: 60, containLabel: true },
  xAxis: {
    type: 'category',
    data: serviceData.value.map(d => d.name),
    axisLabel: { color: '#94a3b8', rotate: serviceData.value.length > 5 ? 30 : 0 },
    axisLine: { lineStyle: { color: '#475569' } }
  },
  yAxis: {
    type: 'value',
    axisLabel: { color: '#94a3b8' },
    splitLine: { lineStyle: { color: 'rgba(71,85,105,0.3)' } }
  },
  series: [{
    type: 'bar',
    data: serviceData.value.map((d, idx) => ({
      value: d.value,
      itemStyle: { color: SERVICE_COLORS[idx % SERVICE_COLORS.length], borderRadius: [6, 6, 0, 0] }
    })),
    barWidth: serviceData.value.length > 10 ? '50%' : '60%'
  }]
}))

onMounted(() => {
  fetchData()
})
</script>
