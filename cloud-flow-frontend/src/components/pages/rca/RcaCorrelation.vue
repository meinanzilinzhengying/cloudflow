<template>
  <div class="space-y-6">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold text-slate-900 dark:text-white">事件关联</h1>
        <p class="text-sm text-slate-500 dark:text-slate-400 mt-1">关联分析相关事件</p>
      </div>
    </div>

    <div class="card p-6">
      <div v-if="loading" class="flex items-center justify-center h-80">
        <Loader2 class="w-8 h-8 text-primary-500 animate-spin" />
      </div>

      <div v-else-if="correlations.length === 0" class="flex flex-col items-center justify-center h-80 text-slate-500">
        <Inbox class="w-12 h-12 mb-3 text-slate-300" />
        <p>暂无数据</p>
      </div>

      <div v-else class="h-80">
        <ECharts :option="correlationOption" class="w-full h-full" autoresize />
      </div>
    </div>

    <div v-if="!loading && correlations.length > 0" class="card">
      <div class="p-4 border-b border-slate-200 dark:border-dark-700">
        <h3 class="text-sm font-semibold text-slate-700 dark:text-slate-200">关联事件列表</h3>
      </div>
      <div class="overflow-x-auto">
        <table class="w-full">
          <thead>
            <tr class="bg-slate-50 dark:bg-dark-700/50">
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500">源事件</th>
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500">目标事件</th>
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500">关联强度</th>
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500">时间</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-slate-200 dark:divide-dark-700">
            <tr v-for="(c, idx) in correlations" :key="idx">
              <td class="px-6 py-3 text-sm text-slate-900 dark:text-white">{{ c.source || c.from || '-' }}</td>
              <td class="px-6 py-3 text-sm text-slate-900 dark:text-white">{{ c.target || c.to || '-' }}</td>
              <td class="px-6 py-3">
                <div class="flex items-center gap-2">
                  <div class="w-20 h-1.5 bg-slate-200 dark:bg-dark-600 rounded-full overflow-hidden">
                    <div class="h-full bg-primary-500" :style="{ width: `${(c.score || c.confidence || c.strength || 0) * 100}%` }"></div>
                  </div>
                  <span class="text-xs text-slate-500">{{ ((c.score || c.confidence || c.strength || 0) * 100).toFixed(0) }}%</span>
                </div>
              </td>
              <td class="px-6 py-3 text-xs text-slate-500">{{ c.time || c.timestamp || '-' }}</td>
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
import { Loader2, Inbox } from 'lucide-vue-next'
import { queryService } from '../../../api'

use([CanvasRenderer, GraphChart, TooltipComponent])

const loading = ref(false)
const correlations = ref([])
const nodes = ref([])
const links = ref([])

const palette = ['#3b82f6', '#8b5cf6', '#14b8a6', '#f59e0b', '#ef4444', '#22c55e', '#06b6d4', '#ec4899']

const correlationOption = computed(() => {
  const nodeList = nodes.value.length > 0 ? nodes.value : []
  const linkList = links.value.length > 0 ? links.value : []

  return {
    tooltip: {
      trigger: 'item',
      backgroundColor: 'rgba(255,255,255,0.95)',
      textStyle: { color: '#1e293b' },
    },
    series: [{
      type: 'graph',
      layout: 'force',
      symbolSize: (data) => data.size || 40,
      roam: true,
      label: { show: true, fontSize: 12, color: '#64748b' },
      lineStyle: { width: 2, color: '#cbd5e1', curveness: 0.2 },
      emphasis: { focus: 'adjacency' },
      force: {
        repulsion: 300,
        gravity: 0.1,
        edgeLength: [80, 150],
      },
      data: nodeList,
      links: linkList,
    }],
  }
})

const buildGraph = (list) => {
  const nodeSet = new Map()
  const builtLinks = []

  list.forEach((item) => {
    const source = item.source || item.from || item.event_a || 'A'
    const target = item.target || item.to || item.event_b || 'B'
    const score = Number(item.score || item.confidence || item.strength || 0.5)

    if (!nodeSet.has(source)) {
      nodeSet.set(source, { name: source, size: 45, itemStyle: { color: palette[nodeSet.size % palette.length] } })
    }
    if (!nodeSet.has(target)) {
      nodeSet.set(target, { name: target, size: 45, itemStyle: { color: palette[nodeSet.size % palette.length] } })
    }
    builtLinks.push({ source, target, value: score })
  })

  nodes.value = Array.from(nodeSet.values())
  links.value = builtLinks
}

const fetchData = async () => {
  loading.value = true
  try {
    const data = await queryService.getCorrelation({ limit: 20 })
    const list = Array.isArray(data) ? data : (data.data || data.items || data.results || data.correlations || [])
    correlations.value = list
    buildGraph(list)
  } catch (err) {
    correlations.value = []
    nodes.value = []
    links.value = []
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  fetchData()
})
</script>
