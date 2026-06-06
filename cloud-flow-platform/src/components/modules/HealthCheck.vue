<template>
  <div>
    <div class="flex items-center justify-between mb-6">
      <div class="flex items-center gap-4">
        <h3 class="font-semibold text-white">健康检查</h3>
        <span 
          class="px-3 py-1 text-sm rounded-full"
          :class="overallStatusClass"
        >
          {{ overallStatusText }}
        </span>
      </div>
      <button @click="refreshStatus" class="px-4 py-2 bg-dark-700 text-white text-sm font-medium rounded-lg hover:bg-dark-600 transition flex items-center gap-2">
        <RefreshCw :class="['w-4 h-4', { 'animate-spin': loading }]" />
        刷新状态
      </button>
    </div>
    
    <!-- 总体健康状态 -->
    <div class="bg-dark-800 rounded-xl p-6 border border-dark-600 mb-6">
      <div class="flex items-center gap-4">
        <div :class="['w-16 h-16 rounded-full flex items-center justify-center', statusClass]">
          <component :is="statusIcon" class="w-8 h-8" />
        </div>
        <div class="flex-1">
          <h3 class="text-xl font-semibold text-white">平台健康状态</h3>
          <p class="text-gray-400 mt-1">{{ healthMessage }}</p>
        </div>
        <div class="text-right">
          <div class="text-3xl font-bold" :class="uptimeClass">{{ platformUptime }}</div>
          <div class="text-sm text-gray-400">运行时长</div>
        </div>
      </div>
    </div>
    
    <!-- 健康指标 -->
    <div class="grid grid-cols-1 md:grid-cols-3 gap-4 mb-6">
      <div class="bg-dark-800 rounded-xl p-5 border border-dark-600">
        <div class="flex items-center justify-between mb-3">
          <span class="text-gray-400 text-sm">服务健康度</span>
          <Activity class="w-4 h-4 text-gray-500" />
        </div>
        <div class="flex items-end gap-2">
          <span class="text-3xl font-bold text-green-400">{{ serviceHealthScore }}%</span>
        </div>
        <div class="mt-3 h-2 bg-dark-700 rounded-full overflow-hidden">
          <div 
            class="h-full bg-green-500 rounded-full"
            :style="{ width: serviceHealthScore + '%' }"
          ></div>
        </div>
      </div>
      
      <div class="bg-dark-800 rounded-xl p-5 border border-dark-600">
        <div class="flex items-center justify-between mb-3">
          <span class="text-gray-400 text-sm">资源健康度</span>
          <HardDrive class="w-4 h-4 text-gray-500" />
        </div>
        <div class="flex items-end gap-2">
          <span class="text-3xl font-bold" :class="resourceHealthScore >= 70 ? 'text-green-400' : 'text-yellow-400'">{{ resourceHealthScore }}%</span>
        </div>
        <div class="mt-3 h-2 bg-dark-700 rounded-full overflow-hidden">
          <div 
            class="h-full rounded-full"
            :class="resourceHealthScore >= 70 ? 'bg-green-500' : 'bg-yellow-500'"
            :style="{ width: resourceHealthScore + '%' }"
          ></div>
        </div>
      </div>
      
      <div class="bg-dark-800 rounded-xl p-5 border border-dark-600">
        <div class="flex items-center justify-between mb-3">
          <span class="text-gray-400 text-sm">告警健康度</span>
          <Bell class="w-4 h-4 text-gray-500" />
        </div>
        <div class="flex items-end gap-2">
          <span class="text-3xl font-bold" :class="alertHealthScore >= 60 ? 'text-green-400' : 'text-red-400'">{{ alertHealthScore }}%</span>
        </div>
        <div class="mt-3 h-2 bg-dark-700 rounded-full overflow-hidden">
          <div 
            class="h-full rounded-full"
            :class="alertHealthScore >= 60 ? 'bg-green-500' : 'bg-red-500'"
            :style="{ width: alertHealthScore + '%' }"
          ></div>
        </div>
      </div>
    </div>
    
    <!-- 服务组件状态 -->
    <div class="bg-dark-800 rounded-xl border border-dark-600 mb-6">
      <div class="px-4 py-3 border-b border-dark-600">
        <h3 class="font-medium text-white">服务组件状态</h3>
      </div>
      <div class="p-4 grid grid-cols-2 md:grid-cols-3 lg:grid-cols-5 gap-4">
        <div v-for="service in services" :key="service.name" class="bg-dark-700 rounded-lg p-4">
          <div class="flex items-center justify-between mb-2">
            <span class="text-sm font-medium text-white">{{ service.name }}</span>
            <div :class="['w-3 h-3 rounded-full', getServiceStatusDot(service.status)]"></div>
          </div>
          <div class="text-xs text-gray-400 mb-1">{{ service.count }} 个实例</div>
          <div class="flex items-center gap-2 text-xs">
            <span 
              class="px-2 py-0.5 rounded-full"
              :class="getServiceStatusClass(service.status)"
            >
              {{ service.status }}
            </span>
          </div>
        </div>
      </div>
    </div>
    
    <!-- 告警规则状态 -->
    <div class="bg-dark-800 rounded-xl border border-dark-600 mb-6">
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
            <div class="text-3xl font-bold text-green-400">{{ alertRules.active }}</div>
            <div class="text-sm text-gray-400">正常</div>
          </div>
          <div class="text-center">
            <div class="text-3xl font-bold text-red-400">{{ alertRules.firing }}</div>
            <div class="text-sm text-gray-400">触发中</div>
          </div>
          <div class="text-center">
            <div class="text-3xl font-bold text-yellow-400">{{ alertRules.muted }}</div>
            <div class="text-sm text-gray-400">已静默</div>
          </div>
          <div class="text-center">
            <div class="text-3xl font-bold text-gray-400">{{ alertRules.disabled }}</div>
            <div class="text-sm text-gray-400">已禁用</div>
          </div>
        </div>
      </div>
    </div>
    
    <!-- 健康检查历史 -->
    <div class="bg-dark-800 rounded-xl border border-dark-600 overflow-hidden">
      <div class="px-4 py-3 border-b border-dark-600 flex items-center justify-between">
        <h3 class="font-medium text-white">健康检查历史</h3>
      </div>
      <div class="max-h-80 overflow-y-auto">
        <div 
          v-for="(check, index) in healthHistory" 
          :key="index"
          class="px-4 py-3 border-b border-dark-700 hover:bg-dark-700/50 transition"
        >
          <div class="flex items-center justify-between">
            <div class="flex items-center gap-3">
              <component 
                :is="check.status === 'healthy' ? CheckCircle : AlertCircle" 
                :class="['w-5 h-5', check.status === 'healthy' ? 'text-green-400' : 'text-red-400']"
              />
              <div>
                <div class="text-sm text-white">{{ check.message }}</div>
                <div class="text-xs text-gray-500">{{ check.time }}</div>
              </div>
            </div>
            <span 
              class="px-2 py-1 text-xs rounded-full"
              :class="check.status === 'healthy' ? 'bg-green-500/20 text-green-400' : 'bg-red-500/20 text-red-400'"
            >
              {{ check.status === 'healthy' ? '正常' : '异常' }}
            </span>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed } from 'vue'
import { RefreshCw, CheckCircle, AlertCircle, XCircle, Activity, HardDrive, Bell } from 'lucide-vue-next'

const loading = ref(false)
const platformUptime = ref('15天 6小时')

const services = ref([
  { name: 'cloud-flow-agent', status: 'healthy', count: 12 },
  { name: 'cloud-flow-center', status: 'healthy', count: 3 },
  { name: 'cloud-flow-edge', status: 'warning', count: 6 },
  { name: 'alert-engine', status: 'healthy', count: 2 },
  { name: 'auth-service', status: 'healthy', count: 2 }
])

const alertRules = ref({
  total: 24,
  active: 18,
  firing: 2,
  muted: 3,
  disabled: 1
})

const healthHistory = ref([
  { status: 'healthy', message: '所有服务健康检查通过', time: '2024-01-15 14:30:00' },
  { status: 'healthy', message: 'cloud-flow-edge 连接恢复正常', time: '2024-01-15 14:25:00' },
  { status: 'warning', message: 'cloud-flow-edge-beijing 连接超时', time: '2024-01-15 14:20:00' },
  { status: 'healthy', message: 'CPU 使用率恢复正常', time: '2024-01-15 14:15:00' },
  { status: 'warning', message: 'CPU 使用率超过阈值 (85%)', time: '2024-01-15 14:10:00' },
  { status: 'healthy', message: '内存使用率恢复正常', time: '2024-01-15 14:05:00' },
  { status: 'warning', message: '内存使用率超过阈值 (90%)', time: '2024-01-15 14:00:00' },
  { status: 'healthy', message: '磁盘空间检查通过', time: '2024-01-15 13:55:00' }
])

const serviceHealthScore = computed(() => {
  const healthyCount = services.value.filter(s => s.status === 'healthy').length
  return Math.round((healthyCount / services.value.length) * 100)
})

const resourceHealthScore = computed(() => 75)

const alertHealthScore = computed(() => {
  if (alertRules.value.firing === 0) return 100
  if (alertRules.value.firing <= 2) return 85
  return 60
})

const overallStatus = computed(() => {
  if (alertRules.value.firing > 3) return 'critical'
  if (alertRules.value.firing > 0) return 'warning'
  return 'healthy'
})

const overallStatusText = computed(() => {
  if (overallStatus.value === 'critical') return '严重'
  if (overallStatus.value === 'warning') return '警告'
  return '健康'
})

const overallStatusClass = computed(() => {
  if (overallStatus.value === 'critical') return 'bg-red-500/20 text-red-400'
  if (overallStatus.value === 'warning') return 'bg-yellow-500/20 text-yellow-400'
  return 'bg-green-500/20 text-green-400'
})

const healthMessage = computed(() => {
  if (overallStatus.value === 'critical') return '检测到多个严重问题，请立即处理'
  if (overallStatus.value === 'warning') return '部分服务存在异常，建议关注'
  return '所有核心服务运行正常'
})

const statusClass = computed(() => {
  if (overallStatus.value === 'critical') return 'bg-red-500/20 text-red-400'
  if (overallStatus.value === 'warning') return 'bg-yellow-500/20 text-yellow-400'
  return 'bg-green-500/20 text-green-400'
})

const statusIcon = computed(() => {
  if (overallStatus.value === 'critical') return XCircle
  if (overallStatus.value === 'warning') return AlertCircle
  return CheckCircle
})

const uptimeClass = computed(() => {
  return 'text-green-400'
})

function getServiceStatusDot(status) {
  if (status === 'healthy') return 'bg-green-500'
  if (status === 'warning') return 'bg-yellow-500'
  return 'bg-red-500'
}

function getServiceStatusClass(status) {
  if (status === 'healthy') return 'bg-green-500/20 text-green-400'
  if (status === 'warning') return 'bg-yellow-500/20 text-yellow-400'
  return 'bg-red-500/20 text-red-400'
}

function refreshStatus() {
  loading.value = true
  setTimeout(() => {
    loading.value = false
  }, 500)
}
</script>
