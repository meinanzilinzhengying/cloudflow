<template>
  <div class="space-y-6">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold text-slate-900 dark:text-white">拓扑变更对比</h1>
        <p class="text-sm text-slate-500 dark:text-slate-400 mt-1">对比不同时间点的拓扑变化</p>
      </div>
      <div class="flex items-center gap-3">
        <input v-model="time1" type="datetime-local" class="input" />
        <span class="text-slate-400">→</span>
        <input v-model="time2" type="datetime-local" class="input" />
        <button @click="fetchData" :disabled="loading" class="btn-primary">
          <Loader2 v-if="loading" class="w-4 h-4 animate-spin" />
          <span v-else>对比</span>
        </button>
      </div>
    </div>

    <div class="grid grid-cols-5 gap-4">
      <div class="card p-4">
        <p class="text-sm text-slate-500">新增节点</p>
        <p class="text-2xl font-bold text-emerald-500 mt-1">
          <Loader2 v-if="loading" class="w-5 h-5 animate-spin inline" />
          <span v-else>{{ diff.addedNodes.length }}</span>
        </p>
      </div>
      <div class="card p-4">
        <p class="text-sm text-slate-500">删除节点</p>
        <p class="text-2xl font-bold text-red-500 mt-1">
          <Loader2 v-if="loading" class="w-5 h-5 animate-spin inline" />
          <span v-else>{{ diff.removedNodes.length }}</span>
        </p>
      </div>
      <div class="card p-4">
        <p class="text-sm text-slate-500">新增连接</p>
        <p class="text-2xl font-bold text-blue-500 mt-1">
          <Loader2 v-if="loading" class="w-5 h-5 animate-spin inline" />
          <span v-else>{{ diff.addedLinks.length }}</span>
        </p>
      </div>
      <div class="card p-4">
        <p class="text-sm text-slate-500">删除连接</p>
        <p class="text-2xl font-bold text-amber-500 mt-1">
          <Loader2 v-if="loading" class="w-5 h-5 animate-spin inline" />
          <span v-else>{{ diff.removedLinks.length }}</span>
        </p>
      </div>
      <div class="card p-4">
        <p class="text-sm text-slate-500">状态变更</p>
        <p class="text-2xl font-bold text-violet-500 mt-1">
          <Loader2 v-if="loading" class="w-5 h-5 animate-spin inline" />
          <span v-else>{{ diff.changedNodes.length }}</span>
        </p>
      </div>
    </div>

    <div class="card p-6">
      <div class="flex items-center justify-between mb-4">
        <h3 class="text-lg font-semibold text-slate-900 dark:text-white">差异拓扑图</h3>
        <div class="flex items-center gap-4 text-xs text-slate-500">
          <div class="flex items-center gap-1"><span class="w-3 h-3 rounded-full bg-emerald-500"></span>新增</div>
          <div class="flex items-center gap-1"><span class="w-3 h-3 rounded-full bg-red-500"></span>删除</div>
          <div class="flex items-center gap-1"><span class="w-3 h-3 rounded-full bg-amber-500"></span>状态变更</div>
          <div class="flex items-center gap-1"><span class="w-3 h-3 rounded-full bg-slate-400"></span>未变更</div>
        </div>
      </div>
      <div class="h-[500px] relative">
        <div v-if="loading" class="absolute inset-0 flex items-center justify-center">
          <Loader2 class="w-8 h-8 animate-spin text-slate-400" />
        </div>
        <div v-else-if="allNodes.length === 0" class="absolute inset-0 flex items-center justify-center text-sm text-slate-400">
          暂无数据
        </div>
        <ECharts v-else :option="diffTopologyOption" class="w-full h-full" autoresize />
      </div>
    </div>

    <div class="grid grid-cols-2 gap-6">
      <div class="card p-6">
        <h3 class="text-lg font-semibold text-slate-900 dark:text-white mb-4">节点变更详情</h3>
        <div v-if="loading" class="space-y-2">
          <div v-for="i in 3" :key="i" class="h-8 bg-slate-100 dark:bg-dark-700 rounded animate-pulse"></div>
        </div>
        <div v-else-if="nodeDiffs.length === 0" class="text-sm text-slate-400 py-4">
          暂无节点变更
        </div>
        <div v-else class="space-y-2">
          <div v-for="item in nodeDiffs" :key="item.name" class="flex items-center justify-between p-2 bg-slate-50 dark:bg-dark-700 rounded">
            <span class="text-sm text-slate-700 dark:text-slate-200">{{ item.name }}</span>
            <span :class="['text-xs px-2 py-1 rounded-full font-medium', diffBadgeClass(item.type)]">
              {{ item.typeLabel }}
            </span>
          </div>
        </div>
      </div>

      <div class="card p-6">
        <h3 class="text-lg font-semibold text-slate-900 dark:text-white mb-4">连接变更详情</h3>
        <div v-if="loading" class="space-y-2">
          <div v-for="i in 3" :key="i" class="h-8 bg-slate-100 dark:bg-dark-700 rounded animate-pulse"></div>
        </div>
        <div v-else-if="linkDiffs.length === 0" class="text-sm text-slate-400 py-4">
          暂无连接变更
        </div>
        <div v-else class="space-y-2">
          <div v-for="(item, idx) in linkDiffs" :key="idx" class="flex items-center justify-between p-2 bg-slate-50 dark:bg-dark-700 rounded">
            <span class="text-sm text-slate-700 dark:text-slate-200">{{ item.source }} → {{ item.target }}</span>
            <span :class="['text-xs px-2 py-1 rounded-full font-medium', diffBadgeClass(item.type)]">
              {{ item.typeLabel }}
            </span>
          </div>
        </div>
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
import { Loader2 } from 'lucide-vue-next'
import { queryService } from '../../../api'

use([CanvasRenderer, GraphChart, TooltipComponent])

const loading = ref(false)
const time1 = ref('')
const time2 = ref('')
const diff = ref({
  addedNodes: [],
  removedNodes: [],
  changedNodes: [],
  addedLinks: [],
  removedLinks: [],
})

const allNodes = computed(() => {
  return [
    ...diff.value.addedNodes.map((n) => ({ ...n, diffType: 'added' })),
    ...diff.value.removedNodes.map((n) => ({ ...n, diffType: 'removed' })),
    ...diff.value.changedNodes.map((n) => ({ ...n, diffType: 'changed' })),
  ]
})

const nodeDiffs = computed(() => [
  ...diff.value.addedNodes.map((n) => ({ name: n.name || n.id, type: 'added', typeLabel: '新增' })),
  ...diff.value.removedNodes.map((n) => ({ name: n.name || n.id, type: 'removed', typeLabel: '删除' })),
  ...diff.value.changedNodes.map((n) => ({ name: n.name || n.id, type: 'changed', typeLabel: `${n.oldStatus || '-'} → ${n.newStatus || '-'}` })),
])

const linkDiffs = computed(() => [
  ...diff.value.addedLinks.map((l) => ({ source: l.source || l.src, target: l.target || l.dst, type: 'added', typeLabel: '新增' })),
  ...diff.value.removedLinks.map((l) => ({ source: l.source || l.src, target: l.target || l.dst, type: 'removed', typeLabel: '删除' })),
])

const diffBadgeClass = (type) => {
  switch (type) {
    case 'added':
      return 'bg-emerald-100 text-emerald-600'
    case 'removed':
      return 'bg-red-100 text-red-600'
    case 'changed':
      return 'bg-amber-100 text-amber-600'
    default:
      return 'bg-slate-100 text-slate-600'
  }
}

const diffNodeColor = (diffType) => {
  switch (diffType) {
    case 'added':
      return '#22c55e'
    case 'removed':
      return '#ef4444'
    case 'changed':
      return '#f59e0b'
    default:
      return '#94a3b8'
  }
}

const diffTopologyOption = computed(() => {
  const chartNodes = allNodes.value.map((n) => ({
    name: n.name || n.id || 'Unknown',
    itemStyle: { color: diffNodeColor(n.diffType) },
    symbolSize: 30,
  }))

  const chartLinks = [
    ...diff.value.addedLinks.map((l) => ({
      source: l.source || l.src,
      target: l.target || l.dst,
      lineStyle: { color: '#22c55e', width: 2, curveness: 0.1 },
    })),
    ...diff.value.removedLinks.map((l) => ({
      source: l.source || l.src,
      target: l.target || l.dst,
      lineStyle: { color: '#ef4444', width: 2, curveness: 0.1, type: 'dashed' },
    })),
  ]

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
      emphasis: { focus: 'adjacency', lineStyle: { width: 3 } },
    }],
  }
})

const fetchData = async () => {
  loading.value = true
  try {
    const params = { type: 'diff' }
    if (time1.value) params.time1 = time1.value
    if (time2.value) params.time2 = time2.value
    const res = await queryService.getTopology(params).catch(() => null)
    const data = res?.data || res?.diff || res || {
      addedNodes: [],
      removedNodes: [],
      changedNodes: [],
      addedLinks: [],
      removedLinks: [],
    }
    diff.value = {
      addedNodes: Array.isArray(data.addedNodes) ? data.addedNodes : [],
      removedNodes: Array.isArray(data.removedNodes) ? data.removedNodes : [],
      changedNodes: Array.isArray(data.changedNodes) ? data.changedNodes : [],
      addedLinks: Array.isArray(data.addedLinks) ? data.addedLinks : [],
      removedLinks: Array.isArray(data.removedLinks) ? data.removedLinks : [],
    }
  } catch {
    diff.value = {
      addedNodes: [],
      removedNodes: [],
      changedNodes: [],
      addedLinks: [],
      removedLinks: [],
    }
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  fetchData()
})
</script>
