<template>
  <div class="space-y-6">
    <!-- Page Header -->
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold text-slate-900 dark:text-white">根因分析</h1>
        <p class="text-sm text-slate-500 dark:text-slate-400 mt-1">智能分析故障根因</p>
      </div>
      <button class="btn-primary">
        <Zap class="w-4 h-4" />
        分析
      </button>
    </div>

    <!-- RCA Steps -->
    <div class="card p-6">
      <div class="space-y-6">
        <!-- Step 1: Incident -->
        <div class="border-l-4 border-primary-500 pl-4">
          <div class="flex items-center gap-3 mb-2">
            <Clock class="w-5 h-5 text-primary-500" />
            <h3 class="text-lg font-semibold text-slate-900 dark:text-white">故障事件</h3>
          </div>
          <div class="p-4 bg-slate-50 dark:bg-dark-700 rounded-xl">
            <div class="flex items-center justify-between">
              <div>
                <p class="font-medium text-slate-900 dark:text-white">{{ currentIncident.title }}</p>
                <p class="text-sm text-slate-500 mt-1">{{ currentIncident.time }}</p>
              </div>
              <span :class="['text-xs px-2 py-1 rounded-full font-medium', getSeverityClass(currentIncident.severity)]">
                {{ currentIncident.severity }}
              </span>
            </div>
          </div>
        </div>

        <!-- Step 2: Abnormal Services -->
        <div class="border-l-4 border-red-500 pl-4">
          <div class="flex items-center gap-3 mb-2">
            <AlertTriangle class="w-5 h-5 text-red-500" />
            <h3 class="text-lg font-semibold text-slate-900 dark:text-white">异常服务</h3>
          </div>
          <div class="grid grid-cols-2 gap-3">
            <div
              v-for="service in abnormalServices"
              :key="service.name"
              class="p-4 bg-slate-50 dark:bg-dark-700 rounded-xl"
            >
              <div class="flex items-center justify-between mb-2">
                <span class="font-medium text-slate-900 dark:text-white">{{ service.name }}</span>
                <span :class="['text-xs px-2 py-1 rounded-full', service.status === 'error' ? 'bg-red-100 text-red-600' : 'bg-amber-100 text-amber-600']">
                  {{ service.status === 'error' ? '故障' : '异常' }}
                </span>
              </div>
              <div class="flex items-center gap-4 text-xs text-slate-500">
                <span>延迟: {{ service.latency }}ms</span>
                <span>错误率: {{ service.errorRate }}%</span>
              </div>
            </div>
          </div>
        </div>

        <!-- Step 3: Abnormal Traces -->
        <div class="border-l-4 border-amber-500 pl-4">
          <div class="flex items-center gap-3 mb-2">
            <GitBranch class="w-5 h-5 text-amber-500" />
            <h3 class="text-lg font-semibold text-slate-900 dark:text-white">异常Trace</h3>
          </div>
          <div class="space-y-3">
            <div
              v-for="trace in abnormalTraces"
              :key="trace.id"
              class="p-4 bg-slate-50 dark:bg-dark-700 rounded-xl"
            >
              <div class="flex items-center justify-between mb-2">
                <span class="font-medium text-slate-900 dark:text-white">{{ trace.operation }}</span>
                <span class="text-xs text-slate-500">{{ trace.duration }}ms</span>
              </div>
              <div class="h-2 bg-slate-200 dark:bg-dark-600 rounded-full overflow-hidden">
                <div
                  class="h-full bg-gradient-to-r from-amber-500 to-red-500 rounded-full"
                  :style="{ width: `${trace.durationPercent}%` }"
                ></div>
              </div>
            </div>
          </div>
        </div>

        <!-- Step 4: Abnormal Logs -->
        <div class="border-l-4 border-purple-500 pl-4">
          <div class="flex items-center gap-3 mb-2">
            <FileText class="w-5 h-5 text-purple-500" />
            <h3 class="text-lg font-semibold text-slate-900 dark:text-white">异常日志</h3>
          </div>
          <div class="p-4 bg-slate-900 rounded-lg font-mono text-sm text-slate-300 overflow-x-auto">
            <div v-for="log in abnormalLogs" :key="log.id" class="flex gap-4 mb-2">
              <span class="text-slate-500">{{ log.time }}</span>
              <span :class="log.level === 'ERROR' ? 'text-red-400' : 'text-amber-400'">{{ log.level }}</span>
              <span class="text-slate-400">{{ log.service }}</span>
              <span class="text-slate-300">{{ log.message }}</span>
            </div>
          </div>
        </div>

        <!-- Step 5: Abnormal Flows -->
        <div class="border-l-4 border-blue-500 pl-4">
          <div class="flex items-center gap-3 mb-2">
            <Network class="w-5 h-5 text-blue-500" />
            <h3 class="text-lg font-semibold text-slate-900 dark:text-white">异常流量</h3>
          </div>
          <div class="grid grid-cols-3 gap-3">
            <div v-for="flow in abnormalFlows" :key="flow.id" class="p-4 bg-slate-50 dark:bg-dark-700 rounded-xl">
              <div class="flex items-center gap-2 mb-2">
                <ArrowRightLeft class="w-4 h-4 text-blue-500" />
                <span class="font-medium text-slate-900 dark:text-white">{{ flow.protocol }}</span>
              </div>
              <p class="text-sm text-slate-500">{{ flow.src }} -> {{ flow.dst }}</p>
              <div class="flex items-center justify-between mt-2 text-xs text-slate-400">
                <span>{{ flow.packets }} packets</span>
                <span>{{ flow.latency }}ms</span>
              </div>
            </div>
          </div>
        </div>

        <!-- Step 6: Root Cause Inference -->
        <div class="border-l-4 border-emerald-500 pl-4">
          <div class="flex items-center gap-3 mb-4">
            <Target class="w-5 h-5 text-emerald-500" />
            <h3 class="text-lg font-semibold text-slate-900 dark:text-white">根因推断</h3>
          </div>
          <div class="space-y-4">
            <div
              v-for="cause in rootCauses"
              :key="cause.id"
              class="p-4 bg-emerald-50 dark:bg-emerald-500/10 rounded-xl border border-emerald-200 dark:border-emerald-500/20"
            >
              <div class="flex items-center justify-between">
                <div>
                  <p class="font-semibold text-emerald-800 dark:text-emerald-300">{{ cause.description }}</p>
                  <p class="text-sm text-emerald-600 dark:text-emerald-400 mt-1">{{ cause.explanation }}</p>
                </div>
                <div class="text-right">
                  <p class="text-2xl font-bold text-emerald-600 dark:text-emerald-400">{{ cause.confidence }}%</p>
                  <p class="text-xs text-emerald-500 dark:text-emerald-500">置信度</p>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { Zap, Clock, AlertTriangle, GitBranch, FileText, Network, ArrowRightLeft, Target } from 'lucide-vue-next'

const currentIncident = ref({
  title: '订单服务响应延迟增加',
  time: '2024-01-15 10:30:00',
  severity: 'critical',
})

const abnormalServices = ref([
  { name: 'Order Service', status: 'error', latency: 567, errorRate: 12.5 },
  { name: 'Payment Service', status: 'warning', latency: 234, errorRate: 3.2 },
])

const abnormalTraces = ref([
  { id: 1, operation: 'createOrder', duration: 890, durationPercent: 85 },
  { id: 2, operation: 'processPayment', duration: 456, durationPercent: 43 },
  { id: 3, operation: 'checkInventory', duration: 678, durationPercent: 64 },
])

const abnormalLogs = ref([
  { id: 1, time: '10:29:45', level: 'ERROR', service: 'order-svc', message: 'timeout connecting to database' },
  { id: 2, time: '10:29:46', level: 'ERROR', service: 'order-svc', message: 'connection pool exhausted' },
  { id: 3, time: '10:29:47', level: 'WARN', service: 'pay-svc', message: 'high latency detected' },
])

const abnormalFlows = ref([
  { id: 1, protocol: 'TCP', src: '192.168.1.100', dst: '10.0.0.5', packets: 1542, latency: 156 },
  { id: 2, protocol: 'TCP', src: '192.168.1.101', dst: '10.0.0.6', packets: 892, latency: 234 },
  { id: 3, protocol: 'UDP', src: '192.168.1.102', dst: '10.0.0.7', packets: 2341, latency: 45 },
])

const rootCauses = ref([
  { id: 1, description: 'Redis延迟增加', explanation: 'Redis响应时间从正常的10ms增加到200ms，导致服务响应延迟', confidence: 92 },
  { id: 2, description: 'MySQL连接池耗尽', explanation: '数据库连接池达到最大连接数，新请求无法获取连接', confidence: 86 },
  { id: 3, description: '网络带宽饱和', explanation: '网络带宽使用率达到95%，影响数据传输', confidence: 72 },
])

const getSeverityClass = (severity) => {
  const classes = {
    critical: 'bg-red-100 text-red-600',
    high: 'bg-amber-100 text-amber-600',
    medium: 'bg-yellow-100 text-yellow-600',
    low: 'bg-blue-100 text-blue-600',
  }
  return classes[severity] || 'bg-slate-100 text-slate-600'
}
</script>
