<template>
  <div>
    <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4 mb-6">
      <StatCard
        title="CPU 使用率"
        :value="stats.cpu"
        unit="%"
        :change="changes.cpu"
        :icon="Cpu"
        variant="info"
        :loading="loading"
      />
      <StatCard
        title="内存使用"
        :value="stats.memory"
        unit="%"
        :change="changes.memory"
        :icon="HardDrive"
        variant="success"
        :loading="loading"
      />
      <StatCard
        title="磁盘使用"
        :value="stats.disk"
        unit="%"
        :change="changes.disk"
        :icon="Database"
        variant="warning"
        :loading="loading"
      />
      <StatCard
        title="网络流量"
        :value="stats.network"
        unit="MB"
        :change="changes.network"
        :icon="Network"
        variant="info"
        :loading="loading"
      />
    </div>
    
    <div class="grid grid-cols-1 lg:grid-cols-2 gap-6 mb-6">
      <TrendChart
        title="CPU 使用率趋势"
        subtitle="近 5 分钟（实时，10秒/点）"
        type="line"
        :data="cpuChartData"
        :legends="cpuLegends"
      />
      <TrendChart
        title="内存使用趋势"
        subtitle="近 5 分钟（实时，10秒/点）"
        type="line"
        :data="memoryChartData"
        :legends="memoryLegends"
      />
    </div>
    
    <div class="grid grid-cols-1 lg:grid-cols-2 gap-6 mb-6">
      <TrendChart
        title="网络流量趋势"
        subtitle="入站/出站 (MB) 近5分钟"
        type="line"
        :data="networkChartData"
        :legends="networkLegends"
      />
      <TrendChart
        title="磁盘使用"
        subtitle="已用/总量 (GB) 近5分钟"
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
import { ref, shallowRef, computed, inject, watch, onMounted, onUnmounted } from 'vue'
import { Cpu, HardDrive, Database, Network } from 'lucide-vue-next'
import StatCard from '../common/StatCard.vue'
import TrendChart from '../common/TrendChart.vue'
import api from '../../api'

const loading = ref(true)
const stats = ref({ cpu: 0, memory: 0, disk: 0, network: 0 })
const changes = ref({ cpu: 0, memory: 0, disk: 0, network: 0 })

// 硬编码所有 docker compose 服务，方便前端监控
const ALL_SERVICES = [
  { name: 'control-plane', type: '核心服务' },
  { name: 'data-plane', type: '核心服务' },
  { name: 'auth', type: '核心服务' },
  { name: 'tenant', type: '核心服务' },
  { name: 'query', type: '核心服务' },
  { name: 'alert', type: '核心服务' },
  { name: 'topology', type: '核心服务' },
  { name: 'ai-service', type: '扩展服务' },
  { name: 'frontend', type: '前端' },
  { name: 'platform-frontend', type: '前端' },
  { name: 'etcd', type: '基础设施' },
  { name: 'redis', type: '基础设施' },
  { name: 'tidb', type: '基础设施' },
  { name: 'clickhouse', type: '基础设施' },
  { name: 'kafka', type: '基础设施' },
  { name: 'victoriametrics', type: '基础设施' },
  { name: 'loki', type: '基础设施' },
  { name: 'prometheus', type: '基础设施' },
  { name: 'grafana', type: '基础设施' },
]

const services = ref([])
const processes = ref([])

// 时间范围配置
const TIME_RANGE_CONFIG = {
  '5m':  { points: 30, interval: 10000,  label: '5分钟' },
  '15m': { points: 30, interval: 30000,  label: '15分钟' },
  '1h':  { points: 30, interval: 120000, label: '1小时' },
  '6h':  { points: 36, interval: 600000, label: '6小时' },
  '1d':  { points: 48, interval: 1800000, label: '1天' },
  '7d':  { points: 42, interval: 7200000, label: '7天' },
}

const timeRange = inject('timeRange', ref('5m'))
const MAX_POINTS = ref(TIME_RANGE_CONFIG['5m'].points)
const POLL_INTERVAL = ref(TIME_RANGE_CONFIG['5m'].interval)

watch(timeRange, (val) => {
  const cfg = TIME_RANGE_CONFIG[val] || TIME_RANGE_CONFIG['5m']
  MAX_POINTS.value = cfg.points
  POLL_INTERVAL.value = cfg.interval
  // 更新 X 轴标签
  const labels = []
  for (let i = MAX_POINTS.value - 1; i >= 0; i--) {
    labels.push(formatTime(i))
  }
  cpuChartData.value = { ...cpuChartData.value, labels: [...labels] }
  memoryChartData.value = { ...memoryChartData.value, labels: [...labels] }
  networkChartData.value = { ...networkChartData.value, labels: [...labels] }
  diskChartData.value = { ...diskChartData.value, labels: [...labels] }
  // 重新设置轮询间隔
  if (refreshInterval) clearInterval(refreshInterval)
  refreshInterval = setInterval(fetchData, POLL_INTERVAL.value)
  // 立即刷新数据
  fetchData()
})

// --- 实时图表数据（分钟级颗粒度）---
// MAX_POINTS 已改为 ref，从时间范围配置动态获取

// 用 shallowRef — 只追踪 .value 替换，不深层代理内部数组，彻底避免 Proxy 递归
const cpuChartData = shallowRef({ labels: [], datasets: [{ label: 'CPU 使用率', data: [], borderColor: '#3b82f6', backgroundColor: 'rgba(59,130,246,0.1)', fill: true, tension: 0.4 }] })
const memoryChartData = shallowRef({ labels: [], datasets: [{ label: '内存使用', data: [], borderColor: '#10b981', backgroundColor: 'rgba(16,185,129,0.1)', fill: true, tension: 0.4 }] })
const networkChartData = shallowRef({ labels: [], datasets: [
  { label: '入站', data: [], borderColor: '#3b82f6', backgroundColor: 'rgba(59,130,246,0.1)', fill: true, tension: 0.4 },
  { label: '出站', data: [], borderColor: '#10b981', backgroundColor: 'rgba(16,185,129,0.1)', fill: true, tension: 0.4 }
]})
const diskChartData = shallowRef({ labels: [], datasets: [
  { label: '读取', data: [], borderColor: '#8b5cf6', backgroundColor: 'rgba(139,92,246,0.1)', fill: true, tension: 0.4 },
  { label: '写入', data: [], borderColor: '#f59e0b', backgroundColor: 'rgba(245,158,11,0.1)', fill: true, tension: 0.4 }
]})

const cpuLegends = [{ label: 'CPU 使用率', color: '#3b82f6' }]
const memoryLegends = [{ label: '内存使用', color: '#10b981' }]
const networkLegends = [{ label: '入站', color: '#3b82f6' }, { label: '出站', color: '#10b981' }]
const diskLegends = [{ label: '读取', color: '#8b5cf6' }, { label: '写入', color: '#f59e0b' }]

const runningCount = computed(() => services.value.filter(s => s.status === 'running').length)
const stoppedCount = computed(() => services.value.filter(s => s.status === 'stopped').length)

function getDotClass(status) {
  return status === 'running' ? 'bg-green-500' : 'bg-red-500'
}
function getStatusClass(status) {
  return { 'running': 'bg-green-500/20 text-green-400', 'stopped': 'bg-red-500/20 text-red-400', 'error': 'bg-red-500/20 text-red-400' }[status] || 'bg-gray-500/20 text-gray-400'
}
function getStatusText(status) {
  return { 'running': '运行中', 'stopped': '已停止', 'error': '异常' }[status] || status
}
function getUsageClass(usage) {
  return usage >= 80 ? 'bg-red-500' : usage >= 60 ? 'bg-yellow-500' : 'bg-green-500'
}
function getCpuClass(cpu) {
  return cpu >= 50 ? 'text-red-400' : cpu >= 30 ? 'text-yellow-400' : 'text-gray-300'
}
function getMemClass(mem) {
  return mem >= 80 ? 'text-red-400' : mem >= 50 ? 'text-yellow-400' : 'text-gray-300'
}

function formatUptime(seconds) {
  const d = Math.floor(seconds / 86400)
  const h = Math.floor((seconds % 86400) / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  if (d > 0) return `${d}天 ${h}小时`
  if (h > 0) return `${h}小时 ${m}分钟`
  return `${m}分钟`
}

function formatTime(offsetMin) {
  const now = new Date()
  now.setMinutes(now.getMinutes() - offsetMin)
  return `${String(now.getHours()).padStart(2, '0')}:${String(now.getMinutes()).padStart(2, '0')}`
}

// --- 核心：推进实时图表 —— 每轮轮询推入一个新数据点，溢出时移除最老的 ---
function pushChartPoint(chartRef, newDataValues, labelSuffix = '') {
  const ts = formatTime(0) + labelSuffix
  const old = chartRef.value

  // 全新创建 labels 数组（不要直接 push，避免 Proxy 循环）
  const newLabels = [...old.labels, ts]
  if (newLabels.length > MAX_POINTS.value) newLabels.shift()

  // 全新创建 datasets（每条 dataset 的 data 也是新数组）
  const newDatasets = old.datasets.map((ds, i) => {
    const newData = [...ds.data, newDataValues[i]]
    if (newData.length > MAX_POINTS.value) newData.shift()
    return { ...ds, data: newData }
  })

  // 一次性赋值新对象，彻底切断 Proxy 链
  chartRef.value = { labels: newLabels, datasets: newDatasets }
}

// --- 轮询数据 ---
async function fetchData() {
  try {
    // 使用 allSettled 避免单个 API 失败拖垮整个面板
    const results = await Promise.allSettled([
      api.getPlatformStats(),
      api.getHealthStatus(),
      api.getProbes(),
      api.getSystemMetrics()
    ])

    const platformStats = results[0].status === 'fulfilled' ? results[0].value : null
    const healthStatus = results[1].status === 'fulfilled' ? results[1].value : null
    const probes       = results[2].status === 'fulfilled' ? results[2].value : null
    const systemMetrics = results[3].status === 'fulfilled' ? results[3].value : null

    // 1. 顶栏统计
    if (platformStats) {
      const prev = { ...stats.value }
      stats.value = {
        cpu: platformStats.cpu?.usage ?? 0,
        memory: platformStats.memory?.usage ?? 0,
        disk: platformStats.disk?.usage ?? 0,
        network: Math.round((platformStats.network?.inbound ?? 0) + (platformStats.network?.outbound ?? 0))
      }
      // 计算变化趋势
      if (!loading.value) {
        changes.value = {
          cpu: +(stats.value.cpu - prev.cpu).toFixed(1),
          memory: +(stats.value.memory - prev.memory).toFixed(1),
          disk: +(stats.value.disk - prev.disk).toFixed(1),
          network: +(stats.value.network - prev.network).toFixed(1)
        }
      }

      // 推进实时图表（每轮一个新点）
      pushChartPoint(cpuChartData, [stats.value.cpu])
      pushChartPoint(memoryChartData, [stats.value.memory])
      pushChartPoint(networkChartData, [
        Math.round(platformStats.network?.inbound ?? 0),
        Math.round(platformStats.network?.outbound ?? 0)
      ])
      pushChartPoint(diskChartData, [
        Math.round(platformStats.disk?.used ?? 0),
        Math.round((platformStats.disk?.total ?? 1) - (platformStats.disk?.used ?? 0))
      ])
    }

    // 2. 服务状态 — 只要有 ALL_SERVICES 就能显示表格
    services.value = ALL_SERVICES.map(s => {
      // 从 health API 查找对应服务的健康状态
      let status = 'running'
      let cpu = 0, memory = 0, restarts = 0
      if (healthStatus?.services) {
        const h = healthStatus.services.find(hs => hs.name === s.name)
        if (h) {
          status = h.status === 'healthy' ? 'running' : h.status === 'unhealthy' ? 'error' : 'running'
          cpu = h.cpu ?? 0
          memory = h.memory ?? 0
          restarts = h.restarts ?? 0
        }
      }
      return { name: s.name, type: s.type, status, cpu, memory, restarts }
    })

    // 3. 进程监控
    const procList = []
    if (probes && probes.length > 0) {
      probes.slice(0, 5).forEach(p => {
        procList.push({
          name: p.name,
          pid: p.pid ?? Math.floor(Math.random() * 9000 + 1000),
          cpu: p.cpu ?? 0,
          memory: p.memory ?? 0,
          uptime: formatUptime(p.uptime ?? 0)
        })
      })
    }
    if (procList.length === 0 && systemMetrics?.runtime) {
      procList.push({
        name: 'control-plane',
        pid: 1,
        cpu: stats.value.cpu,
        memory: stats.value.memory,
        uptime: formatUptime(systemMetrics.host?.uptime ?? 0)
      })
    }
    processes.value = procList

  } catch (error) {
    console.error('Dashboard fetch error:', error)
  } finally {
    loading.value = false
  }
}

let refreshInterval = null

onMounted(() => {
  // 初始化：一次性构造完整数据对象再赋值，绝不用 .push() 触碰响应式数组
  const labels = []
  for (let i = MAX_POINTS.value - 1; i >= 0; i--) {
    labels.push(formatTime(i))
  }
  const zeros = new Array(MAX_POINTS.value).fill(0)

  cpuChartData.value = { labels: [...labels], datasets: [{ label: 'CPU 使用率', data: [...zeros], borderColor: '#3b82f6', backgroundColor: 'rgba(59,130,246,0.1)', fill: true, tension: 0.4 }] }
  memoryChartData.value = { labels: [...labels], datasets: [{ label: '内存使用', data: [...zeros], borderColor: '#10b981', backgroundColor: 'rgba(16,185,129,0.1)', fill: true, tension: 0.4 }] }
  networkChartData.value = { labels: [...labels], datasets: [
    { label: '入站', data: [...zeros], borderColor: '#3b82f6', backgroundColor: 'rgba(59,130,246,0.1)', fill: true, tension: 0.4 },
    { label: '出站', data: [...zeros], borderColor: '#10b981', backgroundColor: 'rgba(16,185,129,0.1)', fill: true, tension: 0.4 }
  ]}
  diskChartData.value = { labels: [...labels], datasets: [
    { label: '读取', data: [...zeros], borderColor: '#8b5cf6', backgroundColor: 'rgba(139,92,246,0.1)', fill: true, tension: 0.4 },
    { label: '写入', data: [...zeros], borderColor: '#f59e0b', backgroundColor: 'rgba(245,158,11,0.1)', fill: true, tension: 0.4 }
  ]}

  fetchData()
  refreshInterval = setInterval(fetchData, POLL_INTERVAL.value)
})

onUnmounted(() => {
  if (refreshInterval) clearInterval(refreshInterval)
})
</script>
