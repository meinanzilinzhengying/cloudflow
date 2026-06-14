<template>
  <div class="space-y-6">
    <div class="flex items-center justify-between">
      <h2 class="text-2xl font-bold">服务调用分析</h2>
      <button @click="loadServiceData" class="btn-primary text-sm">刷新</button>
    </div>

    <!-- 统计卡片 -->
    <div class="grid grid-cols-1 md:grid-cols-4 gap-4">
      <div class="stat-card">
        <div class="text-sm text-gray-400">服务数量</div>
        <div class="text-2xl font-bold text-primary-400">{{ services.length }}</div>
      </div>
      <div class="stat-card">
        <div class="text-sm text-gray-400">调用关系</div>
        <div class="text-2xl font-bold text-blue-400">{{ links.length }}</div>
      </div>
      <div class="stat-card">
        <div class="text-sm text-gray-400">总流量</div>
        <div class="text-2xl font-bold text-green-400">{{ formatBytes(totalBytes) }}</div>
      </div>
      <div class="stat-card">
        <div class="text-sm text-gray-400">总包数</div>
        <div class="text-2xl font-bold text-yellow-400">{{ formatNumber(totalPackets) }}</div>
      </div>
    </div>

    <!-- 服务拓扑图 -->
    <div class="card">
      <h3 class="text-lg font-semibold mb-4">服务调用拓扑</h3>
      <div ref="topoChart" class="w-full h-96"></div>
    </div>

    <!-- Top 调用者 -->
    <div class="grid grid-cols-1 md:grid-cols-2 gap-6">
      <div class="card">
        <h3 class="text-lg font-semibold mb-4">Top 来源 IP</h3>
        <div class="space-y-2">
          <div v-for="(item, idx) in topSources" :key="idx"
            class="flex items-center justify-between p-3 bg-dark-700/50 rounded-lg">
            <div class="flex items-center gap-3">
              <span class="text-xs text-gray-500">#{{ idx + 1 }}</span>
              <span class="text-sm text-primary-400">{{ item.src_ip }}</span>
            </div>
            <div class="flex items-center gap-3">
              <div class="w-24 bg-dark-600 rounded-full h-2">
                <div class="h-2 rounded-full bg-primary-500" :style="`width: ${(item.total_bytes / maxSourceBytes * 100).toFixed(1)}%`"></div>
              </div>
              <span class="text-sm font-mono w-20 text-right">{{ formatBytes(item.total_bytes) }}</span>
            </div>
          </div>
        </div>
      </div>

      <div class="card">
        <h3 class="text-lg font-semibold mb-4">Top 目标 IP</h3>
        <div class="space-y-2">
          <div v-for="(item, idx) in topDestinations" :key="idx"
            class="flex items-center justify-between p-3 bg-dark-700/50 rounded-lg">
            <div class="flex items-center gap-3">
              <span class="text-xs text-gray-500">#{{ idx + 1 }}</span>
              <span class="text-sm text-blue-400">{{ item.dst_ip }}</span>
            </div>
            <div class="flex items-center gap-3">
              <div class="w-24 bg-dark-600 rounded-full h-2">
                <div class="h-2 rounded-full bg-blue-500" :style="`width: ${(item.total_bytes / maxDestBytes * 100).toFixed(1)}%`"></div>
              </div>
              <span class="text-sm font-mono w-20 text-right">{{ formatBytes(item.total_bytes) }}</span>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- 调用明细表 -->
    <div class="card">
      <h3 class="text-lg font-semibold mb-4">调用明细</h3>
      <div class="overflow-x-auto">
        <table class="w-full text-sm">
          <thead>
            <tr class="text-left text-gray-400 border-b border-dark-600">
              <th class="px-4 py-3">源 IP</th>
              <th class="px-4 py-3">目的 IP</th>
              <th class="px-4 py-3">协议</th>
              <th class="px-4 py-3">总字节数</th>
              <th class="px-4 py-3">总包数</th>
              <th class="px-4 py-3">调用次数</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="(link, idx) in links.slice(0, 20)" :key="idx"
              class="border-b border-dark-700 hover:bg-dark-700/50">
              <td class="px-4 py-3 text-primary-400">{{ link.src_ip }}</td>
              <td class="px-4 py-3 text-blue-400">{{ link.dst_ip }}</td>
              <td class="px-4 py-3">
                <span :class="link.protocol === 'TCP' ? 'protocol-tcp' : 'protocol-udp'">
                  {{ link.protocol }}
                </span>
              </td>
              <td class="px-4 py-3 text-gray-300">{{ formatBytes(link.total_bytes) }}</td>
              <td class="px-4 py-3 text-gray-300">{{ formatNumber(link.total_packets) }}</td>
              <td class="px-4 py-3 text-gray-300">{{ link.flow_count }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, onUnmounted, nextTick } from 'vue'
import * as echarts from 'echarts'

const FLOW_API = '/api/flows'

const services = ref([])
const links = ref([])
const topSources = ref([])
const topDestinations = ref([])
const totalBytes = ref(0)
const totalPackets = ref(0)
const maxSourceBytes = ref(1)
const maxDestBytes = ref(1)
const topoChart = ref(null)
let chartInstance = null

async function loadServiceData() {
  try {
    // 加载 Top Talkers
    const res = await fetch(`${FLOW_API}/top-talkers?limit=50`)
    const talkers = await res.json()
    
    // 构建服务和连接
    const serviceSet = new Set()
    const linkMap = new Map()
    
    talkers.forEach(t => {
      serviceSet.add(t.src_ip)
      serviceSet.add(t.dst_ip)
      
      const key = `${t.src_ip}|${t.dst_ip}|${t.protocol}`
      if (!linkMap.has(key)) {
        linkMap.set(key, {
          source: t.src_ip,
          target: t.dst_ip,
          protocol: t.protocol,
          total_bytes: parseInt(t.total_bytes || 0),
          total_packets: parseInt(t.total_packets || 0),
          flow_count: parseInt(t.flow_count || 0)
        })
      }
    })
    
    services.value = Array.from(serviceSet).map(ip => ({ name: ip, id: ip }))
    links.value = Array.from(linkMap.values())
    
    // 计算统计
    totalBytes.value = links.value.reduce((sum, l) => sum + l.total_bytes, 0)
    totalPackets.value = links.value.reduce((sum, l) => sum + l.total_packets, 0)
    
    // Top 来源和目标
    const srcStats = {}
    const dstStats = {}
    links.value.forEach(l => {
      srcStats[l.source] = (srcStats[l.source] || 0) + l.total_bytes
      dstStats[l.target] = (dstStats[l.target] || 0) + l.total_bytes
    })
    
    topSources.value = Object.entries(srcStats)
      .map(([ip, bytes]) => ({ src_ip: ip, total_bytes: bytes }))
      .sort((a, b) => b.total_bytes - a.total_bytes)
      .slice(0, 10)
    
    topDestinations.value = Object.entries(dstStats)
      .map(([ip, bytes]) => ({ dst_ip: ip, total_bytes: bytes }))
      .sort((a, b) => b.total_bytes - a.total_bytes)
      .slice(0, 10)
    
    maxSourceBytes.value = topSources.value[0]?.total_bytes || 1
    maxDestBytes.value = topDestinations.value[0]?.total_bytes || 1
    
    // 渲染拓扑图
    await nextTick()
    renderTopo()
  } catch (e) {
    console.error('加载服务数据失败:', e)
  }
}

function renderTopo() {
  if (!topoChart.value) return
  
  if (chartInstance) {
    chartInstance.dispose()
  }
  
  chartInstance = echarts.init(topoChart.value)
  
  const nodes = services.value.map(s => ({
    id: s.id,
    name: s.name,
    symbolSize: 30 + Math.random() * 20,
    category: 0
  }))
  
  const edges = links.value.map(l => ({
    source: l.source,
    target: l.target,
    value: l.total_bytes,
    lineStyle: {
      width: Math.max(1, Math.min(10, l.total_bytes / 1024 / 1024))
    }
  }))
  
  const option = {
    tooltip: {
      trigger: 'item',
      formatter: (params) => {
        if (params.dataType === 'edge') {
          return `${params.data.source} → ${params.data.target}<br/>流量: ${formatBytes(params.data.value)}`
        }
        return params.name
      }
    },
    legend: { show: false },
    animation: true,
    series: [{
      type: 'graph',
      layout: 'force',
      data: nodes,
      links: edges,
      roam: true,
      label: {
        show: true,
        position: 'right',
        color: '#e5e7eb',
        fontSize: 10
      },
      force: {
        repulsion: 200,
        edgeLength: [80, 200]
      },
      lineStyle: {
        color: 'source',
        curveness: 0.2,
        opacity: 0.6
      },
      itemStyle: {
        color: '#3b82f6'
      }
    }]
  }
  
  chartInstance.setOption(option)
}

function formatBytes(bytes) {
  if (!bytes || bytes === 0) return '0 B'
  bytes = parseInt(bytes)
  if (bytes < 1024) return bytes + ' B'
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB'
  if (bytes < 1024 * 1024 * 1024) return (bytes / 1024 / 1024).toFixed(1) + ' MB'
  return (bytes / 1024 / 1024 / 1024).toFixed(1) + ' GB'
}

function formatNumber(num) {
  if (!num) return '0'
  return parseInt(num).toLocaleString()
}

onMounted(() => {
  loadServiceData()
  setInterval(loadServiceData, 20000)
})

onUnmounted(() => {
  if (chartInstance) {
    chartInstance.dispose()
  }
})
</script>
