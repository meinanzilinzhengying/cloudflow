<template>
  <div class="space-y-6">
    <!-- Page Header -->
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold text-slate-900 dark:text-white">全局流量分析</h1>
        <p class="text-sm text-slate-500 dark:text-slate-400 mt-1">实时监控网络流量与会话数据</p>
      </div>
      <div class="flex items-center gap-3">
        <select class="input w-40">
          <option>1小时</option>
          <option>6小时</option>
          <option>24小时</option>
          <option>7天</option>
        </select>
        <button class="btn-secondary">
          <Download class="w-4 h-4" />
          导出
        </button>
      </div>
    </div>

    <!-- KPI Cards -->
    <div class="grid grid-cols-4 gap-4">
      <div class="card p-4">
        <div class="flex items-center justify-between">
          <div>
            <p class="text-sm text-slate-500">总流量</p>
            <p class="text-2xl font-bold text-slate-900 dark:text-white mt-1">0 GB</p>
          </div>
          <div class="w-10 h-10 rounded-xl bg-primary-50 dark:bg-primary-500/10 flex items-center justify-center">
            <Network class="w-5 h-5 text-primary-500" />
          </div>
        </div>
      </div>
      <div class="card p-4">
        <div class="flex items-center justify-between">
          <div>
            <p class="text-sm text-slate-500">PPS</p>
            <p class="text-2xl font-bold text-accent-500 mt-1">0 K</p>
          </div>
          <div class="w-10 h-10 rounded-xl bg-accent-50 dark:bg-accent-500/10 flex items-center justify-center">
            <Activity class="w-5 h-5 text-accent-500" />
          </div>
        </div>
      </div>
      <div class="card p-4">
        <div class="flex items-center justify-between">
          <div>
            <p class="text-sm text-slate-500">BPS</p>
            <p class="text-2xl font-bold text-emerald-500 mt-1">0 Mbps</p>
          </div>
          <div class="w-10 h-10 rounded-xl bg-emerald-50 dark:bg-emerald-500/10 flex items-center justify-center">
            <TrendingUp class="w-5 h-5 text-emerald-500" />
          </div>
        </div>
      </div>
      <div class="card p-4">
        <div class="flex items-center justify-between">
          <div>
            <p class="text-sm text-slate-500">会话数</p>
            <p class="text-2xl font-bold text-amber-500 mt-1">0</p>
          </div>
          <div class="w-10 h-10 rounded-xl bg-amber-50 dark:bg-amber-500/10 flex items-center justify-center">
            <Users class="w-5 h-5 text-amber-500" />
          </div>
        </div>
      </div>
    </div>

    <!-- Traffic Trends Chart -->
    <div class="card p-6">
      <div class="flex items-center justify-between mb-6">
        <h3 class="text-lg font-semibold text-slate-900 dark:text-white">流量趋势</h3>
        <div class="flex items-center gap-4">
          <div class="flex items-center gap-2">
            <div class="w-3 h-3 rounded-full bg-primary-500"></div>
            <span class="text-xs text-slate-500">入站流量</span>
          </div>
          <div class="flex items-center gap-2">
            <div class="w-3 h-3 rounded-full bg-accent-500"></div>
            <span class="text-xs text-slate-500">出站流量</span>
          </div>
          <div class="flex items-center gap-2">
            <div class="w-3 h-3 rounded-full bg-amber-500"></div>
            <span class="text-xs text-slate-500">PPS</span>
          </div>
        </div>
      </div>
      <div class="h-72">
        <ECharts :option="trafficTrendOption" class="w-full h-full" />
      </div>
    </div>

    <!-- Top Client / Top Server -->
    <div class="grid grid-cols-2 gap-6">
      <!-- Top Client -->
      <div class="card p-6">
        <div class="flex items-center justify-between mb-4">
          <h3 class="text-lg font-semibold text-slate-900 dark:text-white">Top Client</h3>
          <button class="text-xs text-primary-500 hover:text-primary-600 font-medium">查看全部</button>
        </div>
        <div class="space-y-3">
          <div
            v-for="(client, index) in topClients"
            :key="client.ip"
            class="flex items-center gap-3"
          >
            <span class="w-6 text-xs font-medium text-slate-400">{{ index + 1 }}</span>
            <div class="flex-1">
              <div class="flex items-center justify-between mb-1">
                <span class="text-sm font-medium text-slate-700 dark:text-slate-200">{{ client.ip }}</span>
                <span class="text-xs text-slate-500">{{ client.bytes }}</span>
              </div>
              <div class="h-1.5 bg-slate-100 dark:bg-dark-700 rounded-full overflow-hidden">
                <div
                  class="h-full bg-gradient-to-r from-primary-500 to-accent-500 rounded-full"
                  :style="{ width: `${client.percentage}%` }"
                ></div>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- Top Server -->
      <div class="card p-6">
        <div class="flex items-center justify-between mb-4">
          <h3 class="text-lg font-semibold text-slate-900 dark:text-white">Top Server</h3>
          <button class="text-xs text-primary-500 hover:text-primary-600 font-medium">查看全部</button>
        </div>
        <div class="space-y-3">
          <div
            v-for="(server, index) in topServers"
            :key="server.ip"
            class="flex items-center gap-3"
          >
            <span class="w-6 text-xs font-medium text-slate-400">{{ index + 1 }}</span>
            <div class="flex-1">
              <div class="flex items-center justify-between mb-1">
                <span class="text-sm font-medium text-slate-700 dark:text-slate-200">{{ server.ip }}</span>
                <span class="text-xs text-slate-500">{{ server.bytes }}</span>
              </div>
              <div class="h-1.5 bg-slate-100 dark:bg-dark-700 rounded-full overflow-hidden">
                <div
                  class="h-full bg-gradient-to-r from-accent-500 to-emerald-500 rounded-full"
                  :style="{ width: `${server.percentage}%` }"
                ></div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Session List -->
    <div class="card">
      <div class="flex items-center justify-between p-6 border-b border-slate-200 dark:border-dark-700">
        <h3 class="text-lg font-semibold text-slate-900 dark:text-white">五元组会话列表</h3>
        <div class="flex items-center gap-3">
          <input type="text" placeholder="搜索会话..." class="input max-w-xs" />
          <select class="input w-32">
            <option>全部协议</option>
            <option>TCP</option>
            <option>UDP</option>
          </select>
        </div>
      </div>
      <div class="overflow-x-auto">
        <table class="w-full">
          <thead>
            <tr class="bg-slate-50 dark:bg-dark-700/50">
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">源IP</th>
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">目的IP</th>
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">协议</th>
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">源端口</th>
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">目的端口</th>
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">时延(ms)</th>
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">包数</th>
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">字节数</th>
              <th class="text-left px-6 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">操作</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-slate-200 dark:divide-dark-700">
            <tr
              v-for="session in sessions"
              :key="session.id"
              class="hover:bg-slate-50 dark:hover:bg-dark-700/50 cursor-pointer transition-colors"
              @click="openSessionDetail(session)"
            >
              <td class="px-6 py-4 text-sm text-slate-900 dark:text-white">{{ session.srcIp }}</td>
              <td class="px-6 py-4 text-sm text-slate-900 dark:text-white">{{ session.dstIp }}</td>
              <td class="px-6 py-4">
                <span :class="['text-xs px-2 py-1 rounded-full font-medium', session.protocol === 'TCP' ? 'bg-blue-100 text-blue-600' : 'bg-green-100 text-green-600']">
                  {{ session.protocol }}
                </span>
              </td>
              <td class="px-6 py-4 text-sm text-slate-500">{{ session.srcPort }}</td>
              <td class="px-6 py-4 text-sm text-slate-500">{{ session.dstPort }}</td>
              <td class="px-6 py-4 text-sm" :class="session.latency > 100 ? 'text-red-500' : 'text-slate-900 dark:text-white'">{{ session.latency }}</td>
              <td class="px-6 py-4 text-sm text-slate-500">{{ session.packetCount }}</td>
              <td class="px-6 py-4 text-sm text-slate-500">{{ session.bytes }}</td>
              <td class="px-6 py-4">
                <button class="text-xs text-primary-500 hover:text-primary-600 font-medium">查看详情</button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- Session Detail Drawer -->
    <Transition name="drawer">
      <div
        v-if="selectedSession"
        class="fixed inset-0 z-50 flex justify-end"
        @click.self="selectedSession = null"
      >
        <div class="absolute inset-0 bg-slate-900/50"></div>
        <div class="relative w-full max-w-lg bg-white dark:bg-dark-800 shadow-2xl overflow-y-auto">
          <div class="sticky top-0 bg-white dark:bg-dark-800 border-b border-slate-200 dark:border-dark-700 px-6 py-4 flex items-center justify-between">
            <h3 class="text-lg font-semibold text-slate-900 dark:text-white">会话详情</h3>
            <button @click="selectedSession = null" class="p-2 hover:bg-slate-100 dark:hover:bg-dark-700 rounded-lg transition-colors">
              <X class="w-5 h-5 text-slate-500" />
            </button>
          </div>
          <div class="p-6 space-y-6">
            <div class="grid grid-cols-2 gap-4">
              <div class="p-4 bg-slate-50 dark:bg-dark-700 rounded-xl">
                <p class="text-xs text-slate-500 mb-1">源IP</p>
                <p class="text-sm font-medium text-slate-900 dark:text-white">{{ selectedSession.srcIp }}</p>
              </div>
              <div class="p-4 bg-slate-50 dark:bg-dark-700 rounded-xl">
                <p class="text-xs text-slate-500 mb-1">目的IP</p>
                <p class="text-sm font-medium text-slate-900 dark:text-white">{{ selectedSession.dstIp }}</p>
              </div>
              <div class="p-4 bg-slate-50 dark:bg-dark-700 rounded-xl">
                <p class="text-xs text-slate-500 mb-1">协议</p>
                <p class="text-sm font-medium text-slate-900 dark:text-white">{{ selectedSession.protocol }}</p>
              </div>
              <div class="p-4 bg-slate-50 dark:bg-dark-700 rounded-xl">
                <p class="text-xs text-slate-500 mb-1">时延</p>
                <p class="text-sm font-medium text-slate-900 dark:text-white">{{ selectedSession.latency }} ms</p>
              </div>
              <div class="p-4 bg-slate-50 dark:bg-dark-700 rounded-xl">
                <p class="text-xs text-slate-500 mb-1">RTT</p>
                <p class="text-sm font-medium text-slate-900 dark:text-white">{{ selectedSession.rtt }} ms</p>
              </div>
              <div class="p-4 bg-slate-50 dark:bg-dark-700 rounded-xl">
                <p class="text-xs text-slate-500 mb-1">重传率</p>
                <p class="text-sm font-medium text-slate-900 dark:text-white">{{ selectedSession.retransmissionRate }}%</p>
              </div>
            </div>
            <div>
              <h4 class="text-sm font-semibold text-slate-700 dark:text-slate-200 mb-3">流量时序图</h4>
              <div class="h-40 bg-slate-50 dark:bg-dark-700 rounded-xl p-4">
                <ECharts :option="sessionFlowOption" class="w-full h-full" />
              </div>
            </div>
          </div>
        </div>
      </div>
    </Transition>
  </div>
</template>

<script setup>
import { ref, computed } from 'vue'
import { use } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { LineChart, BarChart } from 'echarts/charts'
import { TooltipComponent, GridComponent } from 'echarts/components'
import ECharts from 'vue-echarts'
import { Network, Activity, TrendingUp, Users, Download, X } from 'lucide-vue-next'

use([CanvasRenderer, LineChart, BarChart, TooltipComponent, GridComponent])

const selectedSession = ref(null)

const topClients = ref([
  { ip: '192.168.1.100', bytes: '1.2 GB', percentage: 100 },
  { ip: '192.168.1.101', bytes: '856 MB', percentage: 71 },
  { ip: '192.168.1.102', bytes: '624 MB', percentage: 52 },
  { ip: '192.168.1.103', bytes: '448 MB', percentage: 37 },
  { ip: '192.168.1.104', bytes: '312 MB', percentage: 26 },
])

const topServers = ref([
  { ip: '10.0.0.1', bytes: '2.1 GB', percentage: 100 },
  { ip: '10.0.0.2', bytes: '1.5 GB', percentage: 71 },
  { ip: '10.0.0.3', bytes: '980 MB', percentage: 47 },
  { ip: '10.0.0.4', bytes: '650 MB', percentage: 31 },
  { ip: '10.0.0.5', bytes: '420 MB', percentage: 20 },
])

const sessions = ref([
  { id: 1, srcIp: '192.168.1.100', dstIp: '10.0.0.1', protocol: 'TCP', srcPort: '54321', dstPort: '80', latency: 23, packetCount: 1542, bytes: '12.5 MB', rtt: 45, retransmissionRate: 1.2 },
  { id: 2, srcIp: '192.168.1.101', dstIp: '10.0.0.2', protocol: 'TCP', srcPort: '55001', dstPort: '443', latency: 156, packetCount: 892, bytes: '8.2 MB', rtt: 312, retransmissionRate: 5.8 },
  { id: 3, srcIp: '192.168.1.102', dstIp: '10.0.0.1', protocol: 'UDP', srcPort: '34567', dstPort: '53', latency: 5, packetCount: 2341, bytes: '1.5 MB', rtt: 10, retransmissionRate: 0 },
  { id: 4, srcIp: '192.168.1.103', dstIp: '10.0.0.3', protocol: 'TCP', srcPort: '51234', dstPort: '8080', latency: 45, packetCount: 3421, bytes: '28.3 MB', rtt: 90, retransmissionRate: 2.1 },
  { id: 5, srcIp: '192.168.1.104', dstIp: '10.0.0.4', protocol: 'TCP', srcPort: '49876', dstPort: '3306', latency: 89, packetCount: 1876, bytes: '15.7 MB', rtt: 178, retransmissionRate: 3.5 },
])

const trafficTrendOption = computed(() => ({
  tooltip: { trigger: 'axis', backgroundColor: 'rgba(255,255,255,0.95)', textStyle: { color: '#1e293b' } },
  grid: { left: '3%', right: '4%', bottom: '3%', top: '10%', containLabel: true },
  xAxis: {
    type: 'category',
    data: ['00:00', '02:00', '04:00', '06:00', '08:00', '10:00', '12:00', '14:00', '16:00', '18:00', '20:00', '22:00'],
    axisLine: { lineStyle: { color: '#e2e8f0' } },
    axisLabel: { color: '#64748b', fontSize: 11 },
  },
  yAxis: [
    { type: 'value', name: '流量 (MB)', axisLabel: { color: '#64748b', fontSize: 11 }, splitLine: { lineStyle: { color: '#f1f5f9' } } },
    { type: 'value', name: 'PPS (K)', axisLabel: { color: '#64748b', fontSize: 11 }, splitLine: { show: false } },
  ],
  series: [
    { 
      name: '入站流量', 
      type: 'line', 
      smooth: true, 
      yAxisIndex: 0, 
      lineStyle: { color: '#2563eb', width: 2 }, 
      areaStyle: { 
        color: { 
          type: 'linear', 
          x: 0, y: 0, x2: 0, y2: 1, 
          colorStops: [
            { offset: 0, color: 'rgba(37,99,235,0.15)' }, 
            { offset: 1, color: 'rgba(37,99,235,0)' }
          ] 
        } 
      }, 
      data: [320, 450, 380, 520, 680, 850, 920, 880, 750, 620, 580, 420] 
    },
    { 
      name: '出站流量', 
      type: 'line', 
      smooth: true, 
      yAxisIndex: 0, 
      lineStyle: { color: '#14b8a6', width: 2 }, 
      areaStyle: { 
        color: { 
          type: 'linear', 
          x: 0, y: 0, x2: 0, y2: 1, 
          colorStops: [
            { offset: 0, color: 'rgba(20,184,166,0.15)' }, 
            { offset: 1, color: 'rgba(20,184,166,0)' }
          ] 
        } 
      }, 
      data: [280, 380, 320, 450, 580, 720, 780, 740, 630, 520, 480, 350] 
    },
    { 
      name: 'PPS', 
      type: 'line', 
      smooth: true, 
      yAxisIndex: 1, 
      lineStyle: { color: '#f59e0b', width: 2, type: 'dashed' }, 
      data: [12, 18, 14, 22, 28, 35, 38, 36, 31, 26, 24, 18] 
    },
  ],
}))

const sessionFlowOption = computed(() => ({
  tooltip: { trigger: 'axis', backgroundColor: 'rgba(255,255,255,0.95)', textStyle: { color: '#1e293b' } },
  grid: { left: '3%', right: '4%', bottom: '3%', top: '3%', containLabel: true },
  xAxis: { type: 'category', data: ['0s', '5s', '10s', '15s', '20s', '25s', '30s'], axisLabel: { color: '#64748b', fontSize: 10 } },
  yAxis: { type: 'value', axisLabel: { color: '#64748b', fontSize: 10 }, splitLine: { lineStyle: { color: '#f1f5f9' } } },
  series: [
    { name: '入站', type: 'bar', itemStyle: { color: '#2563eb', borderRadius: [4, 4, 0, 0] }, data: [120, 150, 90, 180, 140, 200, 160] },
    { name: '出站', type: 'bar', itemStyle: { color: '#14b8a6', borderRadius: [4, 4, 0, 0] }, data: [80, 120, 70, 140, 100, 160, 120] },
  ],
}))

const openSessionDetail = (session) => {
  selectedSession.value = session
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
