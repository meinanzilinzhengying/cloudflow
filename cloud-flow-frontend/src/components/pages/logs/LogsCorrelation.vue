<template>
  <div class="space-y-6">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold text-slate-900 dark:text-white">日志关联分析</h1>
        <p class="text-sm text-slate-500 dark:text-slate-400 mt-1">按 Trace ID 聚合分析日志</p>
      </div>
    </div>

    <div v-if="loading" class="card p-6">
      <div class="flex items-center justify-center py-20">
        <div class="text-center">
          <RefreshCw class="w-8 h-8 text-primary-500 animate-spin mx-auto mb-3" />
          <p class="text-slate-500 dark:text-slate-400">加载中...</p>
        </div>
      </div>
    </div>

    <div v-else-if="traces.length === 0" class="card p-6">
      <div class="flex items-center justify-center py-20">
        <div class="text-center">
          <GitBranch class="w-12 h-12 text-slate-300 dark:text-slate-600 mx-auto mb-3" />
          <p class="text-slate-500 dark:text-slate-400">暂无日志数据</p>
        </div>
      </div>
    </div>

    <template v-else>
      <div class="card p-6">
        <div class="flex items-center justify-between mb-4">
          <h3 class="text-lg font-semibold text-slate-900 dark:text-white">Trace 列表</h3>
          <span class="text-sm text-slate-500 dark:text-slate-400">共 {{ traces.length }} 个 Trace</span>
        </div>

        <div class="overflow-x-auto">
          <table class="w-full text-sm">
            <thead>
              <tr class="border-b border-slate-200 dark:border-slate-700">
                <th class="px-4 py-3 text-left font-medium text-slate-500 dark:text-slate-400">Trace ID</th>
                <th class="px-4 py-3 text-right font-medium text-slate-500 dark:text-slate-400">关联日志数</th>
                <th class="px-4 py-3 text-left font-medium text-slate-500 dark:text-slate-400">涉及服务</th>
                <th class="px-4 py-3 text-left font-medium text-slate-500 dark:text-slate-400">最近一条日志时间</th>
                <th class="px-4 py-3 text-left font-medium text-slate-500 dark:text-slate-400">操作</th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="trace in traces"
                :key="trace.traceId"
                :class="[
                  'border-b border-slate-100 dark:border-slate-800 hover:bg-slate-50 dark:hover:bg-slate-800/50 cursor-pointer transition',
                  selectedTraceId === trace.traceId ? 'bg-primary-50 dark:bg-primary-500/10' : ''
                ]"
                @click="toggleTrace(trace.traceId)"
              >
                <td class="px-4 py-3 font-mono text-xs text-slate-900 dark:text-white break-all">{{ trace.traceId }}</td>
                <td class="px-4 py-3 text-right text-slate-700 dark:text-slate-300">{{ trace.logCount }}</td>
                <td class="px-4 py-3">
                  <div class="flex flex-wrap gap-1">
                    <span
                      v-for="svc in trace.services"
                      :key="svc"
                      class="px-2 py-0.5 text-xs rounded bg-slate-100 text-slate-700 dark:bg-slate-700 dark:text-slate-300"
                    >
                      {{ svc }}
                    </span>
                  </div>
                </td>
                <td class="px-4 py-3 text-slate-500 dark:text-slate-400">{{ formatTime(trace.latestTime) }}</td>
                <td class="px-4 py-3">
                  <button class="text-primary-500 hover:text-primary-700 dark:hover:text-primary-300 text-xs">
                    {{ selectedTraceId === trace.traceId ? '收起' : '查看' }}
                  </button>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <div v-if="selectedTraceId && selectedTraceLogs.length > 0" class="card p-6">
        <div class="flex items-center justify-between mb-4">
          <div>
            <h3 class="text-lg font-semibold text-slate-900 dark:text-white">Trace 详情</h3>
            <p class="text-xs font-mono text-slate-500 dark:text-slate-400 mt-1 break-all">{{ selectedTraceId }}</p>
          </div>
          <button
            class="text-sm text-slate-500 dark:text-slate-400 hover:text-slate-700 dark:hover:text-slate-200"
            @click="selectedTraceId = ''"
          >
            关闭
          </button>
        </div>

        <div class="bg-slate-900 rounded-lg p-4 font-mono text-sm overflow-x-auto max-h-[500px] overflow-y-auto">
          <div class="space-y-2">
            <div
              v-for="(log, idx) in selectedTraceLogs"
              :key="idx"
              class="flex gap-4 hover:bg-slate-800/50 rounded px-2 py-1"
            >
              <span class="text-slate-500 whitespace-nowrap shrink-0">{{ formatTime(log.timestamp) }}</span>
              <span :class="['whitespace-nowrap shrink-0', getLogLevelColor(log.level)]">{{ log.level }}</span>
              <span class="text-slate-400 whitespace-nowrap shrink-0">{{ log.service }}</span>
              <span class="text-slate-300 flex-1 break-all">{{ log.message }}</span>
            </div>
          </div>
        </div>
      </div>
    </template>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { GitBranch, RefreshCw } from 'lucide-vue-next'
import { queryService } from '../../../api'

const loading = ref(false)
const rawLogs = ref([])
const selectedTraceId = ref('')

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

const traces = computed(() => {
  const map = new Map()
  rawLogs.value.forEach(log => {
    if (!log.traceId) return
    const key = log.traceId
    if (!map.has(key)) {
      map.set(key, { traceId: key, logs: [], services: new Set(), latestTime: '' })
    }
    const item = map.get(key)
    item.logs.push(log)
    item.services.add(log.service)
    if (!item.latestTime || new Date(log.timestamp) > new Date(item.latestTime)) {
      item.latestTime = log.timestamp
    }
  })
  return Array.from(map.values())
    .map(t => ({
      traceId: t.traceId,
      logCount: t.logs.length,
      services: Array.from(t.services),
      latestTime: t.latestTime,
    }))
    .sort((a, b) => new Date(b.latestTime) - new Date(a.latestTime))
})

const selectedTraceLogs = computed(() => {
  if (!selectedTraceId.value) return []
  return rawLogs.value
    .filter(log => log.traceId === selectedTraceId.value)
    .sort((a, b) => new Date(a.timestamp) - new Date(b.timestamp))
})

const toggleTrace = (traceId) => {
  selectedTraceId.value = selectedTraceId.value === traceId ? '' : traceId
}

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
