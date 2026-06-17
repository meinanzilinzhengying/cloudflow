<template>
  <div class="space-y-6">
    <!-- Page Header -->
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold text-slate-900 dark:text-white">告警中心</h1>
        <p class="text-sm text-slate-500 dark:text-slate-400 mt-1">监控和管理告警事件</p>
      </div>
      <button class="btn-secondary">
        <RefreshCw class="w-4 h-4" />
        刷新
      </button>
    </div>

    <!-- Tab Toggle -->
    <div class="flex items-center gap-2 p-1 bg-slate-100 dark:bg-dark-700 rounded-xl w-fit">
      <button
        v-for="tab in tabs"
        :key="tab.id"
        @click="activeTab = tab.id"
        :class="[
          'px-4 py-2 text-sm font-medium rounded-lg transition-all duration-200',
          activeTab === tab.id
            ? 'bg-white dark:bg-dark-600 text-slate-900 dark:text-white shadow-sm'
            : 'text-slate-500 hover:text-slate-700 dark:text-slate-400'
        ]"
      >
        {{ tab.label }}
      </button>
    </div>

    <!-- Stats Cards -->
    <div v-if="activeTab === 'events'" class="grid grid-cols-4 gap-4">
      <div class="card p-4 border-l-4 border-red-500">
        <div class="flex items-center justify-between">
          <div>
            <p class="text-sm text-slate-500">严重告警</p>
            <p class="text-2xl font-bold text-red-500 mt-1">{{ alertStats.critical }}</p>
          </div>
          <AlertOctagon class="w-10 h-10 text-red-500/20" />
        </div>
      </div>
      <div class="card p-4 border-l-4 border-amber-500">
        <div class="flex items-center justify-between">
          <div>
            <p class="text-sm text-slate-500">高告警</p>
            <p class="text-2xl font-bold text-amber-500 mt-1">{{ alertStats.high }}</p>
          </div>
          <AlertTriangle class="w-10 h-10 text-amber-500/20" />
        </div>
      </div>
      <div class="card p-4 border-l-4 border-yellow-500">
        <div class="flex items-center justify-between">
          <div>
            <p class="text-sm text-slate-500">中告警</p>
            <p class="text-2xl font-bold text-yellow-500 mt-1">{{ alertStats.medium }}</p>
          </div>
          <AlertCircle class="w-10 h-10 text-yellow-500/20" />
        </div>
      </div>
      <div class="card p-4 border-l-4 border-blue-500">
        <div class="flex items-center justify-between">
          <div>
            <p class="text-sm text-slate-500">低告警</p>
            <p class="text-2xl font-bold text-blue-500 mt-1">{{ alertStats.low }}</p>
          </div>
          <Info class="w-10 h-10 text-blue-500/20" />
        </div>
      </div>
    </div>

    <!-- Alerts Timeline -->
    <div v-if="activeTab === 'events'" class="card">
      <div v-if="loading" class="p-6 text-center text-slate-500">加载中...</div>
      <div v-else-if="error" class="p-6 text-center text-red-500">{{ error }}</div>
      <div class="p-6 border-b border-slate-200 dark:border-dark-700">
        <h3 class="text-lg font-semibold text-slate-900 dark:text-white">告警时间线</h3>
      </div>
      <div class="p-6">
        <div class="relative pl-8">
          <div class="absolute left-3 top-0 bottom-0 w-0.5 bg-slate-200 dark:bg-dark-700"></div>
          <div
            v-for="alert in alerts"
            :key="alert.id"
            class="relative mb-6 last:mb-0"
            @click="openAlertDetail(alert)"
          >
            <div :class="['absolute left-0 top-1 w-6 h-6 rounded-full border-2 flex items-center justify-center', getAlertIconBg(alert.severity)]">
              <component :is="getAlertIcon(alert.severity)" class="w-3 h-3" :class="getAlertIconColor(alert.severity)" />
            </div>
            <div class="ml-4 p-4 bg-slate-50 dark:bg-dark-700 rounded-xl hover:bg-slate-100 dark:hover:bg-dark-600 cursor-pointer transition-colors">
              <div class="flex items-start justify-between">
                <div>
                  <h4 class="font-semibold text-slate-900 dark:text-white">{{ alert.title }}</h4>
                  <p class="text-sm text-slate-500 mt-1">{{ alert.service }}</p>
                </div>
                <span :class="['text-xs px-2 py-1 rounded-full font-medium', getAlertBadgeClass(alert.severity)]">
                  {{ alert.severity }}
                </span>
              </div>
              <p class="text-sm text-slate-600 dark:text-slate-300 mt-2">{{ alert.description }}</p>
              <div class="flex items-center gap-4 mt-3 text-xs text-slate-400">
                <span>{{ alert.time }}</span>
                <span>{{ alert.status }}</span>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Rules List -->
    <div v-if="activeTab === 'rules'" class="card">
      <div class="p-6 border-b border-slate-200 dark:border-dark-700 flex items-center justify-between">
        <h3 class="text-lg font-semibold text-slate-900 dark:text-white">告警规则</h3>
        <button class="btn-primary">
          <Plus class="w-4 h-4" />
          添加规则
        </button>
      </div>
      <div class="overflow-x-auto">
        <table class="w-full">
          <thead>
            <tr class="bg-slate-50 dark:bg-dark-700/50">
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500">规则名称</th>
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500">指标</th>
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500">阈值</th>
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500">级别</th>
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500">状态</th>
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500">操作</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-slate-200 dark:divide-dark-700">
            <tr v-for="rule in rules" :key="rule.id">
              <td class="px-6 py-4 text-sm text-slate-900 dark:text-white">{{ rule.name }}</td>
              <td class="px-6 py-4 text-sm text-slate-500">{{ rule.metric }}</td>
              <td class="px-6 py-4 text-sm text-slate-500">{{ rule.threshold }}</td>
              <td class="px-6 py-4">
                <span :class="['text-xs px-2 py-1 rounded-full font-medium', getAlertBadgeClass(rule.severity)]">{{ rule.severity }}</span>
              </td>
              <td class="px-6 py-4">
                <span :class="['text-xs px-2 py-1 rounded-full', rule.enabled ? 'bg-green-100 text-green-600' : 'bg-slate-100 text-slate-500']">{{ rule.enabled ? '启用' : '禁用' }}</span>
              </td>
              <td class="px-6 py-4">
                <button class="text-xs text-primary-500 hover:text-primary-600">编辑</button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- Notification Policy -->
    <div v-if="activeTab === 'notifications'" class="card">
      <div class="p-6 border-b border-slate-200 dark:border-dark-700">
        <h3 class="text-lg font-semibold text-slate-900 dark:text-white">通知策略</h3>
      </div>
      <div class="p-6">
        <div class="grid grid-cols-3 gap-4">
          <div v-for="policy in notificationPolicies" :key="policy.id" class="p-4 bg-slate-50 dark:bg-dark-700 rounded-xl">
            <div class="flex items-center gap-2 mb-2">
              <component :is="policy.icon" class="w-5 h-5" :class="policy.color" />
              <span class="font-medium text-slate-900 dark:text-white">{{ policy.name }}</span>
            </div>
            <p class="text-sm text-slate-500">{{ policy.description }}</p>
          </div>
        </div>
      </div>
    </div>

    <!-- Statistics -->
    <div v-if="activeTab === 'statistics'" class="card p-6">
      <h3 class="text-lg font-semibold text-slate-900 dark:text-white mb-6">告警统计</h3>
      <div class="grid grid-cols-2 gap-6">
        <div>
          <h4 class="text-sm font-medium text-slate-600 dark:text-slate-300 mb-4">告警趋势</h4>
          <div class="h-64">
            <ECharts :option="alertTrendOption" class="w-full h-full" />
          </div>
        </div>
        <div>
          <h4 class="text-sm font-medium text-slate-600 dark:text-slate-300 mb-4">告警分布</h4>
          <div class="h-64">
            <ECharts :option="alertDistributionOption" class="w-full h-full" />
          </div>
        </div>
      </div>
    </div>

    <!-- Alert Detail Drawer -->
    <Transition name="drawer">
      <div
        v-if="selectedAlert"
        class="fixed inset-0 z-50 flex justify-end"
        @click.self="selectedAlert = null"
      >
        <div class="absolute inset-0 bg-slate-900/50"></div>
        <div class="relative w-full max-w-lg bg-white dark:bg-dark-800 shadow-2xl overflow-y-auto">
          <div class="sticky top-0 bg-white dark:bg-dark-800 border-b border-slate-200 dark:border-dark-700 px-6 py-4 flex items-center justify-between">
            <h3 class="text-lg font-semibold text-slate-900 dark:text-white">告警详情</h3>
            <button @click="selectedAlert = null" class="p-2 hover:bg-slate-100 dark:hover:bg-dark-700 rounded-lg transition-colors">
              <X class="w-5 h-5 text-slate-500" />
            </button>
          </div>
          <div class="p-6 space-y-6">
            <div>
              <h2 class="text-xl font-bold text-slate-900 dark:text-white">{{ selectedAlert.title }}</h2>
              <div class="flex items-center gap-2 mt-2">
                <span :class="['text-xs px-2 py-1 rounded-full font-medium', getAlertBadgeClass(selectedAlert.severity)]">{{ selectedAlert.severity }}</span>
                <span class="text-xs text-slate-500">{{ selectedAlert.status }}</span>
              </div>
            </div>

            <div class="p-4 bg-slate-50 dark:bg-dark-700 rounded-xl">
              <p class="text-sm text-slate-600 dark:text-slate-300">{{ selectedAlert.description }}</p>
            </div>

            <div class="grid grid-cols-2 gap-4">
              <div>
                <p class="text-xs text-slate-500 mb-1">服务</p>
                <p class="text-sm font-medium text-slate-900 dark:text-white">{{ selectedAlert.service }}</p>
              </div>
              <div>
                <p class="text-xs text-slate-500 mb-1">触发时间</p>
                <p class="text-sm font-medium text-slate-900 dark:text-white">{{ selectedAlert.time }}</p>
              </div>
              <div>
                <p class="text-xs text-slate-500 mb-1">持续时间</p>
                <p class="text-sm font-medium text-slate-900 dark:text-white">{{ selectedAlert.duration }}</p>
              </div>
              <div>
                <p class="text-xs text-slate-500 mb-1">实例</p>
                <p class="text-sm font-medium text-slate-900 dark:text-white">{{ selectedAlert.instance }}</p>
              </div>
            </div>

            <div>
              <h4 class="text-sm font-semibold text-slate-700 dark:text-slate-200 mb-3">影响范围</h4>
              <div class="p-4 bg-amber-50 dark:bg-amber-500/10 rounded-xl border border-amber-200 dark:border-amber-500/20">
                <div class="space-y-2">
                  <div class="flex items-center gap-2">
                    <Server class="w-4 h-4 text-amber-600" />
                    <span class="text-sm font-medium text-amber-800 dark:text-amber-300">Gateway</span>
                  </div>
                  <div class="flex items-center gap-2 pl-4">
                    <ChevronDown class="w-4 h-4 text-amber-400" />
                    <span class="text-sm font-medium text-amber-700 dark:text-amber-400">Order Service</span>
                  </div>
                  <div class="flex items-center gap-2 pl-8">
                    <ChevronDown class="w-4 h-4 text-amber-400" />
                    <span class="text-sm font-medium text-amber-600 dark:text-amber-500">Payment Service</span>
                  </div>
                </div>
              </div>
            </div>

            <div>
              <h4 class="text-sm font-semibold text-slate-700 dark:text-slate-200 mb-3">相关Trace</h4>
              <div class="space-y-2">
                <div class="p-3 bg-slate-50 dark:bg-dark-700 rounded-lg">
                  <div class="flex items-center justify-between">
                    <span class="text-sm font-medium text-slate-700 dark:text-slate-200">Checkout Flow</span>
                    <span class="text-xs text-red-500">Error</span>
                  </div>
                  <p class="text-xs text-slate-500 mt-1">Trace ID: abc123...</p>
                </div>
              </div>
            </div>

            <div>
              <h4 class="text-sm font-semibold text-slate-700 dark:text-slate-200 mb-3">相关日志</h4>
              <div class="p-4 bg-slate-900 rounded-lg font-mono text-sm text-slate-300">
                {{ selectedAlert.logSample }}
              </div>
            </div>

            <div class="flex gap-3">
              <button class="btn-secondary flex-1">
                <Bell class="w-4 h-4" />
                通知
              </button>
              <button class="btn-primary flex-1">
                <Search class="w-4 h-4" />
                查看RCA
              </button>
            </div>
          </div>
        </div>
      </div>
    </Transition>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { queryService } from '@/api'
import { use } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { LineChart, PieChart } from 'echarts/charts'
import { TooltipComponent, GridComponent, LegendComponent } from 'echarts/components'
import ECharts from 'vue-echarts'
import {
  AlertOctagon,
  AlertTriangle,
  AlertCircle,
  Info,
  RefreshCw,
  Plus,
  X,
  Server,
  ChevronDown,
  Bell,
  Search,
  Mail,
  MessageCircle,
  Phone,
} from 'lucide-vue-next'

use([CanvasRenderer, LineChart, PieChart, TooltipComponent, GridComponent, LegendComponent])

const activeTab = ref('events')

const tabs = [
  { id: 'events', label: '告警事件' },
  { id: 'rules', label: '告警规则' },
  { id: 'notifications', label: '通知策略' },
  { id: 'statistics', label: '告警统计' },
]

// 告警统计数据（基于真实数据计算）
const alertStats = computed(() => {
  const alertsList = alerts.value || []
  return {
    critical: alertsList.filter(a => a.severity === 'critical').length,
    high: alertsList.filter(a => a.severity === 'high').length,
    medium: alertsList.filter(a => a.severity === 'medium').length,
    low: alertsList.filter(a => a.severity === 'low').length,
  }
})

const alerts = ref([])
const loading = ref(false)
const error = ref(null)

// 获取告警列表
async function fetchAlerts() {
  loading.value = true
  error.value = null
  try {
    const res = await queryService.getAlerts({ limit: 100 })
    if (res.data && res.data.alerts) {
      alerts.value = (res.alerts || []).map(a => ({
        id: a.id || a.alert_id || Math.random(),
        title: a.title || a.name || a.rule_name || '告警事件',
        severity: a.severity || a.level || 'medium',
        service: a.service || a.source || a.host || '未知服务',
        description: a.description || a.message || a.summary || '',
        time: a.time || a.created_at || a.timestamp || '刚刚',
        status: a.status || (a.resolved ? '已恢复' : '触发中'),
        duration: a.duration || '-',
        instance: a.instance || a.host || a.source_ip || '-',
        logSample: a.log_sample || a.logSample || '',
      }))
    } else if (Array.isArray(res.data)) {
      alerts.value = (res.data || []).map(a => ({...a, id: a.id || Math.random(), title: a.title || a.name || '---', severity: a.severity || 'medium'}))
    }
  } catch (err) {
    error.value = err.message || '获取告警失败'
    console.error('获取告警列表失败:', err)
  } finally {
    loading.value = false
  }
}

// 获取告警规则（从alert统计数据派生）
function fetchRules() {
  // 规则从告警数据中自动统计分析
  const severityMap = {}
  alerts.value.forEach(a => {
    if (a.service) {
      if (!severityMap[a.service]) severityMap[a.service] = new Set()
      severityMap[a.service].add(a.severity)
    }
  })
  rules.value = Object.entries(severityMap).map(([service, severities]) => ({
    id: service,
    name: `${service} 自动规则`,
    metric: '多指标',
    threshold: '动态',
    severity: [...severities].includes('critical') ? 'critical' : [...severities][0],
    enabled: true,
  }))
}

// 页面加载时获取数据
onMounted(() => {
  fetchAlerts()
  fetchRules()
})

const rules = ref([])

const notificationPolicies = ref([
  { id: 1, name: '邮件通知', description: '发送告警邮件到指定邮箱', icon: Mail, color: 'text-blue-500' },
  { id: 2, name: '钉钉通知', description: '发送告警消息到钉钉群', icon: MessageCircle, color: 'text-green-500' },
  { id: 3, name: '电话通知', description: '严重告警时电话通知', icon: Phone, color: 'text-red-500' },
])

const selectedAlert = ref(null)

const alertTrendOption = computed(() => ({
  tooltip: { trigger: 'axis', backgroundColor: 'rgba(255,255,255,0.95)', textStyle: { color: '#1e293b' } },
  grid: { left: '3%', right: '4%', bottom: '3%', top: '10%', containLabel: true },
  xAxis: { type: 'category', data: ['00:00', '06:00', '12:00', '18:00', '24:00'], axisLabel: { color: '#64748b' } },
  yAxis: { type: 'value', axisLabel: { color: '#64748b' }, splitLine: { lineStyle: { color: '#f1f5f9' } } },
  series: [
    { name: '严重', type: 'line', smooth: true, lineStyle: { color: '#ef4444', width: 2 }, data: [5, 3, 8, 6, 5] },
    { name: '高', type: 'line', smooth: true, lineStyle: { color: '#f59e0b', width: 2 }, data: [10, 8, 15, 12, 12] },
    { name: '中', type: 'line', smooth: true, lineStyle: { color: '#eab308', width: 2 }, data: [15, 20, 25, 22, 23] },
    { name: '低', type: 'line', smooth: true, lineStyle: { color: '#3b82f6', width: 2 }, data: [30, 40, 45, 50, 45] },
  ],
}))

const alertDistributionOption = computed(() => ({
  tooltip: { trigger: 'item', backgroundColor: 'rgba(255,255,255,0.95)', textStyle: { color: '#1e293b' } },
  legend: { bottom: 0, textStyle: { color: '#64748b' } },
  series: [{
    type: 'pie',
    radius: ['40%', '70%'],
    itemStyle: { borderRadius: 8, borderColor: '#fff', borderWidth: 2 },
    label: { show: false },
    emphasis: { label: { show: true, fontSize: 14, fontWeight: 'bold' } },
    data: [
      { value: 5, name: '严重', itemStyle: { color: '#ef4444' } },
      { value: 12, name: '高', itemStyle: { color: '#f59e0b' } },
      { value: 23, name: '中', itemStyle: { color: '#eab308' } },
      { value: 45, name: '低', itemStyle: { color: '#3b82f6' } },
    ],
  }],
}))

const getAlertIcon = (severity) => {
  const icons = { critical: AlertOctagon, high: AlertTriangle, medium: AlertCircle, low: Info }
  return icons[severity] || Info
}

const getAlertIconBg = (severity) => {
  const colors = {
    critical: 'bg-red-100 border-red-500',
    high: 'bg-amber-100 border-amber-500',
    medium: 'bg-yellow-100 border-yellow-500',
    low: 'bg-blue-100 border-blue-500',
  }
  return colors[severity] || 'bg-slate-100 border-slate-500'
}

const getAlertIconColor = (severity) => {
  const colors = { critical: 'text-red-500', high: 'text-amber-500', medium: 'text-yellow-500', low: 'text-blue-500' }
  return colors[severity] || 'text-slate-500'
}

const getAlertBadgeClass = (severity) => {
  const classes = {
    critical: 'bg-red-100 text-red-600',
    high: 'bg-amber-100 text-amber-600',
    medium: 'bg-yellow-100 text-yellow-600',
    low: 'bg-blue-100 text-blue-600',
  }
  return classes[severity] || 'bg-slate-100 text-slate-600'
}

const openAlertDetail = (alert) => {
  selectedAlert.value = alert
}
</script>

<style scoped>
.drawer-enter-active,
.drawer-leave-active {
  transition: all 0.3s ease;
}
.drawer-enter-from,
.drawer-leave-to {
  opacity: 0;
}
.drawer-enter-from > div:last-child,
.drawer-leave-to > div:last-child {
  transform: translateX(100%);
}
</style>
