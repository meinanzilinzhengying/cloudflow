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
        :subtitle="chartSubtitle"
        type="line"
        :data="cpuChartData"
        :legends="cpuLegends"
      />
      <TrendChart
        title="内存使用趋势"
        :subtitle="chartSubtitle"
        type="line"
        :data="memoryChartData"
        :legends="memoryLegends"
      />
    </div>

    <div class="grid grid-cols-1 lg:grid-cols-2 gap-6 mb-6">
      <TrendChart
        title="网络流量趋势"
        :subtitle="networkChartSubtitle"
        type="line"
        :data="networkChartData"
        :legends="networkLegends"
      />
      <TrendChart
        title="磁盘使用"
        :subtitle="diskChartSubtitle"
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
          <span v-if="errorCount > 0" class="flex items-center gap-2">
            <span class="w-2 h-2 rounded-full bg-red-500"></span>
            <span class="text-red-400">{{ errorCount }} 异常</span>
          </span>
          <span v-if="stoppedCount > 0" class="flex items-center gap-2">
            <span class="w-2 h-2 rounded-full bg-gray-500"></span>
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
import { ref, shallowRef, computed, watch, onMounted, onUnmounted } from 'vue'
import { Cpu, HardDrive, Database, Network } from 'lucide-vue-next'
import StatCard from '../common/StatCard.vue'
import TrendChart from '../common/TrendChart.vue'
import api from '../../api'
import { timeRange } from '../../stores/timeRange'

const loading = ref(true)
const stats = ref({ cpu: 0, memory: 0, disk: 0, network: 0 })
const changes = ref({ cpu: 0, memory: 0, disk: 0, network: 0 })

// 硬编码所有 docker compose 服务，方便前端监控
const ALL_SERVICES = [
  { name: 'control-plane', displayName: '控制平面', type: '控制平面' },
  { name: 'data-plane', displayName: '数据平面', type: '数据平面' },
  { name: 'auth', displayName: '认证服务', type: '认证服务' },
  { name: 'tenant', displayName: '租户服务', type: '租户服务' },
  { name: 'query', displayName: '查询服务', type: '查询服务' },
  { name: 'alert', displayName: '告警引擎', type: '告警引擎' },
  { name: 'topology', displayName: '拓扑引擎', type: '拓扑引擎' },
  { name: 'ai-service', displayName: 'AI 服务', type: 'AI 服务' },
  { name: 'frontend', displayName: '前端', type: '前端' },
  { name: 'platform-frontend', displayName: '平台前端', type: '平台前端' },
  { name: 'etcd', displayName: 'etcd 存储', type: 'etcd 存储' },
  { name: 'redis', displayName: 'Redis 缓存', type: 'Redis 缓存' },
  { name: 'tidb', displayName: 'TiDB 数据库', type: 'TiDB 数据库' },
  { name: 'clickhouse', displayName: 'ClickHouse 数据库', type: 'ClickHouse 数据库' },
  { name: 'kafka', displayName: 'Kafka 消息队列', type: 'Kafka 消息队列' },
  { name: 'victoriametrics', displayName: 'VictoriaMetrics 时序库', type: 'VictoriaMetrics 时序库' },
  { name: 'loki', displayName: 'Loki 日志', type: 'Loki 日志' },
  { name: 'prometheus', displayName: 'Prometheus 监控', type: 'Prometheus 监控' },
  { name: 'grafana', displayName: 'Grafana 可视化', type: 'Grafana 可视化' },
]

const services = ref([])
const processes = ref([])

// 时间范围配置：points × interval = 总时间范围
const TIME_RANGE_CONFIG = {
  '5m':  { points: 30, interval: 10000,   label: '5分钟' },   // 30 × 10s  = 5m
  '15m': { points: 30, interval: 30000,   label: '15分钟' },  // 30 × 30s  = 15m
  '1h':  { points: 30, interval: 120000,  label: '1小时' },   // 30 × 2m   = 1h
  '6h':  { points: 36, interval: 600000,  label: '6小时' },   // 36 × 10m  = 6h
  '1d':  { points: 48, interval: 1800000, label: '1天' },     // 48 × 30m  = 24h
  '7d':  { points: 42, interval: 14400000, label: '7天' },    // 42 × 4h   = 7d
}

const MAX_POINTS = ref(TIME_RANGE_CONFIG['5m'].points)
const POLL_INTERVAL = ref(TIME_RANGE_CONFIG['5m'].interval)

// 动态计算图表副标题
const chartSubtitle = computed(() => {
  const cfg = TIME_RANGE_CONFIG[timeRange.value] || TIME_RANGE_CONFIG['5m']
  return `近 ${cfg.label}（实时，${cfg.interval / 1000}秒/点）`
})

const networkChartSubtitle = computed(() => {
  const cfg = TIME_RANGE_CONFIG[timeRange.value] || TIME_RANGE_CONFIG['5m']
  return `入站/出站 (MB) 近${cfg.label}`
})

const diskChartSubtitle = computed(() => {
  const cfg = TIME_RANGE_CONFIG[timeRange.value] || TIME_RANGE_CONFIG['5m']
  return `已用/总量 (GB) 近${cfg.label}`
})

watch(timeRange, (val) => {
  const cfg = TIME_RANGE_CONFIG[val] || TIME_RANGE_CONFIG['5m']
  MAX_POINTS.value = cfg.points
  POLL_INTERVAL.value = cfg.interval
  // 尝试从 localStorage 恢复数据（时间范围匹配时直接恢复）
  const restored = loadChartData()
  if (!restored) {
    // 无历史数据则重新初始化空图表
    initCharts()
  }
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
const errorCount = computed(() => services.value.filter(s => s.status === 'error').length)
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

function formatTime(offsetMs) {
  const now = new Date()
  const target = new Date(now.getTime() - offsetMs)
  const hours = String(target.getHours()).padStart(2, '0')
  const minutes = String(target.getMinutes()).padStart(2, '0')

  // 跨天时显示 月-日 时:分
  if (target.getDate() !== now.getDate() || target.getMonth() !== now.getMonth()) {
    const month = String(target.getMonth() + 1).padStart(2, '0')
    const day = String(target.getDate()).padStart(2, '0')
    return `${month}-${day} ${hours}:${minutes}`
  }
  return `${hours}:${minutes}`
}


// --- localStorage 图表持久化（按时间范围独立存储）---
function getChartStorageKey(range) {
  return `cloudflow_dashboard_charts_${range}`
}
function getChartMetaKey(range) {
  return `cloudflow_dashboard_charts_meta_${range}`
}

// 清理旧的统一 key（兼容旧版本）
try {
  localStorage.removeItem('cloudflow_dashboard_charts')
  localStorage.removeItem('cloudflow_dashboard_charts_meta')
} catch (e) {}

function saveChartData() {
  try {
    const data = {
      cpu: cpuChartData.value,
      memory: memoryChartData.value,
      network: networkChartData.value,
      disk: diskChartData.value,
    }
    const meta = { timeRange: timeRange.value, savedAt: Date.now() }
    localStorage.setItem(getChartStorageKey(timeRange.value), JSON.stringify(data))
    localStorage.setItem(getChartMetaKey(timeRange.value), JSON.stringify(meta))
  } catch (e) {}
}

function loadChartData(range = timeRange.value) {
  try {
    const rawData = localStorage.getItem(getChartStorageKey(range))
    const rawMeta = localStorage.getItem(getChartMetaKey(range))
    if (!rawData || !rawMeta) return false
    const data = JSON.parse(rawData)
    const meta = JSON.parse(rawMeta)
    // 使用对应时间范围的配置判断数据是否过期
    const cfg = TIME_RANGE_CONFIG[range] || TIME_RANGE_CONFIG['5m']
    const maxAge = cfg.interval * cfg.points * 2
    if (Date.now() - meta.savedAt > maxAge) {
      localStorage.removeItem(getChartStorageKey(range))
      localStorage.removeItem(getChartMetaKey(range))
      return false
    }
    if (data.cpu) cpuChartData.value = data.cpu
    if (data.memory) memoryChartData.value = data.memory
    if (data.network) networkChartData.value = data.network
    if (data.disk) diskChartData.value = data.disk
    return true
  } catch (e) { return false }
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
  // 持久化到 localStorage
  saveChartData()
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
      let backendType = s.type
      if (healthStatus?.services) {
        const h = healthStatus.services.find(hs => hs.name === s.name)
        if (h) {
          status = h.status === 'healthy' ? 'running' : h.status === 'unhealthy' ? 'error' : 'running'
          cpu = h.cpu ?? 0
          memory = h.memory ?? 0
          restarts = h.restarts ?? 0
          backendType = h.type || s.type
        }
      }
      return { name: s.displayName, type: backendType, status, cpu, memory, restarts }
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

// --- 初始化图表 labels：按 POLL_INTERVAL 均匀分布，覆盖所选时间范围 ---
function initCharts() {
  const labels = []
  for (let i = MAX_POINTS.value - 1; i >= 0; i--) {
    const offsetMs = i * POLL_INTERVAL.value
    labels.push(formatTime(offsetMs))
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
}

let refreshInterval = null

onMounted(() => {
  // 尝试从 localStorage 恢复历史图表数据
  const restored = loadChartData()
  if (!restored) {
    // 无历史数据则初始化空图表
    initCharts()
  }
  fetchData()
  refreshInterval = setInterval(fetchData, POLL_INTERVAL.value)
})

onUnmounted(() => {
  if (refreshInterval) clearInterval(refreshInterval)
})
</script>
