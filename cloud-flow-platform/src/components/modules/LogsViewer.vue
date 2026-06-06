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
          <option value="center">Center</option>
          <option value="edge">Edge</option>
          <option value="agent">Agent</option>
          <option value="alert">Alert</option>
        </select>
        <select 
          v-model="logLevel"
          class="bg-dark-700 border border-dark-600 text-white text-sm px-3 py-2 rounded-lg focus:outline-none focus:border-primary-500"
        >
          <option value="all">全部级别</option>
          <option value="info">INFO</option>
          <option value="warning">WARNING</option>
          <option value="error">ERROR</option>
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
            自动刷新
          </label>
        </div>
        <button @click="exportLogs" class="px-3 py-1.5 bg-dark-700 text-gray-400 text-sm font-medium rounded-lg hover:bg-dark-600 transition flex items-center gap-2">
          <Download class="w-4 h-4" />
          导出
        </button>
      </div>
      <div class="max-h-[500px] overflow-y-auto font-mono text-sm">
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
            <span class="text-gray-300">{{ log.message }}</span>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed } from 'vue'
import { RefreshCw, Search, Download, FileText, Info, AlertTriangle, XCircle } from 'lucide-vue-next'

const loading = ref(false)
const selectedService = ref('all')
const logLevel = ref('all')
const searchQuery = ref('')
const autoRefresh = ref(false)

const logs = ref([
  { time: '14:35:22', level: 'info', service: 'center', message: '服务启动完成，监听端口 8080' },
  { time: '14:35:21', level: 'info', service: 'center', message: '配置文件加载成功' },
  { time: '14:35:20', level: 'info', service: 'center', message: '数据库连接池初始化完成' },
  { time: '14:35:18', level: 'warning', service: 'edge', message: '连接超时，重试中 (attempt 1/3)' },
  { time: '14:35:15', level: 'error', service: 'edge', message: '无法连接到远程服务器 10.0.0.5:8080' },
  { time: '14:35:12', level: 'info', service: 'agent', message: '探针状态上报成功' },
  { time: '14:35:10', level: 'info', service: 'alert', message: '告警规则 "CPU使用率" 触发，当前值: 87%' },
  { time: '14:35:08', level: 'warning', service: 'center', message: '内存使用率达到 85%，建议关注' },
  { time: '14:35:05', level: 'info', service: 'center', message: '健康检查完成，所有服务正常' },
  { time: '14:35:02', level: 'info', service: 'edge', message: '边缘节点注册成功' },
  { time: '14:35:00', level: 'error', service: 'agent', message: '磁盘空间不足，当前剩余: 2.1GB' },
  { time: '14:34:58', level: 'info', service: 'center', message: '定时任务执行成功: cleanup_expired_sessions' },
  { time: '14:34:55', level: 'warning', service: 'edge', message: '网络延迟较高: 245ms' },
  { time: '14:34:52', level: 'info', service: 'alert', message: '告警通知已发送至 webhook' },
  { time: '14:34:50', level: 'error', service: 'center', message: '请求处理失败: timeout after 30s' }
])

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
    result = result.filter(log => log.message.toLowerCase().includes(query))
  }
  
  return result
})

const totalLogs = computed(() => logs.value.length)
const infoCount = computed(() => logs.value.filter(l => l.level === 'info').length)
const warningCount = computed(() => logs.value.filter(l => l.level === 'warning').length)
const errorCount = computed(() => logs.value.filter(l => l.level === 'error').length)

function getLevelClass(level) {
  const classes = {
    'info': 'bg-blue-500/20 text-blue-400',
    'warning': 'bg-yellow-500/20 text-yellow-400',
    'error': 'bg-red-500/20 text-red-400'
  }
  return classes[level] || 'bg-gray-500/20 text-gray-400'
}

function refreshLogs() {
  loading.value = true
  setTimeout(() => {
    loading.value = false
  }, 500)
}

function exportLogs() {
  const content = filteredLogs.value.map(log => 
    `[${log.time}] [${log.level.toUpperCase()}] [${log.service}] ${log.message}`
  ).join('\n')
  
  const blob = new Blob([content], { type: 'text/plain' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `logs_${new Date().toISOString().slice(0,10)}.txt`
  a.click()
  URL.revokeObjectURL(url)
}
</script>
