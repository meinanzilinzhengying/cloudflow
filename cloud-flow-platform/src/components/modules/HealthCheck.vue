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
          <div class="text-3xl font-bold" :class="uptimeClass">{{ platformUptime || 'N/A' }}</div>
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
          <span class="text-3xl font-bold" :class="serviceHealthScore >= 80 ? 'text-green-400' : serviceHealthScore >= 50 ? 'text-yellow-400' : 'text-red-400'">{{ serviceHealthScore }}%</span>
        </div>
        <div class="mt-3 h-2 bg-dark-700 rounded-full overflow-hidden">
          <div 
            class="h-full rounded-full transition-all duration-500"
            :class="serviceHealthScore >= 80 ? 'bg-green-500' : serviceHealthScore >= 50 ? 'bg-yellow-500' : 'bg-red-500'"
            :style="{ width: serviceHealthScore + '%' }"
          ></div>
        </div>
        <div class="mt-2 text-xs text-gray-500">{{ services.filter(s => s.status === 'healthy').length }}/{{ services.length }} 个服务正常</div>
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
            class="h-full rounded-full transition-all duration-500"
            :class="resourceHealthScore >= 70 ? 'bg-green-500' : 'bg-yellow-500'"
            :style="{ width: resourceHealthScore + '%' }"
          ></div>
        </div>
        <div class="mt-2 text-xs text-gray-500">CPU {{ sysMetrics.cpu }}% / 内存 {{ sysMetrics.memory }}%</div>
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
            class="h-full rounded-full transition-all duration-500"
            :class="alertHealthScore >= 60 ? 'bg-green-500' : 'bg-red-500'"
            :style="{ width: alertHealthScore + '%' }"
          ></div>
        </div>
        <div class="mt-2 text-xs text-gray-500">{{ alertRules.firing > 0 ? alertRules.firing + ' 个告警触发中' : '无触发中的告警' }}</div>
      </div>
    </div>
    
    <!-- 服务组件状态 -->
    <div class="bg-dark-800 rounded-xl border border-dark-600 mb-6">
      <div class="px-4 py-3 border-b border-dark-600">
        <h3 class="font-medium text-white">服务组件状态</h3>
      </div>
      <div v-if="services.length === 0" class="p-8 text-center text-gray-500">
        暂无服务状态数据
      </div>
      <div v-else class="p-4 grid grid-cols-2 md:grid-cols-3 lg:grid-cols-5 gap-4">
        <div v-for="service in services" :key="service.name" class="bg-dark-700 rounded-lg p-4">
          <div class="flex items-center justify-between mb-2">
            <span class="text-sm font-medium text-white truncate" :title="service.name">{{ service.name }}</span>
            <div :class="['w-3 h-3 rounded-full flex-shrink-0', getServiceStatusDot(service.status)]"></div>
          </div>
          <div class="text-xs text-gray-400 mb-1">CPU {{ service.cpu?.toFixed?.(1) ?? service.cpu ?? 0 }}%</div>
          <div class="flex items-center gap-2 text-xs">
            <span 
              class="px-2 py-0.5 rounded-full"
              :class="getServiceStatusClass(service.status)"
            >
              {{ service.status === 'healthy' ? '健康' : service.status === 'unhealthy' ? '异常' : service.status }}
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
        <div class="flex items-center gap-8 flex-wrap">
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
      <div v-if="healthHistory.length === 0" class="p-8 text-center text-gray-500">
        暂无健康检查记录
      </div>
      <div v-else class="max-h-80 overflow-y-auto">
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
import { ref, computed, onMounted } from 'vue'
import { RefreshCw, CheckCircle, AlertCircle, XCircle, Activity, HardDrive, Bell } from 'lucide-vue-next'
import api from '../../api'

const loading = ref(false)
const platformUptime = ref('')
const services = ref([])
const sysMetrics = ref({ cpu: 0, memory: 0, disk: 0 })
const alertRules = ref({
  total: 0,
  active: 0,
  firing: 0,
  muted: 0,
  disabled: 0
})
const healthHistory = ref([])

// --- 计算属性 ---

const serviceHealthScore = computed(() => {
  if (services.value.length === 0) return 0
  const healthyCount = services.value.filter(s => s.status === 'healthy').length
  return Math.round((healthyCount / services.value.length) * 100)
})

const resourceHealthScore = computed(() => {
  const cpu = sysMetrics.value.cpu ?? 0
  const mem = sysMetrics.value.memory ?? 0
  // CPU 和内存使用率越低越好，低于 80% 认为资源健康
  const cpuScore = cpu > 90 ? 30 : cpu > 80 ? 60 : cpu > 50 ? 80 : 100
  const memScore = mem > 90 ? 30 : mem > 80 ? 60 : mem > 50 ? 80 : 100
  return Math.round((cpuScore + memScore) / 2)
})

const alertHealthScore = computed(() => {
  if (alertRules.value.firing === 0) return 100
  if (alertRules.value.firing <= 2) return 85
  if (alertRules.value.firing <= 5) return 60
  return 30
})

const overallStatus = computed(() => {
  const unhealthyCount = services.value.filter(s => s.status !== 'healthy').length
  if (unhealthyCount > 3 || alertRules.value.firing > 3) return 'critical'
  if (unhealthyCount > 0 || alertRules.value.firing > 0) return 'warning'
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
  const unhealthyCount = services.value.filter(s => s.status !== 'healthy').length
  if (overallStatus.value === 'critical') return `检测到 ${unhealthyCount} 个服务异常，请立即处理`
  if (overallStatus.value === 'warning') return `部分服务存在异常（${unhealthyCount} 个），建议关注`
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

// --- 方法 ---

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

function formatUptime(seconds) {
  if (!seconds || seconds <= 0) return 'N/A'
  const days = Math.floor(seconds / 86400)
  const hours = Math.floor((seconds % 86400) / 3600)
  const minutes = Math.floor((seconds % 3600) / 60)
  if (days > 0) return `${days}天${hours.toString().padStart(2, '0')}:${minutes.toString().padStart(2, '0')}`
  return `${hours.toString().padStart(2, '0')}:${minutes.toString().padStart(2, '0')}`
}

async function fetchHealthData() {
  loading.value = true
  try {
    // 并行获取所有需要的数据
    const [healthRes, metricsRes, alertsRes] = await Promise.allSettled([
      api.getHealthStatus(),
      api.getSystemMetrics(),
      api.getAlerts()
    ])

    // 1. 服务健康状态
    if (healthRes.status === 'fulfilled' && healthRes.value) {
      const data = healthRes.value
      if (data.services && Array.isArray(data.services)) {
        services.value = data.services.map(s => ({
          name: s.type || s.name || '未知服务',
          status: s.status || 'unknown',
          cpu: s.cpu ?? 0,
          memory: s.memory ?? 0,
          restarts: s.restarts ?? 0
        }))
      }
    }

    // 2. 系统资源指标
    if (metricsRes.status === 'fulfilled' && metricsRes.value) {
      const data = metricsRes.value
      sysMetrics.value = {
        cpu: data.cpu?.usage ?? 0,
        memory: data.memory?.usage ?? 0,
        disk: data.disk?.usage ?? 0
      }
      // 运行时长
      if (data.host?.uptime) {
        platformUptime.value = formatUptime(data.host.uptime)
      }
    }

    // 3. 告警规则
    if (alertsRes.status === 'fulfilled' && alertsRes.value) {
      const data = alertsRes.value
      if (Array.isArray(data)) {
        alertRules.value = {
          total: data.length,
          active: data.filter(a => a.status === 'active' || a.state === 'active').length,
          firing: data.filter(a => a.status === 'firing' || a.state === 'firing').length,
          muted: data.filter(a => a.status === 'muted' || a.state === 'muted').length,
          disabled: data.filter(a => a.status === 'disabled' || a.state === 'disabled').length
        }
      } else {
        alertRules.value = { total: 0, active: 0, firing: 0, muted: 0, disabled: 0 }
      }
    }

    // 4. 生成健康检查历史记录
    generateHealthHistory()

  } catch (error) {
    console.error('Health check fetch error:', error)
  } finally {
    loading.value = false
  }
}

function generateHealthHistory() {
  const history = []
  const now = new Date()

  // 添加当前检查记录
  const unhealthyServices = services.value.filter(s => s.status !== 'healthy')
  if (unhealthyServices.length > 0) {
    history.push({
      status: 'unhealthy',
      message: `检测到 ${unhealthyServices.length} 个服务异常: ${unhealthyServices.map(s => s.name).join(', ')}`,
      time: now.toLocaleString('zh-CN')
    })
  } else {
    history.push({
      status: 'healthy',
      message: '所有核心服务运行正常',
      time: now.toLocaleString('zh-CN')
    })
  }

  // 添加资源告警记录
  if (sysMetrics.value.cpu > 80) {
    history.push({
      status: 'unhealthy',
      message: `CPU 使用率过高: ${sysMetrics.value.cpu}%`,
      time: now.toLocaleString('zh-CN')
    })
  }
  if (sysMetrics.value.memory > 80) {
    history.push({
      status: 'unhealthy',
      message: `内存使用率过高: ${sysMetrics.value.memory}%`,
      time: now.toLocaleString('zh-CN')
    })
  }

  // 添加告警触发记录
  if (alertRules.value.firing > 0) {
    history.push({
      status: 'unhealthy',
      message: `${alertRules.value.firing} 个告警规则触发中`,
      time: now.toLocaleString('zh-CN')
    })
  }

  healthHistory.value = history
}

function refreshStatus() {
  fetchHealthData()
}

onMounted(() => {
  fetchHealthData()
})
</script>
