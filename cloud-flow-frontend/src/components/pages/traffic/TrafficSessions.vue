<template>
  <div class="space-y-6">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold text-slate-900 dark:text-white">会话分析</h1>
        <p class="text-sm text-slate-500 dark:text-slate-400 mt-1">深入分析五元组网络会话数据</p>
      </div>
      <div class="flex items-center gap-3">
        <select v-model="timeRange" class="input w-32">
          <option value="1h">1小时</option>
          <option value="6h">6小时</option>
          <option value="24h">24小时</option>
        </select>
        <button @click="fetchData" :disabled="loading" class="btn-secondary">
          <RefreshCw v-if="loading" class="w-4 h-4 animate-spin" />
          <RefreshCw v-else class="w-4 h-4" />
          刷新
        </button>
      </div>
    </div>

    <div class="grid grid-cols-4 gap-4">
      <div class="card p-4">
        <p class="text-sm text-slate-500">活跃会话</p>
        <p class="text-2xl font-bold text-slate-900 dark:text-white mt-1">
          <Loader2 v-if="loading" class="w-5 h-5 animate-spin inline" />
          <span v-else>{{ kpi.sessionCount }}</span>
        </p>
      </div>
      <div class="card p-4">
        <p class="text-sm text-slate-500">TCP会话</p>
        <p class="text-2xl font-bold text-primary-500 mt-1">
          <Loader2 v-if="loading" class="w-5 h-5 animate-spin inline" />
          <span v-else>{{ kpi.tcpCount }}</span>
        </p>
      </div>
      <div class="card p-4">
        <p class="text-sm text-slate-500">UDP会话</p>
        <p class="text-2xl font-bold text-accent-500 mt-1">
          <Loader2 v-if="loading" class="w-5 h-5 animate-spin inline" />
          <span v-else>{{ kpi.udpCount }}</span>
        </p>
      </div>
      <div class="card p-4">
        <p class="text-sm text-slate-500">平均时延</p>
        <p class="text-2xl font-bold text-emerald-500 mt-1">
          <Loader2 v-if="loading" class="w-5 h-5 animate-spin inline" />
          <span v-else>{{ kpi.avgLatency }} ms</span>
        </p>
      </div>
    </div>

    <div class="card p-6">
      <h3 class="text-lg font-semibold text-slate-900 dark:text-white mb-6">会话状态分布</h3>
      <div class="h-64 relative">
        <div v-if="loading" class="absolute inset-0 flex items-center justify-center">
          <Loader2 class="w-8 h-8 animate-spin text-slate-400" />
        </div>
        <div v-else-if="sessions.length === 0" class="absolute inset-0 flex items-center justify-center text-sm text-slate-400">
          暂无数据
        </div>
        <ECharts v-else :option="sessionStatusOption" class="w-full h-full" />
      </div>
    </div>

    <div class="card">
      <div class="flex items-center justify-between p-6 border-b border-slate-200 dark:border-dark-700">
        <h3 class="text-lg font-semibold text-slate-900 dark:text-white">五元组会话列表</h3>
        <div class="flex items-center gap-3">
          <input v-model="searchKeyword" type="text" placeholder="搜索IP/端口..." class="input max-w-xs" />
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
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">包数</th>
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">字节数</th>
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">时延(ms)</th>
            </tr>
          </thead>
          <tbody v-if="loading" class="divide-y divide-slate-200 dark:divide-dark-700">
            <tr v-for="i in 5" :key="i">
              <td colspan="8" class="px-6 py-4">
                <div class="h-4 bg-slate-100 dark:bg-dark-700 rounded animate-pulse"></div>
              </td>
            </tr>
          </tbody>
          <tbody v-else-if="filteredSessions.length === 0" class="divide-y divide-slate-200 dark:divide-dark-700">
            <tr>
              <td colspan="8" class="px-6 py-12 text-center text-sm text-slate-400">
                暂无数据
              </td>
            </tr>
          </tbody>
          <tbody v-else class="divide-y divide-slate-200 dark:divide-dark-700">
            <tr
              v-for="session in filteredSessions"
              :key="session.id"
              class="hover:bg-slate-50 dark:hover:bg-dark-700/50 transition-colors"
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
              <td class="px-6 py-4 text-sm text-slate-500">{{ session.packetCount }}</td>
              <td class="px-6 py-4 text-sm text-slate-500">{{ session.bytes }}</td>
              <td class="px-6 py-4 text-sm" :class="session.latency > 100 ? 'text-red-500' : 'text-slate-900 dark:text-white'">{{ session.latency }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, watch } from 'vue'
import { use } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { PieChart } from 'echarts/charts'
import { TooltipComponent, LegendComponent } from 'echarts/components'
import ECharts from 'vue-echarts'
import { Loader2, RefreshCw } from 'lucide-vue-next'
import { queryService } from '../../../api'

use([CanvasRenderer, PieChart, TooltipComponent, LegendComponent])

const loading = ref(false)
const searchKeyword = ref('')
const protocolFilter = ref('all')
const timeRange = ref('1h')
const sessions = ref([])

const kpi = ref({
  sessionCount: 0,
  tcpCount: 0,
  udpCount: 0,
  avgLatency: 0,
})

const formatBytes = (bytes) => {
  if (!bytes || bytes === 0) return '0 B'
  const k = 1024
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + units[i]
}

const filteredSessions = computed(() => {
  let list = sessions.value.map((f, idx) => ({
    id: f.id || idx,
    srcIp: f.src_ip || f.sourceIp || f.src || '-',
    dstIp: f.dst_ip || f.destIp || f.dst || '-',
    protocol: (f.protocol || f.proto || 'TCP').toUpperCase(),
    srcPort: f.src_port || f.sourcePort || '-',
    dstPort: f.dst_port || f.destPort || '-',
    packetCount: f.packet_count || f.packets || 0,
    bytes: formatBytes(f.byte_count || f.bytes || 0),
    latency: f.latency || f.rtt || 0,
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

const sessionStatusOption = computed(() => {
  const tcp = sessions.value.filter((s) => String(s.protocol || '').toUpperCase() === 'TCP').length
  const udp = sessions.value.filter((s) => String(s.protocol || '').toUpperCase() === 'UDP').length
  const other = sessions.value.length - tcp - udp

  const data = []
  if (tcp > 0) data.push({ value: tcp, name: 'TCP', itemStyle: { color: '#2563eb' } })
  if (udp > 0) data.push({ value: udp, name: 'UDP', itemStyle: { color: '#14b8a6' } })
  if (other > 0) data.push({ value: other, name: '其他', itemStyle: { color: '#94a3b8' } })

  return {
    tooltip: { trigger: 'item', backgroundColor: 'rgba(255,255,255,0.95)', textStyle: { color: '#1e293b' } },
    legend: { bottom: 0, textStyle: { color: '#64748b' } },
    series: [{
      type: 'pie',
      radius: ['40%', '70%'],
      avoidLabelOverlap: false,
      itemStyle: { borderRadius: 8, borderColor: '#fff', borderWidth: 2 },
      label: { show: false },
      emphasis: { label: { show: true, fontSize: 14, fontWeight: 'bold' } },
      data,
    }],
  }
})

const fetchData = async () => {
  loading.value = true
  try {
    const res = await queryService.getFlows({ limit: 500, timeRange: timeRange.value }).catch(() => null)
    const flows = (res?.data || res?.flows || res || [])
    sessions.value = Array.isArray(flows) ? flows : []

    const tcp = sessions.value.filter((s) => String(s.protocol || '').toUpperCase() === 'TCP').length
    const udp = sessions.value.filter((s) => String(s.protocol || '').toUpperCase() === 'UDP').length
    const totalLatency = sessions.value.reduce((sum, s) => sum + (s.latency || s.rtt || 0), 0)

    kpi.value = {
      sessionCount: sessions.value.length,
      tcpCount: tcp,
      udpCount: udp,
      avgLatency: sessions.value.length > 0 ? Math.round(totalLatency / sessions.value.length) : 0,
    }
  } catch {
    sessions.value = []
    kpi.value = { sessionCount: 0, tcpCount: 0, udpCount: 0, avgLatency: 0 }
  } finally {
    loading.value = false
  }
}

watch(timeRange, () => {
  fetchData()
})

onMounted(() => {
  fetchData()
})
</script>
