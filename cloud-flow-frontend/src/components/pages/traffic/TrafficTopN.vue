<template>
  <div class="space-y-6">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold text-slate-900 dark:text-white">TopN排行</h1>
        <p class="text-sm text-slate-500 dark:text-slate-400 mt-1">流量排行分析</p>
      </div>
      <div class="flex items-center gap-3">
        <select v-model="metric" class="input w-32">
          <option value="bytes">按流量</option>
          <option value="packets">按包数</option>
          <option value="sessions">按会话数</option>
        </select>
        <select v-model="topN" class="input w-24">
          <option :value="10">Top 10</option>
          <option :value="20">Top 20</option>
          <option :value="50">Top 50</option>
        </select>
        <button @click="fetchData" :disabled="loading" class="btn-secondary">
          <RefreshCw v-if="loading" class="w-4 h-4 animate-spin" />
          <RefreshCw v-else class="w-4 h-4" />
        </button>
      </div>
    </div>

    <div class="grid grid-cols-2 gap-6">
      <div class="card p-6">
        <h3 class="text-lg font-semibold text-slate-900 dark:text-white mb-4">Top 源IP</h3>
        <div v-if="loading" class="space-y-3">
          <div v-for="i in 5" :key="i" class="h-6 bg-slate-100 dark:bg-dark-700 rounded animate-pulse"></div>
        </div>
        <div v-else-if="topSrcIps.length === 0" class="text-center text-sm text-slate-400 py-8">
          暂无数据
        </div>
        <div v-else class="space-y-3">
          <div v-for="(item, i) in topSrcIps" :key="item.ip" class="flex items-center gap-3">
            <span class="w-6 text-xs font-medium text-slate-400">{{ i + 1 }}</span>
            <div class="flex-1">
              <div class="flex items-center justify-between mb-1">
                <span class="text-sm font-medium text-slate-700 dark:text-slate-200">{{ item.ip }}</span>
                <span class="text-xs text-slate-500">{{ item.value }}</span>
              </div>
              <div class="h-2 bg-slate-100 dark:bg-dark-700 rounded-full overflow-hidden">
                <div class="h-full bg-gradient-to-r from-primary-500 to-accent-500 rounded-full" :style="{ width: `${item.percentage}%` }"></div>
              </div>
            </div>
          </div>
        </div>
      </div>

      <div class="card p-6">
        <h3 class="text-lg font-semibold text-slate-900 dark:text-white mb-4">Top 目的IP</h3>
        <div v-if="loading" class="space-y-3">
          <div v-for="i in 5" :key="i" class="h-6 bg-slate-100 dark:bg-dark-700 rounded animate-pulse"></div>
        </div>
        <div v-else-if="topDstIps.length === 0" class="text-center text-sm text-slate-400 py-8">
          暂无数据
        </div>
        <div v-else class="space-y-3">
          <div v-for="(item, i) in topDstIps" :key="item.ip" class="flex items-center gap-3">
            <span class="w-6 text-xs font-medium text-slate-400">{{ i + 1 }}</span>
            <div class="flex-1">
              <div class="flex items-center justify-between mb-1">
                <span class="text-sm font-medium text-slate-700 dark:text-slate-200">{{ item.ip }}</span>
                <span class="text-xs text-slate-500">{{ item.value }}</span>
              </div>
              <div class="h-2 bg-slate-100 dark:bg-dark-700 rounded-full overflow-hidden">
                <div class="h-full bg-gradient-to-r from-accent-500 to-emerald-500 rounded-full" :style="{ width: `${item.percentage}%` }"></div>
              </div>
            </div>
          </div>
        </div>
      </div>

      <div class="card p-6">
        <h3 class="text-lg font-semibold text-slate-900 dark:text-white mb-4">Top 端口</h3>
        <div v-if="loading" class="space-y-3">
          <div v-for="i in 5" :key="i" class="h-6 bg-slate-100 dark:bg-dark-700 rounded animate-pulse"></div>
        </div>
        <div v-else-if="topPorts.length === 0" class="text-center text-sm text-slate-400 py-8">
          暂无数据
        </div>
        <div v-else class="space-y-3">
          <div v-for="(item, i) in topPorts" :key="item.port" class="flex items-center gap-3">
            <span class="w-6 text-xs font-medium text-slate-400">{{ i + 1 }}</span>
            <div class="flex-1">
              <div class="flex items-center justify-between mb-1">
                <span class="text-sm font-medium text-slate-700 dark:text-slate-200">{{ item.port }}/{{ item.protocol }}</span>
                <span class="text-xs text-slate-500">{{ item.value }}</span>
              </div>
              <div class="h-2 bg-slate-100 dark:bg-dark-700 rounded-full overflow-hidden">
                <div class="h-full bg-gradient-to-r from-amber-500 to-red-500 rounded-full" :style="{ width: `${item.percentage}%` }"></div>
              </div>
            </div>
          </div>
        </div>
      </div>

      <div class="card p-6">
        <h3 class="text-lg font-semibold text-slate-900 dark:text-white mb-4">Top 协议</h3>
        <div v-if="loading" class="space-y-3">
          <div v-for="i in 5" :key="i" class="h-6 bg-slate-100 dark:bg-dark-700 rounded animate-pulse"></div>
        </div>
        <div v-else-if="topProtocols.length === 0" class="text-center text-sm text-slate-400 py-8">
          暂无数据
        </div>
        <div v-else class="space-y-3">
          <div v-for="(item, i) in topProtocols" :key="item.protocol" class="flex items-center gap-3">
            <span class="w-6 text-xs font-medium text-slate-400">{{ i + 1 }}</span>
            <div class="flex-1">
              <div class="flex items-center justify-between mb-1">
                <span class="text-sm font-medium text-slate-700 dark:text-slate-200">{{ item.protocol }}</span>
                <span class="text-xs text-slate-500">{{ item.value }}</span>
              </div>
              <div class="h-2 bg-slate-100 dark:bg-dark-700 rounded-full overflow-hidden">
                <div class="h-full bg-gradient-to-r from-violet-500 to-purple-500 rounded-full" :style="{ width: `${item.percentage}%` }"></div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, watch } from 'vue'
import { RefreshCw } from 'lucide-vue-next'
import { queryService } from '../../../api'

const loading = ref(false)
const metric = ref('bytes')
const topN = ref(10)
const flows = ref([])

const formatBytes = (bytes) => {
  if (!bytes || bytes === 0) return '0 B'
  const k = 1024
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + units[i]
}

const aggregateByKey = (items, keyField, valueFn) => {
  const map = new Map()
  items.forEach((f) => {
    const key = f[keyField]
    if (!key && key !== 0) return
    map.set(key, (map.get(key) || 0) + valueFn(f))
  })
  const arr = Array.from(map.entries())
    .map(([k, v]) => ({ key, value: v }))
    .sort((a, b) => b.value - a.value)
    .slice(0, topN.value)
  const max = arr.length > 0 ? arr[0].value : 1
  return { arr, max }
}

const buildTopIps = (items, ipKey) => {
  const { arr, max } = aggregateByKey(items, ipKey, (f) => {
    if (metric.value === 'packets') return f.packet_count || f.packets || 0
    if (metric.value === 'sessions') return 1
    return f.byte_count || f.bytes || 0
  })
  return arr.map((item) => ({
    ip: item.key,
    value: metric.value === 'bytes' ? formatBytes(item.value) : item.value.toString(),
    percentage: max > 0 ? Math.round((item.value / max) * 100) : 0,
  }))
}

const topSrcIps = computed(() => buildTopIps(flows.value, 'src_ip'))
const topDstIps = computed(() => buildTopIps(flows.value, 'dst_ip'))

const topPorts = computed(() => {
  const portMap = new Map()
  flows.value.forEach((f) => {
    const port = f.dst_port || f.destPort
    const proto = (f.protocol || f.proto || 'TCP').toUpperCase()
    if (!port && port !== 0) return
    const key = `${port}/${proto}`
    const value = metric.value === 'packets' ? (f.packet_count || f.packets || 0) : metric.value === 'sessions' ? 1 : (f.byte_count || f.bytes || 0)
    portMap.set(key, (portMap.get(key) || 0) + value)
  })
  const arr = Array.from(portMap.entries())
    .map(([k, v]) => {
      const [port, protocol] = k.split('/')
      return { port, protocol, value: v }
    })
    .sort((a, b) => b.value - a.value)
    .slice(0, topN.value)
  const max = arr.length > 0 ? arr[0].value : 1
  return arr.map((item) => ({
    port: item.port,
    protocol: item.protocol,
    value: metric.value === 'bytes' ? formatBytes(item.value) : item.value.toString(),
    percentage: max > 0 ? Math.round((item.value / max) * 100) : 0,
  }))
})

const topProtocols = computed(() => {
  const protoMap = new Map()
  flows.value.forEach((f) => {
    const proto = (f.protocol || f.proto || 'OTHER').toUpperCase()
    const value = metric.value === 'packets' ? (f.packet_count || f.packets || 0) : metric.value === 'sessions' ? 1 : (f.byte_count || f.bytes || 0)
    protoMap.set(proto, (protoMap.get(proto) || 0) + value)
  })
  const arr = Array.from(protoMap.entries())
    .map(([protocol, value]) => ({ protocol, value }))
    .sort((a, b) => b.value - a.value)
    .slice(0, topN.value)
  const max = arr.length > 0 ? arr[0].value : 1
  return arr.map((item) => ({
    protocol: item.protocol,
    value: metric.value === 'bytes' ? formatBytes(item.value) : item.value.toString(),
    percentage: max > 0 ? Math.round((item.value / max) * 100) : 0,
  }))
})

const fetchData = async () => {
  loading.value = true
  try {
    const res = await queryService.getFlows({ limit: 1000 }).catch(() => null)
    const data = (res?.data || res?.flows || res || [])
    flows.value = Array.isArray(data) ? data : []
  } catch {
    flows.value = []
  } finally {
    loading.value = false
  }
}

watch([metric, topN], () => {})

onMounted(() => {
  fetchData()
})
</script>
