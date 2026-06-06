<template>
  <div>
    <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4 mb-6">
      <StatCard
        title="CPU 使用率"
        :value="stats.cpu"
        unit="%"
        :change="+3.2"
        :icon="Cpu"
        variant="info"
        :loading="loading"
      />
      <StatCard
        title="内存使用"
        :value="stats.memory"
        unit="%"
        :change="-1.5"
        :icon="HardDrive"
        variant="success"
        :loading="loading"
      />
      <StatCard
        title="磁盘使用"
        :value="stats.disk"
        unit="%"
        :change="+0.8"
        :icon="Database"
        variant="warning"
        :loading="loading"
      />
      <StatCard
        title="活跃探针"
        :value="stats.probes"
        :change="+2"
        :icon="Activity"
        variant="success"
        :loading="loading"
      />
    </div>
    
    <div class="grid grid-cols-1 lg:grid-cols-2 gap-6">
      <TrendChart
        title="CPU 使用率趋势"
        subtitle="过去 24 小时"
        type="line"
        :data="cpuChartData"
        :legends="cpuLegends"
      />
      <TrendChart
        title="内存使用趋势"
        subtitle="过去 24 小时"
        type="line"
        :data="memoryChartData"
        :legends="memoryLegends"
      />
    </div>
    
    <div class="mt-6">
      <div class="flex items-center justify-between mb-4">
        <h3 class="font-semibold text-white">平台服务状态</h3>
      </div>
      <div class="bg-dark-800 rounded-xl border border-dark-600 overflow-hidden">
        <table class="w-full">
          <thead>
            <tr class="border-b border-dark-600">
              <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">服务</th>
              <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">状态</th>
              <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">CPU</th>
              <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">内存</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="service in services" :key="service.name" class="border-b border-dark-700 hover:bg-dark-700/50 transition">
              <td class="px-4 py-3 text-sm text-white">{{ service.name }}</td>
              <td class="px-4 py-3">
                <span 
                  class="px-2 py-1 text-xs rounded-full"
                  :class="getStatusClass(service.status)"
                >
                  {{ service.status }}
                </span>
              </td>
              <td class="px-4 py-3 text-sm text-gray-300">{{ service.cpu }}%</td>
              <td class="px-4 py-3 text-sm text-gray-300">{{ service.memory }}%</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, computed } from 'vue'
import { Cpu, HardDrive, Database, Activity } from 'lucide-vue-next'
import StatCard from '../common/StatCard.vue'
import TrendChart from '../common/TrendChart.vue'

const loading = ref(true)
const stats = ref({
  cpu: 45,
  memory: 62,
  disk: 78,
  probes: 12
})

const services = ref([
  { name: 'cloud-flow-center', status: 'running', cpu: 25, memory: 45 },
  { name: 'cloud-flow-edge', status: 'running', cpu: 18, memory: 32 },
  { name: 'cloud-flow-agent-01', status: 'running', cpu: 12, memory: 28 },
  { name: 'cloud-flow-agent-02', status: 'running', cpu: 15, memory: 35 },
  { name: 'alert-engine', status: 'running', cpu: 8, memory: 18 }
])

const cpuChartData = computed(() => ({
  labels: ['00:00', '04:00', '08:00', '12:00', '16:00', '20:00', '24:00'],
  datasets: [{
    label: 'CPU 使用率',
    data: [35, 42, 48, 55, 52, 47, 45],
    borderColor: '#3b82f6',
    backgroundColor: 'rgba(59, 130, 246, 0.1)',
    fill: true,
    tension: 0.4
  }]
}))

const cpuLegends = [{ label: 'CPU 使用率', color: '#3b82f6' }]

const memoryChartData = computed(() => ({
  labels: ['00:00', '04:00', '08:00', '12:00', '16:00', '20:00', '24:00'],
  datasets: [{
    label: '内存使用',
    data: [58, 60, 65, 70, 68, 64, 62],
    borderColor: '#10b981',
    backgroundColor: 'rgba(16, 185, 129, 0.1)',
    fill: true,
    tension: 0.4
  }]
}))

const memoryLegends = [{ label: '内存使用', color: '#10b981' }]

const getStatusClass = (status) => {
  const classes = {
    'running': 'bg-green-500/20 text-green-400',
    'stopped': 'bg-red-500/20 text-red-400',
    'error': 'bg-red-500/20 text-red-400'
  }
  return classes[status] || 'bg-gray-500/20 text-gray-400'
}

onMounted(() => {
  loading.value = false
})
</script>
