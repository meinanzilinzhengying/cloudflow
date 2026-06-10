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
      <div class="p-6 border-b border-slate-200 dark:border-dark-700 flex items-center justify-between">
        <h3 class="text-lg font-semibold text-slate-900 dark:text-white">告警时间线</h3>
        <button @click="fetchData" class="btn-secondary text-sm">
          <RefreshCw class="w-4 h-4" />
          刷新
        </button>
      </div>
      <div class="p-6">
        <div v-if="loading" class="py-12 text-center text-slate-500 dark:text-slate-400">
          <div class="inline-block w-6 h-6 border-2 border-slate-300 border-t-primary-500 rounded-full animate-spin mb-3"></div>
          <p>加载中...</p>
        </div>
        <div v-else-if="alerts.length === 0" class="py-12 text-center text-slate-500 dark:text-slate-400">
          <AlertCircle class="w-12 h-12 mx-auto mb-3 text-slate-300 dark:text-slate-600" />
          <p>暂无告警数据</p>
        </div>
        <div v-else class="relative pl-8">
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
              <p v-if="alert.description" class="text-sm text-slate-600 dark:text-slate-300 mt-2">{{ alert.description }}</p>
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
        <div v-if="loading" class="py-12 text-center text-slate-500 dark:text-slate-400">
          <div class="inline-block w-6 h-6 border-2 border-slate-300 border-t-primary-500 rounded-full animate-spin mb-3"></div>
          <p>加载中...</p>
        </div>
        <div v-else-if="rules.length === 0" class="py-12 text-center text-slate-500 dark:text-slate-400">
          <AlertCircle class="w-12 h-12 mx-auto mb-3 text-slate-300 dark:text-slate-600" />
          <p>暂无规则数据</p>
        </div>
        <table v-else class="w-full">
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

            <div v-if="selectedAlert.description" class="p-4 bg-slate-50 dark:bg-dark-700 rounded-xl">
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

            <div v-if="selectedAlert.raw && Object.keys(selectedAlert.raw).length">
              <h4 class="text-sm font-semibold text-slate-700 dark:text-slate-200 mb-3">关联信息</h4>
              <div class="p-4 bg-slate-50 dark:bg-dark-700 rounded-xl max-h-64 overflow-y-auto">
                <pre class="text-xs text-slate-600 dark:text-slate-300 whitespace-pre-wrap break-all font-mono">{{ JSON.stringify(selectedAlert.raw, null, 2) }}</pre>
              </div>
            </div>

            <div v-if="selectedAlert.logSample">
              <h4 class="text-sm font-semibold text-slate-700 dark:text-slate-200 mb-3">相关日志</h4>
              <div class="p-4 bg-slate-900 rounded-lg font-mono text-sm text-slate-300 max-h-48 overflow-y-auto whitespace-pre-wrap break-all">
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
import { alertService, queryService } from '../../../api'

use([CanvasRenderer, LineChart, PieChart, TooltipComponent, GridComponent, LegendComponent])

const activeTab = ref('events')

const tabs = [
  { id: 'events', label: '告警事件' },
  { id: 'rules', label: '告警规则' },
  { id: 'notifications', label: '通知策略' },
  { id: 'statistics', label: '告警统计' },
]

const loading = ref(false)

const alertStats = ref({
  critical: 0,
  high: 0,
  warning: 0,
  medium: 0,
  low: 0,
  info: 0,
  normal: 0,
})

const alerts = ref([])
const rules = ref([])

const notificationPolicies = ref([
  { id: 1, name: '邮件通知', description: '发送告警邮件到指定邮箱', icon: Mail, color: 'text-blue-500' },
  { id: 2, name: '钉钉通知', description: '发送告警消息到钉钉群', icon: MessageCircle, color: 'text-green-500' },
  { id: 3, name: '电话通知', description: '严重告警时电话通知', icon: Phone, color: 'text-red-500' },
])

const selectedAlert = ref(null)

const normalizeSeverity = (severity) => {
  if (!severity) return 'low'
  const s = String(severity).toLowerCase()
  const map = {
    critical: 'critical',
    fatal: 'critical',
    high: 'high',
    warning: 'warning',
    warn: 'warning',
    medium: 'medium',
    info: 'info',
    low: 'low',
    normal: 'normal',
    debug: 'normal',
  }
  return map[s] || 'low'
}

const formatTime = (t) => {
  if (!t) return '-'
  if (typeof t === 'string' && !t.includes('T') && !t.includes('-') && t.length <= 10) return t
  try {
    const date = new Date(t)
    if (isNaN(date.getTime())) return t
    const now = Date.now()
    const diff = now - date.getTime()
    if (diff < 60 * 1000) return '刚刚'
    if (diff < 60 * 60 * 1000) return `${Math.floor(diff / (60 * 1000))}分钟前`
    if (diff < 24 * 60 * 60 * 1000) return `${Math.floor(diff / (60 * 60 * 1000))}小时前`
    return date.toLocaleString('zh-CN', { hour12: false })
  } catch {
    return t
  }
}

const fetchData = async () => {
  loading.value = true
  try {
    const [alertsData, rulesData] = await Promise.all([
      alertService.getAlerts({ limit: 20 }),
      alertService.getRules(),
    ])

    const alertList = Array.isArray(alertsData)
      ? alertsData
      : (alertsData?.data || alertsData?.alerts || alertsData?.items || [])

    const normalized = alertList.map((a) => ({
      id: a.id || a.alert_id || a._id || Math.random().toString(36).slice(2),
      title: a.title || a.name || a.summary || a.message || '未命名告警',
      severity: normalizeSeverity(a.severity || a.level || a.priority),
      service: a.service || a.service_name || a.module || a.source || '未知服务',
      description: a.description || a.message || a.detail || a.content || '',
      time: formatTime(a.time || a.created_at || a.timestamp || a.triggered_at),
      created_at: a.created_at || a.timestamp || a.triggered_at,
      status: a.status || a.state || (a.resolved ? '已恢复' : '触发中'),
      duration: a.duration || '-',
      instance: a.instance || a.host || a.node || '-',
      logSample: a.log_sample || a.logs || a.log || a.message || '',
      raw: a,
    }))

    alerts.value = normalized

    alertStats.value = {
      critical: 0,
      high: 0,
      warning: 0,
      medium: 0,
      low: 0,
      info: 0,
      normal: 0,
    }
    normalized.forEach((a) => {
      if (alertStats.value[a.severity] !== undefined) {
        alertStats.value[a.severity] += 1
      } else {
        alertStats.value.low += 1
      }
    })

    const ruleList = Array.isArray(rulesData)
      ? rulesData
      : (rulesData?.data || rulesData?.rules || rulesData?.items || [])
    rules.value = ruleList.map((r) => ({
      id: r.id || r.rule_id || Math.random().toString(36).slice(2),
      name: r.name || r.title || '未命名规则',
      metric: r.metric || r.indicator || r.key || '-',
      threshold: r.threshold || r.condition || r.value || '-',
      severity: normalizeSeverity(r.severity || r.level),
      enabled: r.enabled !== undefined ? r.enabled : r.active !== undefined ? r.active : true,
    }))
  } catch (err) {
    console.error('Failed to fetch alert data:', err)
    alerts.value = []
    rules.value = []
    alertStats.value = { critical: 0, high: 0, warning: 0, medium: 0, low: 0, info: 0, normal: 0 }
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  fetchData()
})

const alertTrendOption = computed(() => {
  const s = alertStats.value
  return {
    tooltip: { trigger: 'axis', backgroundColor: 'rgba(255,255,255,0.95)', textStyle: { color: '#1e293b' } },
    grid: { left: '3%', right: '4%', bottom: '3%', top: '10%', containLabel: true },
    xAxis: { type: 'category', data: ['严重', '高', '警告', '中', '低', '信息', '正常'], axisLabel: { color: '#64748b' } },
    yAxis: { type: 'value', axisLabel: { color: '#64748b' }, splitLine: { lineStyle: { color: '#f1f5f9' } } },
    series: [
      {
        name: '告警分布',
        type: 'line',
        smooth: true,
        lineStyle: { color: '#3b82f6', width: 2 },
        itemStyle: { color: '#3b82f6' },
        areaStyle: { color: 'rgba(59, 130, 246, 0.1)' },
        data: [s.critical, s.high, s.warning, s.medium, s.low, s.info, s.normal],
      },
    ],
  }
})

const alertDistributionOption = computed(() => {
  const s = alertStats.value
  const data = [
    { value: s.critical, name: '严重', itemStyle: { color: '#ef4444' } },
    { value: s.high, name: '高', itemStyle: { color: '#f59e0b' } },
    { value: s.warning, name: '警告', itemStyle: { color: '#fb923c' } },
    { value: s.medium, name: '中', itemStyle: { color: '#eab308' } },
    { value: s.low, name: '低', itemStyle: { color: '#3b82f6' } },
    { value: s.info, name: '信息', itemStyle: { color: '#06b6d4' } },
    { value: s.normal, name: '正常', itemStyle: { color: '#22c55e' } },
  ].filter((d) => d.value > 0)
  return {
    tooltip: { trigger: 'item', backgroundColor: 'rgba(255,255,255,0.95)', textStyle: { color: '#1e293b' } },
    legend: { bottom: 0, textStyle: { color: '#64748b' } },
    series: [{
      type: 'pie',
      radius: ['40%', '70%'],
      itemStyle: { borderRadius: 8, borderColor: '#fff', borderWidth: 2 },
      label: { show: false },
      emphasis: { label: { show: true, fontSize: 14, fontWeight: 'bold' } },
      data: data.length ? data : [{ value: 1, name: '暂无数据', itemStyle: { color: '#cbd5e1' } }],
    }],
  }
})

const getAlertIcon = (severity) => {
  const icons = { critical: AlertOctagon, high: AlertTriangle, warning: AlertTriangle, medium: AlertCircle, low: Info, info: Info, normal: Info }
  return icons[severity] || Info
}

const getAlertIconBg = (severity) => {
  const colors = {
    critical: 'bg-red-100 border-red-500',
    high: 'bg-amber-100 border-amber-500',
    warning: 'bg-orange-100 border-orange-500',
    medium: 'bg-yellow-100 border-yellow-500',
    low: 'bg-blue-100 border-blue-500',
    info: 'bg-cyan-100 border-cyan-500',
    normal: 'bg-green-100 border-green-500',
  }
  return colors[severity] || 'bg-slate-100 border-slate-500'
}

const getAlertIconColor = (severity) => {
  const colors = {
    critical: 'text-red-500',
    high: 'text-amber-500',
    warning: 'text-orange-500',
    medium: 'text-yellow-500',
    low: 'text-blue-500',
    info: 'text-cyan-500',
    normal: 'text-green-500',
  }
  return colors[severity] || 'text-slate-500'
}

const getAlertBadgeClass = (severity) => {
  const classes = {
    critical: 'bg-red-100 text-red-600',
    high: 'bg-amber-100 text-amber-600',
    warning: 'bg-orange-100 text-orange-600',
    medium: 'bg-yellow-100 text-yellow-600',
    low: 'bg-blue-100 text-blue-600',
    info: 'bg-cyan-100 text-cyan-600',
    normal: 'bg-green-100 text-green-600',
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
