<template>
  <div class="p-6">
    <div v-if="loading" class="flex items-center justify-center h-64">
      <div class="text-slate-400">Loading...</div>
    </div>
    <div v-else>
      <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4 mb-6">
        <div class="bg-white dark:bg-dark-800 rounded-xl p-4 shadow-sm">
          <p class="text-sm text-slate-500 dark:text-slate-400">Total Traffic</p>
          <p class="text-2xl font-bold text-slate-900 dark:text-white">{{ (totalBytes / 1e9).toFixed(2) }} <span class="text-sm font-normal text-slate-500">GB</span></p>
        </div>
        <div class="bg-white dark:bg-dark-800 rounded-xl p-4 shadow-sm">
          <p class="text-sm text-slate-500 dark:text-slate-400">Total Packets</p>
          <p class="text-2xl font-bold text-slate-900 dark:text-white">{{ (totalPackets / 1e6).toFixed(2) }} <span class="text-sm font-normal text-slate-500">M</span></p>
        </div>
        <div class="bg-white dark:bg-dark-800 rounded-xl p-4 shadow-sm">
          <p class="text-sm text-slate-500 dark:text-slate-400">Sessions</p>
          <p class="text-2xl font-bold text-slate-900 dark:text-white">{{ flows.length }} <span class="text-sm font-normal text-slate-500">pcs</span></p>
        </div>
        <div class="bg-white dark:bg-dark-800 rounded-xl p-4 shadow-sm">
          <p class="text-sm text-slate-500 dark:text-slate-400">Avg Latency</p>
          <p class="text-2xl font-bold text-slate-900 dark:text-white">{{ avgLatency }} <span class="text-sm font-normal text-slate-500">ms</span></p>
        </div>
      </div>

      <div class="bg-white dark:bg-dark-800 rounded-xl p-4 shadow-sm mb-6">
        <h3 class="text-lg font-semibold text-slate-900 dark:text-white mb-4">Traffic Trend</h3>
        <v-chart class="h-72" :option="trafficChartOption" autoresize />
      </div>

      <div class="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <div class="bg-white dark:bg-dark-800 rounded-xl p-4 shadow-sm">
          <h3 class="text-lg font-semibold text-slate-900 dark:text-white mb-4">Protocol Distribution</h3>
          <v-chart class="h-64" :option="protocolChartOption" autoresize />
        </div>
        <div class="bg-white dark:bg-dark-800 rounded-xl p-4 shadow-sm">
          <h3 class="text-lg font-semibold text-slate-900 dark:text-white mb-4">Recent Sessions</h3>
          <div class="overflow-x-auto">
            <table class="w-full text-sm">
              <thead>
                <tr class="border-b dark:border-dark-700 text-slate-500 dark:text-slate-400">
                  <th class="text-left p-2">Time</th>
                  <th class="text-left p-2">Probe</th>
                  <th class="text-left p-2">Protocol</th>
                  <th class="text-right p-2">Bytes</th>
                  <th class="text-right p-2">Packets</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="(flow, idx) in recentFlows" :key="idx" class="border-b dark:border-dark-700 hover:bg-slate-50 dark:hover:bg-dark-700">
                  <td class="p-2 text-slate-600 dark:text-slate-300">{{ formatTime(flow.timestamp) }}</td>
                  <td class="p-2 text-slate-600 dark:text-slate-300">{{ flow.probe_id }}</td>
                  <td class="p-2"><span class="px-2 py-0.5 rounded text-xs bg-primary-50 text-primary-600">{{ flow.protocol }}</span></td>
                  <td class="p-2 text-right text-slate-600 dark:text-slate-300">{{ formatBytes(flow.bytes) }}</td>
                  <td class="p-2 text-right text-slate-600 dark:text-slate-300">{{ formatPackets(flow.packets) }}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import VChart from 'vue-echarts'
import { use } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { LineChart, PieChart } from 'echarts/charts'
import { TooltipComponent, LegendComponent, GridComponent } from 'echarts/components'

use([CanvasRenderer, LineChart, PieChart, TooltipComponent, LegendComponent, GridComponent])

const flows = ref([])
const loading = ref(true)

const fetchFlows = async () => {
  loading.value = true
  try {
    const res = await fetch('/api/query/flows?limit=2000')
    const data = await res.json()
    flows.value = data.records || []
  } catch (e) {
    console.error('Failed to fetch flows', e)
  } finally {
    loading.value = false
  }
}

const totalBytes = computed(() => flows.value.reduce((s, f) => s + (f.bytes || 0), 0))
const totalPackets = computed(() => flows.value.reduce((s, f) => s + (f.packets || 0), 0))
const avgLatency = computed(() => {
  const withLatency = flows.value.filter(f => f.latency_ms > 0)
  if (withLatency.length === 0) return '0.00'
  const avg = withLatency.reduce((s, f) => s + f.latency_ms, 0) / withLatency.length
  return avg.toFixed(2)
})

const trafficChartOption = computed(() => {
  const map = {}
  flows.value.forEach(f => {
    if (!f.timestamp) return
    const d = new Date(f.timestamp)
    const key = d.getHours() + ':' + String(d.getMinutes()).padStart(2, '0')
    map[key] = (map[key] || 0) + (f.bytes || 0)
  })
  const times = Object.keys(map)
  const values = Object.values(map)
  return {
    tooltip: { trigger: 'axis', backgroundColor: 'rgba(255,255,255,0.95)', borderColor: 'rgba(0,0,0,0.1)', textStyle: { color: '#1e293b' } },
    grid: { left: '3%', right: '4%', bottom: '3%', top: '10%', containLabel: true },
    xAxis: { type: 'category', data: times, axisLine: { lineStyle: { color: '#e2e8f0' } }, axisLabel: { color: '#64748b', fontSize: 11 } },
    yAxis: { type: 'value', axisLabel: { formatter: v => (v >= 1e9 ? (v/1e9).toFixed(1) + 'G' : v >= 1e6 ? (v/1e6).toFixed(1) + 'M' : v >= 1e3 ? (v/1e3).toFixed(1) + 'K' : v), color: '#64748b', fontSize: 11 }, splitLine: { lineStyle: { color: '#f1f5f9', type: 'dashed' } } },
    series: [{
      name: 'Traffic', type: 'line', smooth: true, symbol: 'circle', symbolSize: 6,
      lineStyle: { color: '#2563eb', width: 2 }, itemStyle: { color: '#2563eb' },
      areaStyle: { color: { type: 'linear', x: 0, y: 0, x2: 0, y2: 1, colorStops: [{ offset: 0, color: 'rgba(37,99,235,0.15)' }, { offset: 1, color: 'rgba(37,99,235,0)' }] } },
      data: values
    }],
  }
})

const protocolChartOption = computed(() => {
  const map = {}
  flows.value.forEach(f => {
    const p = f.protocol || 'unknown'
    map[p] = (map[p] || 0) + 1
  })
  return {
    tooltip: { trigger: 'item', backgroundColor: 'rgba(255,255,255,0.95)', borderColor: 'rgba(0,0,0,0.1)', textStyle: { color: '#1e293b' } },
    legend: { bottom: 0, textStyle: { color: '#64748b' } },
    series: [{
      type: 'pie', radius: ['40%', '70%'],
      data: Object.entries(map).map(([name, value]) => ({ name, value })),
      emphasis: { itemStyle: { shadowBlur: 10, shadowOffsetX: 0, shadowColor: 'rgba(0,0,0,0.2)' } }
    }]
  }
})

const recentFlows = computed(() => flows.value.slice(0, 50))

const formatTime = (ts) => {
  if (!ts) return '-'
  const d = new Date(ts)
  return d.getHours() + ':' + String(d.getMinutes()).padStart(2, '0') + ':' + String(d.getSeconds()).padStart(2, '0')
}
const formatBytes = (b) => {
  if (b >= 1e9) return (b / 1e9).toFixed(2) + ' GB'
  if (b >= 1e6) return (b / 1e6).toFixed(2) + ' MB'
  if (b >= 1e3) return (b / 1e3).toFixed(2) + ' KB'
  return b + ' B'
}
const formatPackets = (p) => {
  if (p >= 1e6) return (p / 1e6).toFixed(2) + 'M'
  if (p >= 1e3) return (p / 1e3).toFixed(2) + 'K'
  return p || 0
}

onMounted(() => { fetchFlows() })
</script>
