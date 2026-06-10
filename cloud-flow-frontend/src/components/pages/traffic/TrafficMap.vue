<template>
  <div class="space-y-6">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold text-slate-900 dark:text-white">流量地图</h1>
        <p class="text-sm text-slate-500 dark:text-slate-400 mt-1">可视化网络流量地理分布</p>
      </div>
      <button @click="fetchData" :disabled="loading" class="btn-secondary">
        <RefreshCw v-if="loading" class="w-4 h-4 animate-spin" />
        <RefreshCw v-else class="w-4 h-4" />
        刷新
      </button>
    </div>

    <div v-if="hasGeoData" class="grid grid-cols-3 gap-4">
      <div class="card p-4">
        <p class="text-sm text-slate-500">地理位置节点</p>
        <p class="text-2xl font-bold text-slate-900 dark:text-white mt-1">
          <Loader2 v-if="loading" class="w-5 h-5 animate-spin inline" />
          <span v-else>{{ geoStats.locations }}</span>
        </p>
      </div>
      <div class="card p-4">
        <p class="text-sm text-slate-500">总流量</p>
        <p class="text-2xl font-bold text-primary-500 mt-1">
          <Loader2 v-if="loading" class="w-5 h-5 animate-spin inline" />
          <span v-else>{{ formatBytes(geoStats.totalBytes) }}</span>
        </p>
      </div>
      <div class="card p-4">
        <p class="text-sm text-slate-500">活跃IP数</p>
        <p class="text-2xl font-bold text-emerald-500 mt-1">
          <Loader2 v-if="loading" class="w-5 h-5 animate-spin inline" />
          <span v-else>{{ geoStats.ipCount }}</span>
        </p>
      </div>
    </div>

    <div class="card p-6">
      <h3 class="text-lg font-semibold text-slate-900 dark:text-white mb-6">源/目的IP 流量热度</h3>
      <div class="h-96 relative">
        <div v-if="loading" class="absolute inset-0 flex items-center justify-center">
          <Loader2 class="w-8 h-8 animate-spin text-slate-400" />
        </div>
        <div v-else-if="flows.length === 0" class="absolute inset-0 flex items-center justify-center text-sm text-slate-400">
          暂无数据
        </div>
        <ECharts v-else :option="heatmapOption" class="w-full h-full" autoresize />
      </div>
    </div>

    <div class="grid grid-cols-2 gap-6">
      <div class="card p-6">
        <div class="flex items-center justify-between mb-4">
          <h3 class="text-lg font-semibold text-slate-900 dark:text-white">热门源IP</h3>
        </div>
        <div v-if="loading" class="space-y-3">
          <div v-for="i in 5" :key="i" class="h-6 bg-slate-100 dark:bg-dark-700 rounded animate-pulse"></div>
        </div>
        <div v-else-if="srcIpRanking.length === 0" class="text-center text-sm text-slate-400 py-6">
          暂无数据
        </div>
        <div v-else class="space-y-3">
          <div
            v-for="(item, i) in srcIpRanking"
            :key="item.ip"
            class="flex items-center gap-3"
          >
            <span class="w-6 text-xs font-medium text-slate-400">{{ i + 1 }}</span>
            <div class="flex-1">
              <div class="flex items-center justify-between mb-1">
                <span class="text-sm font-medium text-slate-700 dark:text-slate-200">{{ item.ip }}</span>
                <span class="text-xs text-slate-500">{{ item.bytes }}</span>
              </div>
              <div class="h-2 bg-slate-100 dark:bg-dark-700 rounded-full overflow-hidden">
                <div
                  class="h-full bg-gradient-to-r from-primary-500 to-accent-500 rounded-full"
                  :style="{ width: `${item.percentage}%` }"
                ></div>
              </div>
            </div>
          </div>
        </div>
      </div>

      <div class="card p-6">
        <div class="flex items-center justify-between mb-4">
          <h3 class="text-lg font-semibold text-slate-900 dark:text-white">热门目的IP</h3>
        </div>
        <div v-if="loading" class="space-y-3">
          <div v-for="i in 5" :key="i" class="h-6 bg-slate-100 dark:bg-dark-700 rounded animate-pulse"></div>
        </div>
        <div v-else-if="dstIpRanking.length === 0" class="text-center text-sm text-slate-400 py-6">
          暂无数据
        </div>
        <div v-else class="space-y-3">
          <div
            v-for="(item, i) in dstIpRanking"
            :key="item.ip"
            class="flex items-center gap-3"
          >
            <span class="w-6 text-xs font-medium text-slate-400">{{ i + 1 }}</span>
            <div class="flex-1">
              <div class="flex items-center justify-between mb-1">
                <span class="text-sm font-medium text-slate-700 dark:text-slate-200">{{ item.ip }}</span>
                <span class="text-xs text-slate-500">{{ item.bytes }}</span>
              </div>
              <div class="h-2 bg-slate-100 dark:bg-dark-700 rounded-full overflow-hidden">
                <div
                  class="h-full bg-gradient-to-r from-accent-500 to-emerald-500 rounded-full"
                  :style="{ width: `${item.percentage}%` }"
                ></div>
              </div>
            </div>
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
import { HeatmapChart, BarChart } from 'echarts/charts'
import { TooltipComponent, GridComponent, VisualMapComponent } from 'echarts/components'
import ECharts from 'vue-echarts'
import { Loader2, RefreshCw } from 'lucide-vue-next'
import { queryService } from '../../../api'

use([CanvasRenderer, HeatmapChart, BarChart, TooltipComponent, GridComponent, VisualMapComponent])

const loading = ref(false)
const flows = ref([])

const formatBytes = (bytes) => {
  if (!bytes || bytes === 0) return '0 B'
  const k = 1024
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + units[i]
}

const hasGeoData = computed(() => flows.value.some((f) => f.src_country || f.dst_country || f.src_lat || f.dst_lat))

const geoStats = computed(() => {
  const srcIps = new Set()
  const dstIps = new Set()
  const locations = new Set()
  let totalBytes = 0
  flows.value.forEach((f) => {
    if (f.src_ip) srcIps.add(f.src_ip)
    if (f.dst_ip) dstIps.add(f.dst_ip)
    if (f.src_country) locations.add(f.src_country)
    if (f.dst_country) locations.add(f.dst_country)
    totalBytes += f.byte_count || f.bytes || 0
  })
  return {
    locations: locations.size,
    totalBytes,
    ipCount: srcIps.size + dstIps.size,
  }
})

const buildIpRanking = (ipKey) => {
  const map = new Map()
  flows.value.forEach((f) => {
    const ip = f[ipKey]
    if (!ip) return
    const bytes = f.byte_count || f.bytes || 0
    map.set(ip, (map.get(ip) || 0) + bytes)
  })
  const arr = Array.from(map.entries())
    .map(([ip, bytes]) => ({ ip, bytes, rawBytes: bytes }))
    .sort((a, b) => b.rawBytes - a.rawBytes)
    .slice(0, 10)
  const max = arr.length > 0 ? arr[0].rawBytes : 1
  return arr.map((item) => ({
    ip: item.ip,
    bytes: formatBytes(item.rawBytes),
    percentage: max > 0 ? Math.round((item.rawBytes / max) * 100) : 0,
  }))
}

const srcIpRanking = computed(() => buildIpRanking('src_ip'))
const dstIpRanking = computed(() => buildIpRanking('dst_ip'))

const heatmapOption = computed(() => {
  const srcIps = Array.from(new Set(flows.value.map((f) => f.src_ip).filter(Boolean))).slice(0, 15)
  const dstIps = Array.from(new Set(flows.value.map((f) => f.dst_ip).filter(Boolean))).slice(0, 15)

  if (srcIps.length === 0 || dstIps.length === 0) {
    return {
      tooltip: {},
      xAxis: { type: 'category', data: [] },
      yAxis: { type: 'category', data: [] },
      series: [],
    }
  }

  const matrix = {}
  flows.value.forEach((f) => {
    const src = f.src_ip
    const dst = f.dst_ip
    if (!src || !dst) return
    const key = `${src}||${dst}`
    matrix[key] = (matrix[key] || 0) + (f.byte_count || f.bytes || 0)
  })

  const data = []
  let maxValue = 0
  srcIps.forEach((src, i) => {
    dstIps.forEach((dst, j) => {
      const v = matrix[`${src}||${dst}`] || 0
      if (v > maxValue) maxValue = v
      data.push([j, i, v])
    })
  })

  return {
    tooltip: {
      position: 'top',
      backgroundColor: 'rgba(255,255,255,0.95)',
      textStyle: { color: '#1e293b' },
      formatter: (params) => {
        const src = srcIps[params.value[1]]
        const dst = dstIps[params.value[0]]
        return `源: ${src}<br/>目的: ${dst}<br/>流量: ${formatBytes(params.value[2])}`
      },
    },
    grid: { left: 120, right: 40, top: 40, bottom: 80 },
    xAxis: {
      type: 'category',
      data: dstIps,
      axisLabel: { color: '#64748b', rotate: 45, fontSize: 10 },
      splitArea: { show: true },
    },
    yAxis: {
      type: 'category',
      data: srcIps,
      axisLabel: { color: '#64748b', fontSize: 10 },
      splitArea: { show: true },
    },
    visualMap: {
      min: 0,
      max: maxValue > 0 ? maxValue : 1,
      calculable: true,
      orient: 'horizontal',
      left: 'center',
      bottom: 10,
      textStyle: { color: '#64748b' },
      inRange: { color: ['#e0f2fe', '#38bdf8', '#2563eb', '#1e3a8a'] },
    },
    series: [{
      name: '流量',
      type: 'heatmap',
      data,
      label: { show: false },
      emphasis: { itemStyle: { shadowBlur: 10, shadowColor: 'rgba(0,0,0,0.5)' } },
    }],
  }
})

const fetchData = async () => {
  loading.value = true
  try {
    const res = await queryService.getFlows({ limit: 500 }).catch(() => null)
    const data = (res?.data || res?.flows || res || [])
    flows.value = Array.isArray(data) ? data : []
  } catch {
    flows.value = []
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  fetchData()
})
</script>
