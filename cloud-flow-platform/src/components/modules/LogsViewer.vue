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

    <!-- 时间筛选 -->
    <div class="mb-4 p-4 bg-dark-800 rounded-xl border border-dark-600">
      <div class="flex flex-wrap items-center gap-3">
        <span class="text-sm text-gray-400">时间范围：</span>
        <button
          v-for="preset in timePresets"
          :key="preset.value"
          @click="setTimePreset(preset.value)"
          class="px-3 py-1.5 text-xs rounded-lg transition"
          :class="activePreset === preset.value ? 'bg-primary-500/20 text-primary-400 border border-primary-500/30' : 'bg-dark-700 text-gray-400 border border-dark-600 hover:bg-dark-600'"
        >
          {{ preset.label }}
        </button>
        <span class="text-gray-600 mx-1">|</span>
        <div class="flex items-center gap-2">
          <label class="text-xs text-gray-500">从</label>
          <input
            v-model="customStartTime"
            type="datetime-local"
            class="bg-dark-700 border border-dark-600 text-white text-xs px-2 py-1.5 rounded-lg focus:outline-none focus:border-primary-500"
          />
          <label class="text-xs text-gray-500">至</label>
          <input
            v-model="customEndTime"
            type="datetime-local"
            class="bg-dark-700 border border-dark-600 text-white text-xs px-2 py-1.5 rounded-lg focus:outline-none focus:border-primary-500"
          />
          <button
            @click="applyCustomTime"
            class="px-3 py-1.5 text-xs rounded-lg bg-primary-500/20 text-primary-400 border border-primary-500/30 hover:bg-primary-500/30 transition"
          >
            应用
          </button>
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
          <span class="text-sm text-gray-400">共 {{ totalLogs }} 条日志</span>
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
          v-if="pagedLogs.length === 0"
          class="px-4 py-8 text-center text-gray-500"
        >
          <FileText class="w-12 h-12 mx-auto mb-2 opacity-50" />
          <p>{{ loading ? '正在加载日志...' : '暂无日志数据' }}</p>
        </div>
        <div
          v-for="(log, index) in pagedLogs"
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

      <!-- 分页 -->
      <div class="px-4 py-3 border-t border-dark-600 flex items-center justify-between">
        <div class="flex items-center gap-2 text-sm text-gray-400">
          <span>每页 50 条</span>
          <span class="text-gray-600">|</span>
          <span>第 {{ currentPage }} / {{ totalPages }} 页</span>
        </div>
        <div class="flex items-center gap-2">
          <button
            @click="goToPage(1)"
            :disabled="currentPage <= 1"
            class="px-2 py-1 text-xs rounded bg-dark-700 border border-dark-600 text-gray-400 disabled:opacity-40 disabled:cursor-not-allowed hover:bg-dark-600 transition"
          >
            首页
          </button>
          <button
            @click="goToPage(currentPage - 1)"
            :disabled="currentPage <= 1"
            class="px-2 py-1 text-xs rounded bg-dark-700 border border-dark-600 text-gray-400 disabled:opacity-40 disabled:cursor-not-allowed hover:bg-dark-600 transition"
          >
            上一页
          </button>
          <button
            v-for="p in displayPages"
            :key="p"
            @click="goToPage(p)"
            class="px-3 py-1 text-xs rounded border transition"
            :class="p === currentPage ? 'bg-primary-500/20 text-primary-400 border-primary-500/30' : 'bg-dark-700 border-dark-600 text-gray-400 hover:bg-dark-600'"
          >
            {{ p }}
          </button>
          <button
            @click="goToPage(currentPage + 1)"
            :disabled="currentPage >= totalPages"
            class="px-2 py-1 text-xs rounded bg-dark-700 border border-dark-600 text-gray-400 disabled:opacity-40 disabled:cursor-not-allowed hover:bg-dark-600 transition"
          >
            下一页
          </button>
          <button
            @click="goToPage(totalPages)"
            :disabled="currentPage >= totalPages"
            class="px-2 py-1 text-xs rounded bg-dark-700 border border-dark-600 text-gray-400 disabled:opacity-40 disabled:cursor-not-allowed hover:bg-dark-600 transition"
          >
            末页
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import { RefreshCw, Search, Download, FileText, Info, AlertTriangle, XCircle } from 'lucide-vue-next'
import api from '../../api/index.js'

const PAGE_SIZE = 50

const loading = ref(false)
const selectedService = ref('all')
const logLevel = ref('all')
const searchQuery = ref('')
const autoRefresh = ref(false)
const logs = ref([])
const availableServices = ref([])
let refreshTimer = null

// 分页
const currentPage = ref(1)

// 时间筛选
const activePreset = ref('1h')
const customStartTime = ref('')
const customEndTime = ref('')
const timeStart = ref(0)
const timeEnd = ref(0)

const timePresets = [
  { label: '15分钟', value: '15m' },
  { label: '1小时', value: '1h' },
  { label: '6小时', value: '6h' },
  { label: '24小时', value: '24h' },
  { label: '7天', value: '7d' }
]

function setTimePreset(preset) {
  activePreset.value = preset
  const now = Date.now()
  let start = now
  switch (preset) {
    case '15m': start = now - 15 * 60 * 1000; break
    case '1h':  start = now - 60 * 60 * 1000; break
    case '6h':  start = now - 6 * 60 * 60 * 1000; break
    case '24h': start = now - 24 * 60 * 60 * 1000; break
    case '7d':  start = now - 7 * 24 * 60 * 60 * 1000; break
  }
  timeStart.value = start
  timeEnd.value = now
  currentPage.value = 1
  fetchLogs()
}

function applyCustomTime() {
  if (!customStartTime.value || !customEndTime.value) return
  timeStart.value = new Date(customStartTime.value).getTime()
  timeEnd.value = new Date(customEndTime.value).getTime()
  if (timeStart.value >= timeEnd.value) {
    console.warn('开始时间必须早于结束时间')
    return
  }
  activePreset.value = ''
  currentPage.value = 1
  fetchLogs()
}

function initTimePreset() {
  const now = Date.now()
  timeStart.value = now - 60 * 60 * 1000
  timeEnd.value = now
  // 初始化自定义时间输入框
  const pad = (n) => String(n).padStart(2, '0')
  const s = new Date(timeStart.value)
  const e = new Date(timeEnd.value)
  customStartTime.value = `${s.getFullYear()}-${pad(s.getMonth()+1)}-${pad(s.getDate())}T${pad(s.getHours())}:${pad(s.getMinutes())}`
  customEndTime.value = `${e.getFullYear()}-${pad(e.getMonth()+1)}-${pad(e.getDate())}T${pad(e.getHours())}:${pad(e.getMinutes())}`
}

// 从 Loki 响应解析日志数据
function parseLokiResponse(data) {
  if (!data || !data.data || !data.data.result) return []

  const parsed = []
  for (const stream of data.data.result) {
    const labels = stream.stream || {}
    const service = labels.service || labels.container || 'unknown'

    for (const [timestamp, message] of stream.values || []) {
      const level = extractLogLevel(message)
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

  return parsed.sort((a, b) => b.rawTime - a.rawTime)
}

function extractLogLevel(message) {
  if (!message) return 'info'
  const upper = message.toUpperCase()
  if (upper.includes('ERROR') || upper.includes('ERR ') || upper.includes('FATAL') || upper.includes('PANIC')) return 'error'
  if (upper.includes('WARN') || upper.includes('WARNING')) return 'warning'
  if (upper.includes('DEBUG') || upper.includes('TRACE')) return 'debug'
  return 'info'
}

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

// 过滤后的全部日志
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

// 分页后的日志
const pagedLogs = computed(() => {
  const start = (currentPage.value - 1) * PAGE_SIZE
  return filteredLogs.value.slice(start, start + PAGE_SIZE)
})

const totalLogs = computed(() => filteredLogs.value.length)
const totalPages = computed(() => Math.max(1, Math.ceil(filteredLogs.value.length / PAGE_SIZE)))
const infoCount = computed(() => filteredLogs.value.filter(l => l.level === 'info').length)
const warningCount = computed(() => filteredLogs.value.filter(l => l.level === 'warning').length)
const errorCount = computed(() => filteredLogs.value.filter(l => l.level === 'error').length)

// 分页导航显示（最多显示 5 个页码）
const displayPages = computed(() => {
  const total = totalPages.value
  const current = currentPage.value
  const pages = []
  let startPage = Math.max(1, current - 2)
  let endPage = Math.min(total, current + 2)
  if (endPage - startPage < 4) {
    if (startPage === 1) endPage = Math.min(total, startPage + 4)
    else startPage = Math.max(1, endPage - 4)
  }
  for (let i = startPage; i <= endPage; i++) pages.push(i)
  return pages
})

function goToPage(page) {
  if (page < 1 || page > totalPages.value) return
  currentPage.value = page
}

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
    let query = '{service=~".+"}'
    if (selectedService.value !== 'all') {
      query = `{service=~".+",service="${selectedService.value}"}`
    }

    const data = await api.getLogs({
      query,
      start: String(timeStart.value * 1e6),
      end: String(timeEnd.value * 1e6),
      limit: 1000,
      direction: 'backward'
    })

    if (data) {
      logs.value = parseLokiResponse(data)
      currentPage.value = 1
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
  // 刷新时更新时间范围
  if (!activePreset.value) {
    // 自定义时间不自动更新
  } else {
    setTimePreset(activePreset.value)
  }
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

watch(autoRefresh, (val) => {
  if (val) {
    refreshTimer = setInterval(() => {
      if (!activePreset.value) return
      setTimePreset(activePreset.value)
    }, 5000)
  } else {
    if (refreshTimer) {
      clearInterval(refreshTimer)
      refreshTimer = null
    }
  }
})

watch(selectedService, () => {
  currentPage.value = 1
  fetchLogs()
})

watch(logLevel, () => {
  currentPage.value = 1
})

onMounted(() => {
  initTimePreset()
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
