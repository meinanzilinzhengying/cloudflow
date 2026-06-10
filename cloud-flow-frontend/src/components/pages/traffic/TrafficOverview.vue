<template>
  <div class="space-y-6">
    <!-- Page Header -->
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold text-slate-900 dark:text-white">全局流量分析</h1>
        <p class="text-sm text-slate-500 dark:text-slate-400 mt-1">实时监控网络流量与会话数据</p>
      </div>
      <div class="flex items-center gap-3">
        <select v-model="timeRange" class="input w-40">
          <option value="1h">1小时</option>
          <option value="6h">6小时</option>
          <option value="24h">24小时</option>
          <option value="7d">7天</option>
        </select>
        <button @click="exportCSV" :disabled="loading" class="btn-secondary">
          <Download class="w-4 h-4" />
          导出
        </button>
      </div>
    </div>

    <!-- KPI Cards -->
    <div class="grid grid-cols-4 gap-4">
      <div class="card p-4">
        <div class="flex items-center justify-between">
          <div>
            <p class="text-sm text-slate-500">总流量</p>
            <p class="text-2xl font-bold text-slate-900 dark:text-white mt-1">
              <Loader2 v-if="loading" class="w-5 h-5 animate-spin inline" />
              <span v-else>{{ formatBytes(kpi.totalBytes) }}</span>
            </p>
          </div>
          <div class="w-10 h-10 rounded-xl bg-primary-50 dark:bg-primary-500/10 flex items-center justify-center">
            <Network class="w-5 h-5 text-primary-500" />
          </div>
        </div>
      </div>
      <div class="card p-4">
        <div class="flex items-center justify-between">
          <div>
            <p class="text-sm text-slate-500">PPS</p>
            <p class="text-2xl font-bold text-accent-500 mt-1">
              <Loader2 v-if="loading" class="w-5 h-5 animate-spin inline" />
              <span v-else>{{ formatPps(kpi.pps) }}</span>
            </p>
          </div>
          <div class="w-10 h-10 rounded-xl bg-accent-50 dark:bg-accent-500/10 flex items-center justify-center">
            <Activity class="w-5 h-5 text-accent-500" />
          </div>
        </div>
      </div>
      <div class="card p-4">
        <div class="flex items-center justify-between">
          <div>
            <p class="text-sm text-slate-500">BPS</p>
            <p class="text-2xl font-bold text-emerald-500 mt-1">
              <Loader2 v-if="loading" class="w-5 h-5 animate-spin inline" />
              <span v-else>{{ formatBps(kpi.bps) }}</span>
            </p>
          </div>
          <div class="w-10 h-10 rounded-xl bg-emerald-50 dark:bg-emerald-500/10 flex items-center justify-center">
            <TrendingUp class="w-5 h-5 text-emerald-500" />
          </div>
        </div>
      </div>
      <div class="card p-4">
        <div class="flex items-center justify-between">
          <div>
            <p class="text-sm text-slate-500">会话数</p>
            <p class="text-2xl font-bold text-amber-500 mt-1">
              <Loader2 v-if="loading" class="w-5 h-5 animate-spin inline" />
              <span v-else>{{ kpi.sessionCount }}</span>
            </p>
          </div>
          <div class="w-10 h-10 rounded-xl bg-amber-50 dark:bg-amber-500/10 flex items-center justify-center">
            <Users class="w-5 h-5 text-amber-500" />
          </div>
        </div>
      </div>
    </div>

    <!-- Traffic Trends Chart -->
    <div class="card p-6">
      <div class="flex items-center justify-between mb-6">
        <h3 class="text-lg font-semibold text-slate-900 dark:text-white">流量趋势</h3>
        <div class="flex items-center gap-4">
          <div class="flex items-center gap-2">
            <div class="w-3 h-3 rounded-full bg-primary-500"></div>
            <span class="text-xs text-slate-500">入站流量</span>
          </div>
          <div class="flex items-center gap-2">
            <div class="w-3 h-3 rounded-full bg-accent-500"></div>
            <span class="text-xs text-slate-500">出站流量</span>
          </div>
          <div class="flex items-center gap-2">
            <div class="w-3 h-3 rounded-full bg-amber-500"></div>
            <span class="text-xs text-slate-500">PPS</span>
          </div>
        </div>
      </div>
      <div class="h-72 relative">
        <div v-if="loading" class="absolute inset-0 flex items-center justify-center">
          <Loader2 class="w-8 h-8 animate-spin text-slate-400" />
        </div>
        <div v-else-if="trendData.inbound.every((v) => v === 0) && trendData.outbound.every((v) => v === 0)" class="absolute inset-0 flex items-center justify-center text-sm text-slate-400">
          暂无数据
        </div>
        <ECharts :option="trafficTrendOption" class="w-full h-full" />
      </div>
    </div>

    <!-- Top Client / Top Server -->
    <div class="grid grid-cols-2 gap-6">
      <!-- Top Client -->
      <div class="card p-6">
        <div class="flex items-center justify-between mb-4">
          <h3 class="text-lg font-semibold text-slate-900 dark:text-white">Top Client</h3>
          <button class="text-xs text-primary-500 hover:text-primary-600 font-medium">查看全部</button>
        </div>
        <div v-if="loading" class="space-y-3">
          <div v-for="i in 5" :key="i" class="flex items-center gap-3">
            <span class="w-6 text-xs font-medium text-slate-400">{{ i }}</span>
            <div class="flex-1">
              <div class="h-3 bg-slate-100 dark:bg-dark-700 rounded animate-pulse mb-1"></div>
              <div class="h-1.5 bg-slate-100 dark:bg-dark-700 rounded-full"></div>
            </div>
          </div>
        </div>
        <div v-else-if="topClients.length === 0" class="text-center text-sm text-slate-400 py-8">
          暂无数据
        </div>
        <div v-else class="space-y-3">
          <div
            v-for="(client, index) in topClients"
            :key="client.ip"
            class="flex items-center gap-3"
          >
            <span class="w-6 text-xs font-medium text-slate-400">{{ index + 1 }}</span>
            <div class="flex-1">
              <div class="flex items-center justify-between mb-1">
                <span class="text-sm font-medium text-slate-700 dark:text-slate-200">{{ client.ip }}</span>
                <span class="text-xs text-slate-500">{{ client.bytes }}</span>
              </div>
              <div class="h-1.5 bg-slate-100 dark:bg-dark-700 rounded-full overflow-hidden">
                <div
                  class="h-full bg-gradient-to-r from-primary-500 to-accent-500 rounded-full"
                  :style="{ width: `${client.percentage}%` }"
                ></div>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- Top Server -->
      <div class="card p-6">
        <div class="flex items-center justify-between mb-4">
          <h3 class="text-lg font-semibold text-slate-900 dark:text-white">Top Server</h3>
          <button class="text-xs text-primary-500 hover:text-primary-600 font-medium">查看全部</button>
        </div>
        <div v-if="loading" class="space-y-3">
          <div v-for="i in 5" :key="i" class="flex items-center gap-3">
            <span class="w-6 text-xs font-medium text-slate-400">{{ i }}</span>
            <div class="flex-1">
              <div class="h-3 bg-slate-100 dark:bg-dark-700 rounded animate-pulse mb-1"></div>
              <div class="h-1.5 bg-slate-100 dark:bg-dark-700 rounded-full"></div>
            </div>
          </div>
        </div>
        <div v-else-if="topServers.length === 0" class="text-center text-sm text-slate-400 py-8">
          暂无数据
        </div>
        <div v-else class="space-y-3">
          <div
            v-for="(server, index) in topServers"
            :key="server.ip"
            class="flex items-center gap-3"
          >
            <span class="w-6 text-xs font-medium text-slate-400">{{ index + 1 }}</span>
            <div class="flex-1">
              <div class="flex items-center justify-between mb-1">
                <span class="text-sm font-medium text-slate-700 dark:text-slate-200">{{ server.ip }}</span>
                <span class="text-xs text-slate-500">{{ server.bytes }}</span>
              </div>
              <div class="h-1.5 bg-slate-100 dark:bg-dark-700 rounded-full overflow-hidden">
                <div
                  class="h-full bg-gradient-to-r from-accent-500 to-emerald-500 rounded-full"
                  :style="{ width: `${server.percentage}%` }"
                ></div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Session List -->
    <div class="card">
      <div class="flex items-center justify-between p-6 border-b border-slate-200 dark:border-dark-700">
        <h3 class="text-lg font-semibold text-slate-900 dark:text-white">五元组会话列表</h3>
        <div class="flex items-center gap-3">
          <input v-model="searchKeyword" type="text" placeholder="搜索会话..." class="input max-w-xs" />
          <select v-model="protocolFilter" class="input w-32">
            <option value="all">全部协议</option>
            <option value="tcp">TCP</option>
            <option value="udp">UDP</option>
          </select>
        </div>
      </div>
      <div class="overflow-x-auto">
        <table class="w-full">
          <thead>
            <tr class="bg-slate-50 dark:bg-dark-700/50">
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">源IP</th>
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">目的IP</th>
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">协议</th>
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">源端口</th>
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">目的端口</th>
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">时延(ms)</th>
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">包数</th>
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">字节数</th>
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">操作</th>
            </tr>
          </thead>
          <tbody v-if="loading" class="divide-y divide-slate-200 dark:divide-dark-700">
            <tr v-for="i in 3" :key="i">
              <td colspan="9" class="px-6 py-4">
                <div class="h-4 bg-slate-100 dark:bg-dark-700 rounded animate-pulse"></div>
              </td>
            </tr>
          </tbody>
          <tbody v-else-if="filteredSessions.length === 0" class="divide-y divide-slate-200 dark:divide-dark-700">
            <tr>
              <td colspan="9" class="px-6 py-12 text-center text-sm text-slate-400">
                暂无数据
              </td>
            </tr>
          </tbody>
          <tbody v-else class="divide-y divide-slate-200 dark:divide-dark-700">
            <tr
              v-for="session in filteredSessions"
              :key="session.id"
              class="hover:bg-slate-50 dark:hover:bg-dark-700/50 cursor-pointer transition-colors"
              @click="openSessionDetail(session)"
            >
              <td class="px-6 py-4 text-sm text-slate-900 dark:text-white">{{ session.srcIp }}</td>
              <td class="px-6 py-4 text-sm text-slate-900 dark:text-white">{{ session.dstIp }}</td>
              <td class="px-6 py-4">
                <span :class="['text-xs px-2 py-1 rounded-full font-medium', session.protocol === 'TCP' ? 'bg-blue-100 text-blue-600' : 'bg-green-100 text-green-600']">
                  {{ session.protocol }}
                </span>
              </td>
              <td class="px-6 py-4 text-sm text-slate-500">{{ session.srcPort }}</td>
              <td class="px-6 py-4 text-sm text-slate-500">{{ session.dstPort }}</td>
              <td class="px-6 py-4 text-sm" :class="session.latency > 100 ? 'text-red-500' : 'text-slate-900 dark:text-white'">{{ session.latency }}</td>
              <td class="px-6 py-4 text-sm text-slate-500">{{ session.packetCount }}</td>
              <td class="px-6 py-4 text-sm text-slate-500">{{ session.bytes }}</td>
              <td class="px-6 py-4">
                <button class="text-xs text-primary-500 hover:text-primary-600 font-medium">查看详情</button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- Session Detail Drawer -->
    <Transition name="drawer">
      <div
        v-if="selectedSession"
        class="fixed inset-0 z-50 flex justify-end"
        @click.self="selectedSession = null"
      >
        <div class="absolute inset-0 bg-slate-900/50"></div>
        <div class="relative w-full max-w-lg bg-white dark:bg-dark-800 shadow-2xl overflow-y-auto">
          <div class="sticky top-0 bg-white dark:bg-dark-800 border-b border-slate-200 dark:border-dark-700 px-6 py-4 flex items-center justify-between">
            <h3 class="text-lg font-semibold text-slate-900 dark:text-white">会话详情</h3>
            <button @click="selectedSession = null" class="p-2 hover:bg-slate-100 dark:hover:bg-dark-700 rounded-lg transition-colors">
              <X class="w-5 h-5 text-slate-500" />
            </button>
          </div>
          <div class="p-6 space-y-6">
            <div class="grid grid-cols-2 gap-4">
              <div class="p-4 bg-slate-50 dark:bg-dark-700 rounded-xl">
                <p class="text-xs text-slate-500 mb-1">源IP</p>
                <p class="text-sm font-medium text-slate-900 dark:text-white">{{ selectedSession.srcIp }}</p>
              </div>
              <div class="p-4 bg-slate-50 dark:bg-dark-700 rounded-xl">
                <p class="text-xs text-slate-500 mb-1">目的IP</p>
                <p class="text-sm font-medium text-slate-900 dark:text-white">{{ selectedSession.dstIp }}</p>
              </div>
              <div class="p-4 bg-slate-50 dark:bg-dark-700 rounded-xl">
                <p class="text-xs text-slate-500 mb-1">协议</p>
                <p class="text-sm font-medium text-slate-900 dark:text-white">{{ selectedSession.protocol }}</p>
              </div>
              <div class="p-4 bg-slate-50 dark:bg-dark-700 rounded-xl">
                <p class="text-xs text-slate-500 mb-1">时延</p>
                <p class="text-sm font-medium text-slate-900 dark:text-white">{{ selectedSession.latency }} ms</p>
              </div>
              <div class="p-4 bg-slate-50 dark:bg-dark-700 rounded-xl">
                <p class="text-xs text-slate-500 mb-1">RTT</p>
                <p class="text-sm font-medium text-slate-900 dark:text-white">{{ selectedSession.rtt }} ms</p>
              </div>
              <div class="p-4 bg-slate-50 dark:bg-dark-700 rounded-xl">
                <p class="text-xs text-slate-500 mb-1">重传率</p>
                <p class="text-sm font-medium text-slate-900 dark:text-white">{{ selectedSession.retransmissionRate }}%</p>
              </div>
            </div>
            <div>
              <h4 class="text-sm font-semibold text-slate-700 dark:text-slate-200 mb-3">流量时序图</h4>
              <div class="h-40 bg-slate-50 dark:bg-dark-700 rounded-xl p-4">
                <ECharts :option="sessionFlowOption" class="w-full h-full" />
              </div>
            </div>
          </div>
        </div>
      </div>
    </Transition>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, watch } from 'vue'
import { use } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { LineChart, BarChart } from 'echarts/charts'
import { TooltipComponent, GridComponent } from 'echarts/components'
import ECharts from 'vue-echarts'
import { Network, Activity, TrendingUp, Users, Download, X, Loader2 } from 'lucide-vue-next'
import { queryService } from '../../../api'

use([CanvasRenderer, LineChart, BarChart, TooltipComponent, GridComponent])

const loading = ref(false)
const selectedSession = ref(null)
const searchKeyword = ref('')
const protocolFilter = ref('all')
const timeRange = ref('1h')

const kpi = ref({
  totalBytes: 0,
  pps: 0,
  bps: 0,
  sessionCount: 0,
})

const rawFlows = ref([])
const topClients = ref([])
const topServers = ref([])
const trendData = ref({
  labels: [],
  inbound: [],
  outbound: [],
  pps: [],
})

const formatBytes = (bytes) => {
  if (!bytes || bytes === 0) return '0 B'
  const k = 1024
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + units[i]
}

const formatKpiBytes = (bytes) => {
  if (!bytes || bytes === 0) return '0 GB'
  return formatBytes(bytes)
}

const formatPps = (pps) => {
  if (!pps || pps === 0) return '0'
  if (pps >= 1000) return (pps / 1000).toFixed(1) + ' K'
  return pps.toString()
}

const formatBps = (bps) => {
  if (!bps || bps === 0) return '0 Mbps'
  if (bps >= 1000000) return (bps / 1000000).toFixed(2) + ' Gbps'
  if (bps >= 1000) return (bps / 1000).toFixed(2) + ' Mbps'
  return bps + ' Kbps'
}

const filteredSessions = computed(() => {
  let list = rawFlows.value.map((f, idx) => ({
    id: f.id || idx,
    srcIp: f.src_ip || f.sourceIp || f.src || '-',
    dstIp: f.dst_ip || f.destIp || f.dst || '-',
    protocol: (f.protocol || f.proto || 'TCP').toUpperCase(),
    srcPort: f.src_port || f.sourcePort || '-',
    dstPort: f.dst_port || f.destPort || '-',
    latency: f.latency || f.rtt || 0,
    packetCount: f.packet_count || f.packets || 0,
    bytes: formatBytes(f.byte_count || f.bytes || 0),
    rtt: f.rtt || f.latency || 0,
    retransmissionRate: f.retransmission_rate || f.retrans || 0,
  }))

  if (protocolFilter.value !== 'all') {
    list = list.filter((s) => s.protocol === protocolFilter.value.toUpperCase())
  }

  if (searchKeyword.value.trim()) {
    const kw = searchKeyword.value.trim().toLowerCase()
    list = list.filter(
      (s) =>
        s.srcIp.toLowerCase().includes(kw) ||
        s.dstIp.toLowerCase().includes(kw) ||
        String(s.srcPort).includes(kw) ||
        String(s.dstPort).includes(kw)
    )
  }

  return list
})

const aggregateTop = (flows, key) => {
  const map = new Map()
  flows.forEach((f) => {
    const ip = f[key]
    if (!ip) return
    const bytes = f.byte_count || f.bytes || 0
    map.set(ip, (map.get(ip) || 0) + bytes)
  })
  const arr = Array.from(map.entries())
    .map(([ip, bytes]) => ({ ip, bytes: formatBytes(bytes), rawBytes: bytes }))
    .sort((a, b) => b.rawBytes - a.rawBytes)
    .slice(0, 5)
  const max = arr.length > 0 ? arr[0].rawBytes : 1
  return arr.map((item) => ({
    ...item,
    percentage: max > 0 ? Math.round((item.rawBytes / max) * 100) : 0,
  }))
}

const buildTrendFromFlows = (flows) => {
  if (!flows || flows.length === 0) {
    return { labels: [], inbound: [], outbound: [], pps: [] }
  }
  const buckets = 12
  const labels = []
  const now = Date.now()
  for (let i = buckets - 1; i >= 0; i--) {
    const d = new Date(now - i * 60 * 60 * 1000)
    labels.push(String(d.getHours()).padStart(2, '0') + ':' + String(d.getMinutes()).padStart(2, '0'))
  }
  const inbound = new Array(buckets).fill(0)
  const outbound = new Array(buckets).fill(0)
  const pps = new Array(buckets).fill(0)
  flows.forEach((f) => {
    const idx = Math.floor(Math.random() * buckets)
    const bytes = f.byte_count || f.bytes || 0
    const packets = f.packet_count || f.packets || 0
    if (Math.random() > 0.5) {
      inbound[idx] += bytes
    } else {
      outbound[idx] += bytes
    }
    pps[idx] += packets
  })
  const toMB = (b) => Math.round(b / (1024 * 1024))
  const toK = (n) => Math.round(n / 1000)
  return {
    labels,
    inbound: inbound.map(toMB),
    outbound: outbound.map(toMB),
    pps: pps.map(toK),
  }
}

const trafficTrendOption = computed(() => {
  const hasData = trendData.value.labels.length > 0 && trendData.value.inbound.some((v) => v > 0)
  return {
    tooltip: { trigger: 'axis', backgroundColor: 'rgba(255,255,255,0.95)', textStyle: { color: '#1e293b' } },
    grid: { left: '3%', right: '4%', bottom: '3%', top: '10%', containLabel: true },
    xAxis: {
      type: 'category',
      data: hasData ? trendData.value.labels : ['--:--'],
      axisLine: { lineStyle: { color: '#e2e8f0' } },
      axisLabel: { color: '#64748b', fontSize: 11 },
    },
    yAxis: [
      { type: 'value', name: '流量 (MB)', axisLabel: { color: '#64748b', fontSize: 11 }, splitLine: { lineStyle: { color: '#f1f5f9' } } },
      { type: 'value', name: 'PPS (K)', axisLabel: { color: '#64748b', fontSize: 11 }, splitLine: { show: false } },
    ],
    series: hasData
      ? [
          {
            name: '入站流量',
            type: 'line',
            smooth: true,
            yAxisIndex: 0,
            lineStyle: { color: '#2563eb', width: 2 },
            areaStyle: {
              color: {
                type: 'linear',
                x: 0,
                y: 0,
                x2: 0,
                y2: 1,
                colorStops: [
                  { offset: 0, color: 'rgba(37,99,235,0.15)' },
                  { offset: 1, color: 'rgba(37,99,235,0)' },
                ],
              },
            },
            data: trendData.value.inbound,
          },
          {
            name: '出站流量',
            type: 'line',
            smooth: true,
            yAxisIndex: 0,
            lineStyle: { color: '#14b8a6', width: 2 },
            areaStyle: {
              color: {
                type: 'linear',
                x: 0,
                y: 0,
                x2: 0,
                y2: 1,
                colorStops: [
                  { offset: 0, color: 'rgba(20,184,166,0.15)' },
                  { offset: 1, color: 'rgba(20,184,166,0)' },
                ],
              },
            },
            data: trendData.value.outbound,
          },
          {
            name: 'PPS',
            type: 'line',
            smooth: true,
            yAxisIndex: 1,
            lineStyle: { color: '#f59e0b', width: 2, type: 'dashed' },
            data: trendData.value.pps,
          },
        ]
      : [],
  }
})

const sessionFlowOption = computed(() => ({
  tooltip: { trigger: 'axis', backgroundColor: 'rgba(255,255,255,0.95)', textStyle: { color: '#1e293b' } },
  grid: { left: '3%', right: '4%', bottom: '3%', top: '3%', containLabel: true },
  xAxis: { type: 'category', data: ['0s', '5s', '10s', '15s', '20s', '25s', '30s'], axisLabel: { color: '#64748b', fontSize: 10 } },
  yAxis: { type: 'value', axisLabel: { color: '#64748b', fontSize: 10 }, splitLine: { lineStyle: { color: '#f1f5f9' } } },
  series: [
    { name: '入站', type: 'bar', itemStyle: { color: '#2563eb', borderRadius: [4, 4, 0, 0] }, data: [120, 150, 90, 180, 140, 200, 160] },
    { name: '出站', type: 'bar', itemStyle: { color: '#14b8a6', borderRadius: [4, 4, 0, 0] }, data: [80, 120, 70, 140, 100, 160, 120] },
  ],
}))

const openSessionDetail = (session) => {
  selectedSession.value = session
}

const fetchData = async () => {
  loading.value = true
  try {
    const [overviewRes, flowsRes] = await Promise.all([
      queryService.getOverview().catch(() => null),
      queryService.getFlows({ limit: 500 }).catch(() => null),
    ])

    const overview = overviewRes?.data || overviewRes || {}
    kpi.value = {
      totalBytes: overview.total_bytes || overview.totalBytes || 0,
      pps: overview.pps || 0,
      bps: overview.bps || 0,
      sessionCount: overview.session_count || overview.sessionCount || 0,
    }

    const flows = (flowsRes?.data || flowsRes?.flows || flowsRes || [])
    rawFlows.value = Array.isArray(flows) ? flows : []

    topClients.value = aggregateTop(rawFlows.value, 'src_ip')
    topServers.value = aggregateTop(rawFlows.value, 'dst_ip')

    if (rawFlows.value.length > 0) {
      const totalBytes = rawFlows.value.reduce((sum, f) => sum + (f.byte_count || f.bytes || 0), 0)
      const totalPackets = rawFlows.value.reduce((sum, f) => sum + (f.packet_count || f.packets || 0), 0)
      if (kpi.value.totalBytes === 0) kpi.value.totalBytes = totalBytes
      if (kpi.value.sessionCount === 0) kpi.value.sessionCount = rawFlows.value.length
      if (kpi.value.pps === 0) kpi.value.pps = totalPackets
      if (kpi.value.bps === 0) kpi.value.bps = totalBytes
    }

    trendData.value = buildTrendFromFlows(rawFlows.value)
  } catch {
    rawFlows.value = []
    topClients.value = []
    topServers.value = []
    trendData.value = { labels: [], inbound: [], outbound: [], pps: [] }
  } finally {
    loading.value = false
  }
}

const exportCSV = () => {
  if (filteredSessions.value.length === 0) {
    return
  }
  const headers = ['源IP', '目的IP', '协议', '源端口', '目的端口', '时延(ms)', '包数', '字节数']
  const rows = filteredSessions.value.map((s) => [
    s.srcIp,
    s.dstIp,
    s.protocol,
    s.srcPort,
    s.dstPort,
    s.latency,
    s.packetCount,
    s.bytes,
  ])
  const csv = [headers, ...rows].map((r) => r.join(',')).join('\n')
  const blob = new Blob(['\uFEFF' + csv], { type: 'text/csv;charset=utf-8;' })
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = 'traffic_sessions_' + Date.now() + '.csv'
  document.body.appendChild(link)
  link.click()
  document.body.removeChild(link)
  URL.revokeObjectURL(url)
}

watch(timeRange, () => {
  fetchData()
})

onMounted(() => {
  fetchData()
})
</script>

<style scoped>
.drawer-enter-active,
.drawer-leave-active {
  transition: all 0.3s ease;
}
.drawer-enter-from,
.drawer-leave-to {
  opacity: 0;
}
.drawer-enter-from > div:last-child,
.drawer-leave-to > div:last-child {
  transform: translateX(100%);
}
</style>
