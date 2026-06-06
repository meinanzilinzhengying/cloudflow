<template>
  <div>
    <div class="flex items-center justify-between mb-6">
      <h3 class="font-semibold text-white">健康检查</h3>
      <button @click="refreshStatus" class="px-4 py-2 bg-dark-700 text-white text-sm font-medium rounded-lg hover:bg-dark-600 transition flex items-center gap-2">
        <RefreshCw :class="['w-4 h-4', { 'animate-spin': loading }]" />
        刷新状态
      </button>
    </div>
    
    <!-- 总体状态 -->
    <div class="bg-dark-800 rounded-xl p-6 border border-dark-600 mb-6">
      <div class="flex items-center gap-4">
        <div :class="['w-16 h-16 rounded-full flex items-center justify-center', statusClass]">
          <component :is="statusIcon" class="w-8 h-8" />
        </div>
        <div>
          <h3 class="text-xl font-semibold text-white">平台健康状态: {{ statusText }}</h3>
          <p class="text-gray-400 mt-1">所有核心服务运行正常</p>
        </div>
      </div>
    </div>
    
    <div class="grid grid-cols-1 lg:grid-cols-2 gap-6 mb-6">
      <!-- 服务组件状态 -->
      <div class="bg-dark-800 rounded-xl border border-dark-600">
        <div class="px-4 py-3 border-b border-dark-600">
          <h3 class="font-medium text-white">服务组件状态</h3>
        </div>
        <div class="p-4 grid grid-cols-2 md:grid-cols-3 gap-4">
          <div v-for="service in services" :key="service.name" class="bg-dark-700 rounded-lg p-4 text-center">
            <div :class="['w-3 h-3 rounded-full mx-auto mb-2', getServiceStatusClass(service.status)]"></div>
            <div class="text-sm font-medium text-white">{{ service.name }}</div>
            <div class="text-xs text-gray-400 mt-1">{{ service.count }} 个实例</div>
            <div class="text-xs mt-1" :class="getServiceStatusTextClass(service.status)">{{ service.status }}</div>
          </div>
        </div>
      </div>
      
      <!-- 告警规则状态 -->
      <div class="bg-dark-800 rounded-xl border border-dark-600">
        <div class="px-4 py-3 border-b border-dark-600">
          <h3 class="font-medium text-white">告警规则状态</h3>
        </div>
        <div class="p-4">
          <div class="flex items-center gap-8">
            <div class="text-center">
              <div class="text-3xl font-bold text-white">{{ alertRules.total }}</div>
              <div class="text-sm text-gray-400">总规则数</div>
            </div>
            <div class="text-center">
              <div class="text-3xl font-bold text-red-400">{{ alertRules.active }}</div>
              <div class="text-sm text-gray-400">触发中</div>
            </div>
            <div class="text-center">
              <div class="text-3xl font-bold text-yellow-400">{{ alertRules.muted }}</div>
              <div class="text-sm text-gray-400">已静默</div>
            </div>
          </div>
        </div>
      </div>
    </div>
    
    <!-- 服务详情列表 -->
    <div class="bg-dark-800 rounded-xl border border-dark-600 overflow-hidden">
      <div class="px-4 py-3 border-b border-dark-600">
        <h3 class="font-medium text-white">服务详情</h3>
      </div>
      <table class="w-full">
        <thead>
          <tr class="border-b border-dark-600">
            <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">服务名称</th>
            <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">类型</th>
            <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">状态</th>
            <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">响应时间</th>
            <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">最后检查</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="service in serviceDetails" :key="service.id" class="border-b border-dark-700 hover:bg-dark-700/50 transition">
            <td class="px-4 py-3 text-sm text-white">{{ service.name }}</td>
            <td class="px-4 py-3 text-sm text-gray-300">{{ service.type }}</td>
            <td class="px-4 py-3">
              <span 
                class="px-2 py-1 text-xs rounded-full"
                :class="service.status === 'online' ? 'bg-green-500/20 text-green-400' : 'bg-red-500/20 text-red-400'"
              >
                {{ service.status === 'online' ? '正常' : '异常' }}
              </span>
            </td>
            <td class="px-4 py-3 text-sm text-gray-300">{{ service.responseTime }}</td>
            <td class="px-4 py-3 text-sm text-gray-400">{{ service.lastCheck }}</td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<script setup>
import { ref, computed } from 'vue'
import { RefreshCw, CheckCircle, AlertCircle, XCircle } from 'lucide-vue-next'

const loading = ref(false)

const services = ref([
  { name: 'cloud-flow-agent', status: 'healthy', count: 12 },
  { name: 'cloud-flow-center', status: 'healthy', count: 3 },
  { name: 'cloud-flow-edge', status: 'healthy', count: 6 },
  { name: 'alert-engine', status: 'healthy', count: 2 },
  { name: 'auth-service', status: 'healthy', count: 2 }
])

const alertRules = ref({
  total: 24,
  active: 3,
  muted: 2
})

const serviceDetails = ref([
  { id: 1, name: 'cloud-flow-center-01', type: 'Center', status: 'online', responseTime: '28ms', lastCheck: '30秒前' },
  { id: 2, name: 'cloud-flow-center-02', type: 'Center', status: 'online', responseTime: '32ms', lastCheck: '30秒前' },
  { id: 3, name: 'cloud-flow-edge-beijing', type: 'Edge', status: 'online', responseTime: '45ms', lastCheck: '30秒前' },
  { id: 4, name: 'cloud-flow-edge-shanghai', type: 'Edge', status: 'offline', responseTime: '-', lastCheck: '15分钟前' },
  { id: 5, name: 'alert-engine-main', type: 'Service', status: 'online', responseTime: '15ms', lastCheck: '30秒前' },
  { id: 6, name: 'auth-service', type: 'Service', status: 'online', responseTime: '22ms', lastCheck: '30秒前' }
])

const overallStatus = computed(() => {
  const hasOffline = serviceDetails.value.some(s => s.status === 'offline')
  if (hasOffline) return 'warning'
  return 'healthy'
})

const statusClass = computed(() => {
  if (overallStatus.value === 'healthy') return 'bg-green-500/20 text-green-400'
  if (overallStatus.value === 'warning') return 'bg-yellow-500/20 text-yellow-400'
  return 'bg-red-500/20 text-red-400'
})

const statusIcon = computed(() => {
  if (overallStatus.value === 'healthy') return CheckCircle
  if (overallStatus.value === 'warning') return AlertCircle
  return XCircle
})

const statusText = computed(() => {
  if (overallStatus.value === 'healthy') return '健康'
  if (overallStatus.value === 'warning') return '警告'
  return '异常'
})

function getServiceStatusClass(status) {
  if (status === 'healthy' || status === 'online') return 'bg-green-500'
  if (status === 'warning') return 'bg-yellow-500'
  return 'bg-red-500'
}

function getServiceStatusTextClass(status) {
  if (status === 'healthy' || status === 'online') return 'text-green-400'
  if (status === 'warning') return 'text-yellow-400'
  return 'text-red-400'
}

function refreshStatus() {
  loading.value = true
  setTimeout(() => {
    loading.value = false
  }, 500)
}
</script>
