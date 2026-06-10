<template>
  <div class="space-y-6">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold text-slate-900 dark:text-white">流量回放</h1>
        <p class="text-sm text-slate-500 dark:text-slate-400 mt-1">历史流量数据回放分析</p>
      </div>
      <button @click="fetchData" :disabled="loading" class="btn-secondary">
        <RefreshCw v-if="loading" class="w-4 h-4 animate-spin" />
        <RefreshCw v-else class="w-4 h-4" />
        加载数据
      </button>
    </div>

    <div class="card p-6">
      <div class="flex items-center gap-4 mb-6">
        <div class="text-sm text-slate-600 dark:text-slate-300 whitespace-nowrap">
          当前时间点:
        </div>
        <div class="text-sm font-medium text-slate-900 dark:text-white whitespace-nowrap">
          <Loader2 v-if="loading" class="w-4 h-4 animate-spin inline" />
          <span v-else>{{ currentTimeLabel }}</span>
        </div>
        <div class="flex-1">
          <input
            type="range"
            :min="0"
            :max="Math.max(0, timestamps.length - 1)"
            :value="currentIndex"
            @input="onSliderChange"
            :disabled="loading || timestamps.length === 0"
            class="w-full h-2 bg-slate-200 dark:bg-dark-700 rounded-lg appearance-none cursor-pointer accent-primary-500"
          />
        </div>
        <button
          @click="togglePlay"
          :disabled="loading || timestamps.length === 0"
          class="btn-primary"
        >
          <Pause v-if="isPlaying" class="w-4 h-4" />
          <Play v-else class="w-4 h-4" />
          {{ isPlaying ? '暂停' : '播放' }}
        </button>
        <span class="text-sm text-slate-500 whitespace-nowrap">{{ currentIndex + 1 }} / {{ timestamps.length }}</span>
      </div>

      <div class="grid grid-cols-4 gap-4 mb-6">
        <div class="p-4 bg-slate-50 dark:bg-dark-700 rounded-xl">
          <p class="text-xs text-slate-500">总流量</p>
          <p class="text-xl font-bold text-primary-500 mt-1">
            <Loader2 v-if="loading" class="w-4 h-4 animate-spin inline" />
            <span v-else>{{ snapshotStats.totalBytes }}</span>
          </p>
        </div>
        <div class="p-4 bg-slate-50 dark:bg-dark-700 rounded-xl">
          <p class="text-xs text-slate-500">包数</p>
          <p class="text-xl font-bold text-accent-500 mt-1">
            <Loader2 v-if="loading" class="w-4 h-4 animate-spin inline" />
            <span v-else>{{ snapshotStats.totalPackets }}</span>
          </p>
        </div>
        <div class="p-4 bg-slate-50 dark:bg-dark-700 rounded-xl">
          <p class="text-xs text-slate-500">活跃会话</p>
          <p class="text-xl font-bold text-emerald-500 mt-1">
            <Loader2 v-if="loading" class="w-4 h-4 animate-spin inline" />
            <span v-else>{{ snapshotStats.sessionCount }}</span>
          </p>
        </div>
        <div class="p-4 bg-slate-50 dark:bg-dark-700 rounded-xl">
          <p class="text-xs text-slate-500">活跃IP</p>
          <p class="text-xl font-bold text-violet-500 mt-1">
            <Loader2 v-if="loading" class="w-4 h-4 animate-spin inline" />
            <span v-else>{{ snapshotStats.ipCount }}</span>
          </p>
        </div>
      </div>

      <div class="h-72 relative">
        <div v-if="loading" class="absolute inset-0 flex items-center justify-center">
          <Loader2 class="w-8 h-8 animate-spin text-slate-400" />
        </div>
        <div v-else-if="timestamps.length === 0" class="absolute inset-0 flex items-center justify-center text-sm text-slate-400">
          暂无数据
        </div>
        <ECharts v-else :option="replayChartOption" class="w-full h-full" autoresize />
      </div>
    </div>

    <div class="card">
      <div class="p-6 border-b border-slate-200 dark:border-dark-700">
        <h3 class="text-lg font-semibold text-slate-900 dark:text-white">当前时间点 Top 会话</h3>
      </div>
      <div class="overflow-x-auto">
        <table class="w-full">
          <thead>
            <tr class="bg-slate-50 dark:bg-dark-700/50">
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">源IP</th>
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">目的IP</th>
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">协议</th>
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">端口</th>
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">流量</th>
            </tr>
          </thead>
          <tbody v-if="loading" class="divide-y divide-slate-200 dark:divide-dark-700">
            <tr v-for="i in 3" :key="i">
              <td colspan="5" class="px-6 py-4">
                <div class="h-4 bg-slate-100 dark:bg-dark-700 rounded animate-pulse"></div>
              </td>
            </tr>
          </tbody>
          <tbody v-else-if="currentSnapshot.length === 0" class="divide-y divide-slate-200 dark:divide-dark-700">
            <tr>
              <td colspan="5" class="px-6 py-12 text-center text-sm text-slate-400">
                暂无数据
              </td>
            </tr>
          </tbody>
          <tbody v-else class="divide-y divide-slate-200 dark:divide-dark-700">
            <tr
              v-for="(item, i) in currentSnapshot"
              :key="i"
              class="hover:bg-slate-50 dark:hover:bg-dark-700/50"
            >
              <td class="px-6 py-4 text-sm text-slate-900 dark:text-white">{{ item.src }}</td>
              <td class="px-6 py-4 text-sm text-slate-900 dark:text-white">{{ item.dst }}</td>
              <td class="px-6 py-4">
                <span :class="['text-xs px-2 py-1 rounded-full font-medium', item.protocol === 'TCP' ? 'bg-blue-100 text-blue-600' : 'bg-green-100 text-green-600']">
                  {{ item.protocol }}
                </span>
              </td>
              <td class="px-6 py-4 text-sm text-slate-500">{{ item.port }}</td>
              <td class="px-6 py-4 text-sm text-slate-500">{{ item.bytes }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onBeforeUnmount, watch } from 'vue'
import { use } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { BarChart, LineChart } from 'echarts/charts'
import { TooltipComponent, GridComponent, MarkLineComponent } from 'echarts/components'
import ECharts from 'vue-echarts'
import { Play, Pause, RefreshCw, Loader2 } from 'lucide-vue-next'
import { queryService } from '../../../api'

use([CanvasRenderer, BarChart, LineChart, TooltipComponent, GridComponent, MarkLineComponent])

const loading = ref(false)
const flows = ref([])
const timestamps = ref([])
const currentIndex = ref(0)
const isPlaying = ref(false)
let playInterval = null

const formatBytes = (bytes) => {
  if (!bytes || bytes === 0) return '0 B'
  const k = 1024
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + units[i]
}

const formatTimestamp = (ts) => {
  if (!ts) return '-'
  const d = new Date(ts)
  if (isNaN(d.getTime())) return String(ts)
  return d.toLocaleString('zh-CN', { hour12: false })
}

const currentTimeLabel = computed(() => {
  if (timestamps.value.length === 0) return '-'
  return formatTimestamp(timestamps.value[currentIndex.value])
})

const flowsByTime = computed(() => {
  if (timestamps.value.length === 0) return []
  return timestamps.value.map((ts, idx) => {
    const filtered = flows.value.filter((f) => {
      const fts = f.timestamp || f.time || f.ts
      if (!fts) return idx === 0
      return true
    })
    return { idx, ts, flows: filtered }
  })
})

const snapshotStats = computed(() => {
  if (flows.value.length === 0) {
    return { totalBytes: '0 B', totalPackets: '0', sessionCount: '0', ipCount: '0' }
  }
  const totalBytes = flows.value.reduce((sum, f) => sum + (f.byte_count || f.bytes || 0), 0)
  const totalPackets = flows.value.reduce((sum, f) => sum + (f.packet_count || f.packets || 0), 0)
  const ipSet = new Set()
  flows.value.forEach((f) => {
    if (f.src_ip) ipSet.add(f.src_ip)
    if (f.dst_ip) ipSet.add(f.dst_ip)
  })
  return {
    totalBytes: formatBytes(totalBytes),
    totalPackets: totalPackets.toString(),
    sessionCount: flows.value.length.toString(),
    ipCount: ipSet.size.toString(),
  }
})

const currentSnapshot = computed(() => {
  return flows.value
    .slice(0, 10)
    .map((f) => ({
      src: f.src_ip || f.sourceIp || '-',
      dst: f.dst_ip || f.destIp || '-',
      protocol: (f.protocol || f.proto || 'TCP').toUpperCase(),
      port: f.dst_port || f.destPort || '-',
      bytes: formatBytes(f.byte_count || f.bytes || 0),
    }))
})

const replayChartOption = computed(() => {
  if (timestamps.value.length === 0) {
    return { xAxis: { data: [] }, yAxis: {}, series: [] }
  }
  const labels = timestamps.value.map((ts) => formatTimestamp(ts))
  const bytesData = timestamps.value.map((ts, idx) => {
    const bucketFlows = flows.value.filter((_, i) => Math.floor(i / Math.max(1, Math.floor(flows.value.length / timestamps.value.length))) === idx || idx === 0)
    return bucketFlows.reduce((sum, f) => sum + (f.byte_count || f.bytes || 0), 0)
  })

  return {
    tooltip: {
      trigger: 'axis',
      backgroundColor: 'rgba(255,255,255,0.95)',
      textStyle: { color: '#1e293b' },
      formatter: (params) => {
        const p = params[0]
        return `${p.name}<br/>流量: ${formatBytes(p.value)}`
      },
    },
    grid: { left: '3%', right: '4%', bottom: '15%', top: '10%', containLabel: true },
    xAxis: {
      type: 'category',
      data: labels,
      axisLabel: { color: '#64748b', fontSize: 10, rotate: 30 },
    },
    yAxis: {
      type: 'value',
      axisLabel: { color: '#64748b', fontSize: 10 },
      splitLine: { lineStyle: { color: '#e2e8f0' } },
    },
    series: [{
      type: 'bar',
      data: bytesData,
      itemStyle: {
        color: (params) => (params.dataIndex === currentIndex.value ? '#2563eb' : '#93c5fd'),
        borderRadius: [4, 4, 0, 0],
      },
    }],
  }
})

const onSliderChange = (e) => {
  currentIndex.value = parseInt(e.target.value, 10)
}

const togglePlay = () => {
  if (isPlaying.value) {
    stopPlay()
  } else {
    startPlay()
  }
}

const startPlay = () => {
  if (timestamps.value.length === 0) return
  isPlaying.value = true
  if (currentIndex.value >= timestamps.value.length - 1) {
    currentIndex.value = 0
  }
  playInterval = setInterval(() => {
    if (currentIndex.value < timestamps.value.length - 1) {
      currentIndex.value += 1
    } else {
      stopPlay()
    }
  }, 1000)
}

const stopPlay = () => {
  isPlaying.value = false
  if (playInterval) {
    clearInterval(playInterval)
    playInterval = null
  }
}

const fetchData = async () => {
  loading.value = true
  stopPlay()
  try {
    const res = await queryService.getFlows({ limit: 200 }).catch(() => null)
    const data = (res?.data || res?.flows || res || [])
    flows.value = Array.isArray(data) ? data : []

    const uniqueTs = new Set()
    flows.value.forEach((f) => {
      const ts = f.timestamp || f.time || f.ts
      if (ts) uniqueTs.add(ts)
    })
    if (uniqueTs.size === 0) {
      const buckets = Math.min(20, Math.max(1, flows.value.length))
      const now = Date.now()
      for (let i = 0; i < buckets; i++) {
        timestamps.value.push(now - (buckets - i) * 60000)
      }
    } else {
      timestamps.value = Array.from(uniqueTs).sort()
    }
    currentIndex.value = 0
  } catch {
    flows.value = []
    timestamps.value = []
  } finally {
    loading.value = false
  }
}

watch(isPlaying, (v) => {
  if (!v) stopPlay()
})

onMounted(() => {
  fetchData()
})

onBeforeUnmount(() => {
  stopPlay()
})
</script>
