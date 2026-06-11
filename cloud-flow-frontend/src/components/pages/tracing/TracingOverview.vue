<template>
  <div class="space-y-6">
    <!-- Page Header -->
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold text-slate-900 dark:text-white">Trace查询</h1>
        <p class="text-sm text-slate-500 dark:text-slate-400 mt-1">分布式链路追踪分析</p>
      </div>
    </div>

    <!-- Search Filters -->
    <div class="card p-6">
      <div class="grid grid-cols-5 gap-4">
        <div>
          <label class="block text-xs font-medium text-slate-500 mb-2">Service</label>
          <select class="input w-full">
            <option>All Services</option>
            <option>API Gateway</option>
            <option>User Service</option>
            <option>Order Service</option>
            <option>Payment Service</option>
          </select>
        </div>
        <div>
          <label class="block text-xs font-medium text-slate-500 mb-2">Operation</label>
          <input type="text" placeholder="Search operation..." class="input w-full" />
        </div>
        <div>
          <label class="block text-xs font-medium text-slate-500 mb-2">Status</label>
          <select class="input w-full">
            <option>All</option>
            <option>Success</option>
            <option>Error</option>
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
          <button class="btn-primary w-full">
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
      <div class="overflow-x-auto">
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
              v-for="trace in traces"
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
              <div class="relative pl-8 border-l-2 border-slate-200 dark:border-dark-700">
                <div
                  v-for="(span, index) in selectedTrace.spansData"
                  :key="span.id"
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
                        :style="{ width: `${(span.duration / selectedTrace.duration) * 100}%` }"
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
import { ref } from 'vue'
import { Search, X } from 'lucide-vue-next'

const selectedTimeRange = ref('1h')

const timeRanges = [
  { label: '1小时', value: '1h' },
  { label: '6小时', value: '6h' },
  { label: '24小时', value: '24h' },
  { label: '7天', value: '7d' },
]

const traces = ref([
  { id: 'abc123...', duration: 1234, spans: 15, status: 'error', service: 'Order Service', operation: 'createOrder', time: '2分钟前' },
  { id: 'def456...', duration: 456, spans: 8, status: 'success', service: 'API Gateway', operation: 'handleRequest', time: '5分钟前' },
  { id: 'ghi789...', duration: 789, spans: 12, status: 'success', service: 'User Service', operation: 'getUser', time: '8分钟前' },
  { id: 'jkl012...', duration: 234, spans: 5, status: 'success', service: 'Payment Service', operation: 'processPayment', time: '12分钟前' },
  { id: 'mno345...', duration: 1567, spans: 20, status: 'error', service: 'Order Service', operation: 'checkout', time: '15分钟前' },
])

const selectedTrace = ref(null)

const openTraceDetail = (trace) => {
  selectedTrace.value = {
    ...trace,
    spansData: [
      { id: 1, service: 'Gateway', operation: 'HTTP GET /api/order', duration: 1234, startTime: 0, status: 'OK' },
      { id: 2, service: 'User', operation: 'getUserById', duration: 234, startTime: 50, status: 'OK' },
      { id: 3, service: 'User -> Redis', operation: 'cache.get', duration: 34, startTime: 60, status: 'OK' },
      { id: 4, service: 'User -> Mysql', operation: 'SELECT', duration: 180, startTime: 100, status: 'OK' },
      { id: 5, service: 'Order', operation: 'createOrder', duration: 890, startTime: 300, status: 'Error' },
      { id: 6, service: 'Order -> Kafka', operation: 'produce', duration: 45, startTime: 320, status: 'OK' },
    ],
  }
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
  return colors[service.split(' ')[0]] || 'bg-slate-500 border-slate-200'
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
  return colors[service.split(' ')[0]] || 'bg-gradient-to-r from-slate-500 to-slate-400'
}
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
