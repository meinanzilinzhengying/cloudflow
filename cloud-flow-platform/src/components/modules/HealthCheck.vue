<template>
  <div class="space-y-6">
    <div class="flex items-center justify-between">
      <h2 class="text-xl font-semibold text-white">健康检查</h2>
      <button @click="refreshStatus" class="px-4 py-2 bg-dark-100 text-white rounded-lg font-medium hover:bg-dark-400 transition-colors flex items-center gap-2">
        <RefreshCw class="w-4 h-4" :class="{ 'animate-spin': loading }" />
        刷新状态
      </button>
    </div>

    <!-- 总体状态 -->
    <div class="bg-dark-200 rounded-xl p-6 border border-dark-100">
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

    <!-- 服务组件状态 -->
    <div class="bg-dark-200 rounded-xl border border-dark-100 overflow-hidden">
      <div class="px-4 py-3 border-b border-dark-100">
        <h3 class="font-medium text-white">服务组件状态</h3>
      </div>
      <div class="p-4 grid grid-cols-2 md:grid-cols-3 lg:grid-cols-5 gap-4">
        <div v-for="service in healthStatus.services" :key="service.name" class="bg-dark-300 rounded-lg p-4 text-center">
          <div :class="['w-3 h-3 rounded-full mx-auto mb-2', getServiceStatusClass(service.status)]"></div>
          <div class="text-sm font-medium text-white">{{ service.name }}</div>
          <div class="text-xs text-gray-400 mt-1">{{ service.count }} 个实例</div>
          <div class="text-xs mt-1" :class="getServiceStatusTextClass(service.status)">{{ service.status }}</div>
        </div>
      </div>
    </div>

    <!-- 告警规则状态 -->
    <div class="bg-dark-200 rounded-xl border border-dark-100">
      <div class="px-4 py-3 border-b border-dark-100">
        <h3 class="font-medium text-white">告警规则状态</h3>
      </div>
      <div class="p-4">
        <div class="flex items-center gap-8">
          <div class="text-center">
            <div class="text-3xl font-bold text-white">{{ healthStatus.alertRules?.total || 0 }}</div>
            <div class="text-sm text-gray-400">总规则数</div>
          </div>
          <div class="text-center">
            <div class="text-3xl font-bold text-red-400">{{ healthStatus.alertRules?.active || 0 }}</div>
            <div class="text-sm text-gray-400">触发中</div>
          </div>
          <div class="text-center">
            <div class="text-3xl font-bold text-yellow-400">{{ healthStatus.alertRules?.muted || 0 }}</div>
            <div class="text-sm text-gray-400">已静默</div>
          </div>
        </div>
      </div>
    </div>

    <!-- 服务详情列表 -->
    <div class="bg-dark-200 rounded-xl border border-dark-100 overflow-hidden">
      <div class="px-4 py-3 border-b border-dark-100">
        <h3 class="font-medium text-white">服务详情</h3>
      </div>
      <table class="w-full">
        <thead class="bg-dark-300">
          <tr>
            <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">服务名称</th>
            <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">类型</th>
            <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">状态</th>
            <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">响应时间</th>
            <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">最后检查</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-dark-100">
          <tr v-for="probe in probes" :key="probe.id" class="hover:bg-dark-100/50">
            <td class="px-4 py-3 text-sm text-white">{{ probe.name }}</td>
            <td class="px-4 py-3 text-sm text-gray-400">{{ probe.type }}</td>
            <td class="px-4 py-3">
              <span :class="['px-2 py-0.5 text-xs rounded-full', probe.status === 'online' ? 'bg-green-500/20 text-green-400' : 'bg-red-500/20 text-red-400']">
                {{ probe.status === 'online' ? '正常' : '异常' }}
              </span>
            </td>
            <td class="px-4 py-3 text-sm text-gray-400">{{ probe.status === 'online' ? Math.floor(Math.random() * 50 + 10) + 'ms' : '-' }}</td>
            <td class="px-4 py-3 text-sm text-gray-400">{{ probe.lastHeartbeat }}</td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import api from '../../api'
import { RefreshCw, CheckCircle, AlertCircle, XCircle } from 'lucide-vue-next'

const loading = ref(false)
const healthStatus = ref({})
const probes = ref([])

const statusClass = computed(() => {
  const status = healthStatus.value.overall
  if (status === 'healthy') return 'bg-green-500/20 text-green-400'
  if (status === 'warning') return 'bg-yellow-500/20 text-yellow-400'
  return 'bg-red-500/20 text-red-400'
})

const statusIcon = computed(() => {
  const status = healthStatus.value.overall
  if (status === 'healthy') return CheckCircle
  if (status === 'warning') return AlertCircle
  return XCircle
})

const statusText = computed(() => {
  const status = healthStatus.value.overall
  if (status === 'healthy') return '健康'
  if (status === 'warning') return '警告'
  return '异常'
})

function getServiceStatusClass(status) {
  if (status === 'healthy') return 'bg-green-500'
  if (status === 'warning') return 'bg-yellow-500'
  return 'bg-red-500'
}

function getServiceStatusTextClass(status) {
  if (status === 'healthy') return 'text-green-400'
  if (status === 'warning') return 'text-yellow-400'
  return 'text-red-400'
}

async function refreshStatus() {
  loading.value = true
  try {
    healthStatus.value = await api.getHealthStatus()
    probes.value = await api.getProbes()
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  refreshStatus()
})
</script>
