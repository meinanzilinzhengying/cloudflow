<template>
  <div class="space-y-6">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold text-slate-900 dark:text-white">Namespace拓扑</h1>
        <p class="text-sm text-slate-500 dark:text-slate-400 mt-1">IP网段级别的网络拓扑关系</p>
      </div>
      <div class="flex items-center gap-3">
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
        <p class="text-sm text-slate-500">网段数</p>
        <p class="text-2xl font-bold text-slate-900 dark:text-white mt-1">{{ subnets.length }}</p>
      </div>
      <div class="card p-4">
        <p class="text-sm text-slate-500">网段连接</p>
        <p class="text-2xl font-bold text-primary-500 mt-1">{{ subnetLinks.length }}</p>
      </div>
      <div class="card p-4">
        <p class="text-sm text-slate-500">总IP数</p>
        <p class="text-2xl font-bold text-accent-500 mt-1">{{ totalIPs }}</p>
      </div>
      <div class="card p-4">
        <p class="text-sm text-slate-500">总流量</p>
        <p class="text-2xl font-bold text-emerald-500 mt-1">{{ totalTraffic }}</p>
      </div>
    </div>

    <div class="card p-6">
      <div class="flex items-center justify-between mb-4">
        <h3 class="text-lg font-semibold text-slate-900 dark:text-white">网段拓扑图</h3>
        <div class="text-xs text-slate-500">节点按 /24 子网分组，连线粗细=流量</div>
      </div>
      <div class="h-[400px]">
        <div v-if="loading" class="flex items-center justify-center h-full text-slate-400">加载中...</div>
        <ECharts v-else :option="graphOption" class="w-full h-full" />
      </div>
    </div>

    <div class="card p-6">
      <h3 class="text-lg font-semibold text-slate-900 dark:text-white mb-4">网段明细</h3>
      <div class="overflow-x-auto">
        <table class="w-full">
          <thead>
            <tr class="bg-slate-50 dark:bg-dark-700/50">
              <th class="text-left px-4 py-3 text-xs font-semibold text-slate-500 uppercase">源网段</th>
              <th class="text-left px-4 py-3 text-xs font-semibold text-slate-500 uppercase">目的网段</th>
              <th class="text-left px-4 py-3 text-xs font-semibold text-slate-500 uppercase">协议</th>
              <th class="text-right px-4 py-3 text-xs font-semibold text-slate-500 uppercase">流量</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-slate-200 dark:divide-dark-700">
            <tr v-for="(e, i) in subnetLinks" :key="i" class="hover:bg-slate-50 dark:hover:bg-dark-700/50">
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
const filterProto = ref('')
const subnetEdges = ref([])
const subnetNodes = ref([])

const getSubnet = (ip) => {
  if (!ip) return 'unknown'
  const parts = ip.split('.')
  if (parts.length === 4) return parts[0] + '.' + parts[1] + '.' + parts[2] + '.0/24'
  return ip
}

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
  
  const edgeMap = {}
  const subnetSet = new Set()
  
  list.forEach(f => {
    const s = getSubnet(f.src_ip)
    const d = getSubnet(f.dst_ip)
    if (s === d) return  // Skip same-subnet traffic
    subnetSet.add(s)
    subnetSet.add(d)
    const key = s + '|' + d
    if (!edgeMap[key]) edgeMap[key] = { bytes: 0, proto: f.protocol, srcSubnet: s, dstSubnet: d }
    edgeMap[key].bytes += f.bytes || 0
  })
  
  const sorted = Object.values(edgeMap).sort((a, b) => b.bytes - a.bytes)
  const maxB = sorted.length ? sorted[0].bytes : 1
  
  subnetEdges.value = sorted.slice(0, 40).map(e => ({
    src: e.srcSubnet, dst: e.dstSubnet, proto: e.proto, bytes: formatBytes(e.bytes)
  }))
  
  const activeSubnets = new Set()
  sorted.slice(0, 40).forEach(e => { activeSubnets.add(e.srcSubnet); activeSubnets.add(e.dstSubnet) })
  
  const links = sorted.slice(0, 40).map(e => ({
    source: e.srcSubnet, target: e.dstSubnet,
    lineStyle: { width: Math.max(1, (e.bytes / maxB) * 8), color: e.proto === 'TCP' ? '#3b82f6' : '#10b981', curveness: 0.2, opacity: 0.7 },
    value: e.bytes
  }))
  
  subnetNodes.value = Array.from(activeSubnets).map(sn => {
    let total = 0
    sorted.forEach(e => { if (e.srcSubnet === sn || e.dstSubnet === sn) total += e.bytes })
    return { name: sn, symbolSize: Math.max(25, Math.min(55, (total / (maxB || 1)) * 30 + 20)), itemStyle: { color: '#6366f1' }, label: { show: true, fontSize: 10 } }
  })
  
  subnetEdges.value = sorted.slice(0, 40).map(e => ({
    src: e.srcSubnet, dst: e.dstSubnet, proto: e.proto, bytes: formatBytes(e.bytes)
  }))
}

const subnets = computed(() => subnetNodes.value)
const subnetLinks = computed(() => subnetEdges.value)

const totalIPs = computed(() => {
  const s = new Set(); flows.value.forEach(f => { if (f.src_ip) s.add(f.src_ip); if (f.dst_ip) s.add(f.dst_ip) }); return s.size
})

const totalTraffic = computed(() => {
  const t = subnetEdges.value.reduce((s, e) => s + parseFloat(e.bytes), 0)
  if (t >= 1e9) return (t / 1e9).toFixed(2) + ' GB'
  if (t >= 1e6) return (t / 1e6).toFixed(2) + ' MB'
  if (t >= 1e3) return (t / 1e3).toFixed(2) + ' KB'
  return t + ' B'
})

const formatBytes = b => { if (!b) return '0 B'; if (b>=1e9) return (b/1e9).toFixed(2)+' GB'; if (b>=1e6) return (b/1e6).toFixed(2)+' MB'; if (b>=1e3) return (b/1e3).toFixed(2)+' KB'; return b+' B' }

const graphOption = computed(() => ({
  tooltip: { trigger: 'item', formatter: params => {
    if (params.dataType === 'edge') return '<strong>' + params.data.source + ' → ' + params.data.target + '</strong><br/>流量: ' + formatBytes(params.data.value)
    return '<strong>' + params.name + '</strong>'
  }},
  series: [{
    type: 'graph', layout: 'force',
    force: { repulsion: 500, edgeLength: [100, 250], gravity: 0.05, friction: 0.1 },
    roam: true, draggable: true,
    data: subnetNodes.value,
    links: [],
    lineStyle: { curveness: 0.3 },
    label: { show: true, position: 'bottom', fontSize: 11, color: '#64748b' },
    emphasis: { focus: 'adjacency', lineStyle: { width: 4 } },
    zoom: 0.8
  }]
}))

// Need to set links separately to avoid reactivity issues
import { watch } from 'vue'
watch(subnetNodes, () => {
  // Option is rebuilt by computed, links are part of the data
}, { deep: true })

onMounted(() => { fetchData(); setInterval(fetchData, 30000) })
</script>
