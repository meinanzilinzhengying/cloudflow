<template>
  <div class="space-y-6">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold text-slate-900 dark:text-white">日志检索</h1>
        <p class="text-sm text-slate-500 dark:text-slate-400 mt-1">搜索和分析日志数据</p>
      </div>
    </div>

    <div class="card p-6">
      <div class="flex items-center gap-4 mb-6">
        <div class="flex-1 relative">
          <Search class="absolute left-3 top-1/2 -translate-y-1/2 w-5 h-5 text-slate-400" />
          <input type="text" placeholder="搜索日志..." class="input pl-10 w-full" />
        </div>
        <select class="input w-40">
          <option>所有服务</option>
          <option>API Gateway</option>
          <option>User Service</option>
        </select>
        <select class="input w-32">
          <option>所有级别</option>
          <option>ERROR</option>
          <option>WARN</option>
          <option>INFO</option>
        </select>
        <button class="btn-primary">搜索</button>
      </div>

      <div class="bg-slate-900 rounded-lg p-4 font-mono text-sm overflow-x-auto">
        <div class="space-y-2">
          <div v-for="log in logs" :key="log.id" class="flex gap-4">
            <span class="text-slate-500 whitespace-nowrap">{{ log.time }}</span>
            <span :class="['whitespace-nowrap', getLogLevelColor(log.level)]">{{ log.level }}</span>
            <span class="text-slate-400 whitespace-nowrap">{{ log.service }}</span>
            <span class="text-slate-300 flex-1">{{ log.message }}</span>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { Search } from 'lucide-vue-next'

const logs = ref([
  { id: 1, time: '2024-01-15 10:30:45', level: 'INFO', service: 'api-gw', message: 'Request received: GET /api/v1/users' },
  { id: 2, time: '2024-01-15 10:30:45', level: 'INFO', service: 'user-svc', message: 'User lookup successful for user_id=123' },
  { id: 3, time: '2024-01-15 10:30:46', level: 'ERROR', service: 'order-svc', message: 'Database connection failed: timeout' },
  { id: 4, time: '2024-01-15 10:30:47', level: 'WARN', service: 'pay-svc', message: 'High latency detected: 500ms' },
  { id: 5, time: '2024-01-15 10:30:48', level: 'INFO', service: 'api-gw', message: 'Response sent: 200 OK' },
])

const getLogLevelColor = (level) => {
  const colors = {
    ERROR: 'text-red-400',
    WARN: 'text-amber-400',
    INFO: 'text-green-400',
    DEBUG: 'text-blue-400',
  }
  return colors[level] || 'text-slate-400'
}
</script>
