<template>
  <div class="space-y-6">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold text-slate-900 dark:text-white">Pod拓扑</h1>
        <p class="text-sm text-slate-500 dark:text-slate-400 mt-1">Pod级别的依赖关系</p>
      </div>
      <button @click="fetchData" :disabled="loading" class="btn-secondary">
        <RefreshCw v-if="loading" class="w-4 h-4 animate-spin" />
        <RefreshCw v-else class="w-4 h-4" />
        刷新
      </button>
    </div>

    <div class="grid grid-cols-4 gap-4 mb-4">
      <div class="card p-4">
        <p class="text-sm text-slate-500">节点数</p>
        <p class="text-2xl font-bold text-slate-900 dark:text-white mt-1">
          <Loader2 v-if="loading" class="w-5 h-5 animate-spin inline" />
          <span v-else>{{ nodes.length }}</span>
        </p>
      </div>
      <div class="card p-4">
        <p class="text-sm text-slate-500">连接数</p>
        <p class="text-2xl font-bold text-primary-500 mt-1">
          <Loader2 v-if="loading" class="w-5 h-5 animate-spin inline" />
          <span v-else>{{ links.length }}</span>
        </p>
      </div>
      <div class="card p-4">
        <p class="text-sm text-slate-500">正常节点</p>
        <p class="text-2xl font-bold text-emerald-500 mt-1">
          <Loader2 v-if="loading" class="w-5 h-5 animate-spin inline" />
          <span v-else>{{ healthyCount }}</span>
        </p>
      </div>
      <div class="card p-4">
        <p class="text-sm text-slate-500">告警节点</p>
        <p class="text-2xl font-bold text-amber-500 mt-1">
          <Loader2 v-if="loading" class="w-5 h-5 animate-spin inline" />
          <span v-else>{{ warningCount }}</span>
        </p>
      </div>
    </div>

    <div class="card p-6">
      <div class="h-[500px] relative">
        <div v-if="loading" class="absolute inset-0 flex items-center justify-center">
          <Loader2 class="w-8 h-8 animate-spin text-slate-400" />
        </div>
        <div v-else-if="nodes.length === 0" class="absolute inset-0 flex items-center justify-center text-sm text-slate-400">
          暂无数据
        </div>
        <ECharts v-else :option="podTopologyOption" class="w-full h-full" autoresize />
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { use } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { GraphChart } from 'echarts/charts'
import { TooltipComponent, LegendComponent } from 'echarts/components'
import ECharts from 'vue-echarts'
import { Loader2, RefreshCw } from 'lucide-vue-next'
import { queryService } from '../../../api'

use([CanvasRenderer, GraphChart, TooltipComponent, LegendComponent])

const loading = ref(false)
const topologyData = ref({ nodes: [], links: [] })

const nodes = computed(() => topologyData.value.nodes || [])
const links = computed(() => topologyData.value.links || [])

const healthyCount = computed(() => nodes.value.filter((n) => n.status === 'healthy' || !n.status).length)
const warningCount = computed(() => nodes.value.filter((n) => n.status === 'warning' || n.status === 'error' || n.status === 'offline').length)

const statusColor = (status) => {
  switch (status) {
    case 'warning':
    case 'warn':
      return '#f59e0b'
    case 'error':
    case 'critical':
      return '#ef4444'
    case 'offline':
    case 'down':
      return '#94a3b8'
    case 'healthy':
    case 'ok':
    case 'normal':
    default:
      return '#22c55e'
  }
}

const podTopologyOption = computed(() => {
  const chartNodes = nodes.value.map((n) => ({
    name: n.name || n.id || n.label || 'Unknown',
    id: n.id || n.name,
    itemStyle: { color: statusColor(n.status) },
    symbolSize: n.size || 30,
    category: n.namespace || n.group || 'default',
  }))

  const categories = Array.from(new Set(chartNodes.map((n) => n.category))).map((c) => ({ name: c }))

  const chartLinks = links.value.map((l) => ({
    source: l.source || l.src || l.from,
    target: l.target || l.dst || l.to,
    lineStyle: {
      width: l.value ? Math.min(5, Math.max(1, l.value / 10)) : 1.5,
      color: l.status === 'error' ? '#ef4444' : '#cbd5e1',
    },
  }))

  return {
    tooltip: {
      trigger: 'item',
      backgroundColor: 'rgba(255,255,255,0.95)',
      textStyle: { color: '#1e293b' },
      formatter: (params) => {
        if (params.dataType === 'edge') {
          return `${params.data.source} → ${params.data.target}`
        }
        return params.name
      },
    },
    legend: [{
      data: categories.map((c) => c.name),
      textStyle: { color: '#64748b' },
    }],
    animationDurationUpdate: 1500,
    animationEasingUpdate: 'quinticInOut',
    series: [{
      type: 'graph',
      layout: 'force',
      roam: true,
      draggable: true,
      label: { show: true, position: 'bottom', fontSize: 11, color: '#64748b' },
      force: { repulsion: 400, edgeLength: [80, 200] },
      edgeSymbol: ['none', 'arrow'],
      edgeSymbolSize: 8,
      data: chartNodes,
      links: chartLinks,
      categories,
      lineStyle: { width: 1.5, color: '#cbd5e1', curveness: 0.1, opacity: 0.7 },
      emphasis: { focus: 'adjacency', lineStyle: { width: 3 } },
    }],
  }
})

const fetchData = async () => {
  loading.value = true
  try {
    const res = await queryService.getTopology({ type: 'pod' }).catch(() => null)
    const data = res?.data || res?.topology || res || { nodes: [], links: [] }
    topologyData.value = {
      nodes: Array.isArray(data.nodes) ? data.nodes : [],
      links: Array.isArray(data.links) ? data.links : [],
    }
  } catch {
    topologyData.value = { nodes: [], links: [] }
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  fetchData()
})
</script>
