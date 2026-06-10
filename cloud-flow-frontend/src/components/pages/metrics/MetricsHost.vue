<template>
  <div class="space-y-6">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold text-slate-900 dark:text-white">主机指标</h1>
        <p class="text-sm text-slate-500 dark:text-slate-400 mt-1">监控主机性能指标</p>
      </div>
      <button class="btn-secondary" @click="fetchData" :disabled="loading">
        <RefreshCw class="w-4 h-4" :class="{ 'animate-spin': loading }" />
        刷新
      </button>
    </div>

    <div class="grid grid-cols-2 md:grid-cols-5 gap-4">
      <div class="card p-4">
        <div class="flex items-center justify-between">
          <div>
            <p class="text-sm text-slate-500">主机总数</p>
            <p class="text-2xl font-bold text-slate-900 dark:text-white mt-1">{{ totalHosts }}</p>
          </div>
          <div class="w-10 h-10 rounded-xl bg-primary-50 dark:bg-primary-500/10 flex items-center justify-center">
            <Server class="w-5 h-5 text-primary-500" />
          </div>
        </div>
      </div>
      <div class="card p-4">
        <div class="flex items-center justify-between">
          <div>
            <p class="text-sm text-slate-500">平均CPU</p>
            <p class="text-2xl font-bold text-primary-500 mt-1">{{ avgCpu }}%</p>
          </div>
          <div class="w-10 h-10 rounded-xl bg-accent-50 dark:bg-accent-500/10 flex items-center justify-center">
            <Cpu class="w-5 h-5 text-accent-500" />
          </div>
        </div>
      </div>
      <div class="card p-4">
        <div class="flex items-center justify-between">
          <div>
            <p class="text-sm text-slate-500">平均内存</p>
            <p class="text-2xl font-bold text-accent-500 mt-1">{{ avgMemory }}%</p>
          </div>
          <div class="w-10 h-10 rounded-xl bg-emerald-50 dark:bg-emerald-500/10 flex items-center justify-center">
            <Database class="w-5 h-5 text-emerald-500" />
          </div>
        </div>
      </div>
      <div class="card p-4">
        <div class="flex items-center justify-between">
          <div>
            <p class="text-sm text-slate-500">平均磁盘</p>
            <p class="text-2xl font-bold text-emerald-500 mt-1">{{ avgDisk }}%</p>
          </div>
          <div class="w-10 h-10 rounded-xl bg-amber-50 dark:bg-amber-500/10 flex items-center justify-center">
            <HardDrive class="w-5 h-5 text-amber-500" />
          </div>
        </div>
      </div>
      <div class="card p-4">
        <div class="flex items-center justify-between">
          <div>
            <p class="text-sm text-slate-500">网络吞吐</p>
            <p class="text-2xl font-bold text-amber-500 mt-1">{{ networkThroughput }} MB/s</p>
          </div>
          <div class="w-10 h-10 rounded-xl bg-violet-50 dark:bg-violet-500/10 flex items-center justify-center">
            <Activity class="w-5 h-5 text-violet-500" />
          </div>
        </div>
      </div>
    </div>

    <div class="grid grid-cols-1 lg:grid-cols-2 gap-6">
      <div class="card p-6">
        <h3 class="text-lg font-semibold text-slate-900 dark:text-white mb-4">CPU趋势</h3>
        <div v-if="loading" class="h-64 flex items-center justify-center">
          <p class="text-sm text-slate-400 dark:text-slate-500">加载中...</p>
        </div>
        <div v-else-if="!hasTimeseries" class="h-64 flex items-center justify-center">
          <p class="text-sm text-slate-400 dark:text-slate-500">暂无数据</p>
        </div>
        <div v-else class="h-64">
          <ECharts :option="cpuOption" class="w-full h-full" />
        </div>
      </div>
      <div class="card p-6">
        <h3 class="text-lg font-semibold text-slate-900 dark:text-white mb-4">内存趋势</h3>
        <div v-if="loading" class="h-64 flex items-center justify-center">
          <p class="text-sm text-slate-400 dark:text-slate-500">加载中...</p>
        </div>
        <div v-else-if="!hasTimeseries" class="h-64 flex items-center justify-center">
          <p class="text-sm text-slate-400 dark:text-slate-500">暂无数据</p>
        </div>
        <div v-else class="h-64">
          <ECharts :option="memoryOption" class="w-full h-full" />
        </div>
      </div>
    </div>

    <div class="card p-6">
      <h3 class="text-lg font-semibold text-slate-900 dark:text-white mb-4">主机列表</h3>
      <div v-if="loading" class="space-y-3">
        <div v-for="i in 3" :key="i" class="h-16 bg-slate-100 dark:bg-dark-700 rounded-lg animate-pulse"></div>
      </div>
      <div v-else-if="!hosts.length" class="h-40 flex items-center justify-center">
        <p class="text-sm text-slate-400 dark:text-slate-500">暂无数据</p>
      </div>
      <div v-else class="overflow-x-auto">
        <table class="w-full text-sm">
          <thead>
            <tr class="text-left text-slate-500 dark:text-slate-400 border-b border-slate-200 dark:border-dark-700">
              <th class="pb-3 pr-4 font-medium">主机名</th>
              <th class="pb-3 pr-4 font-medium">IP</th>
              <th class="pb-3 pr-4 font-medium">CPU%</th>
              <th class="pb-3 pr-4 font-medium">内存%</th>
              <th class="pb-3 pr-4 font-medium">磁盘%</th>
              <th class="pb-3 pr-4 font-medium">网络入</th>
              <th class="pb-3 pr-4 font-medium">网络出</th>
              <th class="pb-3 font-medium">状态</th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="host in hosts"
              :key="host.key"
              class="border-b border-slate-100 dark:border-dark-700 hover:bg-slate-50 dark:hover:bg-dark-700/50"
            >
              <td class="py-3 pr-4 font-medium text-slate-900 dark:text-white">{{ host.name }}</td>
              <td class="py-3 pr-4 text-slate-600 dark:text-slate-300">{{ host.ip }}</td>
              <td class="py-3 pr-4">
                <span :class="host.cpu >= 80 ? 'text-red-500' : host.cpu >= 60 ? 'text-amber-500' : 'text-emerald-500'">
                  {{ host.cpu }}%
                </span>
              </td>
              <td class="py-3 pr-4">
                <span :class="host.memory >= 80 ? 'text-red-500' : host.memory >= 60 ? 'text-amber-500' : 'text-emerald-500'">
                  {{ host.memory }}%
                </span>
              </td>
              <td class="py-3 pr-4">
                <span :class="host.disk >= 80 ? 'text-red-500' : host.disk >= 60 ? 'text-amber-500' : 'text-emerald-500'">
                  {{ host.disk }}%
                </span>
              </td>
              <td class="py-3 pr-4 text-slate-600 dark:text-slate-300">{{ host.rx }} MB/s</td>
              <td class="py-3 pr-4 text-slate-600 dark:text-slate-300">{{ host.tx }} MB/s</td>
              <td class="py-3">
                <span
                  :class="[
                    'text-xs px-2 py-0.5 rounded-full font-medium',
                    host.status === 'healthy' ? 'bg-emerald-100 text-emerald-600 dark:bg-emerald-500/20 dark:text-emerald-400' :
                    host.status === 'warning' ? 'bg-amber-100 text-amber-600 dark:bg-amber-500/20 dark:text-amber-400' :
                    host.status === 'error' ? 'bg-red-100 text-red-600 dark:bg-red-500/20 dark:text-red-400' :
                    'bg-slate-100 text-slate-600 dark:bg-slate-500/20 dark:text-slate-400'
                  ]"
                >
                  {{ host.statusText }}
                </span>
              </td>
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
import { LineChart } from 'echarts/charts'
import { TooltipComponent, GridComponent } from 'echarts/components'
import ECharts from 'vue-echarts'
import { Server, Cpu, Database, HardDrive, Activity, RefreshCw } from 'lucide-vue-next'
import { queryService } from '../../../api'

use([CanvasRenderer, LineChart, TooltipComponent, GridComponent])

const loading = ref(true)
const hosts = ref([])
const timeseries = ref({ labels: [], cpu: [], memory: [] })

const pick = (obj, keys) => {
  if (!obj) return null
  for (const k of keys) {
    if (obj[k] !== undefined && obj[k] !== null) return obj[k]
  }
  return null
}

const normalizeStatus = (s) => {
  if (!s) return 'normal'
  const str = String(s).toLowerCase()
  if (str.includes('error') || str.includes('critical') || str.includes('down') || str.includes('故障')) return 'error'
  if (str.includes('warn') || str.includes('degrade') || str.includes('告警')) return 'warning'
  if (str.includes('healthy') || str.includes('ok') || str.includes('running') || str.includes('正常')) return 'healthy'
  return str
}

const totalHosts = computed(() => hosts.value.length)

const avgCpu = computed(() => {
  if (!hosts.value.length) return 0
  const sum = hosts.value.reduce((acc, h) => acc + h.cpu, 0)
  return Math.round(sum / hosts.value.length)
})

const avgMemory = computed(() => {
  if (!hosts.value.length) return 0
  const sum = hosts.value.reduce((acc, h) => acc + h.memory, 0)
  return Math.round(sum / hosts.value.length)
})

const avgDisk = computed(() => {
  if (!hosts.value.length) return 0
  const sum = hosts.value.reduce((acc, h) => acc + h.disk, 0)
  return Math.round(sum / hosts.value.length)
})

const networkThroughput = computed(() => {
  if (!hosts.value.length) return 0
  const sum = hosts.value.reduce((acc, h) => acc + h.rx + h.tx, 0)
  return Math.round(sum)
})

const hasTimeseries = computed(() =>
  timeseries.value.labels.length > 0 && (timeseries.value.cpu.length > 0 || timeseries.value.memory.length > 0)
)

const baseLineOption = (data, color, formatter) => ({
  tooltip: { trigger: 'axis', backgroundColor: 'rgba(255,255,255,0.95)', textStyle: { color: '#1e293b' } },
  grid: { left: '3%', right: '4%', bottom: '3%', top: '10%', containLabel: true },
  xAxis: {
    type: 'category',
    data: timeseries.value.labels,
    axisLabel: { color: '#64748b' },
    axisLine: { lineStyle: { color: '#e2e8f0' } },
  },
  yAxis: {
    type: 'value',
    max: 100,
    axisLabel: { color: '#64748b', formatter },
    splitLine: { lineStyle: { color: '#f1f5f9' } },
  },
  series: [{
    type: 'line',
    smooth: true,
    lineStyle: { color, width: 2 },
    itemStyle: { color },
    areaStyle: {
      color: {
        type: 'linear',
        x: 0, y: 0, x2: 0, y2: 1,
        colorStops: [
          { offset: 0, color: `${color}26` },
          { offset: 1, color: `${color}00` },
        ],
      },
    },
    data,
  }],
})

const cpuOption = computed(() => baseLineOption(timeseries.value.cpu, '#2563eb', '{value}%'))
const memoryOption = computed(() => baseLineOption(timeseries.value.memory, '#14b8a6', '{value}%'))

const parseHostsList = (raw) => {
  if (!raw) return []
  const list = Array.isArray(raw) ? raw : pick(raw, ['hosts', 'data', 'items', 'results', 'nodes']) || []
  if (!Array.isArray(list)) return []

  return list
    .map((h, idx) => {
      const rawStatus = pick(h, ['status', 'state', 'health']) || 'healthy'
      const status = normalizeStatus(rawStatus)
      const statusText = status === 'healthy' ? '运行正常' : status === 'warning' ? '告警' : status === 'error' ? '故障' : '未知'
      return {
        key: pick(h, ['id', 'host_id', 'hostname', 'name']) || `host-${idx}`,
        name: pick(h, ['host_name', 'hostname', 'name']) || 'Unknown',
        ip: pick(h, ['ip', 'ip_address', 'address', 'host_ip']) || '-',
        cpu: Math.round(Number(pick(h, ['cpu_usage', 'cpuUsage', 'cpu', 'cpu_percent']) || 0)),
        memory: Math.round(Number(pick(h, ['memory_usage', 'memoryUsage', 'mem', 'memory_percent']) || 0)),
        disk: Math.round(Number(pick(h, ['disk_usage', 'diskUsage', 'disk', 'disk_percent']) || 0)),
        rx: Math.round(Number(pick(h, ['network_rx', 'network_in', 'inbound', 'rx_bytes', 'rx']) || 0)),
        tx: Math.round(Number(pick(h, ['network_tx', 'network_out', 'outbound', 'tx_bytes', 'tx']) || 0)),
        status,
        statusText,
      }
    })
    .filter((h) => h.name !== 'Unknown' || h.ip !== '-')
}

const parseTimeseries = (raw) => {
  if (!raw) return null
  const list = Array.isArray(raw) ? raw : pick(raw, ['timeseries', 'metrics', 'data', 'items', 'series']) || []
  if (!Array.isArray(list) || !list.length) return null

  const labels = []
  const cpu = []
  const memory = []

  list.forEach((item) => {
    const ts = pick(item, ['timestamp', 'time', 'ts'])
    if (ts) {
      const d = new Date(typeof ts === 'number' ? ts : ts)
      const h = String(d.getHours()).padStart(2, '0')
      const m = String(d.getMinutes()).padStart(2, '0')
      labels.push(`${h}:${m}`)
    } else {
      labels.push('')
    }
    cpu.push(Math.round(Number(pick(item, ['cpu', 'cpu_usage', 'cpuUsage']) || 0)))
    memory.push(Math.round(Number(pick(item, ['memory', 'mem', 'memory_usage', 'memoryUsage']) || 0)))
  })

  return { labels, cpu, memory }
}

const fetchData = async () => {
  loading.value = true
  try {
    const [metricsRes, overviewRes] = await Promise.allSettled([
      queryService.getMetrics({ limit: 100, type: 'host' }),
      queryService.getOverview(),
    ])

    const metrics = metricsRes.status === 'fulfilled' ? metricsRes.value : null
    const overview = overviewRes.status === 'fulfilled' ? overviewRes.value : null

    const listFromMetrics = metrics ? parseHostsList(metrics) : []
    const listFromOverview = overview ? parseHostsList(pick(overview, ['hosts', 'nodes', 'data'])) : []

    if (listFromMetrics.length) {
      hosts.value = listFromMetrics
    } else if (listFromOverview.length) {
      hosts.value = listFromOverview
    } else {
      hosts.value = []
    }

    const tsFromMetrics = metrics ? parseTimeseries(metrics) : null
    const tsFromOverview = overview ? parseTimeseries(pick(overview, ['timeseries', 'metrics'])) : null
    timeseries.value = tsFromMetrics || tsFromOverview || { labels: [], cpu: [], memory: [] }
  } catch (err) {
    console.warn('MetricsHost fetch error:', err)
    hosts.value = []
    timeseries.value = { labels: [], cpu: [], memory: [] }
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  fetchData()
})
</script>
