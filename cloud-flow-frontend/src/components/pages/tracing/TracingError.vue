<template>
  <div class="space-y-6">
    <div class="flex items-center justify-between">
      <h2 class="text-2xl font-bold">错误请求分析</h2>
      <button @click="loadErrorFlows" class="btn-primary text-sm">刷新</button>
    </div>

    <!-- KPI 卡片 -->
    <div class="grid grid-cols-1 md:grid-cols-4 gap-4">
      <div class="stat-card">
        <div class="text-sm text-gray-400">错误流数</div>
        <div class="text-2xl font-bold text-red-400">{{ errorFlows.length }}</div>
      </div>
      <div class="stat-card">
        <div class="text-sm text-gray-400">TCP RST 流</div>
        <div class="text-2xl font-bold text-orange-400">{{ rstCount }}</div>
      </div>
      <div class="stat-card">
        <div class="text-sm text-gray-400">超时流</div>
        <div class="text-2xl font-bold text-yellow-400">{{ timeoutCount }}</div>
      </div>
      <div class="stat-card">
        <div class="text-sm text-gray-400">错误率</div>
        <div class="text-2xl font-bold text-pink-400">{{ errorRate }}%</div>
      </div>
    </div>

    <!-- 错误类型分布 -->
    <div class="grid grid-cols-1 md:grid-cols-2 gap-6">
      <div class="card">
        <h3 class="text-lg font-semibold mb-4">错误类型分布</h3>
        <div class="space-y-3">
          <div v-for="(type, idx) in errorTypes" :key="idx"
            class="flex items-center justify-between p-3 bg-dark-700/50 rounded-lg">
            <div class="flex items-center gap-3">
              <div :class="`w-3 h-3 rounded-full ${type.color}`"></div>
              <span class="text-sm">{{ type.name }}</span>
            </div>
            <div class="flex items-center gap-3">
              <div class="w-32 bg-dark-600 rounded-full h-2">
                <div :class="`h-2 rounded-full ${type.color}`" :style="`width: ${type.percent}%`"></div>
              </div>
              <span class="text-sm font-mono w-12 text-right">{{ type.count }}</span>
            </div>
          </div>
        </div>
      </div>

      <div class="card">
        <h3 class="text-lg font-semibold mb-4">错误流列表</h3>
        <div class="overflow-x-auto max-h-96">
          <table class="w-full text-sm">
            <thead>
              <tr class="text-left text-gray-400 border-b border-dark-600">
                <th class="px-4 py-3">时间</th>
                <th class="px-4 py-3">源 → 目的</th>
                <th class="px-4 py-3">错误类型</th>
                <th class="px-4 py-3">字节数</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="(flow, idx) in errorFlows.slice(0, 20)" :key="idx"
                class="border-b border-dark-700 hover:bg-dark-700/50">
                <td class="px-4 py-3 text-gray-300">{{ formatTime(flow.timestamp) }}</td>
                <td class="px-4 py-3">
                  <span class="text-primary-400">{{ flow.src_ip }}:{{ flow.src_port }}</span>
                  →
                  <span class="text-blue-400">{{ flow.dst_ip }}:{{ flow.dst_port }}</span>
                </td>
                <td class="px-4 py-3">
                  <span :class="getErrorClass(flow)" class="px-2 py-1 rounded text-xs">
                    {{ getErrorType(flow) }}
                  </span>
                </td>
                <td class="px-4 py-3 text-gray-300">{{ formatBytes(flow.bytes) }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'

const FLOW_API = '/api/flows'

const errorFlows = ref([])

const rstCount = computed(() => {
  return errorFlows.value.filter(f => getErrorType(f) === 'TCP RST').length
})

const timeoutCount = computed(() => {
  return errorFlows.value.filter(f => getErrorType(f) === '超时').length
})

const errorRate = computed(() => {
  // 假设总流数是错误流数的 10 倍
  if (errorFlows.value.length === 0) return 0
  return (errorFlows.value.length / (errorFlows.value.length * 10) * 100).toFixed(2)
})

const errorTypes = computed(() => {
  const types = {}
  errorFlows.value.forEach(f => {
    const type = getErrorType(f)
    types[type] = (types[type] || 0) + 1
  })
  
  const colors = {
    'TCP RST': 'bg-red-500',
    '超时': 'bg-yellow-500',
    '连接失败': 'bg-orange-500',
    '其他': 'bg-gray-500'
  }
  
  return Object.entries(types).map(([name, count]) => ({
    name,
    count,
    percent: (count / errorFlows.value.length * 100).toFixed(1),
    color: colors[name] || 'bg-gray-500'
  }))
})

async function loadErrorFlows() {
  try {
    const res = await fetch(`${FLOW_API}?limit=1000`)
    const data = await res.json()
    let flows = data.data || []
    
    // 检测错误流（字节数为0或非常少，可能是 RST 或连接失败）
    errorFlows.value = flows.filter(f => {
      const bytes = parseInt(f.bytes || 0)
      const packets = parseInt(f.packets || 0)
      // 错误特征：字节数=0 且 包数<=3（可能是 SYN 后没后续）
      return bytes === 0 && packets <= 3
    })
  } catch (e) {
    console.error('加载错误流失败:', e)
  }
}

function getErrorType(flow) {
  const bytes = parseInt(flow.bytes || 0)
  const packets = parseInt(flow.packets || 0)
  
  if (bytes === 0 && packets <= 2) return 'TCP RST'
  if (bytes === 0 && packets <= 5) return '连接失败'
  if (bytes < 100) return '超时'
  return '其他'
}

function getErrorClass(flow) {
  const type = getErrorType(flow)
  if (type === 'TCP RST') return 'bg-red-500/20 text-red-400'
  if (type === '超时') return 'bg-yellow-500/20 text-yellow-400'
  if (type === '连接失败') return 'bg-orange-500/20 text-orange-400'
  return 'bg-gray-500/20 text-gray-400'
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
  loadErrorFlows()
  setInterval(loadErrorFlows, 15000)
})
</script>
