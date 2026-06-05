<template>
  <div>
    <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4 mb-6">
      <div class="bg-dark-800 rounded-xl p-5 border border-dark-600">
        <div class="flex items-center justify-between mb-3">
          <span class="text-gray-400 text-sm">平均延迟</span>
          <Clock class="w-4 h-4 text-gray-500" />
        </div>
        <p class="text-2xl font-bold text-white">{{ latencyStats.avg }}<span class="text-sm text-gray-400 ml-1">ms</span></p>
        <div class="mt-3 flex items-center gap-4 text-xs text-gray-400">
          <span>P50: {{ latencyStats.p50 }}ms</span>
          <span>P95: {{ latencyStats.p95 }}ms</span>
          <span>P99: {{ latencyStats.p99 }}ms</span>
        </div>
      </div>
      
      <div class="bg-dark-800 rounded-xl p-5 border border-dark-600">
        <div class="flex items-center justify-between mb-3">
          <span class="text-gray-400 text-sm">建连时延</span>
          <Zap class="w-4 h-4 text-gray-500" />
        </div>
        <p class="text-2xl font-bold text-green-400">{{ connectionTime }}<span class="text-sm text-gray-400 ml-1">ms</span></p>
        <p class="text-xs text-gray-500 mt-2">平均 TCP 握手时间</p>
      </div>
      
      <div class="bg-dark-800 rounded-xl p-5 border border-dark-600">
        <div class="flex items-center justify-between mb-3">
          <span class="text-gray-400 text-sm">重传率</span>
          <RefreshCw class="w-4 h-4 text-gray-500" />
        </div>
        <p class="text-2xl font-bold" :class="retransRate > 5 ? 'text-red-400' : 'text-green-400'">{{ retransRate }}<span class="text-sm text-gray-400 ml-1">%</span></p>
        <p class="text-xs text-gray-500 mt-2">数据包重传比例</p>
      </div>
      
      <div class="bg-dark-800 rounded-xl p-5 border border-dark-600">
        <div class="flex items-center justify-between mb-3">
          <span class="text-gray-400 text-sm">丢包率</span>
          <AlertTriangle class="w-4 h-4 text-gray-500" />
        </div>
        <p class="text-2xl font-bold" :class="packetLoss > 1 ? 'text-red-400' : 'text-green-400'">{{ packetLoss }}<span class="text-sm text-gray-400 ml-1">%</span></p>
        <p class="text-xs text-gray-500 mt-2">网络数据包丢失</p>
      </div>
    </div>
    
    <div class="grid grid-cols-1 lg:grid-cols-2 gap-6">
      <TrendChart
        title="延迟趋势"
        subtitle="毫秒"
        type="line"
        :data="latencyChartData"
        :legends="latencyLegends"
      />
      <TrendChart
        title="丢包率趋势"
        type="line"
        :data="packetLossChartData"
        :legends="packetLossLegends"
      />
    </div>
    
    <div class="mt-6">
      <h3 class="font-semibold text-white mb-4">网络质量评分</h3>
      <div class="grid grid-cols-1 md:grid-cols-3 gap-6">
        <div 
          v-for="metric in networkMetrics" 
          :key="metric.name"
          class="bg-dark-800 rounded-xl p-5 border border-dark-600"
        >
          <div class="flex items-center justify-between mb-4">
            <div class="flex items-center gap-3">
              <component :is="metric.icon" class="w-5 h-5" :class="metric.iconColor" />
              <span class="text-white font-medium">{{ metric.name }}</span>
            </div>
            <span 
              class="text-lg font-bold"
              :class="getScoreClass(metric.score)"
            >
              {{ metric.score }}
            </span>
          </div>
          <div class="h-2 bg-dark-700 rounded-full overflow-hidden">
            <div 
              class="h-full rounded-full transition-all duration-500"
              :class="getScoreBarClass(metric.score)"
              :style="{ width: metric.score + '%' }"
            ></div>
          </div>
          <p class="text-xs text-gray-400 mt-2">{{ metric.description }}</p>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { Clock, Zap, RefreshCw, AlertTriangle, Wifi, Cpu, HardDrive } from 'lucide-vue-next'
import TrendChart from '../common/TrendChart.vue'
import { networkApi } from '../../api'

const latencyStats = ref({ avg: 15.2, p50: 12, p95: 28, p99: 45 })
const connectionTime = ref(42)
const retransRate = ref(3.2)
const packetLoss = ref(0.3)

const latencyChartData = computed(() => ({
  labels: ['00:00', '02:00', '04:00', '06:00', '08:00', '10:00', '12:00'],
  datasets: [
    {
      label: '平均延迟',
      data: [12, 15, 18, 14, 20, 16, 15],
      borderColor: '#3b82f6',
      backgroundColor: 'rgba(59, 130, 246, 0.1)',
      fill: true,
      tension: 0.4
    },
    {
      label: 'P99延迟',
      data: [35, 42, 48, 38, 55, 45, 42],
      borderColor: '#f59e0b',
      backgroundColor: 'rgba(245, 158, 11, 0.1)',
      fill: true,
      tension: 0.4
    }
  ]
}))

const latencyLegends = [
  { label: '平均延迟', color: '#3b82f6' },
  { label: 'P99延迟', color: '#f59e0b' }
]

const packetLossChartData = computed(() => ({
  labels: ['00:00', '02:00', '04:00', '06:00', '08:00', '10:00', '12:00'],
  datasets: [{
    label: '丢包率',
    data: [0.2, 0.5, 0.3, 0.8, 0.4, 0.6, 0.3],
    borderColor: '#ef4444',
    backgroundColor: 'rgba(239, 68, 68, 0.1)',
    fill: true,
    tension: 0.4
  }]
}))

const packetLossLegends = [
  { label: '丢包率', color: '#ef4444' }
]

const networkMetrics = ref([
  { name: '网络稳定性', icon: Wifi, iconColor: 'text-blue-400', score: 92, description: '整体网络连接稳定性' },
  { name: '系统响应', icon: Cpu, iconColor: 'text-green-400', score: 88, description: '服务器处理能力' },
  { name: '存储性能', icon: HardDrive, iconColor: 'text-purple-400', score: 95, description: '数据读写速度' }
])

const getScoreClass = (score) => {
  if (score >= 90) return 'text-green-400'
  if (score >= 70) return 'text-yellow-400'
  return 'text-red-400'
}

const getScoreBarClass = (score) => {
  if (score >= 90) return 'bg-green-500'
  if (score >= 70) return 'bg-yellow-500'
  return 'bg-red-500'
}

onMounted(async () => {
  try {
    const data = await networkApi.getAnalysis()
    if (data) {
      latencyStats.value = data.latency || latencyStats.value
      connectionTime.value = data.connectionTime || connectionTime.value
      retransRate.value = data.retransRate || retransRate.value
      packetLoss.value = data.packetLoss || packetLoss.value
    }
  } catch (error) {
    console.error('Failed to fetch network analysis:', error)
  }
})
</script>
