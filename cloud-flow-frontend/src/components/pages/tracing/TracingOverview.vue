<template>
  <div class="space-y-6">
    <!-- Page Header -->
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold text-slate-900 dark:text-white">Trace查询</h1>
        <p class="text-sm text-slate-500 dark:text-slate-400 mt-1">分布式链路追踪分析</p>
      </div>
    </div>

    <!-- KPI Cards -->
    <div class="grid grid-cols-4 gap-4">
      <div class="card p-6">
        <div class="text-sm text-slate-500 dark:text-slate-400">总 Trace 数</div>
        <div class="mt-2 text-3xl font-bold text-slate-900 dark:text-white">{{ totalTraces }}</div>
      </div>
      <div class="card p-6">
        <div class="text-sm text-slate-500 dark:text-slate-400">错误 Trace 数</div>
        <div class="mt-2 text-3xl font-bold text-red-500">{{ errorTraces }}</div>
      </div>
      <div class="card p-6">
        <div class="text-sm text-slate-500 dark:text-slate-400">平均延迟</div>
        <div class="mt-2 text-3xl font-bold text-slate-900 dark:text-white">{{ avgDuration }}ms</div>
      </div>
      <div class="card p-6">
        <div class="text-sm text-slate-500 dark:text-slate-400">P99 延迟</div>
        <div class="mt-2 text-3xl font-bold text-slate-900 dark:text-white">{{ p99Duration }}ms</div>
      </div>
    </div>

    <!-- Search Filters -->
    <div class="card p-6">
      <div class="grid grid-cols-5 gap-4">
        <div>
          <label class="block text-xs font-medium text-slate-500 mb-2">Service</label>
          <select v-model="filterService" class="input w-full">
            <option value="">All Services</option>
            <option v-for="svc in serviceList" :key="svc" :value="svc">{{ svc }}</option>
          </select>
        </div>
        <div>
          <label class="block text-xs font-medium text-slate-500 mb-2">Operation</label>
          <input v-model="filterOperation" type="text" placeholder="Search operation..." class="input w-full" />
        </div>
        <div>
          <label class="block text-xs font-medium text-slate-500 mb-2">Status</label>
          <select v-model="filterStatus" class="input w-full">
            <option value="">All</option>
            <option value="success">Success</option>
            <option value="error">Error</option>
          </select>
        </div>
        <div>
          <label class="block text-xs font-medium text-slate-500 mb-2">Duration</label>
          <select class="input w-full">
            <option>Any</option>
            <option>&lt; 100ms</option>
            <option>100ms - 500ms</option>
            <option>500ms - 1s</option>
            <option>&gt; 1s</option>
          </select>
        </div>
        <div class="flex items-end">
          <button @click="fetchData" class="btn-primary w-full">
            <Search class="w-4 h-4" />
            查询
          </button>
        </div>
      </div>
    </div>

    <!-- Time Range -->
    <div class="flex items-center gap-3">
      <button
        v-for="range in timeRanges"
        :key="range.value"
        @click="selectedTimeRange = range.value"
        :class="[
          'px-4 py-2 text-sm font-medium rounded-lg transition-all',
          selectedTimeRange === range.value
            ? 'bg-primary-500 text-white'
            : 'bg-slate-100 dark:bg-dark-700 text-slate-600 dark:text-slate-300 hover:bg-slate-200 dark:hover:bg-dark-600'
        ]"
      >
        {{ range.label }}
      </button>
    </div>

    <!-- Trace List -->
    <div class="card">
      <div v-if="loading" class="p-12 text-center text-slate-500">加载中...</div>
      <div v-else-if="filteredTraces.length === 0" class="p-12 text-center text-slate-500">暂无 Trace 数据</div>
      <div v-else class="overflow-x-auto">
        <table class="w-full">
          <thead>
            <tr class="bg-slate-50 dark:bg-dark-700/50">
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">Trace ID</th>
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">Duration</th>
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">Spans</th>
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">Status</th>
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">Service</th>
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">Operation</th>
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">Time</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-slate-200 dark:divide-dark-700">
            <tr
              v-for="trace in filteredTraces"
              :key="trace.id"
              class="hover:bg-slate-50 dark:hover:bg-dark-700/50 cursor-pointer transition-colors"
              @click="openTraceDetail(trace)"
            >
              <td class="px-6 py-4">
                <code class="text-sm text-primary-500 font-mono">{{ trace.id }}</code>
              </td>
              <td class="px-6 py-4">
                <span :class="['text-sm font-medium', trace.duration > 500 ? 'text-red-500' : 'text-slate-900 dark:text-white']">{{ trace.duration }}ms</span>
              </td>
              <td class="px-6 py-4 text-sm text-slate-500">{{ trace.spans }}</td>
              <td class="px-6 py-4">
                <span :class="['text-xs px-2 py-1 rounded-full font-medium', trace.status === 'success' ? 'bg-green-100 text-green-600' : 'bg-red-100 text-red-600']">
                  {{ trace.status === 'success' ? 'OK' : 'Error' }}
                </span>
              </td>
              <td class="px-6 py-4 text-sm text-slate-700 dark:text-slate-200">{{ trace.service }}</td>
              <td class="px-6 py-4 text-sm text-slate-700 dark:text-slate-200">{{ trace.operation }}</td>
              <td class="px-6 py-4 text-sm text-slate-500">{{ trace.time }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- Trace Detail Modal -->
    <Transition name="modal">
      <div
        v-if="selectedTrace"
        class="fixed inset-0 z-50 flex items-center justify-center p-4"
        @click.self="selectedTrace = null"
      >
        <div class="absolute inset-0 bg-slate-900/50"></div>
        <div class="relative w-full max-w-4xl bg-white dark:bg-dark-800 shadow-2xl rounded-2xl overflow-hidden">
          <div class="bg-slate-50 dark:bg-dark-700 px-6 py-4 flex items-center justify-between">
            <div>
              <h3 class="text-lg font-semibold text-slate-900 dark:text-white">Trace Detail</h3>
              <code class="text-xs text-slate-500 font-mono">{{ selectedTrace.id }}</code>
            </div>
            <button @click="selectedTrace = null" class="p-2 hover:bg-slate-200 dark:hover:bg-dark-600 rounded-lg transition-colors">
              <X class="w-5 h-5 text-slate-500" />
            </button>
          </div>
          <div class="p-6 max-h-[70vh] overflow-y-auto">
            <!-- Trace Overview -->
            <div class="flex items-center justify-between mb-6 pb-4 border-b border-slate-200 dark:border-dark-700">
              <div class="flex items-center gap-4">
                <span :class="['px-3 py-1 rounded-full text-sm font-medium', selectedTrace.status === 'success' ? 'bg-green-100 text-green-600' : 'bg-red-100 text-red-600']">
                  {{ selectedTrace.status === 'success' ? 'Success' : 'Error' }}
                </span>
                <span class="text-sm text-slate-500">{{ selectedTrace.spans }} spans</span>
              </div>
              <div class="text-right">
                <p class="text-2xl font-bold text-slate-900 dark:text-white">{{ selectedTrace.duration }}ms</p>
                <p class="text-xs text-slate-500">Total Duration</p>
              </div>
            </div>

            <!-- Waterfall Chart -->
            <div class="mb-6">
              <h4 class="text-sm font-semibold text-slate-700 dark:text-slate-200 mb-4">Timeline</h4>
              <div v-if="!selectedTrace.spansData || selectedTrace.spansData.length === 0" class="text-sm text-slate-500">暂无 Span 数据</div>
              <div v-else class="relative pl-8 border-l-2 border-slate-200 dark:border-dark-700">
                <div
                  v-for="(span, index) in selectedTrace.spansData"
                  :key="span.id || index"
                  class="relative mb-4"
                >
                  <div :class="['absolute left-0 top-1 w-4 h-4 rounded-full -translate-x-[9px] border-4', getSpanColor(span.service)]"></div>
                  <div class="ml-4">
                    <div class="flex items-center justify-between mb-1">
                      <span class="text-sm font-medium text-slate-700 dark:text-slate-200">{{ span.service }} - {{ span.operation }}</span>
                      <span class="text-xs text-slate-500">{{ span.duration }}ms</span>
                    </div>
                    <div class="h-3 bg-slate-100 dark:bg-dark-700 rounded-full overflow-hidden">
                      <div
                        :class="['h-full rounded-full', getSpanBarColor(span.service)]"
                        :style="{ width: selectedTrace.duration > 0 ? (span.duration / selectedTrace.duration) * 100 + '%' : '0%' }"
                      ></div>
                    </div>
                    <div class="flex items-center gap-2 mt-1">
                      <span class="text-xs text-slate-400">{{ span.startTime }}ms</span>
                      <span class="text-xs text-slate-400">|</span>
                      <span class="text-xs text-slate-400">{{ span.status }}</span>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </Transition>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { Search, X } from 'lucide-vue-next'
import { queryService } from '../../../api'

const selectedTimeRange = ref('1h')

const timeRanges = [
  { label: '1小时', value: '1h' },
  { label: '6小时', value: '6h' },
  { label: '24小时', value: '24h' },
  { label: '7天', value: '7d' },
]

const loading = ref(false)
const traces = ref([])
const totalTraces = ref(0)
const errorTraces = ref(0)
const avgDuration = ref(0)
const p99Duration = ref(0)

const filterService = ref('')
const filterOperation = ref('')
const filterStatus = ref('')

const selectedTrace = ref(null)

const serviceList = computed(() => {
  const services = new Set()
  traces.value.forEach((t) => {
    if (t.service) services.add(t.service)
  })
  return Array.from(services)
})

const filteredTraces = computed(() => {
  return traces.value.filter((t) => {
    if (filterService.value && t.service !== filterService.value) return false
    if (filterOperation.value && t.operation && !t.operation.toLowerCase().includes(filterOperation.value.toLowerCase())) return false
    if (filterStatus.value && t.status !== filterStatus.value) return false
    return true
  })
})

const fetchData = async () => {
  loading.value = true
  try {
    const data = await queryService.getOTELTraces({ limit: 20 })
    processTraceData(data)
  } catch (err) {
    console.error('Failed to fetch traces:', err)
    traces.value = []
    totalTraces.value = 0
    errorTraces.value = 0
    avgDuration.value = 0
    p99Duration.value = 0
  } finally {
    loading.value = false
  }
}

const processTraceData = (data) => {
  if (!data) {
    traces.value = []
    return
  }

  let list = []
  if (Array.isArray(data)) list = data
  else if (Array.isArray(data.traces)) list = data.traces
  else if (Array.isArray(data.data)) list = data.data
  else if (Array.isArray(data.items)) list = data.items

  traces.value = list.map((item, idx) => {
    const id = item.traceId || item.trace_id || item.id || `trace-${idx}`
    const duration = Number(item.duration || item.durationMs || item.duration_ms || 0)
    const spans = item.spans || item.spanCount || item.span_count || (Array.isArray(item.spansData) ? item.spansData.length : 0)
    const status = item.status || (item.error ? 'error' : 'success')
    const service = item.service || item.serviceName || item.service_name || ''
    const operation = item.operation || item.operationName || item.operation_name || item.name || ''
    const time = item.time || item.timestamp || ''

    let spansData = []
    if (Array.isArray(item.spans)) {
      spansData = item.spans.map((s, i) => ({
        id: s.id || s.spanId || s.span_id || i,
        service: s.service || s.serviceName || '',
        operation: s.operation || s.operationName || s.name || '',
        duration: Number(s.duration || s.durationMs || 0),
        startTime: s.startTime || s.start_time || 0,
        status: s.status || (s.error ? 'Error' : 'OK'),
      }))
    }

    return { id, duration, spans, status, service, operation, time, spansData }
  })

  const durations = traces.value.map(t => t.duration).filter(d => d > 0)
  totalTraces.value = traces.value.length
  errorTraces.value = traces.value.filter(t => t.status === 'error').length
  avgDuration.value = durations.length > 0 ? Math.round(durations.reduce((a, b) => a + b, 0) / durations.length) : 0
  p99Duration.value = durations.length > 0 ? calculateP99(durations) : 0
}

const calculateP99 = (values) => {
  if (values.length === 0) return 0
  const sorted = [...values].sort((a, b) => a - b)
  const idx = Math.ceil(0.99 * sorted.length) - 1
  return sorted[idx] || sorted[sorted.length - 1]
}

const openTraceDetail = (trace) => {
  selectedTrace.value = { ...trace }
}

const getSpanColor = (service) => {
  const colors = {
    'Gateway': 'bg-primary-500 border-primary-200',
    'User': 'bg-accent-500 border-accent-200',
    'Order': 'bg-red-500 border-red-200',
    'Payment': 'bg-emerald-500 border-emerald-200',
    'Redis': 'bg-amber-500 border-amber-200',
    'Mysql': 'bg-blue-500 border-blue-200',
    'Kafka': 'bg-purple-500 border-purple-200',
  }
  return colors[service?.split(' ')[0]] || 'bg-slate-500 border-slate-200'
}

const getSpanBarColor = (service) => {
  const colors = {
    'Gateway': 'bg-gradient-to-r from-primary-500 to-primary-400',
    'User': 'bg-gradient-to-r from-accent-500 to-accent-400',
    'Order': 'bg-gradient-to-r from-red-500 to-red-400',
    'Payment': 'bg-gradient-to-r from-emerald-500 to-emerald-400',
    'Redis': 'bg-gradient-to-r from-amber-500 to-amber-400',
    'Mysql': 'bg-gradient-to-r from-blue-500 to-blue-400',
    'Kafka': 'bg-gradient-to-r from-purple-500 to-purple-400',
  }
  return colors[service?.split(' ')[0]] || 'bg-gradient-to-r from-slate-500 to-slate-400'
}

onMounted(() => {
  fetchData()
})
</script>

<style scoped>
.modal-enter-active,
.modal-leave-active {
  transition: all 0.3s ease;
}
.modal-enter-from,
.modal-leave-to {
  opacity: 0;
}
.modal-enter-from > div:last-child,
.modal-leave-to > div:last-child {
  transform: scale(0.95) translateY(20px);
}
</style>
