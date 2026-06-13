<template>
  <div class="space-y-6">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold text-slate-900 dark:text-white">流量回放</h1>
        <p class="text-sm text-slate-500 dark:text-slate-400 mt-1">历史流量数据回放分析</p>
      </div>
      <button class="btn-primary" @click="fetchData">
        <RefreshCw class="w-4 h-4" />
        刷新数据
      </button>
    </div>

    <div class="grid grid-cols-3 gap-4">
      <div class="card p-4">
        <p class="text-sm text-slate-500">回放区间</p>
        <p class="text-lg font-bold text-slate-900 dark:text-white mt-1">{{ timeRange }}</p>
      </div>
      <div class="card p-4">
        <p class="text-sm text-slate-500">可回放记录</p>
        <p class="text-2xl font-bold text-primary-500 mt-1">{{ flows.length }}</p>
      </div>
      <div class="card p-4">
        <p class="text-sm text-slate-500">时间跨度</p>
        <p class="text-lg font-bold text-accent-500 mt-1">{{ duration }}</p>
      </div>
    </div>

    <div class="card p-6">
      <h3 class="text-lg font-semibold text-slate-900 dark:text-white mb-4">流量时间线</h3>
      <div class="h-72">
        <ECharts :option="timelineOption" class="w-full h-full" />
      </div>
    </div>

    <div class="card p-6">
      <h3 class="text-lg font-semibold text-slate-900 dark:text-white mb-4">最近流量记录</h3>
      <div class="overflow-x-auto">
        <table class="w-full">
          <thead><tr class="bg-slate-50 dark:bg-dark-700/50">
            <th class="text-left px-4 py-3 text-xs font-semibold text-slate-500 uppercase">时间</th>
            <th class="text-left px-4 py-3 text-xs font-semibold text-slate-500 uppercase">源IP</th>
            <th class="text-left px-4 py-3 text-xs font-semibold text-slate-500 uppercase">目的IP</th>
            <th class="text-left px-4 py-3 text-xs font-semibold text-slate-500 uppercase">协议</th>
            <th class="text-right px-4 py-3 text-xs font-semibold text-slate-500 uppercase">流量</th>
          </tr></thead>
          <tbody class="divide-y divide-slate-200 dark:divide-dark-700">
            <tr v-for="(f, i) in recentFlows" :key="i" class="hover:bg-slate-50 dark:hover:bg-dark-700/50">
              <td class="px-4 py-3 text-sm text-slate-500">{{ formatTime(f.timestamp) }}</td>
              <td class="px-4 py-3 text-sm font-mono text-slate-900 dark:text-white">{{ f.src_ip }}</td>
              <td class="px-4 py-3 text-sm font-mono text-slate-900 dark:text-white">{{ f.dst_ip }}</td>
              <td class="px-4 py-3"><span class="text-xs px-2 py-1 rounded-full font-medium" :class="f.protocol==='TCP'?'bg-blue-100 text-blue-600':'bg-green-100 text-green-600'">{{ f.protocol }}</span></td>
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
import { BarChart } from 'echarts/charts'
import { TooltipComponent, GridComponent } from 'echarts/components'
import VChart from 'vue-echarts'
import { RefreshCw } from 'lucide-vue-next'

use([CanvasRenderer, BarChart, TooltipComponent, GridComponent])
const ECharts = VChart

const flows = ref([])
const loading = ref(true)

const fetchData = async () => {
  loading.value = true
  try {
    const r = await fetch('/api/query/flows?limit=2000')
    if (r.ok) {
      const d = await r.json()
      flows.value = d.records || []
    }
  } catch(e) { console.error(e) }
  loading.value = false
}

const recentFlows = computed(() => flows.value.slice(0, 50))

const timeRange = computed(() => {
  if (!flows.value.length) return '-'
  const ts = flows.value.map(f => f.timestamp).filter(Boolean).sort()
  if (!ts.length) return '-'
  const first = ts[0].substring(11, 19)
  const last = ts[ts.length-1].substring(11, 19)
  return first + ' ~ ' + last
})

const duration = computed(() => {
  if (!flows.value.length) return '-'
  const ts = flows.value.map(f => f.timestamp).filter(Boolean).sort()
  if (ts.length < 2) return '-'
  const ms = new Date(ts[ts.length-1]) - new Date(ts[0])
  const mins = Math.floor(ms / 60000)
  if (mins < 60) return mins + ' 分钟'
  return Math.floor(mins / 60) + ' 小时 ' + (mins % 60) + ' 分钟'
})

const formatTime = ts => { if (!ts) return '-'; const d = new Date(ts); return String(d.getHours()).padStart(2,'0')+':'+String(d.getMinutes()).padStart(2,'0')+':'+String(d.getSeconds()).padStart(2,'0') }
const formatBytes = b => { if (!b) return '0 B'; if (b>=1e9) return (b/1e9).toFixed(2)+' GB'; if (b>=1e6) return (b/1e6).toFixed(2)+' MB'; if (b>=1e3) return (b/1e3).toFixed(2)+' KB'; return b+' B' }

const timelineOption = computed(() => {
  const map = {}
  flows.value.forEach(f => {
    if (!f.timestamp) return
    const key = f.timestamp.substring(11, 16)
    map[key] = (map[key] || 0) + (f.bytes || 0)
  })
  const keys = Object.keys(map).sort()
  const vals = keys.map(k => (map[k] / 1048576).toFixed(2))
  return {
    tooltip: { trigger: 'axis' },
    grid: { left: '3%', right: '4%', bottom: '10%', top: '5%', containLabel: true },
    xAxis: { type: 'category', data: keys, axisLabel: { rotate: 45, fontSize: 10 } },
    yAxis: { type: 'value', name: 'MB' },
    series: [{ type: 'bar', data: vals, itemStyle: { color: '#6366f1', borderRadius: [4, 4, 0, 0] } }]
  }
})

onMounted(() => { fetchData(); setInterval(fetchData, 30000) })
</script>
