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
        title="网络 I/O"
        :value="stats.network"
        unit="MB/s"
        :change="+12"
        :icon="Network"
        variant="info"
        :loading="loading"
      />
    </div>
    
    <div class="grid grid-cols-1 lg:grid-cols-2 gap-6 mb-6">
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
    
    <div class="grid grid-cols-1 lg:grid-cols-2 gap-6 mb-6">
      <TrendChart
        title="网络流量趋势"
        subtitle="入站/出站 (MB/s)"
        type="line"
        :data="networkChartData"
        :legends="networkLegends"
      />
      <TrendChart
        title="磁盘 I/O"
        subtitle="读写速度 (MB/s)"
        type="line"
        :data="diskChartData"
        :legends="diskLegends"
      />
    </div>
    
    <!-- 服务状态 -->
    <div class="bg-dark-800 rounded-xl border border-dark-600 overflow-hidden mb-6">
      <div class="px-4 py-3 border-b border-dark-600 flex items-center justify-between">
        <h3 class="font-semibold text-white">服务状态</h3>
        <div class="flex items-center gap-4 text-sm">
          <span class="flex items-center gap-2">
            <span class="w-2 h-2 rounded-full bg-green-500"></span>
            <span class="text-gray-400">{{ runningCount }} 运行中</span>
          </span>
          <span class="flex items-center gap-2">
            <span class="w-2 h-2 rounded-full bg-red-500"></span>
            <span class="text-gray-400">{{ stoppedCount }} 已停止</span>
          </span>
        </div>
      </div>
      <table class="w-full">
        <thead>
          <tr class="border-b border-dark-600">
            <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">服务</th>
            <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">类型</th>
            <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">状态</th>
            <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">CPU</th>
            <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">内存</th>
            <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">重启次数</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="service in services" :key="service.name" class="border-b border-dark-700 hover:bg-dark-700/50 transition">
            <td class="px-4 py-3">
              <div class="flex items-center gap-2">
                <span class="w-2 h-2 rounded-full" :class="getDotClass(service.status)"></span>
                <span class="text-sm text-white">{{ service.name }}</span>
              </div>
            </td>
            <td class="px-4 py-3 text-sm text-gray-300">{{ service.type }}</td>
            <td class="px-4 py-3">
              <span 
                class="px-2 py-1 text-xs rounded-full"
                :class="getStatusClass(service.status)"
              >
                {{ getStatusText(service.status) }}
              </span>
            </td>
            <td class="px-4 py-3">
              <div class="flex items-center gap-2">
                <div class="w-16 h-2 bg-dark-700 rounded-full overflow-hidden">
                  <div 
                    class="h-full rounded-full"
                    :class="getUsageClass(service.cpu)"
                    :style="{ width: service.cpu + '%' }"
                  ></div>
                </div>
                <span class="text-xs text-gray-400">{{ service.cpu }}%</span>
              </div>
            </td>
            <td class="px-4 py-3">
              <div class="flex items-center gap-2">
                <div class="w-16 h-2 bg-dark-700 rounded-full overflow-hidden">
                  <div 
                    class="h-full rounded-full"
                    :class="getUsageClass(service.memory)"
                    :style="{ width: service.memory + '%' }"
                  ></div>
                </div>
                <span class="text-xs text-gray-400">{{ service.memory }}%</span>
              </div>
            </td>
            <td class="px-4 py-3 text-sm text-gray-400">{{ service.restarts }}</td>
          </tr>
        </tbody>
      </table>
    </div>
    
    <!-- 进程监控 -->
    <div class="bg-dark-800 rounded-xl border border-dark-600 overflow-hidden">
      <div class="px-4 py-3 border-b border-dark-600">
        <h3 class="font-semibold text-white">进程监控</h3>
      </div>
      <table class="w-full">
        <thead>
          <tr class="border-b border-dark-600">
            <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">进程名</th>
            <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">PID</th>
            <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">CPU %</th>
            <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">内存 %</th>
            <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">运行时长</th>
            <th class="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase">状态</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="process in processes" :key="process.pid" class="border-b border-dark-700 hover:bg-dark-700/50 transition">
            <td class="px-4 py-3 text-sm text-white">{{ process.name }}</td>
            <td class="px-4 py-3 text-sm text-gray-400">{{ process.pid }}</td>
            <td class="px-4 py-3 text-sm" :class="getCpuClass(process.cpu)">{{ process.cpu }}%</td>
            <td class="px-4 py-3 text-sm" :class="getMemClass(process.memory)">{{ process.memory }}%</td>
            <td class="px-4 py-3 text-sm text-gray-400">{{ process.uptime }}</td>
            <td class="px-4 py-3">
              <span class="px-2 py-1 text-xs rounded-full bg-green-500/20 text-green-400">运行中</span>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { Cpu, HardDrive, Database, Network } from 'lucide-vue-next'
import StatCard from '../common/StatCard.vue'
import TrendChart from '../common/TrendChart.vue'

const loading = ref(true)
const stats = ref({
  cpu: 0,
  memory: 0,
  disk: 0,
  network: 0
})

const services = ref([])
const processes = ref([])

const runningCount = computed(() => services.value.filter(s => s.status === 'running').length)
const stoppedCount = computed(() => services.value.filter(s => s.status === 'stopped').length)

const cpuChartData = computed(() => ({
  labels: ['00:00', '04:00', '08:00', '12:00', '16:00', '20:00', '24:00'],
  datasets: [{
    label: 'CPU 使用率',
    data: [],
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
    data: [],
    borderColor: '#10b981',
    backgroundColor: 'rgba(16, 185, 129, 0.1)',
    fill: true,
    tension: 0.4
  }]
}))

const memoryLegends = [{ label: '内存使用', color: '#10b981' }]

const networkChartData = computed(() => ({
  labels: ['00:00', '04:00', '08:00', '12:00', '16:00', '20:00', '24:00'],
  datasets: [
    {
      label: '入站',
      data: [],
      borderColor: '#3b82f6',
      backgroundColor: 'rgba(59, 130, 246, 0.1)',
      fill: true,
      tension: 0.4
    },
    {
      label: '出站',
      data: [],
      borderColor: '#10b981',
      backgroundColor: 'rgba(16, 185, 129, 0.1)',
      fill: true,
      tension: 0.4
    }
  ]
}))

const networkLegends = [
  { label: '入站', color: '#3b82f6' },
  { label: '出站', color: '#10b981' }
]

const diskChartData = computed(() => ({
  labels: ['00:00', '04:00', '08:00', '12:00', '16:00', '20:00', '24:00'],
  datasets: [
    {
      label: '读取',
      data: [],
      borderColor: '#8b5cf6',
      backgroundColor: 'rgba(139, 92, 246, 0.1)',
      fill: true,
      tension: 0.4
    },
    {
      label: '写入',
      data: [],
      borderColor: '#f59e0b',
      backgroundColor: 'rgba(245, 158, 11, 0.1)',
      fill: true,
      tension: 0.4
    }
  ]
}))

const diskLegends = [
  { label: '读取', color: '#8b5cf6' },
  { label: '写入', color: '#f59e0b' }
]

function getDotClass(status) {
  return status === 'running' ? 'bg-green-500' : 'bg-red-500'
}

function getStatusClass(status) {
  const classes = {
    'running': 'bg-green-500/20 text-green-400',
    'stopped': 'bg-red-500/20 text-red-400',
    'error': 'bg-red-500/20 text-red-400'
  }
  return classes[status] || 'bg-gray-500/20 text-gray-400'
}

function getStatusText(status) {
  const texts = {
    'running': '运行中',
    'stopped': '已停止',
    'error': '异常'
  }
  return texts[status] || status
}

function getUsageClass(usage) {
  if (usage >= 80) return 'bg-red-500'
  if (usage >= 60) return 'bg-yellow-500'
  return 'bg-green-500'
}

function getCpuClass(cpu) {
  if (cpu >= 50) return 'text-red-400'
  if (cpu >= 30) return 'text-yellow-400'
  return 'text-gray-300'
}

function getMemClass(mem) {
  if (mem >= 80) return 'text-red-400'
  if (mem >= 50) return 'text-yellow-400'
  return 'text-gray-300'
}

onMounted(() => {
  loading.value = false
})
</script>
