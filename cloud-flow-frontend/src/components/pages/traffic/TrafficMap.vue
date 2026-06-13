<template>
  <div class="space-y-6">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold text-slate-900 dark:text-white">流量地图</h1>
        <p class="text-sm text-slate-500 dark:text-slate-400 mt-1">可视化网络流量分布与IP间通信关系</p>
      </div>
      <div class="flex items-center gap-3">
        <select v-model="filterProtocol" @change="buildGraph" class="input w-28">
          <option value="">全部协议</option>
          <option value="TCP">TCP</option>
          <option value="UDP">UDP</option>
        </select>
        <button class="btn-secondary" @click="fetchData"><RefreshCw class="w-4 h-4" />刷新</button>
      </div>
    </div>

    <div class="grid grid-cols-4 gap-4">
      <div class="card p-4">
        <p class="text-sm text-slate-500">节点数</p>
        <p class="text-2xl font-bold text-slate-900 dark:text-white mt-1">{{ nodes.length }}</p>
      </div>
      <div class="card p-4">
        <p class="text-sm text-slate-500">连接数</p>
        <p class="text-2xl font-bold text-primary-500 mt-1">{{ links.length }}</p>
      </div>
      <div class="card p-4">
        <p class="text-sm text-slate-500">总流量</p>
        <p class="text-2xl font-bold text-accent-500 mt-1">{{ totalTraffic }}</p>
      </div>
      <div class="card p-4">
        <p class="text-sm text-slate-500">最大流量连接</p>
        <p class="text-base font-bold text-emerald-500 mt-1 truncate">{{ maxEdgeLabel }}</p>
      </div>
    </div>

    <div class="card p-6">
      <div class="flex items-center justify-between mb-4">
        <h3 class="text-lg font-semibold text-slate-900 dark:text-white">IP通信拓扑图</h3>
        <div class="flex items-center gap-4 text-xs text-slate-500">
          <span class="flex items-center gap-1"><span class="w-3 h-3 rounded-full bg-blue-500"></span>TCP</span>
          <span class="flex items-center gap-1"><span class="w-3 h-3 rounded-full bg-emerald-500"></span>UDP</span>
        </div>
      </div>
      <div class="h-[500px]">
        <div v-if="loading" class="flex items-center justify-center h-full text-slate-400">加载中...</div>
        <ECharts v-else :option="chartOption" class="w-full h-full" />
      </div>
    </div>

    <div class="card p-6">
      <h3 class="text-lg font-semibold text-slate-900 dark:text-white mb-4">连接明细</h3>
      <div class="overflow-x-auto">
        <table class="w-full">
          <thead><tr class="bg-slate-50 dark:bg-dark-700/50">
            <th class="text-left px-4 py-3 text-xs font-semibold text-slate-500 uppercase">源IP</th>
            <th class="text-left px-4 py-3 text-xs font-semibold text-slate-500 uppercase">目的IP</th>
            <th class="text-left px-4 py-3 text-xs font-semibold text-slate-500 uppercase">协议</th>
            <th class="text-right px-4 py-3 text-xs font-semibold text-slate-500 uppercase">流量</th>
          </tr></thead>
          <tbody class="divide-y divide-slate-200 dark:divide-dark-700">
            <tr v-for="(e, i) in topEdges" :key="i" class="hover:bg-slate-50 dark:hover:bg-dark-700/50">
              <td class="px-4 py-3 text-sm font-mono text-slate-900 dark:text-white">{{ e.src }}</td>
              <td class="px-4 py-3 text-sm font-mono text-slate-900 dark:text-white">{{ e.dst }}</td>
              <td class="px-4 py-3"><span class="text-xs px-2 py-1 rounded-full font-medium" :class="e.proto==='TCP'?'bg-blue-100 text-blue-600':'bg-green-100 text-green-600'">{{ e.proto }}</span></td>
              <td class="px-4 py-3 text-sm text-right text-slate-500">{{ e.bytes }}</td>
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
import VChart from 'vue-echarts'
import { RefreshCw } from 'lucide-vue-next'

use([CanvasRenderer, GraphChart, TooltipComponent])

const ECharts = VChart

const flows = ref([])
const loading = ref(true)
const filterProtocol = ref('')
const nodes = ref([])
const links = ref([])

const fetchData = async () => {
  loading.value = true
  try {
    const r = await fetch('/api/query/flows?limit=2000')
    if (r.ok) {
      const d = await r.json()
      flows.value = d.records || []
    }
  } catch(e) { console.error(e) }
  buildGraph()
  loading.value = false
}

const buildGraph = () => {
  const edgeMap = {}
  const nodeSet = new Set()
  let list = filterProtocol.value ? flows.value.filter(f => f.protocol === filterProtocol.value) : flows.value

  list.forEach(f => {
    if (!f.src_ip || !f.dst_ip) return
    nodeSet.add(f.src_ip)
    nodeSet.add(f.dst_ip)
    const key = f.src_ip + '|' + f.dst_ip
    if (!edgeMap[key]) edgeMap[key] = { bytes: 0, proto: f.protocol }
    edgeMap[key].bytes += f.bytes || 0
  })

  const sorted = Object.entries(edgeMap).sort((a, b) => b[1].bytes - a[1].bytes).slice(0, 40)
  const maxB = sorted.length ? sorted[0][1].bytes : 1

  const resultLinks = []
  const activeNodes = new Set()
  sorted.forEach(([key, v]) => {
    const [src, dst] = key.split('|')
    activeNodes.add(src)
    activeNodes.add(dst)
    resultLinks.push({
      source: src, target: dst,
      lineStyle: {
        width: Math.max(1, (v.bytes / maxB) * 8),
        color: v.proto === 'TCP' ? '#3b82f6' : '#10b981',
        curveness: 0.2, opacity: 0.7
      }
    })
  })

  const nodeArr = Array.from(activeNodes)
  const resultNodes = nodeArr.map(ip => {
    let totalB = 0
    sorted.forEach(([key, v]) => { if (key.includes(ip)) totalB += v.bytes })
    return {
      name: ip,
      symbolSize: Math.max(20, Math.min(60, (totalB / (maxB || 1)) * 30 + 20)),
      itemStyle: { color: '#6366f1' },
      label: { show: true, fontSize: 10, color: '#64748b' }
    }
  })

  nodes.value = resultNodes
  links.value = resultLinks
}

const totalTraffic = computed(() => {
  const t = links.value.reduce((s, l) => s + (l.lineStyle.width || 0) * 1000, 0)
  if (t >= 1e9) return (t / 1e9).toFixed(2) + ' GB'
  if (t >= 1e6) return (t / 1e6).toFixed(2) + ' MB'
  if (t >= 1e3) return (t / 1e3).toFixed(2) + ' KB'
  return Math.round(t) + ' B'
})

const maxEdgeLabel = computed(() => {
  if (!links.value.length) return '-'
  const max = links.value.reduce((a, b) => ((a.lineStyle?.width || 0) > (b.lineStyle?.width || 0)) ? a : b)
  return (max.source || '') + ' → ' + (max.target || '')
})

const topEdges = computed(() => {
  return links.value.slice(0, 20).map(l => ({
    src: l.source, dst: l.target,
    proto: l.lineStyle.color === '#3b82f6' ? 'TCP' : 'UDP',
    bytes: (l.lineStyle.width * 1000).toFixed(0) + ' B'
  }))
})

const chartOption = computed(() => ({
  tooltip: {
    trigger: 'item',
    formatter: params => {
      if (params.dataType === 'node') return '<strong>' + params.name + '</strong><br/>IP节点'
      if (params.dataType === 'edge') return '<strong>' + params.data.source + ' → ' + params.data.target + '</strong><br/>流量级别: ' + params.data.lineStyle.width.toFixed(1)
      return ''
    }
  },
  series: [{
    type: 'graph', layout: 'force',
    force: { repulsion: 300, edgeLength: [50, 150], gravity: 0.05, friction: 0.1 },
    roam: true, draggable: true,
    data: nodes.value,
    links: links.value,
    lineStyle: { color: 'source', curveness: 0.3 },
    label: { show: true, position: 'bottom', fontSize: 10, color: '#64748b' },
    emphasis: { focus: 'adjacency', lineStyle: { width: 4 } },
    zoom: 0.8
  }]
}))

onMounted(() => { fetchData(); setInterval(fetchData, 30000) })
</script>
