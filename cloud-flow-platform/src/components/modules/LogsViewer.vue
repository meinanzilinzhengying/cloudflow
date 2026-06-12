<template>
  <div>
    <div class="flex items-center justify-between mb-6">
      <h3 class="font-semibold text-white">日志查看</h3>
      <div class="flex items-center gap-3">
        <select
          v-model="selectedService"
          class="bg-dark-700 border border-dark-600 text-white text-sm px-3 py-2 rounded-lg focus:outline-none focus:border-primary-500"
        >
          <option value="all">全部服务</option>
          <option v-for="svc in availableServices" :key="svc" :value="svc">{{ svc }}</option>
        </select>
        <select
          v-model="logLevel"
          class="bg-dark-700 border border-dark-600 text-white text-sm px-3 py-2 rounded-lg focus:outline-none focus:border-primary-500"
        >
          <option value="all">全部级别</option>
          <option value="info">INFO</option>
          <option value="warning">WARNING</option>
          <option value="error">ERROR</option>
          <option value="debug">DEBUG</option>
        </select>
        <button @click="refreshLogs" class="px-4 py-2 bg-dark-700 text-white text-sm font-medium rounded-lg hover:bg-dark-600 transition flex items-center gap-2">
          <RefreshCw :class="['w-4 h-4', { 'animate-spin': loading }]" />
          刷新
        </button>
      </div>
    </div>

    <!-- 实时日志统计 -->
    <div class="grid grid-cols-1 md:grid-cols-4 gap-4 mb-6">
      <div class="bg-dark-800 rounded-xl p-4 border border-dark-600">
        <div class="flex items-center justify-between">
          <div>
            <p class="text-gray-400 text-sm">总日志数</p>
            <p class="text-2xl font-bold text-white">{{ totalLogs }}</p>
          </div>
          <FileText class="w-8 h-8 text-gray-600" />
        </div>
      </div>
      <div class="bg-dark-800 rounded-xl p-4 border border-dark-600">
        <div class="flex items-center justify-between">
          <div>
            <p class="text-gray-400 text-sm">INFO</p>
            <p class="text-2xl font-bold text-blue-400">{{ infoCount }}</p>
          </div>
          <Info class="w-8 h-8 text-blue-400" />
        </div>
      </div>
      <div class="bg-dark-800 rounded-xl p-4 border border-dark-600">
        <div class="flex items-center justify-between">
          <div>
            <p class="text-gray-400 text-sm">WARNING</p>
            <p class="text-2xl font-bold text-yellow-400">{{ warningCount }}</p>
          </div>
          <AlertTriangle class="w-8 h-8 text-yellow-400" />
        </div>
      </div>
      <div class="bg-dark-800 rounded-xl p-4 border border-dark-600">
        <div class="flex items-center justify-between">
          <div>
            <p class="text-gray-400 text-sm">ERROR</p>
            <p class="text-2xl font-bold text-red-400">{{ errorCount }}</p>
          </div>
          <XCircle class="w-8 h-8 text-red-400" />
        </div>
      </div>
    </div>

    <!-- 日志搜索 -->
    <div class="mb-6">
      <div class="relative">
        <Search class="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-gray-400" />
        <input
          v-model="searchQuery"
          type="text"
          placeholder="搜索日志内容..."
          class="w-full pl-10 pr-4 py-3 bg-dark-800 border border-dark-600 rounded-lg text-white placeholder-gray-500 focus:outline-none focus:border-primary-500"
        />
      </div>
    </div>

    <!-- 日志列表 -->
    <div class="bg-dark-800 rounded-xl border border-dark-600 overflow-hidden">
      <div class="px-4 py-3 border-b border-dark-600 flex items-center justify-between">
        <div class="flex items-center gap-4">
          <span class="text-sm text-gray-400">共 {{ filteredLogs.length }} 条日志</span>
          <label class="flex items-center gap-2 text-sm text-gray-400 cursor-pointer">
            <input
              type="checkbox"
              v-model="autoRefresh"
              class="w-4 h-4 rounded bg-dark-700 border-dark-600 text-primary-500 focus:ring-primary-500"
            />
            自动刷新 (5秒)
          </label>
        </div>
        <button @click="exportLogs" class="px-3 py-1.5 bg-dark-700 text-gray-400 text-sm font-medium rounded-lg hover:bg-dark-600 transition flex items-center gap-2">
          <Download class="w-4 h-4" />
          导出
        </button>
      </div>
      <div class="max-h-[500px] overflow-y-auto font-mono text-sm">
        <div
          v-if="filteredLogs.length === 0"
          class="px-4 py-8 text-center text-gray-500"
        >
          <FileText class="w-12 h-12 mx-auto mb-2 opacity-50" />
          <p>{{ loading ? '正在加载日志...' : '暂无日志数据' }}</p>
        </div>
        <div
          v-for="(log, index) in filteredLogs"
          :key="index"
          class="px-4 py-2 border-b border-dark-700 hover:bg-dark-700/50 transition"
        >
          <div class="flex items-start gap-3">
            <span
              class="px-2 py-0.5 text-xs rounded flex-shrink-0 mt-0.5"
              :class="getLevelClass(log.level)"
            >
              {{ log.level.toUpperCase() }}
            </span>
            <span class="text-gray-500 flex-shrink-0">{{ log.time }}</span>
            <span class="text-primary-400 flex-shrink-0">[{{ log.service }}]</span>
            <span class="text-gray-300 break-all">{{ log.message }}</span>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import { RefreshCw, Search, Download, FileText, Info, AlertTriangle, XCircle } from 'lucide-vue-next'
import api from '../../api/index.js'

const loading = ref(false)
const selectedService = ref('all')
const logLevel = ref('all')
const searchQuery = ref('')
const autoRefresh = ref(false)
const logs = ref([])
const availableServices = ref([])
let refreshTimer = null

// 从 Loki 响应解析日志数据
function parseLokiResponse(data) {
  if (!data || !data.data || !data.data.result) return []

  const parsed = []
  for (const stream of data.data.result) {
    const labels = stream.stream || {}
    const service = labels.service || labels.container || 'unknown'

    for (const [timestamp, message] of stream.values || []) {
      // 从消息内容提取日志级别
      const level = extractLogLevel(message)
      // 将纳秒时间戳转为可读格式
      const time = formatTimestamp(timestamp)

      parsed.push({
        service,
        level,
        time,
        message: message.trim(),
        rawTime: timestamp
      })
    }
  }

  // 按时间倒序排列
  return parsed.sort((a, b) => b.rawTime - a.rawTime)
}

// 从日志消息中提取级别
function extractLogLevel(message) {
  if (!message) return 'info'
  const upper = message.toUpperCase()
  if (upper.includes('ERROR') || upper.includes('ERR ') || upper.includes('FATAL') || upper.includes('PANIC')) return 'error'
  if (upper.includes('WARN') || upper.includes('WARNING')) return 'warning'
  if (upper.includes('DEBUG') || upper.includes('TRACE')) return 'debug'
  return 'info'
}

// 格式化纳秒时间戳
function formatTimestamp(ns) {
  try {
    const ms = Math.floor(Number(ns) / 1e6)
    const date = new Date(ms)
    const pad = (n) => String(n).padStart(2, '0')
    return `${date.getFullYear()}-${pad(date.getMonth()+1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`
  } catch {
    return String(ns)
  }
}

const filteredLogs = computed(() => {
  let result = logs.value

  if (selectedService.value !== 'all') {
    result = result.filter(log => log.service === selectedService.value)
  }

  if (logLevel.value !== 'all') {
    result = result.filter(log => log.level === logLevel.value)
  }

  if (searchQuery.value) {
    const query = searchQuery.value.toLowerCase()
    result = result.filter(log =>
      log.message.toLowerCase().includes(query) ||
      log.service.toLowerCase().includes(query)
    )
  }

  return result
})

const totalLogs = computed(() => filteredLogs.value.length)
const infoCount = computed(() => filteredLogs.value.filter(l => l.level === 'info').length)
const warningCount = computed(() => filteredLogs.value.filter(l => l.level === 'warning').length)
const errorCount = computed(() => filteredLogs.value.filter(l => l.level === 'error').length)

function getLevelClass(level) {
  const classes = {
    'info': 'bg-blue-500/20 text-blue-400',
    'warning': 'bg-yellow-500/20 text-yellow-400',
    'error': 'bg-red-500/20 text-red-400',
    'debug': 'bg-gray-500/20 text-gray-400'
  }
  return classes[level] || 'bg-gray-500/20 text-gray-400'
}

async function fetchLogs() {
  loading.value = true
  try {
    // 构建 Loki 查询
    let query = '{service=~".+"}'
    if (selectedService.value !== 'all') {
      query = `{service=~".+",service="${selectedService.value}"}`
    }

    // 查询最近 1 小时的日志
    const end = Date.now() * 1e6  // 纳秒
    const start = end - 60 * 60 * 1e9  // 1 小时前

    const data = await api.getLogs({
      query,
      start: String(start),
      end: String(end),
      limit: 500,
      direction: 'backward'
    })

    if (data) {
      logs.value = parseLokiResponse(data)
    }
  } catch (err) {
    console.error('获取日志失败:', err)
  } finally {
    loading.value = false
  }
}

async function fetchServices() {
  try {
    const data = await api.getLogServices()
    if (data && data.data) {
      availableServices.value = data.data
        .filter(s => s && s !== 'unknown')
        .sort()
    }
  } catch (err) {
    console.error('获取服务列表失败:', err)
  }
}

function refreshLogs() {
  fetchLogs()
}

function exportLogs() {
  if (filteredLogs.value.length === 0) return

  const content = filteredLogs.value.map(log =>
    `[${log.time}] [${log.level.toUpperCase()}] [${log.service}] ${log.message}`
  ).join('\n')

  const blob = new Blob([content], { type: 'text/plain' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `logs_${new Date().toISOString().slice(0,10)}_${new Date().toISOString().slice(11,19).replace(/:/g,'')}.txt`
  a.click()
  URL.revokeObjectURL(url)
}

// 自动刷新
watch(autoRefresh, (val) => {
  if (val) {
    refreshTimer = setInterval(() => {
      fetchLogs()
    }, 5000)
  } else {
    if (refreshTimer) {
      clearInterval(refreshTimer)
      refreshTimer = null
    }
  }
})

onMounted(() => {
  fetchServices()
  fetchLogs()
})

onUnmounted(() => {
  if (refreshTimer) {
    clearInterval(refreshTimer)
    refreshTimer = null
  }
})
</script>
