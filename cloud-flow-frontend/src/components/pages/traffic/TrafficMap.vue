<template>
  <div class="space-y-6">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold text-slate-900 dark:text-white">流量地图</h1>
        <p class="text-sm text-slate-500 dark:text-slate-400 mt-1">可视化网络流量分布与IP间通信关系</p>
      </div>
      <div class="flex items-center gap-3">
        <select v-model="filterProtocol" class="input w-28">
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
        <p class="text-2xl font-bold text-slate-900 dark:text-white mt-1">{{ nodeCount }}</p>
      </div>
      <div class="card p-4">
        <p class="text-sm text-slate-500">连接数</p>
        <p class="text-2xl font-bold text-primary-500 mt-1">{{ edgeCount }}</p>
      </div>
      <div class="card p-4">
        <p class="text-sm text-slate-500">总流量</p>
        <p class="text-2xl font-bold text-accent-500 mt-1">{{ totalTraffic }}</p>
      </div>
      <div class="card p-4">
        <p class="text-sm text-slate-500">最大流量连接</p>
        <p class="text-lg font-bold text-emerald-500 mt-1 truncate">{{ maxEdgeLabel }}</p>
      </div>
    </div>

    <div class="card p-6">
      <div class="flex items-center justify-between mb-4">
        <h3 class="text-lg font-semibold text-slate-900 dark:text-white">IP通信拓扑图</h3>
        <div class="flex items-center gap-4 text-xs text-slate-500">
          <span class="flex items-center gap-1"><span class="w-3 h-3 rounded-full bg-blue-500"></span>TCP</span>
          <span class="flex items-center gap-1"><span class="w-3 h-3 rounded-full bg-emerald-500"></span>UDP</span>
          <span class="flex items-center gap-1"><span class="w-3 h-3 rounded-full bg-amber-500"></span>ICMP</span>
        </div>
      </div>
      <div class="h-[600px]">
        <div v-if="loading" class="flex items-center justify-center h-full text-slate-400">加载中...</div>
        <ECharts v-else :option="graphOption" class="w-full h-full" />
      </div>
    </div>

    <div class="card p-6">
      <h3 class="text-lg font-semibold text-slate-900 dark:text-white mb-4">连接明细</h3>
      <div class="overflow-x-auto">
        <table class="w-full">
          <thead>
            <tr class="bg-slate-50 dark:bg-dark-700/50">
              <th class="text-left px-4 py-3 text-xs font-semibold text-slate-500 uppercase">源IP</th>
              <th class="text-left px-4 py-3 text-xs font-semibold text-slate-500 uppercase">目的IP</th>
              <th class="text-left px-4 py-3 text-xs font-semibold text-slate-500 uppercase">协议</th>
              <th class="text-right px-4 py-3 text-xs font-semibold text-slate-500 uppercase">流量</th>
              <th class="text-right px-4 py-3 text-xs font-semibold text-slate-500 uppercase">包数</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-slate-200 dark:divide-dark-700">
            <tr v-for="(e, i) in topEdges" :key="i" class="hover:bg-slate-50 dark:hover:bg-dark-700/50">
              <td class="px-4 py-3 text-sm text-slate-900 dark:text-white">{{ e.src }}</td>
              <td class="px-4 py-3 text-sm text-slate-900 dark:text-white">{{ e.dst }}</td>
              <td class="px-4 py-3"><span class="text-xs px-2 py-1 rounded-full font-medium" :class="e.proto === 'TCP' ? 'bg-blue-100 text-blue-600' : 'bg-green-100 text-green-600'">{{ e.proto }}</span></td>
              <td class="px-4 py-3 text-sm text-right text-slate-500">{{ e.bytes }}</td>
              <td class="px-4 py-3 text-sm text-right text-slate-500">{{ e.packets }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import * as echarts from 'echarts'
import { use } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { GraphChart } from 'echarts/charts'
import { TooltipComponent } from 'echarts/components'
import VChart from 'vue-echarts'
import { RefreshCw, Globe } from 'lucide-vue-next'

use([CanvasRenderer, GraphChart, TooltipComponent])

const flows = ref([])
const loading = ref(true)
const filterProtocol = ref('')
const graphData = ref({ nodes: [], links: [] })

const fetchData = async () => {
  loading.value = true
  try {
    const res = await fetch('/api/query/flows?limit=2000')
    if (res.ok) {
      const data = await res.json()
      flows.value = data.records || []
      buildGraph()
    }
  } catch(e) { console.error(e) }
  loading.value = false
}

const buildGraph = () => {
  const edgeMap = {}  // key: "src|dst"
  const nodeSet = new Set()
  
  let list = flows.value
  if (filterProtocol.value) {
    list = list.filter(f => f.protocol === filterProtocol.value)
  }
  
  list.forEach(f => {
    const src = f.src_ip || '?'
    const dst = f.dst_ip || '?'
    nodeSet.add(src)
    nodeSet.add(dst)
    const key = src + '|' + dst
    if (!edgeMap[key]) edgeMap[key] = { bytes: 0, packets: 0, proto: f.protocol }
    edgeMap[key].bytes += f.bytes || 0
    edgeMap[key].packets += f.packets || 0
  })
  
  // Sort edges by bytes desc
  const edges = Object.entries(edgeMap).sort((a, b) => b[1].bytes - a[1].bytes)
  const topEdges = edges.slice(0, 50)  // Top 50 edges for the graph
  
  // Assign colors by traffic volume
  const maxBytes = topEdges.length ? topEdges[0][1].bytes : 1
  
  const links = topEdges.map(([key, v]) => {
    const [src, dst] = key.split('|')
    const width = Math.max(1, (v.bytes / maxBytes) * 8)
    const color = v.proto === 'TCP' ? '#3b82f6' : v.proto === 'UDP' ? '#10b981' : '#f59e0b'
    return { source: src, target: dst, value: v.bytes, lineStyle: { width, color, opacity: 0.6, curveness: 0.2 }, label: { show: false } }
  })
  
  // Build nodes from active nodes in top edges
  const activeNodes = new Set()
  links.forEach(l => { activeNodes.add(l.source); activeNodes.add(l.target) })
  
  const nodeArray = Array.from(activeNodes)
  const totalBytes = nodeArray.reduce((s, ip) => {
    let bytes = 0
    topEdges.forEach(([key, v]) => { if (key.startsWith(ip + '|') || key.endsWith('|' + ip)) bytes += v.bytes })
    return s + bytes
  }, 0)
  
  const nodes = nodeArray.map(ip => {
    let bytes = 0
    topEdges.forEach(([key, v]) => { if (key.startsWith(ip + '|') || key.endsWith('|' + ip)) bytes += v.bytes })
    const symbolSize = Math.max(20, Math.min(60, (bytes / (totalBytes / Math.max(1, nodeArray.length))) * 15 + 20))
    return { name: ip, symbolSize, itemStyle: { color: '#6366f1' }, label: { show: true, fontSize: 10 } }
  })
  
  graphData.value = { nodes, links }
}

const nodeCount = computed(() => graphData.value.nodes.length)
const edgeCount = computed(() => graphData.value.links.length)

const totalTraffic = computed(() => {
  const t = graphData.value.links.reduce((s, l) => s + (l.value || 0), 0)
  if (t >= 1e9) return (t / 1e9).toFixed(2) + ' GB'
  if (t >= 1e6) return (t / 1e6).toFixed(2) + ' MB'
  if (t >= 1e3) return (t / 1e3).toFixed(2) + ' KB'
  return t + ' B'
})

const maxEdgeLabel = computed(() => {
  if (!graphData.value.links.length) return '-'
  const max = graphData.value.links.reduce((a, b) => (a.value || 0) > (b.value || 0) ? a : b)
  return (max.source || '') + ' → ' + (max.target || '')
})

const topEdges = computed(() => {
  const result = []
  graphData.value.links.forEach(l => {
    result.push({ src: l.source, dst: l.target, bytes: formatBytes(l.value), packets: Math.round(l.value / 1000 + 1), proto: l.lineStyle.color === '#3b82f6' ? 'TCP' : 'UDP' })
  })
  return result.slice(0, 20)
})

const formatBytes = (b) => {
  if (!b) return '0 B'
  if (b >= 1e9) return (b / 1e9).toFixed(2) + ' GB'
  if (b >= 1e6) return (b / 1e6).toFixed(2) + ' MB'
  if (b >= 1e3) return (b / 1e3).toFixed(2) + ' KB'
  return b + ' B'
}

const graphOption = computed(() => ({
  tooltip: { trigger: 'item', formatter: params => {
    if (params.dataType === 'node') return '<strong>' + params.name + '</strong><br/>IP节点'
    if (params.dataType === 'edge') return '<strong>' + params.data.source + ' → ' + params.data.target + '</strong><br/>流量: ' + formatBytes(params.data.value)
    return ''
  }},
  series: [{
    type: 'graph',
    layout: 'force',
    force: { repulsion: 500, edgeLength: [80, 200], gravity: 0.1, friction: 0.1 },
    roam: true,
    draggable: true,
    data: graphData.value.nodes,
    links: graphData.value.links,
    lineStyle: { color: 'source', curveness: 0.3, width: 2 },
    edgeLabel: { show: false },
    label: { show: true, position: 'bottom', fontSize: 10, color: '#64748b' },
    emphasis: { focus: 'adjacency', lineStyle: { width: 4 } },
    zoom: 1,
  }]
}))

onMounted(() => { fetchData(); setInterval(fetchData, 30000) })
</script>
