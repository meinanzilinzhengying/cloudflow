<template>
  <div class="space-y-6">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold text-slate-900 dark:text-white">会话分析</h1>
        <p class="text-sm text-slate-500 dark:text-slate-400 mt-1">深入分析网络会话数据</p>
      </div>
      <div class="flex items-center gap-3">
        <button class="btn-secondary" @click="fetchData">
          <RefreshCw class="w-4 h-4" />
          刷新
        </button>
      </div>
    </div>

    <div class="grid grid-cols-4 gap-4">
      <div class="card p-4">
        <p class="text-sm text-slate-500">总会话</p>
        <p class="text-2xl font-bold text-slate-900 dark:text-white mt-1">{{ flows.length }}</p>
      </div>
      <div class="card p-4">
        <p class="text-sm text-slate-500">TCP会话</p>
        <p class="text-2xl font-bold text-primary-500 mt-1">{{ tcpCount }}</p>
      </div>
      <div class="card p-4">
        <p class="text-sm text-slate-500">UDP会话</p>
        <p class="text-2xl font-bold text-accent-500 mt-1">{{ udpCount }}</p>
      </div>
      <div class="card p-4">
        <p class="text-sm text-slate-500">总包数</p>
        <p class="text-2xl font-bold text-emerald-500 mt-1">{{ totalPackets }}</p>
      </div>
    </div>

    <div class="grid grid-cols-1 lg:grid-cols-2 gap-6">
      <div class="card p-6">
        <h3 class="text-lg font-semibold text-slate-900 dark:text-white mb-6">协议分布</h3>
        <div class="h-72">
          <ECharts :option="protocolOption" class="w-full h-full" />
        </div>
      </div>
      <div class="card p-6">
        <h3 class="text-lg font-semibold text-slate-900 dark:text-white mb-6">端口分布 (Top 10)</h3>
        <div class="space-y-3">
          <div v-if="topPorts.length === 0" class="flex items-center justify-center h-64 text-slate-400">暂无数据</div>
          <div v-for="(p, i) in topPorts" :key="p.port" class="flex items-center gap-3">
            <span class="w-5 text-xs font-medium text-slate-400">{{ i + 1 }}</span>
            <div class="flex-1">
              <div class="flex items-center justify-between mb-1">
                <span class="text-sm font-medium text-slate-700 dark:text-slate-200">{{ p.port }}</span>
                <span class="text-xs text-slate-500">{{ p.count }}</span>
              </div>
              <div class="h-1.5 bg-slate-100 dark:bg-dark-700 rounded-full overflow-hidden">
                <div class="h-full bg-gradient-to-r from-primary-500 to-accent-500 rounded-full" :style="{ width: p.pct + '%' }"></div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <div class="card">
      <div class="flex items-center justify-between p-6 border-b border-slate-200 dark:border-dark-700">
        <h3 class="text-lg font-semibold text-slate-900 dark:text-white">会话明细</h3>
        <div class="flex items-center gap-3">
          <input v-model="searchQuery" type="text" placeholder="搜索 IP..." class="input max-w-xs" />
          <select v-model="filterProtocol" class="input w-32">
            <option value="">全部</option>
            <option value="TCP">TCP</option>
            <option value="UDP">UDP</option>
          </select>
        </div>
      </div>
      <div class="overflow-x-auto">
        <table class="w-full">
          <thead>
            <tr class="bg-slate-50 dark:bg-dark-700/50">
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500 uppercase">时间</th>
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500 uppercase">源IP</th>
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500 uppercase">目的IP</th>
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500 uppercase">协议</th>
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500 uppercase">源端口</th>
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500 uppercase">目的端口</th>
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500 uppercase">包数</th>
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500 uppercase">字节数</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-slate-200 dark:divide-dark-700">
            <tr v-for="(flow, idx) in filteredFlows" :key="idx" class="hover:bg-slate-50 dark:hover:bg-dark-700/50 transition-colors">
              <td class="px-6 py-4 text-sm text-slate-600 dark:text-slate-300">{{ formatTime(flow.timestamp) }}</td>
              <td class="px-6 py-4 text-sm text-slate-900 dark:text-white">{{ flow.src_ip }}</td>
              <td class="px-6 py-4 text-sm text-slate-900 dark:text-white">{{ flow.dst_ip }}</td>
              <td class="px-6 py-4">
                <span :class="['text-xs px-2 py-1 rounded-full font-medium', flow.protocol === 'TCP' ? 'bg-blue-100 text-blue-600' : 'bg-green-100 text-green-600']">{{ appProto(flow.protocol, flow.dst_port) }}</span>
              </td>
              <td class="px-6 py-4 text-sm text-slate-500">{{ flow.src_port }}</td>
              <td class="px-6 py-4 text-sm text-slate-500">{{ flow.dst_port }}</td>
              <td class="px-6 py-4 text-sm text-slate-500">{{ flow.packets }}</td>
              <td class="px-6 py-4 text-sm text-slate-500">{{ formatBytes(flow.bytes) }}</td>
            </tr>
          </tbody>
        </table>
      </div>
      <div class="p-4 text-center text-sm text-slate-400" v-if="!loading && flows.length === 0">暂无数据</div>
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

const PROTOCOL_MAP = {
  80: 'HTTP', 443: 'HTTPS', 8080: 'HTTP', 8443: 'HTTPS', 3000: 'Grafana',
  3306: 'MySQL', 5432: 'PostgreSQL', 6379: 'Redis', 27017: 'MongoDB', 8123: 'ClickHouse',
  22: 'SSH', 21: 'FTP', 53: 'DNS', 123: 'NTP', 161: 'SNMP', 389: 'LDAP',
  2379: 'etcd', 6443: 'K8s API', 9092: 'Kafka', 9093: 'Alertmanager',
  9090: 'Prometheus', 16686: 'Jaeger', 4318: 'OTel',
  9002: 'DataPlane', 8007: 'QuerySvc', 8009: 'AlertSvc', 8006: 'AuthSvc',
  8001: 'ControlPlane', 8010: 'TenantSvc', 8082: 'AISvc',
  10808: 'Proxy', 137: 'NetBIOS', 5353: 'mDNS', 1900: 'UPnP',
};
const appProto = (p, port) => { return (port && PROTOCOL_MAP[port]) ? PROTOCOL_MAP[port] : p; };

import { RefreshCw } from 'lucide-vue-next'

use([CanvasRenderer, PieChart, TooltipComponent, LegendComponent])

const flows = ref([])
const loading = ref(true)
const searchQuery = ref('')
const filterProtocol = ref('')

const fetchData = async () => {
  loading.value = true
  try {
    const res = await fetch('/api/query/flows?limit=2000')
    if (res.ok) {
      const data = await res.json()
      flows.value = data.records || []
    }
  } catch (e) {
    console.error('Failed to fetch flows:', e)
  } finally {
    loading.value = false
  }
}

const tcpCount = computed(() => flows.value.filter(f => f.protocol === 'TCP').length)
const udpCount = computed(() => flows.value.filter(f => f.protocol === 'UDP').length)
const totalPackets = computed(() => flows.value.reduce((s, f) => s + (f.packets || 0), 0))

const filteredFlows = computed(() => {
  let list = flows.value
  if (filterProtocol.value) list = list.filter(f => f.protocol === filterProtocol.value)
  if (searchQuery.value) {
    const q = searchQuery.value.toLowerCase()
    list = list.filter(f => (f.src_ip && f.src_ip.includes(q)) || (f.dst_ip && f.dst_ip.includes(q)))
  }
  return list.slice(0, 100)
})

const protocolOption = computed(() => {
  const map = {}
  flows.value.forEach(f => { const p = appProto(f.protocol, f.dst_port); map[p] = (map[p] || 0) + 1 })
  const colors = { 'TCP': '#2563eb', 'UDP': '#14b8a6', 'ICMP': '#f59e0b', 'IP': '#8b5cf6' }
  const data = Object.entries(map).map(([name, value]) => ({ name, value, itemStyle: { color: colors[name] || '#94a3b8' } }))
  return {
    tooltip: { trigger: 'item' },
    legend: { bottom: 0, textStyle: { fontSize: 11 } },
    series: [{ type: 'pie', radius: ['40%', '70%'], data, emphasis: { itemStyle: { shadowBlur: 10 } } }]
  }
})

const topPorts = computed(() => {
  const map = {}
  flows.value.forEach(f => { if (f.dst_port) { const k = f.protocol + '/' + f.dst_port; map[k] = (map[k] || 0) + 1 } })
  const sorted = Object.entries(map).sort((a, b) => b[1] - a[1]).slice(0, 10)
  const max = sorted.length ? sorted[0][1] : 1
  return sorted.map(([port, count]) => ({ port, count, pct: ((count / max) * 100).toFixed(0) }))
})

const formatTime = (ts) => {
  if (!ts) return '-'
  const d = new Date(ts)
  return String(d.getHours()).padStart(2,'0') + ':' + String(d.getMinutes()).padStart(2,'0') + ':' + String(d.getSeconds()).padStart(2,'0')
}

const formatBytes = (b) => {
  if (!b) return '0 B'
  if (b >= 1e9) return (b / 1e9).toFixed(2) + ' GB'
  if (b >= 1e6) return (b / 1e6).toFixed(2) + ' MB'
  if (b >= 1e3) return (b / 1e3).toFixed(2) + ' KB'
  return b + ' B'
}

onMounted(() => { fetchData(); setInterval(fetchData, 30000) })
</script>
