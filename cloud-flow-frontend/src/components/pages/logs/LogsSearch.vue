<template>
  <div class="space-y-6">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold text-slate-900 dark:text-white">日志检索</h1>
        <p class="text-sm text-slate-500 dark:text-slate-400 mt-1">搜索和分析日志数据</p>
      </div>
    </div>

    <div class="grid grid-cols-2 md:grid-cols-4 gap-4">
      <KPICard
        title="总日志数"
        :value="totalLogs"
        :icon="FileText"
        color="primary"
        :loading="loading"
      />
      <KPICard
        title="错误日志数"
        :value="errorLogs"
        :icon="AlertCircle"
        color="danger"
        :loading="loading"
      />
      <KPICard
        title="警告日志数"
        :value="warnLogs"
        :icon="AlertTriangle"
        color="warning"
        :loading="loading"
      />
      <KPICard
        title="信息日志数"
        :value="infoLogs"
        :icon="Info"
        color="success"
        :loading="loading"
      />
    </div>

    <div class="card p-6">
      <div class="flex flex-wrap items-center gap-4 mb-6">
        <div class="flex-1 min-w-[200px] relative">
          <Search class="absolute left-3 top-1/2 -translate-y-1/2 w-5 h-5 text-slate-400" />
          <input
            v-model="searchKeyword"
            type="text"
            placeholder="搜索日志..."
            class="input pl-10 w-full"
            @input="handleSearch"
          />
        </div>
        <select v-model="selectedLevel" class="input w-32" @change="handleSearch">
          <option value="">所有级别</option>
          <option value="ERROR">ERROR</option>
          <option value="WARN">WARN</option>
          <option value="INFO">INFO</option>
          <option value="DEBUG">DEBUG</option>
        </select>
        <select v-model="timeRange" class="input w-36" @change="handleSearch">
          <option value="1h">最近1小时</option>
          <option value="6h">最近6小时</option>
          <option value="24h">最近24小时</option>
          <option value="7d">最近7天</option>
        </select>
        <button class="btn-primary" @click="handleSearch">
          <RefreshCw :class="['w-4 h-4 mr-2', loading ? 'animate-spin' : '']" />
          刷新
        </button>
      </div>

      <div v-if="loading" class="flex items-center justify-center py-20">
        <div class="text-center">
          <RefreshCw class="w-8 h-8 text-primary-500 animate-spin mx-auto mb-3" />
          <p class="text-slate-500 dark:text-slate-400">加载中...</p>
        </div>
      </div>

      <div v-else-if="filteredLogs.length === 0" class="flex items-center justify-center py-20">
        <div class="text-center">
          <FileText class="w-12 h-12 text-slate-300 dark:text-slate-600 mx-auto mb-3" />
          <p class="text-slate-500 dark:text-slate-400">暂无日志数据</p>
        </div>
      </div>

      <div v-else class="bg-slate-900 rounded-lg p-4 font-mono text-sm overflow-x-auto max-h-[600px] overflow-y-auto">
        <div class="space-y-2">
          <div
            v-for="(log, idx) in filteredLogs"
            :key="idx"
            class="flex gap-4 hover:bg-slate-800/50 rounded px-2 py-1"
          >
            <span class="text-slate-500 whitespace-nowrap shrink-0">{{ formatTime(log.timestamp) }}</span>
            <span :class="['whitespace-nowrap shrink-0', getLogLevelColor(log.level)]">{{ log.level }}</span>
            <span class="text-slate-400 whitespace-nowrap shrink-0">{{ log.service }}</span>
            <span class="text-slate-300 flex-1 break-all">{{ log.message }}</span>
            <span v-if="log.source" class="text-slate-500 whitespace-nowrap shrink-0">{{ log.source }}</span>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { Search, FileText, AlertCircle, AlertTriangle, Info, RefreshCw } from 'lucide-vue-next'
import KPICard from '../../common/KPICard.vue'
import { queryService } from '../../../api'

const loading = ref(false)
const rawLogs = ref([])
const searchKeyword = ref('')
const selectedLevel = ref('')
const timeRange = ref('24h')

const normalizeLog = (log) => {
  const message = log.message || log.content || log.log || log.body || ''
  const levelRaw = log.level || log.severity || 'INFO'
  const level = typeof levelRaw === 'string' ? levelRaw.toUpperCase() : String(levelRaw).toUpperCase()
  const service = log.service || log.service_name || log.hostname || 'unknown'
  const timestamp = log.timestamp || log.time || log.created_at || new Date().toISOString()
  const traceId = log.trace_id || log.traceId || ''
  const source = log.source || log.container || log.pod || ''
  return { message, level, service, timestamp, traceId, source }
}

const fetchData = async () => {
  loading.value = true
  try {
    const response = await queryService.getOTELLogs({ limit: 200 })
    let logs = []
    if (Array.isArray(response)) {
      logs = response
    } else if (Array.isArray(response?.data)) {
      logs = response.data
    } else if (Array.isArray(response?.logs)) {
      logs = response.logs
    } else if (Array.isArray(response?.result)) {
      logs = response.result
    }
    rawLogs.value = logs.map(normalizeLog)
  } catch (error) {
    console.error('Failed to fetch logs:', error)
    rawLogs.value = []
  } finally {
    loading.value = false
  }
}

const totalLogs = computed(() => rawLogs.value.length)
const errorLogs = computed(() => rawLogs.value.filter(l => l.level === 'ERROR').length)
const warnLogs = computed(() => rawLogs.value.filter(l => l.level === 'WARN' || l.level === 'WARNING').length)
const infoLogs = computed(() => rawLogs.value.filter(l => l.level === 'INFO').length)

const filteredLogs = computed(() => {
  let result = rawLogs.value
  if (selectedLevel.value) {
    result = result.filter(log => log.level === selectedLevel.value)
  }
  if (searchKeyword.value.trim()) {
    const kw = searchKeyword.value.trim().toLowerCase()
    result = result.filter(log => log.message.toLowerCase().includes(kw))
  }
  return result
})

const handleSearch = () => {}

const formatTime = (ts) => {
  if (!ts) return ''
  try {
    const d = new Date(ts)
    if (isNaN(d.getTime())) return String(ts).slice(0, 19)
    const pad = (n) => String(n).padStart(2, '0')
    return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
  } catch {
    return String(ts)
  }
}

const getLogLevelColor = (level) => {
  const colors = {
    ERROR: 'text-red-400',
    WARN: 'text-amber-400',
    WARNING: 'text-amber-400',
    INFO: 'text-green-400',
    DEBUG: 'text-blue-400',
  }
  return colors[level] || 'text-slate-400'
}

onMounted(() => {
  fetchData()
})
</script>
