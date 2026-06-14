<template>
  <div class="space-y-6">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold text-slate-900 dark:text-white">进程拓扑</h1>
        <p class="text-sm text-slate-500 dark:text-slate-400 mt-1">基于IP通信的进程级拓扑关系</p>
      </div>
      <div class="flex items-center gap-3">
        <input v-model="searchIP" type="text" placeholder="搜索 IP..." class="input w-48" />
        <select v-model="filterProto" class="input w-24">
          <option value="">全部</option>
          <option value="TCP">TCP</option>
          <option value="UDP">UDP</option>
        </select>
        <button class="btn-secondary" @click="fetchData"><RefreshCw class="w-4 h-4" />刷新</button>
      </div>
    </div>

    <div class="grid grid-cols-4 gap-4">
      <div class="card p-4">
        <p class="text-sm text-slate-500">活跃节点</p>
        <p class="text-2xl font-bold text-slate-900 dark:text-white mt-1">{{ nodes.length }}</p>
      </div>
      <div class="card p-4">
        <p class="text-sm text-slate-500">通信连接</p>
        <p class="text-2xl font-bold text-primary-500 mt-1">{{ links.length }}</p>
      </div>
      <div class="card p-4">
        <p class="text-sm text-slate-500">源IP数</p>
        <p class="text-2xl font-bold text-accent-500 mt-1">{{ srcCount }}</p>
      </div>
      <div class="card p-4">
        <p class="text-sm text-slate-500">目的IP数</p>
        <p class="text-2xl font-bold text-emerald-500 mt-1">{{ dstCount }}</p>
      </div>
    </div>

    <div class="card p-6">
      <div class="flex items-center justify-between mb-4">
        <h3 class="text-lg font-semibold text-slate-900 dark:text-white">通信拓扑图</h3>
        <div class="flex items-center gap-4 text-xs text-slate-500">
          <span class="flex items-center gap-1"><span class="w-3 h-3 rounded-full bg-indigo-500"></span>源IP</span>
          <span class="flex items-center gap-1"><span class="w-3 h-3 rounded-full bg-emerald-500"></span>目的IP</span>
          <span class="flex items-center gap-1"><span class="w-3 h-3 rounded bg-blue-400" style="height:2px;width:20px"></span>TCP</span>
          <span class="flex items-center gap-1"><span class="w-3 h-3 rounded bg-green-400" style="height:2px;width:20px"></span>UDP</span>
        </div>
      </div>
      <div class="h-[500px]">
        <div v-if="loading" class="flex items-center justify-center h-full text-slate-400">加载中...</div>
        <ECharts v-else :option="graphOption" class="w-full h-full" />
      </div>
    </div>

    <div class="card">
      <div class="flex items-center justify-between p-6 border-b border-slate-200 dark:border-dark-700">
        <h3 class="text-lg font-semibold text-slate-900 dark:text-white">连接明细</h3>
        <span class="text-sm text-slate-400">共 {{ filteredFlows.length }} 条连接</span>
      </div>
      <div class="overflow-x-auto max-h-80 overflow-y-auto">
        <table class="w-full">
          <thead>
            <tr class="bg-slate-50 dark:bg-dark-700/50 sticky top-0">
              <th class="text-left px-4 py-3 text-xs font-semibold text-slate-500 uppercase">源IP</th>
              <th class="text-left px-4 py-3 text-xs font-semibold text-slate-500 uppercase">源端口</th>
              <th class="text-left px-4 py-3 text-xs font-semibold text-slate-500 uppercase">目的IP</th>
              <th class="text-left px-4 py-3 text-xs font-semibold text-slate-500 uppercase">目的端口</th>
              <th class="text-left px-4 py-3 text-xs font-semibold text-slate-500 uppercase">协议</th>
              <th class="text-right px-4 py-3 text-xs font-semibold text-slate-500 uppercase">包数</th>
              <th class="text-right px-4 py-3 text-xs font-semibold text-slate-500 uppercase">字节</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-slate-200 dark:divide-dark-700">
            <tr v-for="(f, i) in filteredFlows" :key="i" class="hover:bg-slate-50 dark:hover:bg-dark-700/50">
              <td class="px-4 py-3 text-sm font-mono text-slate-900 dark:text-white">{{ f.src_ip }}</td>
              <td class="px-4 py-3 text-sm text-slate-500">{{ f.src_port }}</td>
              <td class="px-4 py-3 text-sm font-mono text-slate-900 dark:text-white">{{ f.dst_ip }}</td>
              <td class="px-4 py-3 text-sm text-slate-500">{{ f.dst_port }}</td>
              <td class="px-4 py-3"><span class="text-xs px-2 py-1 rounded-full font-medium" :class="f.protocol==='TCP'?'bg-blue-100 text-blue-600':'bg-green-100 text-green-600'">{{ f.protocol }}</span></td>
              <td class="px-4 py-3 text-sm text-right text-slate-500">{{ f.packets }}</td>
              <td class="px-4 py-3 text-sm text-right text-slate-500">{{ formatBytes(f.bytes) }}</td>
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
const searchIP = ref('')
const filterProto = ref('')
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
  let list = flows.value
  if (filterProto.value) list = list.filter(f => f.protocol === filterProto.value)
  
  // Group by (src_ip, dst_ip) to get aggregate traffic
  const edgeMap = {}
  const srcSet = new Set()
  const dstSet = new Set()
  
  list.forEach(f => {
    if (!f.src_ip || !f.dst_ip) return
    const key = f.src_ip + '|' + f.dst_ip
    if (!edgeMap[key]) edgeMap[key] = { bytes: 0, proto: f.protocol }
    edgeMap[key].bytes += f.bytes || 0
    srcSet.add(f.src_ip)
    dstSet.add(f.dst_ip)
  })
  
  const sortedEdges = Object.entries(edgeMap).sort((a, b) => b[1].bytes - a[1].bytes).slice(0, 60)
  const maxB = sortedEdges.length ? sortedEdges[0][1].bytes : 1
  const activeSrc = new Set()
  const activeDst = new Set()
  
  const resultLinks = []
  sortedEdges.forEach(([key, v]) => {
    const [src, dst] = key.split('|')
    activeSrc.add(src)
    activeDst.add(dst)
    resultLinks.push({
      source: src, target: dst,
      lineStyle: {
        width: Math.max(1, (v.bytes / maxB) * 6),
        color: v.proto === 'TCP' ? '#3b82f6' : '#10b981',
        curveness: 0.2, opacity: 0.7
      },
      value: v.bytes
    })
  })
  
  const allNodes = [...new Set([...activeSrc, ...activeDst])]
  const resultNodes = allNodes.map(ip => {
    let total = 0
    sortedEdges.forEach(([key, v]) => { if (key.includes(ip)) total += v.bytes })
    const isSrc = activeSrc.has(ip)
    const isDst = activeDst.has(ip)
    return {
      name: ip,
      symbolSize: Math.max(20, Math.min(50, (total / (maxB || 1)) * 25 + 18)),
      itemStyle: { color: isSrc && isDst ? '#8b5cf6' : isSrc ? '#6366f1' : '#10b981' },
      label: { show: true, fontSize: 10, color: '#64748b' },
      category: isSrc && isDst ? 'both' : isSrc ? 'source' : 'dest'
    }
  })
  
  nodes.value = resultNodes
  links.value = resultLinks
}

const srcCount = computed(() => {
  const s = new Set(); flows.value.forEach(f => { if (f.src_ip) s.add(f.src_ip) }); return s.size
})
const dstCount = computed(() => {
  const s = new Set(); flows.value.forEach(f => { if (f.dst_ip) s.add(f.dst_ip) }); return s.size
})

const filteredFlows = computed(() => {
  let list = flows.value
  if (filterProto.value) list = list.filter(f => f.protocol === filterProto.value)
  if (searchIP.value) {
    const q = searchIP.value.toLowerCase()
    list = list.filter(f => (f.src_ip && f.src_ip.includes(q)) || (f.dst_ip && f.dst_ip.includes(q)))
  }
  return list.slice(0, 100)
})

const formatBytes = b => { if (!b) return '0 B'; if (b>=1e9) return (b/1e9).toFixed(2)+' GB'; if (b>=1e6) return (b/1e6).toFixed(2)+' MB'; if (b>=1e3) return (b/1e3).toFixed(2)+' KB'; return b+' B' }

const graphOption = computed(() => ({
  tooltip: {
    trigger: 'item',
    formatter: params => {
      if (params.dataType === 'node') return '<strong>' + params.name + '</strong><br/>IP节点'
      if (params.dataType === 'edge') return '<strong>' + params.data.source + ' → ' + params.data.target + '</strong><br/>流量: ' + formatBytes(params.data.value)
      return ''
    }
  },
  series: [{
    type: 'graph', layout: 'force',
    force: { repulsion: 400, edgeLength: [60, 180], gravity: 0.08, friction: 0.1 },
    roam: true, draggable: true,
    data: nodes.value,
    links: links.value,
    categories: [{ name: 'source', itemStyle: { color: '#6366f1' } }, { name: 'dest', itemStyle: { color: '#10b981' } }, { name: 'both', itemStyle: { color: '#8b5cf6' } }],
    lineStyle: { curveness: 0.3 },
    label: { show: true, position: 'bottom', fontSize: 10, color: '#64748b' },
    emphasis: { focus: 'adjacency', lineStyle: { width: 4 } }
  }]
}))

onMounted(() => { fetchData(); setInterval(fetchData, 30000) })
</script>
