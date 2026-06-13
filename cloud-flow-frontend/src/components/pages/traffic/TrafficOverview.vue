<template>
  <div class="space-y-6">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold text-slate-900 dark:text-white">全局流量分析</h1>
        <p class="text-sm text-slate-500 dark:text-slate-400 mt-1">实时监控网络流量与会话数据</p>
      </div>
      <div class="flex items-center gap-3">
        <button class="btn-secondary" @click="fetchFlows">
          <RefreshCw class="w-4 h-4" />
          刷新
        </button>
      </div>
    </div>

    <div class="grid grid-cols-4 gap-4">
      <div class="card p-4">
        <div class="flex items-center justify-between">
          <div>
            <p class="text-sm text-slate-500">总流量</p>
            <p class="text-2xl font-bold text-slate-900 dark:text-white mt-1">{{ formatKPI(totalBytes) }}</p>
          </div>
          <div class="w-10 h-10 rounded-xl bg-primary-50 dark:bg-primary-500/10 flex items-center justify-center">
            <Network class="w-5 h-5 text-primary-500" />
          </div>
        </div>
      </div>
      <div class="card p-4">
        <div class="flex items-center justify-between">
          <div>
            <p class="text-sm text-slate-500">总包数</p>
            <p class="text-2xl font-bold text-accent-500 mt-1">{{ formatKPI(totalPackets) }}</p>
          </div>
          <div class="w-10 h-10 rounded-xl bg-accent-50 dark:bg-accent-500/10 flex items-center justify-center">
            <Activity class="w-5 h-5 text-accent-500" />
          </div>
        </div>
      </div>
      <div class="card p-4">
        <div class="flex items-center justify-between">
          <div>
            <p class="text-sm text-slate-500">连接数</p>
            <p class="text-2xl font-bold text-emerald-500 mt-1">{{ flows.length }}</p>
          </div>
          <div class="w-10 h-10 rounded-xl bg-emerald-50 dark:bg-emerald-500/10 flex items-center justify-center">
            <TrendingUp class="w-5 h-5 text-emerald-500" />
          </div>
        </div>
      </div>
      <div class="card p-4">
        <div class="flex items-center justify-between">
          <div>
            <p class="text-sm text-slate-500">平均时延</p>
            <p class="text-2xl font-bold text-amber-500 mt-1">{{ avgLatency }} ms</p>
          </div>
          <div class="w-10 h-10 rounded-xl bg-amber-50 dark:bg-amber-500/10 flex items-center justify-center">
            <Users class="w-5 h-5 text-amber-500" />
          </div>
        </div>
      </div>
    </div>

    <div class="card p-6">
      <div class="flex items-center justify-between mb-6">
        <h3 class="text-lg font-semibold text-slate-900 dark:text-white">流量趋势</h3>
      </div>
      <div v-if="loading" class="h-72 flex items-center justify-center text-slate-400">加载中...</div>
      <div v-else class="h-72">
        <ECharts :option="trafficTrendOption" class="w-full h-full" />
      </div>
    </div>

    <div class="grid grid-cols-2 gap-6">
      <div class="card p-6">
        <div class="flex items-center justify-between mb-4">
          <h3 class="text-lg font-semibold text-slate-900 dark:text-white">Top 源 IP</h3>
        </div>
        <div class="space-y-3" v-if="topSources.length">
          <div v-for="(item, idx) in topSources" :key="idx" class="flex items-center gap-3">
            <span class="w-6 text-xs font-medium text-slate-400">{{ idx + 1 }}</span>
            <div class="flex-1">
              <div class="flex items-center justify-between mb-1">
                <span class="text-sm font-medium text-slate-700 dark:text-slate-200">{{ item.ip }}</span>
                <span class="text-xs text-slate-500">{{ formatBytes(item.bytes) }}</span>
              </div>
              <div class="h-1.5 bg-slate-100 dark:bg-dark-700 rounded-full overflow-hidden">
                <div class="h-full bg-gradient-to-r from-primary-500 to-accent-500 rounded-full" :style="{'width': item.pct + '%'}"></div>
              </div>
            </div>
          </div>
        </div>
        <div v-else class="text-sm text-slate-400 text-center py-4">暂无数据</div>
      </div>
      <div class="card p-6">
        <div class="flex items-center justify-between mb-4">
          <h3 class="text-lg font-semibold text-slate-900 dark:text-white">Top 目的 IP</h3>
        </div>
        <div class="space-y-3" v-if="topDests.length">
          <div v-for="(item, idx) in topDests" :key="idx" class="flex items-center gap-3">
            <span class="w-6 text-xs font-medium text-slate-400">{{ idx + 1 }}</span>
            <div class="flex-1">
              <div class="flex items-center justify-between mb-1">
                <span class="text-sm font-medium text-slate-700 dark:text-slate-200">{{ item.ip }}</span>
                <span class="text-xs text-slate-500">{{ formatBytes(item.bytes) }}</span>
              </div>
              <div class="h-1.5 bg-slate-100 dark:bg-dark-700 rounded-full overflow-hidden">
                <div class="h-full bg-gradient-to-r from-accent-500 to-emerald-500 rounded-full" :style="{'width': item.pct + '%'}"></div>
              </div>
            </div>
          </div>
        </div>
        <div v-else class="text-sm text-slate-400 text-center py-4">暂无数据</div>
      </div>
    </div>

    <div class="card">
      <div class="flex items-center justify-between p-6 border-b border-slate-200 dark:border-dark-700">
        <h3 class="text-lg font-semibold text-slate-900 dark:text-white">五元组会话列表</h3>
        <div class="flex items-center gap-3">
          <input v-model="searchQuery" type="text" placeholder="搜索 IP..." class="input max-w-xs" />
          <select v-model="filterProtocol" class="input w-32">
            <option value="">全部协议</option>
            <option value="TCP">TCP</option>
            <option value="UDP">UDP</option>
            <option value="ICMP">ICMP</option>
          </select>
        </div>
      </div>
      <div class="overflow-x-auto">
        <table class="w-full">
          <thead>
            <tr class="bg-slate-50 dark:bg-dark-700/50">
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500 uppercase">源IP</th>
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500 uppercase">目的IP</th>
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500 uppercase">协议</th>
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500 uppercase">源端口</th>
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500 uppercase">目的端口</th>
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500 uppercase">包数</th>
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500 uppercase">字节数</th>
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500 uppercase">操作</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-slate-200 dark:divide-dark-700">
            <tr v-for="(flow, idx) in filteredFlows" :key="idx"
              class="hover:bg-slate-50 dark:hover:bg-dark-700/50 cursor-pointer transition-colors"
              @click="selectedFlow = flow">
              <td class="px-6 py-4 text-sm text-slate-900 dark:text-white">{{ flow.src_ip }}</td>
              <td class="px-6 py-4 text-sm text-slate-900 dark:text-white">{{ flow.dst_ip }}</td>
              <td class="px-6 py-4">
                <span :class="flow.protocol === 'TCP' ? 'bg-blue-100 text-blue-600' : flow.protocol === 'UDP' ? 'bg-green-100 text-green-600' : 'bg-gray-100 text-gray-600'" class="text-xs px-2 py-1 rounded-full font-medium">{{ appProto(flow.protocol, flow.dst_port) }}</span>
              </td>
              <td class="px-6 py-4 text-sm text-slate-500">{{ flow.src_port }}</td>
              <td class="px-6 py-4 text-sm text-slate-500">{{ flow.dst_port }}</td>
              <td class="px-6 py-4 text-sm text-slate-500">{{ flow.packets }}</td>
              <td class="px-6 py-4 text-sm text-slate-500">{{ formatBytes(flow.bytes) }}</td>
              <td class="px-6 py-4">
                <button class="text-xs text-primary-500 hover:text-primary-600 font-medium" @click.stop="selectedFlow = flow">详情</button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
      <div class="p-4 text-center text-sm text-slate-400" v-if="!loading && flows.length === 0">暂无数据</div>
    </div>

    <Transition name="drawer">
      <div v-if="selectedFlow" class="fixed inset-0 z-50 flex justify-end" @click.self="selectedFlow = null">
        <div class="absolute inset-0 bg-slate-900/50"></div>
        <div class="relative w-full max-w-lg bg-white dark:bg-dark-800 shadow-2xl overflow-y-auto">
          <div class="sticky top-0 bg-white dark:bg-dark-800 border-b border-slate-200 dark:border-dark-700 px-6 py-4 flex items-center justify-between">
            <h3 class="text-lg font-semibold text-slate-900 dark:text-white">流量详情</h3>
            <button @click="selectedFlow = null" class="p-2 hover:bg-slate-100 dark:hover:bg-dark-700 rounded-lg"><X class="w-5 h-5 text-slate-500" /></button>
          </div>
          <div class="p-6 space-y-4">
            <div class="grid grid-cols-2 gap-4">
              <div class="p-4 bg-slate-50 dark:bg-dark-700 rounded-xl">
                <p class="text-xs text-slate-500 mb-1">源IP</p>
                <p class="text-sm font-medium text-slate-900 dark:text-white">{{ selectedFlow.src_ip }}</p>
              </div>
              <div class="p-4 bg-slate-50 dark:bg-dark-700 rounded-xl">
                <p class="text-xs text-slate-500 mb-1">目的IP</p>
                <p class="text-sm font-medium text-slate-900 dark:text-white">{{ selectedFlow.dst_ip }}</p>
              </div>
              <div class="p-4 bg-slate-50 dark:bg-dark-700 rounded-xl">
                <p class="text-xs text-slate-500 mb-1">协议</p>
                <p class="text-sm font-medium text-slate-900 dark:text-white">{{ selectedFlow.protocol }}</p>
              </div>
              <div class="p-4 bg-slate-50 dark:bg-dark-700 rounded-xl">
                <p class="text-xs text-slate-500 mb-1">源端口</p>
                <p class="text-sm font-medium text-slate-900 dark:text-white">{{ selectedFlow.src_port }}</p>
              </div>
              <div class="p-4 bg-slate-50 dark:bg-dark-700 rounded-xl">
                <p class="text-xs text-slate-500 mb-1">目的端口</p>
                <p class="text-sm font-medium text-slate-900 dark:text-white">{{ selectedFlow.dst_port }}</p>
              </div>
              <div class="p-4 bg-slate-50 dark:bg-dark-700 rounded-xl">
                <p class="text-xs text-slate-500 mb-1">探针</p>
                <p class="text-sm font-medium text-slate-900 dark:text-white">{{ selectedFlow.probe_id }}</p>
              </div>
            </div>
          </div>
        </div>
      </div>
    </Transition>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { use } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { LineChart, BarChart } from 'echarts/charts'
import { TooltipComponent, GridComponent } from 'echarts/components'
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

import { Network, Activity, TrendingUp, Users, RefreshCw, X } from 'lucide-vue-next'

use([CanvasRenderer, LineChart, BarChart, TooltipComponent, GridComponent])

const flows = ref([])
const loading = ref(true)
const selectedFlow = ref(null)
const searchQuery = ref('')
const filterProtocol = ref('')

const fetchFlows = async () => {
  loading.value = true
  try {
    const res = await fetch('/api/query/flows?limit=2000')
    if (!res.ok) throw new Error('HTTP ' + res.status)
    const data = await res.json()
    flows.value = data.records || []
  } catch (e) {
    console.error('Failed to fetch flows:', e)
  } finally {
    loading.value = false
  }
}

const totalBytes = computed(() => flows.value.reduce((s, f) => s + (f.bytes || 0), 0))
const totalPackets = computed(() => flows.value.reduce((s, f) => s + (f.packets || 0), 0))
const avgLatency = computed(() => {
  const valid = flows.value.filter(f => f.latency_ms > 0)
  if (!valid.length) return '0.00'
  return (valid.reduce((s, f) => s + f.latency_ms, 0) / valid.length).toFixed(2)
})

const filteredFlows = computed(() => {
  let list = flows.value
  if (filterProtocol.value) {
    list = list.filter(f => f.protocol === filterProtocol.value)
  }
  if (searchQuery.value) {
    const q = searchQuery.value.toLowerCase()
    list = list.filter(f => (f.src_ip && f.src_ip.includes(q)) || (f.dst_ip && f.dst_ip.includes(q)))
  }
  return list.slice(0, 100)
})

const trafficTrendOption = computed(() => {
  const map = {}
  flows.value.forEach(f => {
    if (!f.timestamp || !f.bytes) return
    const d = new Date(f.timestamp)
    const key = String(d.getHours()).padStart(2,'0') + ':' + String(d.getMinutes()).padStart(2,'0')
    map[key] = (map[key] || 0) + f.bytes
  })
  const keys = Object.keys(map).sort()
  const vals = keys.map(k => (map[k] / 1048576).toFixed(2))
  return {
    tooltip: { trigger: 'axis' },
    grid: { left: '3%', right: '4%', bottom: '10%', top: '5%', containLabel: true },
    xAxis: { type: 'category', data: keys, axisLabel: { rotate: 45, fontSize: 10 } },
    yAxis: { type: 'value', name: 'MB' },
    series: [{
      name: '流量', type: 'line', smooth: true,
      lineStyle: { color: '#2563eb', width: 2 },
      areaStyle: { color: { type: 'linear', x: 0, y: 0, x2: 0, y2: 1, colorStops: [{ offset: 0, color: 'rgba(37,99,235,0.15)' }, { offset: 1, color: 'rgba(37,99,235,0)' }] } },
      data: vals
    }]
  }
})

const topSources = computed(() => {
  const map = {}
  flows.value.forEach(f => { if (f.src_ip) map[f.src_ip] = (map[f.src_ip] || 0) + (f.bytes || 0) })
  const sorted = Object.entries(map).sort((a, b) => b[1] - a[1]).slice(0, 5)
  const max = sorted.length ? Math.max(...sorted.map(s => s[1])) : 1
  return sorted.map(([ip, bytes]) => ({ ip, bytes, pct: ((bytes / max) * 100).toFixed(0) }))
})

const topDests = computed(() => {
  const map = {}
  flows.value.forEach(f => { if (f.dst_ip) map[f.dst_ip] = (map[f.dst_ip] || 0) + (f.bytes || 0) })
  const sorted = Object.entries(map).sort((a, b) => b[1] - a[1]).slice(0, 5)
  const max = sorted.length ? Math.max(...sorted.map(s => s[1])) : 1
  return sorted.map(([ip, bytes]) => ({ ip, bytes, pct: ((bytes / max) * 100).toFixed(0) }))
})

const formatKPI = (v) => {
  if (!v) return '0 B'
  if (v >= 1e12) return (v / 1e12).toFixed(2) + ' TB'
  if (v >= 1e9) return (v / 1e9).toFixed(2) + ' GB'
  if (v >= 1e6) return (v / 1e6).toFixed(2) + ' MB'
  if (v >= 1e3) return (v / 1e3).toFixed(2) + ' KB'
  return v + ' B'
}

const formatBytes = (b) => {
  if (!b) return '0 B'
  if (b >= 1e9) return (b / 1e9).toFixed(2) + ' GB'
  if (b >= 1e6) return (b / 1e6).toFixed(2) + ' MB'
  if (b >= 1e3) return (b / 1e3).toFixed(2) + ' KB'
  return b + ' B'
}

onMounted(() => { fetchFlows(); setInterval(fetchFlows, 30000) })
</script>

<style scoped>
.drawer-enter-active, .drawer-leave-active { transition: all 0.3s ease; }
.drawer-enter-from, .drawer-leave-to { opacity: 0; }
.drawer-enter-from > div:last-child, .drawer-leave-to > div:last-child { transform: translateX(100%); }
</style>
