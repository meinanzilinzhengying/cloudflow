<template>
  <div class="space-y-6 animate-fade-in">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold text-slate-900 dark:text-white">运营驾驶舱</h1>
        <p class="text-sm text-slate-500 dark:text-slate-400 mt-1">实时监控云原生网络流量与系统健康状态</p>
      </div>
      <div class="flex items-center gap-3">
        <div :class="['flex items-center gap-2 px-3 py-1.5 rounded-lg border', systemHealthy ? 'bg-green-50 dark:bg-green-500/10 border-green-200 dark:border-green-500/20' : 'bg-red-50 dark:bg-red-500/10 border-red-200 dark:border-red-500/20']">
          <div :class="['w-2 h-2 rounded-full', systemHealthy ? 'bg-green-500 animate-pulse' : 'bg-red-500']"></div>
          <span :class="['text-xs font-medium', systemHealthy ? 'text-green-600 dark:text-green-400' : 'text-red-600 dark:text-red-400']">
            {{ systemHealthy ? '探针运行中' : '探针异常' }}
          </span>
        </div>
      </div>
    </div>

    <div class="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-6 gap-4">
      <KPICard v-for="(kpi, index) in kpis" :key="kpi.id" :title="kpi.title" :value="kpi.value" :unit="kpi.unit" :change="kpi.change" :icon="kpi.icon" :color="kpi.color" :loading="loading" :style="{ animationDelay: (index * 50) + 'ms' }" class="animate-slide-up" />
    </div>

    <div class="grid grid-cols-1 lg:grid-cols-3 gap-6">
      <div class="lg:col-span-2 card p-6">
        <div class="flex items-center justify-between mb-6">
          <div>
            <h3 class="text-lg font-semibold text-slate-900 dark:text-white">流量趋势</h3>
            <p class="text-sm text-slate-500 dark:text-slate-400 mt-0.5">每10秒聚合流量</p>
          </div>
        </div>
        <div class="h-64">
          <div v-if="!loading && !hasTrafficData" class="flex items-center justify-center h-full text-slate-400 dark:text-slate-500">
            <div class="text-center">
              <p class="text-sm">暂无流量数据</p>
            </div>
          </div>
          <ECharts v-else :option="trafficChartOption" class="w-full h-full" />
        </div>
      </div>

      <div class="card p-6">
        <div class="flex items-center justify-between mb-6">
          <div>
            <h3 class="text-lg font-semibold text-slate-900 dark:text-white">Top 源 IP</h3>
            <p class="text-sm text-slate-500 dark:text-slate-400 mt-0.5">按流量排名</p>
          </div>
        </div>
        <div class="space-y-3">
          <div v-if="loading" class="space-y-3">
            <div v-for="i in 5" :key="'ts-skel-' + i" class="animate-pulse"><div class="h-4 bg-slate-100 dark:bg-dark-700 rounded w-3/4 mb-1"></div><div class="h-1.5 bg-slate-100 dark:bg-dark-700 rounded w-full"></div></div>
          </div>
          <div v-else-if="topSources.length === 0" class="flex flex-col items-center justify-center py-12 text-slate-400 dark:text-slate-500">
            <p class="text-sm">暂无数据</p>
          </div>
          <template v-else>
            <div v-for="(item, index) in topSources" :key="item.ip" class="flex items-center gap-3">
              <span class="w-6 text-xs font-medium text-slate-400">{{ index + 1 }}</span>
              <div class="flex-1 min-w-0">
                <div class="flex items-center justify-between mb-1">
                  <span class="text-sm font-medium text-slate-700 dark:text-slate-200 truncate">{{ item.ip }}</span>
                  <span class="text-xs text-slate-500 dark:text-slate-400">{{ item.bytes }}</span>
                </div>
                <div class="h-1.5 bg-slate-100 dark:bg-dark-700 rounded-full overflow-hidden">
                  <div class="h-full bg-gradient-to-r from-primary-500 to-accent-500 rounded-full transition-all duration-500" :style="{ width: item.pct + '%' }"></div>
                </div>
              </div>
            </div>
          </template>
        </div>
      </div>
    </div>

    <div class="grid grid-cols-1 lg:grid-cols-3 gap-6">
      <div class="card p-6">
        <div class="flex items-center justify-between mb-4">
          <div>
            <h3 class="text-lg font-semibold text-slate-900 dark:text-white">探针状态</h3>
            <p class="text-sm text-slate-500 dark:text-slate-400 mt-0.5">eBPF 探针运行信息</p>
          </div>
        </div>
        <div v-if="probeStatus" class="space-y-3">
          <div class="p-4 rounded-xl bg-slate-50 dark:bg-dark-700/50">
            <p class="text-xs text-slate-500 mb-1">运行状态</p>
            <p class="text-sm font-medium" :class="probeStatus.status === 'running' ? 'text-green-600 dark:text-green-400' : 'text-red-600 dark:text-red-400'">
              {{ probeStatus.status === 'running' ? '● 运行中' : '○ 已停止' }}
            </p>
          </div>
          <div class="grid grid-cols-2 gap-3">
            <div class="p-4 rounded-xl bg-slate-50 dark:bg-dark-700/50">
              <p class="text-xs text-slate-500 mb-1">采集数据</p>
              <p class="text-sm font-medium text-slate-900 dark:text-white">{{ probeStatus.flows_total || '0' }} 条</p>
            </div>
            <div class="p-4 rounded-xl bg-slate-50 dark:bg-dark-700/50">
              <p class="text-xs text-slate-500 mb-1">运行时间</p>
              <p class="text-sm font-medium text-slate-900 dark:text-white">{{ probeUptime }}</p>
            </div>
          </div>
        </div>
        <div v-else class="flex items-center justify-center py-12 text-slate-400">加载中...</div>
      </div>

      <div class="card p-6">
        <div class="flex items-center justify-between mb-4">
          <div>
            <h3 class="text-lg font-semibold text-slate-900 dark:text-white">实时告警</h3>
            <p class="text-sm text-slate-500 dark:text-slate-400 mt-0.5">最近告警事件</p>
          </div>
        </div>
        <div class="space-y-3 max-h-80 overflow-y-auto">
          <div v-if="loading" class="space-y-3">
            <div v-for="i in 3" :key="'al-skel-' + i" class="animate-pulse flex gap-3 p-3"><div class="w-8 h-8 bg-slate-100 dark:bg-dark-700 rounded-lg"></div><div class="flex-1"><div class="h-4 bg-slate-100 dark:bg-dark-700 rounded w-2/3 mb-1"></div><div class="h-3 bg-slate-100 dark:bg-dark-700 rounded w-1/2"></div></div></div>
          </div>
          <div v-else class="flex flex-col items-center justify-center py-12 text-slate-400 dark:text-slate-500">
            <p class="text-sm">暂无告警</p>
          </div>
        </div>
      </div>

      <div class="card p-6">
        <div class="flex items-center justify-between mb-4">
          <div>
            <h3 class="text-lg font-semibold text-slate-900 dark:text-white">协议分布</h3>
            <p class="text-sm text-slate-500 dark:text-slate-400 mt-0.5">按协议统计</p>
          </div>
        </div>
        <div class="h-64">
          <ECharts :option="protocolChartOption" class="w-full h-full" />
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import ECharts from 'vue-echarts'
import { use } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { LineChart, PieChart } from 'echarts/charts'
import { TooltipComponent, LegendComponent, GridComponent } from 'echarts/components'

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

import { TrendingUp, Activity, Server, Layers, Bell } from 'lucide-vue-next'
import KPICard from '../common/KPICard.vue'

use([CanvasRenderer, LineChart, PieChart, TooltipComponent, LegendComponent, GridComponent])

const loading = ref(true)
const systemHealthy = ref(false)
const flows = ref([])
const probeStatus = ref(null)
const probeUptime = ref('')

const kpis = ref([
  { id: 1, title: '总流量', value: '0', unit: 'GB', change: '-', icon: TrendingUp, color: 'primary' },
  { id: 2, title: '总包数', value: '0', unit: 'M', change: '-', icon: Activity, color: 'accent' },
  { id: 3, title: '连接数', value: '0', unit: '个', change: '-', icon: Layers, color: 'success' },
  { id: 4, title: '平均时延', value: '0', unit: 'ms', change: '-', icon: Activity, color: 'warning' },
  { id: 5, title: '探测状态', value: '检查中', unit: '', change: '-', icon: Server, color: 'danger' },
  { id: 6, title: '数据记录', value: '0', unit: '条', change: '-', icon: Bell, color: 'primary' },
])

const topSources = ref([])

const hasTrafficData = computed(() => {
  const s = trafficChartOption.value.series
  return s && s[0] && s[0].data && s[0].data.length > 0
})

const trafficChartOption = computed(() => {
  const map = {}
  flows.value.forEach(function(f) {
    if (!f.timestamp || !f.bytes) return
    const d = new Date(f.timestamp)
    const key = String(d.getHours()).padStart(2,'0') + ':' + String(d.getMinutes()).padStart(2,'0')
    map[key] = (map[key] || 0) + f.bytes
  })
  const keys = Object.keys(map).sort()
  const vals = keys.map(function(k) { return (map[k] / 1048576).toFixed(2) })
  return {
    tooltip: { trigger: 'axis' },
    grid: { left: '3%', right: '4%', bottom: '10%', top: '5%', containLabel: true },
    xAxis: { type: 'category', data: keys, axisLabel: { rotate: 45, fontSize: 10 } },
    yAxis: { type: 'value', name: 'MB' },
    series: [{ name: '流量', type: 'line', smooth: true, lineStyle: { color: '#2563eb', width: 2 }, areaStyle: { color: { type: 'linear', x: 0, y: 0, x2: 0, y2: 1, colorStops: [{ offset: 0, color: 'rgba(37,99,235,0.15)' }, { offset: 1, color: 'rgba(37,99,235,0)' }] } }, data: vals }]
  }
})

const protocolChartOption = computed(() => {
  const map = {}
  flows.value.forEach(function(f) { var p = appProto(f.protocol, f.dst_port); map[p] = (map[p] || 0) + 1 })
  const data = Object.entries(map).map(function(e) { return { name: e[0], value: e[1] } })
  return {
    tooltip: { trigger: 'item' },
    legend: { bottom: 0, textStyle: { fontSize: 11 } },
    series: [{ type: 'pie', radius: ['40%', '70%'], data: data, emphasis: { itemStyle: { shadowBlur: 10 } } }]
  }
})

const fetchData = async function() {
  loading.value = true
  try {
    // Get flows data
    const res = await fetch('/api/query/flows?limit=2000')
    if (res.ok) {
      const data = await res.json()
      flows.value = data.records || []
    }
  } catch(e) { console.warn('[Dashboard] Flows fetch error:', e) }

  // Calculate KPIs from real data
  const totalB = flows.value.reduce(function(s, f) { return s + (f.bytes || 0) }, 0)
  const totalP = flows.value.reduce(function(s, f) { return s + (f.packets || 0) }, 0)
  const validLat = flows.value.filter(function(f) { return f.latency_ms > 0 })
  const avgL = validLat.length ? (validLat.reduce(function(s, f) { return s + f.latency_ms }, 0) / validLat.length).toFixed(2) : '0'

  kpis.value = [
    { id: 1, title: '总流量', value: formatKPI(totalB), unit: '', change: '', icon: TrendingUp, color: 'primary' },
    { id: 2, title: '总包数', value: formatKPI(totalP), unit: '', change: '', icon: Activity, color: 'accent' },
    { id: 3, title: '连接数', value: String(flows.value.length), unit: '条', change: '', icon: Layers, color: 'success' },
    { id: 4, title: '平均时延', value: avgL, unit: 'ms', change: '', icon: Activity, color: 'warning' },
    { id: 5, title: '探测状态', value: '检查中', unit: '', change: '', icon: Server, color: 'danger' },
    { id: 6, title: '数据记录', value: String(totalP), unit: '包', change: '', icon: Bell, color: 'primary' },
  ]

  // Top sources
  const srcMap = {}
  flows.value.forEach(function(f) { if (f.src_ip) srcMap[f.src_ip] = (srcMap[f.src_ip] || 0) + (f.bytes || 0) })
  const sorted = Object.entries(srcMap).sort(function(a, b) { return b[1] - a[1] }).slice(0, 5)
  const maxVal = sorted.length ? Math.max.apply(null, sorted.map(function(s) { return s[1] })) : 1
  topSources.value = sorted.map(function(e) { return { ip: e[0], bytes: formatBytes(e[1]), pct: ((e[1] / maxVal) * 100).toFixed(0) } })

  // Get probe status
  try {
    const pr = await fetch('http://192.168.58.131:9090/api/probe/status')
    if (pr.ok) {
      const pd = await pr.json()
      probeStatus.value = pd
      systemHealthy.value = pd.status === 'running'
      kpis.value[4] = { id: 5, title: '探测状态', value: pd.status === 'running' ? '运行中' : '已停止', unit: '', change: '', icon: Server, color: pd.status === 'running' ? 'success' : 'danger' }
      kpis.value[5] = { id: 6, title: '数据记录', value: pd.flows_total || '0', unit: '条', change: '', icon: Bell, color: 'primary' }
      // Parse uptime
      if (pd.uptime) {
        var parts = pd.uptime.split(' ')
        if (parts.length >= 4) probeUptime.value = parts[2] + ' ' + parts[3]
      }
    }
  } catch(e) { console.warn('[Dashboard] Probe fetch error:', e) }

  loading.value = false
}

const formatKPI = function(v) {
  if (!v) return '0'
  if (v >= 1e12) return (v / 1e12).toFixed(2) + 'T'
  if (v >= 1e9) return (v / 1e9).toFixed(2) + 'G'
  if (v >= 1e6) return (v / 1e6).toFixed(2) + 'M'
  if (v >= 1e3) return (v / 1e3).toFixed(2) + 'K'
  return String(v)
}
const formatBytes = function(b) {
  if (!b) return '0 B'
  if (b >= 1e9) return (b / 1e9).toFixed(2) + ' GB'
  if (b >= 1e6) return (b / 1e6).toFixed(2) + ' MB'
  if (b >= 1e3) return (b / 1e3).toFixed(2) + ' KB'
  return b + ' B'
}

onMounted(function() { fetchData(); setInterval(fetchData, 30000) })
</script>
