<template>
  <div class="space-y-6">
    <div class="flex items-center justify-between">
      <h2 class="text-2xl font-bold">慢请求分析</h2>
      <div class="flex items-center gap-3">
        <select v-model="sortBy" class="bg-dark-700 border border-dark-600 rounded-lg px-3 py-2 text-sm">
          <option value="bytes">按字节数排序</option>
          <option value="packets">按包数排序</option>
          <option value="latency">按延迟排序</option>
        </select>
        <button @click="loadSlowFlows" class="btn-primary text-sm">刷新</button>
      </div>
    </div>

    <!-- KPI 卡片 -->
    <div class="grid grid-cols-1 md:grid-cols-4 gap-4">
      <div class="stat-card">
        <div class="text-sm text-gray-400">慢流总数</div>
        <div class="text-2xl font-bold text-red-400">{{ slowFlows.length }}</div>
      </div>
      <div class="stat-card">
        <div class="text-sm text-gray-400">平均字节数</div>
        <div class="text-2xl font-bold text-orange-400">{{ formatBytes(avgBytes) }}</div>
      </div>
      <div class="stat-card">
        <div class="text-sm text-gray-400">最大延迟</div>
        <div class="text-2xl font-bold text-yellow-400">{{ maxLatency }} ms</div>
      </div>
      <div class="stat-card">
        <div class="text-sm text-gray-400">平均包数</div>
        <div class="text-2xl font-bold text-purple-400">{{ avgPackets }}</div>
      </div>
    </div>

    <!-- 慢流列表 -->
    <div class="card">
      <h3 class="text-lg font-semibold mb-4">慢请求列表</h3>
      <div class="overflow-x-auto">
        <table class="w-full text-sm">
          <thead>
            <tr class="text-left text-gray-400 border-b border-dark-600">
              <th class="px-4 py-3">时间</th>
              <th class="px-4 py-3">源 → 目的</th>
              <th class="px-4 py-3">协议</th>
              <th class="px-4 py-3">字节数</th>
              <th class="px-4 py-3">包数</th>
              <th class="px-4 py-3">延迟(ms)</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="(flow, idx) in slowFlows" :key="idx"
              class="border-b border-dark-700 hover:bg-dark-700/50">
              <td class="px-4 py-3 text-gray-300">{{ formatTime(flow.timestamp) }}</td>
              <td class="px-4 py-3">
                <span class="text-primary-400">{{ flow.src_ip }}:{{ flow.src_port }}</span>
                →
                <span class="text-blue-400">{{ flow.dst_ip }}:{{ flow.dst_port }}</span>
              </td>
              <td class="px-4 py-3">
                <span :class="flow.protocol === 'TCP' ? 'protocol-tcp' : 'protocol-udp'">
                  {{ flow.protocol }}
                </span>
              </td>
              <td class="px-4 py-3 text-gray-300">{{ formatBytes(flow.bytes) }}</td>
              <td class="px-4 py-3 text-gray-300">{{ flow.packets }}</td>
              <td class="px-4 py-3">
                <span :class="flow.latency_ms > 100 ? 'text-red-400 font-bold' : 'text-yellow-400'">
                  {{ flow.latency_ms }}
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

const FLOW_API = '/api/flows'

const slowFlows = ref([])
const sortBy = ref('bytes')

const avgBytes = computed(() => {
  if (slowFlows.value.length === 0) return 0
  const total = slowFlows.value.reduce((sum, f) => sum + parseInt(f.bytes || 0), 0)
  return Math.round(total / slowFlows.value.length)
})

const maxLatency = computed(() => {
  if (slowFlows.value.length === 0) return 0
  return Math.max(...slowFlows.value.map(f => parseFloat(f.latency_ms || 0)))
})

const avgPackets = computed(() => {
  if (slowFlows.value.length === 0) return 0
  const total = slowFlows.value.reduce((sum, f) => sum + parseInt(f.packets || 0), 0)
  return Math.round(total / slowFlows.value.length)
})

async function loadSlowFlows() {
  try {
    const res = await fetch(`${FLOW_API}?limit=500`)
    const data = await res.json()
    let flows = data.data || []
    
    // 过滤出"慢"流（字节数 > 10KB 或 包数 > 100 或 延迟 > 50ms）
    flows = flows.filter(f => 
      parseInt(f.bytes || 0) > 10240 || 
      parseInt(f.packets || 0) > 100 || 
      parseFloat(f.latency_ms || 0) > 50
    )
    
    // 排序
    if (sortBy.value === 'bytes') {
      flows.sort((a, b) => parseInt(b.bytes || 0) - parseInt(a.bytes || 0))
    } else if (sortBy.value === 'packets') {
      flows.sort((a, b) => parseInt(b.packets || 0) - parseInt(a.packets || 0))
    } else if (sortBy.value === 'latency') {
      flows.sort((a, b) => parseFloat(b.latency_ms || 0) - parseFloat(a.latency_ms || 0))
    }
    
    slowFlows.value = flows.slice(0, 100)
  } catch (e) {
    console.error('加载慢流失败:', e)
  }
}

function formatTime(ts) {
  if (!ts) return '--'
  return ts.split(' ')[1] || ts
}

function formatBytes(bytes) {
  if (!bytes || bytes === '0') return '0 B'
  bytes = parseInt(bytes)
  if (bytes < 1024) return bytes + ' B'
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB'
  return (bytes / 1024 / 1024).toFixed(1) + ' MB'
}

onMounted(() => {
  loadSlowFlows()
  setInterval(loadSlowFlows, 15000)
})
</script>
