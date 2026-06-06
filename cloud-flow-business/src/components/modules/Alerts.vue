<template>
  <div>
    <div class="flex items-center justify-between mb-6">
      <div class="flex items-center gap-4">
        <div class="flex items-center gap-2 bg-dark-800 px-4 py-2 rounded-lg border border-dark-600">
          <select 
            v-model="selectedSeverity" 
            class="bg-dark-700 border-none text-white text-sm px-3 py-1 rounded-md focus:outline-none"
          >
            <option value="all">全部级别</option>
            <option value="critical">严重</option>
            <option value="warning">警告</option>
            <option value="info">信息</option>
          </select>
        </div>
        <div class="flex items-center gap-2 bg-dark-800 px-4 py-2 rounded-lg border border-dark-600">
          <select 
            v-model="selectedStatus" 
            class="bg-dark-700 border-none text-white text-sm px-3 py-1 rounded-md focus:outline-none"
          >
            <option value="all">全部状态</option>
            <option value="active">活跃</option>
            <option value="resolved">已解决</option>
          </select>
        </div>
      </div>
      <button class="px-4 py-2 bg-primary-500 text-white text-sm font-medium rounded-lg hover:bg-primary-600 transition">
        清除全部告警
      </button>
    </div>
    
    <div class="grid grid-cols-1 md:grid-cols-3 gap-4 mb-6">
      <div class="bg-dark-800 rounded-xl p-4 border border-dark-600">
        <div class="flex items-center justify-between">
          <div>
            <p class="text-gray-400 text-sm">总告警数</p>
            <p class="text-2xl font-bold text-white">{{ alertStats.total }}</p>
          </div>
          <div class="w-10 h-10 bg-gray-500/20 rounded-lg flex items-center justify-center">
            <AlertCircle class="w-5 h-5 text-gray-400" />
          </div>
        </div>
      </div>
      
      <div class="bg-dark-800 rounded-xl p-4 border border-red-500/30">
        <div class="flex items-center justify-between">
          <div>
            <p class="text-gray-400 text-sm">严重告警</p>
            <p class="text-2xl font-bold text-red-400">{{ alertStats.critical }}</p>
          </div>
          <div class="w-10 h-10 bg-red-500/20 rounded-lg flex items-center justify-center">
            <AlertTriangle class="w-5 h-5 text-red-400" />
          </div>
        </div>
      </div>
      
      <div class="bg-dark-800 rounded-xl p-4 border border-yellow-500/30">
        <div class="flex items-center justify-between">
          <div>
            <p class="text-gray-400 text-sm">警告</p>
            <p class="text-2xl font-bold text-yellow-400">{{ alertStats.warning }}</p>
          </div>
          <div class="w-10 h-10 bg-yellow-500/20 rounded-lg flex items-center justify-center">
            <AlertCircle class="w-5 h-5 text-yellow-400" />
          </div>
        </div>
      </div>
    </div>
    
    <div class="bg-dark-800 rounded-xl border border-dark-600 overflow-hidden">
      <div class="max-h-[500px] overflow-y-auto">
        <div 
          v-for="alert in filteredAlerts" 
          :key="alert.id"
          class="p-4 border-b border-dark-700 hover:bg-dark-700/50 transition"
        >
          <div class="flex items-start justify-between">
            <div class="flex items-start gap-3">
              <div 
                class="w-3 h-3 rounded-full mt-1.5"
                :class="getSeverityClass(alert.severity)"
              ></div>
              <div>
                <div class="flex items-center gap-2">
                  <h4 class="text-white font-medium">{{ alert.title }}</h4>
                  <span 
                    class="px-2 py-0.5 text-xs rounded-full"
                    :class="getStatusClass(alert.status)"
                  >
                    {{ alert.status === 'active' ? '活跃' : '已解决' }}
                  </span>
                </div>
                <p class="text-gray-400 text-sm mt-1">{{ alert.message }}</p>
                <div class="flex items-center gap-4 mt-2 text-xs text-gray-500">
                  <span>{{ alert.time }}</span>
                  <span>{{ alert.source }}</span>
                  <span>{{ alert.resource }}</span>
                </div>
              </div>
            </div>
            <div class="flex items-center gap-2">
              <button 
                v-if="alert.status === 'active'"
                @click="handleResolve(alert.id)"
                class="px-3 py-1.5 bg-green-500/20 text-green-400 text-xs font-medium rounded-md hover:bg-green-500/30 transition"
              >
                解决
              </button>
              <button class="px-3 py-1.5 bg-dark-700 text-gray-400 text-xs font-medium rounded-md hover:bg-dark-600 transition">
                详情
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { AlertCircle, AlertTriangle } from 'lucide-vue-next'
import { alertsApi } from '../../api'

const selectedSeverity = ref('all')
const selectedStatus = ref('all')

const alertStats = ref({ total: 15, critical: 3, warning: 8, info: 4 })

const alerts = ref([
  { id: 1, severity: 'critical', title: '高丢包率告警', message: '检测到高丢包率，超过阈值 5%', time: '2分钟前', source: 'node-1', resource: 'network', status: 'active' },
  { id: 2, severity: 'critical', title: '连接数超限', message: 'TCP 连接数超过安全阈值', time: '5分钟前', source: 'node-2', resource: 'tcp', status: 'active' },
  { id: 3, severity: 'critical', title: '服务不可用', message: 'API 服务响应超时', time: '8分钟前', source: 'api-server', resource: 'service', status: 'active' },
  { id: 4, severity: 'warning', title: '重传率偏高', message: '重传率达到 8%，建议检查网络', time: '12分钟前', source: 'node-1', resource: 'network', status: 'active' },
  { id: 5, severity: 'warning', title: '内存使用率高', message: '内存使用率达到 85%', time: '15分钟前', source: 'node-3', resource: 'system', status: 'active' },
  { id: 6, severity: 'info', title: '配置更新', message: '配置文件已更新', time: '30分钟前', source: 'config-server', resource: 'config', status: 'resolved' }
])

const filteredAlerts = computed(() => {
  return alerts.value.filter(alert => {
    const severityMatch = selectedSeverity.value === 'all' || alert.severity === selectedSeverity.value
    const statusMatch = selectedStatus.value === 'all' || alert.status === selectedStatus.value
    return severityMatch && statusMatch
  })
})

const getSeverityClass = (severity) => {
  const classes = {
    critical: 'bg-red-500',
    warning: 'bg-yellow-500',
    info: 'bg-blue-500'
  }
  return classes[severity] || 'bg-gray-500'
}

const getStatusClass = (status) => {
  return status === 'active' 
    ? 'bg-red-500/20 text-red-400' 
    : 'bg-green-500/20 text-green-400'
}

const handleResolve = (id) => {
  const alert = alerts.value.find(a => a.id === id)
  if (alert) {
    alert.status = 'resolved'
    alertStats.value.critical = alerts.value.filter(a => a.severity === 'critical' && a.status === 'active').length
    alertStats.value.warning = alerts.value.filter(a => a.severity === 'warning' && a.status === 'active').length
    alertStats.value.total = alerts.value.filter(a => a.status === 'active').length
  }
}

onMounted(async () => {
  try {
    const data = await alertsApi.getList()
    if (data && data.alerts) {
      alerts.value = data.alerts
      alertStats.value = {
        total: data.total || alertStats.value.total,
        critical: data.critical || alertStats.value.critical,
        warning: data.warning || alertStats.value.warning,
        info: data.info || alertStats.value.info
      }
    }
  } catch (error) {
    console.error('Failed to fetch alerts:', error)
  }
})
</script>
