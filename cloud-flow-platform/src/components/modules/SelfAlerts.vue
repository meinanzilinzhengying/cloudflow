<template>
  <div>
    <div class="flex items-center justify-between mb-6">
      <h3 class="font-semibold text-white">平台告警</h3>
      <button @click="fetchAlerts" :disabled="loading" class="px-4 py-2 bg-dark-700 text-white text-sm font-medium rounded-lg hover:bg-dark-600 transition flex items-center gap-2 disabled:opacity-50">
        <RefreshCw class="w-4 h-4" :class="{ 'animate-spin': loading }" />
        刷新
      </button>
    </div>
    
    <div class="grid grid-cols-1 md:grid-cols-4 gap-4 mb-6">
      <div class="bg-dark-800 rounded-xl p-4 border border-dark-600">
        <div class="text-sm text-gray-400 mb-1">全部告警</div>
        <div class="text-2xl font-bold text-white">{{ alerts.length }}</div>
      </div>
      <div class="bg-dark-800 rounded-xl p-4 border border-dark-600">
        <div class="text-sm text-gray-400 mb-1">触发中</div>
        <div class="text-2xl font-bold text-red-400">{{ firingCount }}</div>
      </div>
      <div class="bg-dark-800 rounded-xl p-4 border border-dark-600">
        <div class="text-sm text-gray-400 mb-1">已解决</div>
        <div class="text-2xl font-bold text-green-400">{{ resolvedCount }}</div>
      </div>
      <div class="bg-dark-800 rounded-xl p-4 border border-dark-600">
        <div class="text-sm text-gray-400 mb-1">严重告警</div>
        <div class="text-2xl font-bold text-red-400">{{ criticalCount }}</div>
      </div>
    </div>
    
    <div class="bg-dark-800 rounded-xl border border-dark-600 overflow-hidden">
      <div class="px-4 py-3 border-b border-dark-600 flex items-center justify-between flex-wrap gap-2">
        <h3 class="font-medium text-white">告警列表</h3>
        <div class="flex gap-2 flex-wrap">
          <button 
            v-for="status in statusFilters" 
            :key="status.value"
            @click="activeStatus = status.value"
            :class="[
              'px-3 py-1 rounded text-xs font-medium transition',
              activeStatus === status.value 
                ? 'bg-primary-500 text-white' 
                : 'bg-dark-700 text-gray-400 hover:text-white'
            ]"
          >
            {{ status.label }}
          </button>
          <button 
            v-for="lv in levelFilters" 
            :key="lv.value"
            @click="activeLevel = lv.value"
            :class="[
              'px-3 py-1 rounded text-xs font-medium transition',
              activeLevel === lv.value 
                ? getLevelBtnClass(lv.value)
                : 'bg-dark-700 text-gray-400 hover:text-white'
            ]"
          >
            {{ lv.label }}
          </button>
        </div>
      </div>
      <div v-if="loading" class="p-8 text-center text-gray-500">
        <RefreshCw class="w-6 h-6 animate-spin mx-auto mb-2" />
        加载中...
      </div>
      <div v-else-if="error" class="p-8 text-center text-red-400">
        {{ error }}
      </div>
      <table v-else class="w-full">
        <thead>
          <tr class="border-b border-dark-600">
            <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">告警级别</th>
            <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">告警标题</th>
            <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">来源</th>
            <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">当前值</th>
            <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">阈值</th>
            <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">发生时间</th>
            <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">状态</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="alert in filteredAlerts" :key="alert.id" class="border-b border-dark-700 hover:bg-dark-700/50 transition">
            <td class="px-4 py-3">
              <span 
                class="px-2 py-1 text-xs rounded-full font-medium"
                :class="getLevelClass(alert.level)"
              >
                {{ getLevelText(alert.level) }}
              </span>
            </td>
            <td class="px-4 py-3 text-sm text-white">{{ alert.title }}</td>
            <td class="px-4 py-3 text-sm text-gray-400">{{ alert.source }}</td>
            <td class="px-4 py-3 text-sm text-yellow-400 font-mono">{{ alert.value || '-' }}</td>
            <td class="px-4 py-3 text-sm text-gray-400 font-mono">{{ alert.threshold || '-' }}</td>
            <td class="px-4 py-3 text-sm text-gray-400">{{ alert.time }}</td>
            <td class="px-4 py-3">
              <span 
                class="px-2 py-1 text-xs rounded-full"
                :class="alert.status === 'firing' ? 'bg-red-500/20 text-red-400' : 'bg-green-500/20 text-green-400'"
              >
                {{ alert.status === 'firing' ? '触发中' : '已解决' }}
              </span>
            </td>
          </tr>
          <tr v-if="filteredAlerts.length === 0">
            <td colspan="7" class="px-4 py-8 text-center text-gray-500">
              当前筛选条件下暂无告警
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { RefreshCw } from 'lucide-vue-next'
import api from '../../api'

const statusFilters = [
  { label: '全部', value: 'all' },
  { label: '触发中', value: 'firing' },
  { label: '已解决', value: 'resolved' }
]

const levelFilters = [
  { label: '全部级别', value: 'all' },
  { label: '严重', value: 'critical' },
  { label: '警告', value: 'warning' },
  { label: '信息', value: 'info' }
]

const activeStatus = ref('all')
const activeLevel = ref('all')
const loading = ref(false)
const error = ref('')

const alerts = ref([])

const firingCount = computed(() => alerts.value.filter(a => a.status === 'firing').length)
const resolvedCount = computed(() => alerts.value.filter(a => a.status === 'resolved').length)
const criticalCount = computed(() => alerts.value.filter(a => a.level === 'critical').length)

const filteredAlerts = computed(() => {
  let result = alerts.value
  if (activeStatus.value !== 'all') {
    result = result.filter(a => a.status === activeStatus.value)
  }
  if (activeLevel.value !== 'all') {
    result = result.filter(a => a.level === activeLevel.value)
  }
  return result
})

function getLevelClass(level) {
  if (level === 'critical') return 'bg-red-500/20 text-red-400'
  if (level === 'warning') return 'bg-yellow-500/20 text-yellow-400'
  return 'bg-blue-500/20 text-blue-400'
}

function getLevelBtnClass(level) {
  if (level === 'critical') return 'bg-red-500 text-white'
  if (level === 'warning') return 'bg-yellow-500 text-white'
  return 'bg-blue-500 text-white'
}

function getLevelText(level) {
  if (level === 'critical') return '严重'
  if (level === 'warning') return '警告'
  return '信息'
}

async function fetchAlerts() {
  loading.value = true
  error.value = ''
  try {
    const data = await api.getAlerts()
    if (data && Array.isArray(data)) {
      alerts.value = data
    } else {
      alerts.value = []
      if (data === null) {
        error.value = '无法连接到后端，请检查 control-plane 是否正常运行'
      }
    }
  } catch (e) {
    error.value = '加载告警失败: ' + (e.message || '未知错误')
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  fetchAlerts()
})
</script>
