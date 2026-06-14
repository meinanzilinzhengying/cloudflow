<template>
  <div class="space-y-6">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold text-slate-900 dark:text-white">Service拓扑</h1>
        <p class="text-sm text-slate-500 dark:text-slate-400 mt-1">Kubernetes Service 依赖关系</p>
      </div>
      <button class="btn-secondary" @click="fetchData"><RefreshCw class="w-4 h-4" />刷新</button>
    </div>

    <div class="grid grid-cols-4 gap-4">
      <div class="card p-4">
        <p class="text-sm text-slate-500">Services</p>
        <p class="text-2xl font-bold text-slate-900 dark:text-white mt-1">{{ services.length }}</p>
      </div>
      <div class="card p-4">
        <p class="text-sm text-slate-500">Pods</p>
        <p class="text-2xl font-bold text-primary-500 mt-1">{{ pods.length }}</p>
      </div>
      <div class="card p-4">
        <p class="text-sm text-slate-500">Namespaces</p>
        <p class="text-2xl font-bold text-accent-500 mt-1">{{ namespaces.length }}</p>
      </div>
      <div class="card p-4">
        <p class="text-sm text-slate-500">Nodes</p>
        <p class="text-2xl font-bold text-emerald-500 mt-1">{{ nodes.length }}</p>
      </div>
    </div>

    <div class="card p-6">
      <div class="flex items-center justify-between mb-4">
        <h3 class="text-lg font-semibold text-slate-900 dark:text-white">Service 拓扑图</h3>
        <div class="flex items-center gap-3 text-xs text-slate-500">
          <span class="flex items-center gap-1"><span class="w-3 h-3 rounded-full bg-indigo-500"></span>Service</span>
          <span class="flex items-center gap-1"><span class="w-3 h-3 rounded-full bg-emerald-500"></span>Pod</span>
          <span class="flex items-center gap-1"><span class="w-3 h-3 rounded-full bg-amber-500"></span>Node</span>
        </div>
      </div>
      <div class="h-[500px] flex items-center justify-center">
        <div v-if="loading" class="text-slate-400">加载中...</div>
        <ECharts v-else-if="!graphNodes.length" class="w-full h-full" :option="emptyOption" />
        <ECharts v-else :option="graphOption" class="w-full h-full" />
      </div>
    </div>

    <div class="grid grid-cols-2 gap-6">
      <div class="card p-6">
        <h3 class="text-lg font-semibold text-slate-900 dark:text-white mb-4">Services 列表</h3>
        <div class="space-y-2 max-h-60 overflow-y-auto">
          <div v-for="svc in services" :key="svc.metadata.uid" class="p-3 rounded-lg bg-slate-50 dark:bg-dark-700/50">
            <div class="flex items-center justify-between">
              <span class="text-sm font-medium">{{ svc.metadata.name }}</span>
              <span class="text-xs px-2 py-0.5 rounded bg-indigo-100 text-indigo-600">{{ svc.spec.type || 'ClusterIP' }}</span>
            </div>
            <p class="text-xs text-slate-500 mt-0.5">{{ svc.metadata.namespace }} · {{ svc.spec.clusterIP }}</p>
          </div>
        </div>
      </div>
      <div class="card p-6">
        <h3 class="text-lg font-semibold text-slate-900 dark:text-white mb-4">Pods 列表</h3>
        <div class="space-y-2 max-h-60 overflow-y-auto">
          <div v-for="pod in pods" :key="pod.metadata.uid" class="p-3 rounded-lg bg-slate-50 dark:bg-dark-700/50">
            <div class="flex items-center justify-between">
              <span class="text-sm font-medium truncate">{{ pod.metadata.name }}</span>
              <span class="text-xs px-2 py-0.5 rounded" :class="pod.status.phase==='Running'?'bg-green-100 text-green-600':'bg-yellow-100 text-yellow-600'">{{ pod.status.phase }}</span>
            </div>
            <p class="text-xs text-slate-500 mt-0.5">{{ pod.metadata.namespace }} · {{ pod.status.podIP || '-' }}</p>
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
import VChart from 'vue-echarts'
import { RefreshCw } from 'lucide-vue-next'

use([CanvasRenderer, GraphChart, TooltipComponent])
const ECharts = VChart

const loading = ref(true)
const services = ref([])
const pods = ref([])
const namespaces = ref([])
const nodes = ref([])

const fetchData = async () => {
  loading.value = true
  try {
    const [s, p, n, nd] = await Promise.all([
      fetch('/api/k8s/services').then(r => r.json()),
      fetch('/api/k8s/pods').then(r => r.json()),
      fetch('/api/k8s/namespaces').then(r => r.json()),
      fetch('/api/k8s/nodes').then(r => r.json()),
    ])
    services.value = s.items || []
    pods.value = p.items || []
    namespaces.value = n.items || []
    nodes.value = nd.items || []
  } catch(e) { console.error(e) }
  loading.value = false
}

const graphNodes = computed(() => {
  const result = []
  services.value.forEach(s => {
    result.push({ name: s.metadata.name, symbolSize: 35, itemStyle: { color: '#6366f1' }, category: 0, namespace: s.metadata.namespace, clusterIP: s.spec.clusterIP })
  })
  pods.value.forEach(p => {
    result.push({ name: p.metadata.name, symbolSize: 25, itemStyle: { color: p.status.phase==='Running'?'#10b981':'#f59e0b' }, category: 1, namespace: p.metadata.namespace, podIP: p.status.podIP })
  })
  nodes.value.forEach(n => {
    result.push({ name: n.metadata.name, symbolSize: 30, itemStyle: { color: '#f59e0b' }, category: 2 })
  })
  return result
})

const graphOption = computed(() => ({
  tooltip: { trigger: 'item', formatter: p => `<strong>${p.name}</strong><br/>${p.data.namespace||''} ${p.data.clusterIP||p.data.podIP||''}` },
  series: [{
    type: 'graph', layout: 'force',
    force: { repulsion: 300, edgeLength: [80, 200], gravity: 0.05 },
    roam: true, draggable: true,
    data: graphNodes.value,
    links: [],
    categories: [
      { name: 'Service', itemStyle: { color: '#6366f1' } },
      { name: 'Pod', itemStyle: { color: '#10b981' } },
      { name: 'Node', itemStyle: { color: '#f59e0b' } },
    ],
    label: { show: true, position: 'bottom', fontSize: 9, color: '#64748b' },
    emphasis: { focus: 'adjacency' }
  }]
}))

const emptyOption = computed(() => ({
  series: [{ type: 'graph', data: [], links: [] }]
}))

onMounted(() => { fetchData(); setInterval(fetchData, 60000) })
</script>
