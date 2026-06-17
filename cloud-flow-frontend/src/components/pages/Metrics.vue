<template>
  <div class="space-y-6 p-4">
    <h1 class="text-2xl font-bold text-slate-900 dark:text-white">指标监控</h1>
    
    <!-- KPI Card -->
    <div class="grid grid-cols-4 gap-4">
      <div class="card p-4 border-l-4 border-blue-500">
        <p class="text-sm text-slate-500">总流量</p>
        <p class="text-2xl font-bold mt-1">{{ fmtBytes(totalBytes) }}</p>
      </div>
      <div class="card p-4 border-l-4 border-green-500">
        <p class="text-sm text-slate-500">总包数</p>
        <p class="text-2xl font-bold mt-1">{{ fmtNum(totalPackets) }}</p>
      </div>
      <div class="card p-4 border-l-4 border-purple-500">
        <p class="text-sm text-slate-500">源IP数</p>
        <p class="text-2xl font-bold mt-1">{{ uniqueIPs }}</p>
      </div>
      <div class="card p-4 border-l-4 border-amber-500">
        <p class="text-sm text-slate-500">HTTP请求</p>
        <p class="text-2xl font-bold mt-1">{{ httpCount }}</p>
      </div>
    </div>

    <!-- VM2 Host Metrics -->
    <div class="card p-6">
      <h3 class="text-lg font-semibold mb-4">探针主机指标（VM2 - BookStack）</h3>
      <div v-if="hostMetrics.cpu" class="grid grid-cols-4 gap-4">
        <div>
          <p class="text-xs text-slate-500 mb-1">CPU使用率</p>
          <div class="h-2 bg-slate-200 rounded-full">
            <div class="h-2 bg-blue-500 rounded-full" :style="{width: hostMetrics.cpu + '%'}"></div>
          </div>
          <p class="text-sm font-medium mt-1">{{ hostMetrics.cpu }}%</p>
        </div>
        <div>
          <p class="text-xs text-slate-500 mb-1">内存使用率</p>
          <div class="h-2 bg-slate-200 rounded-full">
            <div class="h-2 bg-green-500 rounded-full" :style="{width: hostMetrics.mem + '%'}"></div>
          </div>
          <p class="text-sm font-medium mt-1">{{ hostMetrics.mem }}%</p>
        </div>
        <div>
          <p class="text-xs text-slate-500 mb-1">磁盘使用率</p>
          <div class="h-2 bg-slate-200 rounded-full">
            <div class="h-2 bg-amber-500 rounded-full" :style="{width: hostMetrics.disk + '%'}"></div>
          </div>
          <p class="text-sm font-medium mt-1">{{ hostMetrics.disk }}%</p>
        </div>
        <div>
          <p class="text-xs text-slate-500 mb-1">网络RX</p>
          <p class="text-sm font-medium">{{ fmtBytes(hostMetrics.netRx) }}</p>
        </div>
      </div>
      <div v-else class="text-slate-400 text-sm">正在加载主机指标...</div>
    </div>

    <!-- Top Protocols -->
    <div class="grid grid-cols-2 gap-6">
      <div class="card p-6">
        <h3 class="text-lg font-semibold mb-4">Top 源IP</h3>
        <div v-for="ip in topSrcIPs" :key="ip.ip" class="flex justify-between py-2 border-b border-slate-100 text-sm">
          <span class="font-mono">{{ ip.ip }}</span>
          <span class="text-slate-500">{{ fmtBytes(ip.bytes) }} ({{ ip.count }} flows)</span>
        </div>
      </div>
      <div class="card p-6">
        <h3 class="text-lg font-semibold mb-4">Top 目的IP</h3>
        <div v-for="ip in topDstIPs" :key="ip.ip" class="flex justify-between py-2 border-b border-slate-100 text-sm">
          <span class="font-mono">{{ ip.ip }}</span>
          <span class="text-slate-500">{{ fmtBytes(ip.bytes) }} ({{ ip.count }} flows)</span>
        </div>
      </div>
    </div>

    <p class="text-xs text-slate-400 text-center">数据来源: ClickHouse cloudflow.flows / cloudflow.host_metrics | eBPF探针 VM2 → VM1</p>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { queryService } from '@/api'

const totalBytes = ref(0)
const totalPackets = ref(0)
const uniqueIPs = ref(0)
const httpCount = ref(0)
const topSrcIPs = ref([])
const topDstIPs = ref([])
const hostMetrics = ref({})

onMounted(async () => {
  try {
    // 从query-service获取overview数据
    const res = await queryService.getOverview()
    const d = res || {}
    totalBytes.value = d.total_bytes || d.totalBytes || 0
    totalPackets.value = d.total_packets || d.totalPackets || 0
    uniqueIPs.value = d.unique_ips || d.uniqueIPs || 0
    httpCount.value = d.http_count || d.httpCount || 0
    topSrcIPs.value = d.top_src_ips || d.topSrcIPs || []
    topDstIPs.value = d.top_dst_ips || d.topDstIPs || []
  } catch(e) {
    console.log('Query服务未就绪，尝试从flows直接获取...', e.message)
  }

  try {
    // 从VM2探针获取主机指标
    const r = await fetch('/api/probe/metrics')
    const m = await r.json()
    if (m.success && m.data) {
      hostMetrics.value = {
        cpu: m.data.cpu_percent || m.data.CPUPercent || 0,
        mem: m.data.memory_percent || m.data.MemoryPercent || 0,
        disk: m.data.disk_percent || m.data.DiskPercent || 0,
        netRx: m.data.net_rx_bytes || m.data.NetRxBytes || 0,
      }
    }
  } catch(e) {
    console.log('探针指标加载中...', e.message)
  }
})

function fmtBytes(b) {
  if (!b) return '0 B'
  const u = ['B','KB','MB','GB','TB']
  let i = 0; let v = Number(b)
  while (v >= 1024 && i < 4) { v /= 1024; i++ }
  return v.toFixed(1) + ' ' + u[i]
}
function fmtNum(n) { return Number(n || 0).toLocaleString() }
</script>
