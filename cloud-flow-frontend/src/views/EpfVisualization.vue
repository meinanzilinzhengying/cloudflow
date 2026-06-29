<template>
  <div class="epf-viz">
    <div class="page-header">
      <h2 class="page-title">eBPF 数据可视化</h2>
      <div class="header-actions">
        <el-select v-model="timeRange" placeholder="时间范围" size="small" style="width: 120px" @change="fetchAllData">
          <el-option label="最近5分钟" value="5m" />
          <el-option label="最近30分钟" value="30m" />
          <el-option label="最近1小时" value="1h" />
          <el-option label="最近6小时" value="6h" />
        </el-select>
        <el-select v-model="probeFilter" placeholder="探针" size="small" clearable style="width: 140px" @change="fetchAllData">
          <el-option label="全部" value="" />
          <el-option v-for="p in probes" :key="p" :label="p" :value="p" />
        </el-select>
        <el-switch v-model="autoRefresh" active-text="自动刷新" size="small" />
      </div>
    </div>

    <el-row :gutter="16" class="stats-row">
      <el-col :span="4" v-for="(stat, i) in statCards" :key="i">
        <el-card shadow="hover" class="stats-card" :body-style="{ padding: '16px' }">
          <div class="stat-icon" :style="{ background: stat.bg }">
            <el-icon :size="24" :color="stat.color"><component :is="stat.icon" /></el-icon>
          </div>
          <div class="stat-value">{{ stat.value }}</div>
          <div class="stat-label">{{ stat.label }}</div>
        </el-card>
      </el-col>
    </el-row>

    <el-row :gutter="16" class="chart-row">
      <el-col :span="16">
        <el-card class="chart-card" :body-style="{ padding: '16px' }">
          <div class="card-title">流量趋势</div>
          <v-chart :option="trendOption" autoresize style="height: 320px" />
        </el-card>
      </el-col>
      <el-col :span="8">
        <el-card class="chart-card" :body-style="{ padding: '16px' }">
          <div class="card-title">协议分布</div>
          <v-chart :option="protocolPieOption" autoresize style="height: 320px" />
        </el-card>
      </el-col>
    </el-row>

    <el-row :gutter="16" class="chart-row">
      <el-col :span="12">
        <el-card class="chart-card" :body-style="{ padding: '16px' }">
          <div class="card-title">Top 目标端口</div>
          <v-chart :option="topPortsOption" autoresize style="height: 280px" />
        </el-card>
      </el-col>
      <el-col :span="12">
        <el-card class="chart-card" :body-style="{ padding: '16px' }">
          <div class="card-title">采集器事件分布</div>
          <v-chart :option="collectorOption" autoresize style="height: 280px" />
        </el-card>
      </el-col>
    </el-row>

    <el-row :gutter="16">
      <el-col :span="12">
        <el-card class="table-card" :body-style="{ padding: '16px' }">
          <div class="card-title">最近 HTTP 请求</div>
          <el-table :data="httpLogs" size="small" style="width: 100%" height="320" stripe>
            <el-table-column prop="timestamp" label="时间" width="155" :formatter="fmtTime" />
            <el-table-column prop="src_ip" label="源IP" width="120" />
            <el-table-column prop="dst_ip" label="目标IP" width="120" />
            <el-table-column prop="dst_port" label="端口" width="60" />
            <el-table-column prop="bytes" label="字节" width="80" :formatter="(r: any) => fmtBytes(r.bytes)" />
            <el-table-column prop="details" label="详情" min-width="160" show-overflow-tooltip />
          </el-table>
        </el-card>
      </el-col>
      <el-col :span="12">
        <el-card class="table-card" :body-style="{ padding: '16px' }">
          <div class="card-title">最近网络流量</div>
          <el-table :data="networkLogs" size="small" style="width: 100%" height="320" stripe>
            <el-table-column prop="timestamp" label="时间" width="155" :formatter="fmtTime" />
            <el-table-column prop="src_ip" label="源IP" width="120" />
            <el-table-column prop="dst_ip" label="目标IP" width="120" />
            <el-table-column prop="protocol" label="协议" width="60" />
            <el-table-column prop="dst_port" label="端口" width="60" />
            <el-table-column prop="bytes" label="字节" width="80" :formatter="(r: any) => fmtBytes(r.bytes)" />
            <el-table-column prop="packets" label="包数" width="60" />
          </el-table>
        </el-card>
      </el-col>
    </el-row>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, shallowRef } from 'vue'
import VChart from 'vue-echarts'
import { use } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { LineChart, PieChart, BarChart } from 'echarts/charts'
import { GridComponent, TooltipComponent, LegendComponent, DataZoomComponent } from 'echarts/components'
import { queryClickHouse, PROBE_FILTER } from '@/api/stb'
import { Monitor, Connection, Cpu, Document, Timer } from '@element-plus/icons-vue'

use([CanvasRenderer, LineChart, PieChart, BarChart, GridComponent, TooltipComponent, LegendComponent, DataZoomComponent])

const timeRange = ref('30m')
const probeFilter = ref('')
const autoRefresh = ref(false)
let refreshTimer: number | null = null

const probes = ref<string[]>([])
const totalEvents = ref(0)
const totalBytes = ref(0)
const httpCount = ref(0)
const tcpCount = ref(0)
const udpCount = ref(0)
const httpLogs = ref<any[]>([])
const networkLogs = ref<any[]>([])

const statCards = computed(() => [
  { label: '总事件数', value: totalEvents.value.toLocaleString(), icon: Monitor, color: '#00CCFF', bg: 'rgba(0,204,255,0.15)' },
  { label: '总流量', value: fmtBytes(totalBytes.value), icon: Connection, color: '#6BEDB7', bg: 'rgba(107,237,183,0.15)' },
  { label: 'HTTP 请求数', value: httpCount.value.toLocaleString(), icon: Document, color: '#FFA940', bg: 'rgba(255,169,64,0.15)' },
  { label: 'TCP 连接数', value: tcpCount.value.toLocaleString(), icon: Cpu, color: '#FF6B6B', bg: 'rgba(255,107,107,0.15)' },
  { label: 'UDP 连接数', value: udpCount.value.toLocaleString(), icon: Timer, color: '#B37FEB', bg: 'rgba(179,127,235,0.15)' },
  { label: '采集器数', value: '6', icon: Monitor, color: '#36CFC9', bg: 'rgba(54,207,201,0.15)' },
])

function fmtBytes(b: number): string {
  if (b >= 1073741824) return (b / 1073741824).toFixed(1) + ' GB'
  if (b >= 1048576) return (b / 1048576).toFixed(1) + ' MB'
  if (b >= 1024) return (b / 1024).toFixed(1) + ' KB'
  return b + ' B'
}
function fmtTime(row: any) {
  if (!row.timestamp) return ''
  return String(row.timestamp).replace('T', ' ').substring(0, 19)
}

const darkTooltip = { backgroundColor: 'rgba(5,56,90,0.95)', borderColor: '#0ABAFF', textStyle: { color: '#fff' } }
const axisStyle = { axisLine: { lineStyle: { color: 'rgba(255,255,255,0.3)' } }, axisLabel: { color: '#fff' }, splitLine: { lineStyle: { color: 'rgba(255,255,255,0.08)' } } }

const trendOption = shallowRef({
  backgroundColor: 'transparent',
  tooltip: { ...darkTooltip, trigger: 'axis' },
  dataZoom: [{ type: 'inside' }, { type: 'slider', height: 20, bottom: 5 }],
  grid: { left: 60, right: 20, top: 20, bottom: 50 },
  xAxis: { type: 'category', data: [] as string[], ...axisStyle },
  yAxis: { type: 'value', ...axisStyle, axisLabel: { color: '#fff', formatter: (v: number) => fmtBytes(v) } },
  series: [
    { name: '上行', type: 'line', smooth: true, areaStyle: { color: { type: 'linear', x: 0, y: 0, x2: 0, y2: 1, colorStops: [{ offset: 0, color: 'rgba(0,204,255,0.35)' }, { offset: 1, color: 'rgba(0,204,255,0.02)' }] } }, itemStyle: { color: '#00CCFF' }, lineStyle: { color: '#00CCFF', width: 2 }, data: [] as number[] },
    { name: '下行', type: 'line', smooth: true, areaStyle: { color: { type: 'linear', x: 0, y: 0, x2: 0, y2: 1, colorStops: [{ offset: 0, color: 'rgba(107,237,183,0.35)' }, { offset: 1, color: 'rgba(107,237,183,0.02)' }] } }, itemStyle: { color: '#6BEDB7' }, lineStyle: { color: '#6BEDB7', width: 2 }, data: [] as number[] },
  ]
})

const protocolPieOption = shallowRef({
  backgroundColor: 'transparent',
  tooltip: { ...darkTooltip, trigger: 'item', formatter: '{b}: {c} ({d}%)' },
  legend: { orient: 'vertical', right: 10, top: 'center', textStyle: { color: '#fff' } },
  series: [{
    type: 'pie', radius: ['40%', '70%'], center: ['40%', '50%'],
    itemStyle: { borderRadius: 6, borderColor: '#05385A', borderWidth: 2 },
    label: { show: false },
    emphasis: { label: { show: true, color: '#fff', fontSize: 14 } },
    data: [] as { name: string; value: number }[],
  }]
})

const topPortsOption = shallowRef({
  backgroundColor: 'transparent',
  tooltip: { ...darkTooltip, trigger: 'axis' },
  grid: { left: 60, right: 20, top: 10, bottom: 30 },
  xAxis: { type: 'value', ...axisStyle },
  yAxis: { type: 'category', data: [] as string[], ...axisStyle, inverse: true },
  series: [{
    type: 'bar', barWidth: 18,
    itemStyle: { borderRadius: [0, 4, 4, 0], color: { type: 'linear', x: 0, y: 0, x2: 1, y2: 0, colorStops: [{ offset: 0, color: '#0ABAFF' }, { offset: 1, color: '#00CCFF' }] } },
    data: [] as number[],
  }]
})

const collectorOption = shallowRef({
  backgroundColor: 'transparent',
  tooltip: { ...darkTooltip, trigger: 'item', formatter: '{b}: {c}' },
  legend: { orient: 'vertical', right: 10, top: 'center', textStyle: { color: '#fff', fontSize: 11 } },
  series: [{
    type: 'pie', radius: '65%', center: ['40%', '50%'],
    roseType: 'area',
    itemStyle: { borderRadius: 6, borderColor: '#05385A', borderWidth: 2 },
    label: { show: false },
    data: [] as { name: string; value: number }[],
  }]
})

function probeClause(): string {
  return PROBE_FILTER || (probeFilter.value ? ` AND probe_id = '${probeFilter.value}'` : '')
}
function timeClause(): string {
  return ` AND timestamp > now() - INTERVAL ${timeRange.value}`
}

async function fetchProbes() {
  const r = await queryClickHouse('SELECT DISTINCT probe_id FROM cloudflow.ebpf_events ORDER BY probe_id')
  probes.value = r.map((x: any) => x.probe_id).filter(Boolean)
}

async function fetchStats() {
  const where = `WHERE 1=1${probeClause()}${timeClause()}`
  const r = await queryClickHouse(
    `SELECT count() as total, sum(bytes) as tb, countIf(event_type='http') as http, countIf(protocol='TCP') as tcp, countIf(protocol='UDP') as udp FROM cloudflow.ebpf_events ${where}`
  )
  if (r.length > 0) {
    totalEvents.value = Number(r[0].total) || 0
    totalBytes.value = Number(r[0].tb) || 0
    httpCount.value = Number(r[0].http) || 0
    tcpCount.value = Number(r[0].tcp) || 0
    udpCount.value = Number(r[0].udp) || 0
  }
}

async function fetchTrend() {
  const where = `WHERE 1=1${probeClause()}${timeClause()}`
  const r = await queryClickHouse(
    `SELECT toStartOfMinute(timestamp) as t, sum(bytes) as bytes FROM cloudflow.ebpf_events ${where} GROUP BY t ORDER BY t`
  )
  const times = r.map((x: any) => String(x.t).substring(11, 16))
  const bytes = r.map((x: any) => Number(x.bytes) || 0)
  trendOption.value = { ...trendOption.value, xAxis: { ...trendOption.value.xAxis, data: times }, series: [{ ...trendOption.value.series[0], data: bytes }, { ...trendOption.value.series[1], data: bytes.map((v: number) => Math.round(v * 0.6)) }] }
}

async function fetchProtocolPie() {
  const where = `WHERE protocol != ''${probeClause()}${timeClause()}`
  const r = await queryClickHouse(
    `SELECT protocol, count() as cnt FROM cloudflow.ebpf_events ${where} GROUP BY protocol ORDER BY cnt DESC`
  )
  const colors = ['#00CCFF', '#6BEDB7', '#FFA940', '#FF6B6B', '#B37FEB', '#36CFC9', '#FFC53D']
  protocolPieOption.value = { ...protocolPieOption.value, series: [{ ...protocolPieOption.value.series[0], data: r.map((x: any, i: number) => ({ name: x.protocol, value: Number(x.cnt), itemStyle: { color: colors[i % colors.length] } })) }] }
}

async function fetchTopPorts() {
  const where = `WHERE dst_port > 0${probeClause()}${timeClause()}`
  const r = await queryClickHouse(
    `SELECT toString(dst_port) as port, count() as cnt FROM cloudflow.ebpf_events ${where} GROUP BY port ORDER BY cnt DESC LIMIT 8`
  )
  topPortsOption.value = { ...topPortsOption.value, yAxis: { ...topPortsOption.value.yAxis, data: r.map((x: any) => x.port).reverse() }, series: [{ ...topPortsOption.value.series[0], data: r.map((x: any) => Number(x.cnt)).reverse() }] }
}

async function fetchCollectors() {
  const where = `WHERE 1=1${probeClause()}${timeClause()}`
  const r = await queryClickHouse(
    `SELECT category, count() as cnt FROM cloudflow.ebpf_events ${where} GROUP BY category ORDER BY cnt DESC`
  )
  const colors = ['#00CCFF', '#6BEDB7', '#FFA940', '#FF6B6B', '#B37FEB', '#36CFC9', '#FFC53D']
  collectorOption.value = { ...collectorOption.value, series: [{ ...collectorOption.value.series[0], data: r.map((x: any, i: number) => ({ name: x.category, value: Number(x.cnt), itemStyle: { color: colors[i % colors.length] } })) }] }
}

async function fetchHttpLogs() {
  const where = `WHERE event_type = 'http'${probeClause()}${timeClause()}`
  httpLogs.value = await queryClickHouse(
    `SELECT timestamp, src_ip, dst_ip, dst_port, bytes, details FROM cloudflow.ebpf_events ${where} ORDER BY timestamp DESC LIMIT 20`
  )
}

async function fetchNetworkLogs() {
  const where = `WHERE event_type = 'flow'${probeClause()}${timeClause()}`
  networkLogs.value = await queryClickHouse(
    `SELECT timestamp, src_ip, dst_ip, protocol, dst_port, bytes, packets FROM cloudflow.ebpf_events ${where} ORDER BY timestamp DESC LIMIT 20`
  )
}

async function fetchAllData() {
  await Promise.all([
    fetchStats(), fetchTrend(), fetchProtocolPie(),
    fetchTopPorts(), fetchCollectors(), fetchHttpLogs(), fetchNetworkLogs()
  ])
}

onMounted(() => {
  fetchProbes()
  fetchAllData()
})

onUnmounted(() => {
  if (refreshTimer !== null) clearInterval(refreshTimer)
})

import { watch } from 'vue'
watch(autoRefresh, v => {
  if (v) refreshTimer = window.setInterval(fetchAllData, 15000)
  else { if (refreshTimer !== null) { clearInterval(refreshTimer); refreshTimer = null } }
})
</script>

<style scoped lang="scss">
.epf-viz {
  padding: 20px;
}
.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 20px;
  .page-title {
    font-size: 18px;
    font-weight: 600;
    color: #FFFFFF;
  }
  .header-actions {
    display: flex;
    gap: 12px;
    align-items: center;
  }
}
.stats-row {
  margin-bottom: 16px;
}
.stats-card {
  border-radius: 8px;
  background: rgba(255, 255, 255, 0.03);
  border: 1px solid rgba(255, 255, 255, 0.06);
  text-align: center;
  .stat-icon {
    width: 44px;
    height: 44px;
    border-radius: 10px;
    display: flex;
    align-items: center;
    justify-content: center;
    margin: 0 auto 10px;
  }
  .stat-value {
    font-size: 22px;
    font-weight: 700;
    color: #FFFFFF;
    font-family: 'Arial', sans-serif;
  }
  .stat-label {
    font-size: 12px;
    color: rgba(255, 255, 255, 0.5);
    margin-top: 4px;
  }
}
.chart-row {
  margin-bottom: 16px;
}
.chart-card, .table-card {
  border-radius: 8px;
  background: rgba(255, 255, 255, 0.03);
  border: 1px solid rgba(255, 255, 255, 0.06);
  margin-bottom: 16px;
  .card-title {
    font-size: 14px;
    font-weight: 600;
    color: rgba(255, 255, 255, 0.9);
    margin-bottom: 12px;
  }
}
</style>
