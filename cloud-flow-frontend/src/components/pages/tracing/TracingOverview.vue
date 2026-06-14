<template>
  <div class="space-y-6">
    <div class="flex items-center justify-between">
      <h2 class="text-2xl font-bold">Trace 查询</h2>
      <div class="flex items-center gap-3">
        <input v-model="filters.src_ip" placeholder="源IP"
          class="bg-dark-700 border border-dark-600 rounded-lg px-3 py-2 text-sm w-32" />
        <input v-model="filters.dst_ip" placeholder="目的IP"
          class="bg-dark-700 border border-dark-600 rounded-lg px-3 py-2 text-sm w-32" />
        <button @click="loadFlows" class="btn-primary text-sm">查询</button>
        <button @click="autoRefresh = !autoRefresh" :class="autoRefresh ? 'btn-success' : 'btn-secondary'">
          {{ autoRefresh ? '自动刷新中' : '自动刷新' }}
        </button>
      </div>
    </div>

    <!-- 统计卡片 -->
    <div class="grid grid-cols-1 md:grid-cols-4 gap-4">
      <div class="stat-card">
        <div class="text-sm text-gray-400">总流数</div>
        <div class="text-2xl font-bold text-primary-400">{{ stats.total_flows }}</div>
      </div>
      <div class="stat-card">
        <div class="text-sm text-gray-400">总字节数</div>
        <div class="text-2xl font-bold text-blue-400">{{ formatBytes(stats.total_bytes) }}</div>
      </div>
      <div class="stat-card">
        <div class="text-sm text-gray-400">总包数</div>
        <div class="text-2xl font-bold text-green-400">{{ formatNumber(stats.total_packets) }}</div>
      </div>
      <div class="stat-card">
        <div class="text-sm text-gray-400">平均延迟</div>
        <div class="text-2xl font-bold text-yellow-400">{{ stats.avg_latency }} ms</div>
      </div>
    </div>

    <!-- 流列表 -->
    <div class="card">
      <div class="overflow-x-auto">
        <table class="w-full text-sm">
          <thead>
            <tr class="text-left text-gray-400 border-b border-dark-600">
              <th class="px-4 py-3">时间</th>
              <th class="px-4 py-3">源IP:端口</th>
              <th class="px-4 py-3">目的IP:端口</th>
              <th class="px-4 py-3">协议</th>
              <th class="px-4 py-3">字节数</th>
              <th class="px-4 py-3">包数</th>
              <th class="px-4 py-3">延迟(ms)</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="(flow, idx) in flows" :key="idx"
              class="border-b border-dark-700 hover:bg-dark-700/50 cursor-pointer"
              @click="showFlowDetail(flow)">
              <td class="px-4 py-3 text-gray-300">{{ formatTime(flow.timestamp) }}</td>
              <td class="px-4 py-3">
                <span class="text-primary-400">{{ flow.src_ip }}</span>:{{ flow.src_port }}
              </td>
              <td class="px-4 py-3">
                <span class="text-blue-400">{{ flow.dst_ip }}</span>:{{ flow.dst_port }}
              </td>
              <td class="px-4 py-3">
                <span :class="flow.protocol === 'TCP' ? 'protocol-tcp' : 'protocol-udp'">
                  {{ flow.protocol }}
                </span>
              </td>
              <td class="px-4 py-3 text-gray-300">{{ formatBytes(flow.bytes) }}</td>
              <td class="px-4 py-3 text-gray-300">{{ flow.packets }}</td>
              <td class="px-4 py-3">
                <span :class="flow.latency_ms > 100 ? 'text-red-400' : 'text-green-400'">
                  {{ flow.latency_ms }}
                </span>
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <!-- 分页 -->
      <div class="flex items-center justify-between px-4 py-3 border-t border-dark-600">
        <div class="text-sm text-gray-400">
          显示 {{ flows.length }} 条，共 {{ total }} 条
        </div>
        <div class="flex gap-2">
          <button @click="page > 1 && page--" :disabled="page <= 1" class="btn-secondary text-sm">上一页</button>
          <span class="px-3 py-1 text-sm">{{ page }}</span>
          <button @click="page++" :disabled="flows.length < pageSize" class="btn-secondary text-sm">下一页</button>
        </div>
      </div>
    </div>

    <!-- 流详情弹窗 -->
    <div v-if="selectedFlow" class="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
      <div class="bg-dark-800 rounded-xl p-6 w-1/2 max-h-1/2 overflow-y-auto">
        <div class="flex items-center justify-between mb-4">
          <h3 class="text-lg font-bold">流详情</h3>
          <button @click="selectedFlow = null" class="text-gray-400 hover:text-white">✕</button>
        </div>
        <div class="space-y-3 text-sm">
          <div><span class="text-gray-400">时间：</span>{{ selectedFlow.timestamp }}</div>
          <div><span class="text-gray-400">探针：</span>{{ selectedFlow.probe_id }}</div>
          <div><span class="text-gray-400">源：</span>{{ selectedFlow.src_ip }}:{{ selectedFlow.src_port }}</div>
          <div><span class="text-gray-400">目的：</span>{{ selectedFlow.dst_ip }}:{{ selectedFlow.dst_port }}</div>
          <div><span class="text-gray-400">协议：</span>{{ selectedFlow.protocol }}</div>
          <div><span class="text-gray-400">字节数：</span>{{ formatBytes(selectedFlow.bytes) }}</div>
          <div><span class="text-gray-400">包数：</span>{{ selectedFlow.packets }}</div>
          <div><span class="text-gray-400">延迟：</span>{{ selectedFlow.latency_ms }} ms</div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, onUnmounted } from 'vue'

const FLOW_API = '/api/flows'

const flows = ref([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(100)
const autoRefresh = ref(true)
const selectedFlow = ref(null)
const filters = ref({
  src_ip: '',
  dst_ip: ''
})

const stats = ref({
  total_flows: 0,
  total_bytes: 0,
  total_packets: 0,
  avg_latency: 0
})

let refreshTimer = null

async function loadFlows() {
  try {
    const params = new URLSearchParams({
      limit: pageSize.value,
      offset: (page.value - 1) * pageSize.value
    })
    if (filters.value.src_ip) params.append('src_ip', filters.value.src_ip)
    if (filters.value.dst_ip) params.append('dst_ip', filters.value.dst_ip)
    
    const res = await fetch(`${FLOW_API}?${params}`)
    const data = await res.json()
    flows.value = data.data || []
    total.value = data.total || 0
    
    // 加载统计
    loadStats()
  } catch (e) {
    console.error('加载流数据失败:', e)
  }
}

async function loadStats() {
  try {
    const res = await fetch(`${FLOW_API}/stats`)
    stats.value = await res.json()
  } catch (e) {
    console.error('加载统计失败:', e)
  }
}

function showFlowDetail(flow) {
  selectedFlow.value = flow
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
  if (bytes < 1024 * 1024 * 1024) return (bytes / 1024 / 1024).toFixed(1) + ' MB'
  return (bytes / 1024 / 1024 / 1024).toFixed(1) + ' GB'
}

function formatNumber(num) {
  if (!num) return '0'
  return parseInt(num).toLocaleString()
}

onMounted(() => {
  loadFlows()
  if (autoRefresh.value) {
    refreshTimer = setInterval(loadFlows, 10000)
  }
})

onUnmounted(() => {
  if (refreshTimer) clearInterval(refreshTimer)
})
</script>
