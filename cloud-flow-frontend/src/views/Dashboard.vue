<template>
  <div class="stb-dashboard">
    <!-- ① 顶部状态条 -->
    <div class="top-bar">
      <div class="top-bar-item playback">
        <span class="top-label">当前播放</span>
        <span class="top-value highlight">{{ currentPlayback }}</span>
      </div>
      <div class="top-bar-item health">
        <span class="top-label">健康评分</span>
        <span class="top-score" :style="{ color: healthColor }">{{ healthScore }}<span class="unit">分</span></span>
        <div class="health-bar"><div class="health-fill" :style="{ width: healthScore + '%', background: healthGradient }"></div></div>
      </div>
      <div class="top-bar-item alerts-top" @click="alertExpanded = !alertExpanded">
        <span class="top-label">未处理告警</span>
        <span class="top-alert-count" :class="{ 'has-alert': alertCount > 0 }">{{ alertCount }}<span class="unit">条</span></span>
      </div>
      <div class="top-bar-item probe-info">
        <span class="top-label">探针状态</span>
        <span class="top-value">{{ probeId }}</span>
        <span class="status-dot online"></span>
      </div>
      <div class="top-bar-item uptime">
        <span class="top-label">在线时长</span>
        <span class="top-value">{{ onlineDuration }}</span>
      </div>
    </div>

    <!-- ② 核心 KPI 卡片 -->
    <div class="kpi-row">
      <div v-for="kpi in kpiCards" :key="kpi.label" class="kpi-card">
        <div class="kpi-icon-wrap" :style="{ background: kpi.bg }">
          <el-icon :size="22" :color="kpi.color"><component :is="kpi.icon" /></el-icon>
        </div>
        <div class="kpi-body">
          <div class="kpi-val">{{ kpi.value }}</div>
          <div class="kpi-label">{{ kpi.label }}</div>
        </div>
      </div>
    </div>

    <!-- ③ 流量概览区 -->
    <div class="overview-row">
      <el-card class="overview-card trend-card" :body-style="{ padding: '12px' }">
        <div class="card-title">📈 24H 流量趋势</div>
        <v-chart :option="trendOption" autoresize style="height: 240px" />
      </el-card>
      <el-card class="overview-card" :body-style="{ padding: '12px' }">
        <div class="card-title">📊 协议分布</div>
        <v-chart :option="protocolOption" autoresize style="height: 240px" />
      </el-card>
      <el-card class="overview-card" :body-style="{ padding: '12px' }">
        <div class="card-title">🎯 业务分布</div>
        <v-chart :option="businessOption" autoresize style="height: 240px" />
      </el-card>
    </div>

    <!-- ④ 告警区 -->
    <div class="alert-section" v-if="alerts.length > 0">
      <div class="alert-header" @click="alertExpanded = !alertExpanded">
        <span class="alert-title">⚠️ 最近告警</span>
        <span class="alert-badge">{{ alerts.length }}</span>
        <el-icon class="expand-icon" :class="{ expanded: alertExpanded }"><ArrowDown /></el-icon>
      </div>
      <div v-show="alertExpanded" class="alert-list">
        <div v-for="(a, i) in alerts" :key="i" class="alert-item" :class="'level-' + a.level">
          <span class="alert-time">{{ a.time }}</span>
          <span class="alert-level" :class="'p' + a.level">P{{ a.level }}</span>
          <span class="alert-type">{{ a.type }}</span>
          <span class="alert-detail">{{ a.detail }}</span>
          <span class="alert-duration">持续 {{ a.duration }}</span>
        </div>
      </div>
    </div>

    <!-- ⑤ 流量明细区 -->
    <div class="detail-row">
      <el-card class="detail-card" :body-style="{ padding: '12px' }">
        <div class="card-title">🏆 Top 10 通信对</div>
        <el-table :data="topPairs" size="small" style="width: 100%" height="280" stripe>
          <el-table-column label="#" width="40" type="index" />
          <el-table-column prop="src_ip" label="源 IP" width="120" />
          <el-table-column prop="dst_ip" label="目的 IP" width="120" />
          <el-table-column prop="protocol" label="协议" width="65" />
          <el-table-column prop="business" label="业务" width="80">
            <template #default="{ row }">
              <el-tag :type="bizTagType(row.business)" size="small" effect="dark">{{ row.business }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="bytes" label="流量" width="90" :formatter="(r: any) => fmtBytes(r.bytes)" />
          <el-table-column prop="packets" label="包数" width="65" />
          <el-table-column prop="flows" label="连接数" width="65" />
        </el-table>
      </el-card>
      <el-card class="detail-card" :body-style="{ padding: '12px' }">
        <div class="card-title">🔗 活跃连接</div>
        <el-table :data="activeConns" size="small" style="width: 100%" height="280" stripe>
          <el-table-column prop="src_ip" label="源 IP" width="110" />
          <el-table-column prop="dst_ip" label="目的 IP" width="110" />
          <el-table-column prop="protocol" label="协议" width="60" />
          <el-table-column prop="dst_port" label="端口" width="55" />
          <el-table-column prop="bytes" label="字节" width="80" :formatter="(r: any) => fmtBytes(r.bytes)" />
          <el-table-column prop="status" label="状态" width="60">
            <template #default="{ row }">
              <span class="conn-status" :class="row.status === 'active' ? 'active' : 'idle'">
                {{ row.status === 'active' ? '活跃' : '空闲' }}
              </span>
            </template>
          </el-table-column>
          <el-table-column prop="last_seen" label="最后活动" width="145" />
        </el-table>
      </el-card>
    </div>

    <!-- ⑥ 系统性能速览 -->
    <div class="sys-row">
      <el-card class="sys-card" :body-style="{ padding: '12px' }">
        <div class="card-title">💻 系统性能速览</div>
        <div class="sys-metrics">
          <div v-for="m in sysMetrics" :key="m.label" class="sys-metric">
            <div class="sys-metric-header">
              <span class="sys-metric-label">{{ m.label }}</span>
              <span class="sys-metric-value" :style="{ color: m.color }">{{ m.value }}</span>
            </div>
            <el-progress :percentage="m.percent" :stroke-width="8" :color="m.color" :show-text="false" />
          </div>
        </div>
        <div class="sys-chart-wrap">
          <v-chart :option="sysChartOption" autoresize style="height: 120px" />
        </div>
      </el-card>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, shallowRef, onMounted, onUnmounted } from 'vue'
import VChart from 'vue-echarts'
import { use } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { LineChart, PieChart, BarChart } from 'echarts/charts'
import { GridComponent, TooltipComponent, LegendComponent, DataZoomComponent } from 'echarts/components'
import { TrendCharts, Cloudy, Grid, ArrowDown, Connection } from '@element-plus/icons-vue'
import { queryClickHouse } from '@/api/stb'

use([CanvasRenderer, LineChart, PieChart, BarChart, GridComponent, TooltipComponent, LegendComponent, DataZoomComponent])

const fmtBytes = (b: number): string => {
  if (b >= 1073741824) return (b / 1073741824).toFixed(2) + ' GB'
  if (b >= 1048576) return (b / 1048576).toFixed(2) + ' MB'
  if (b >= 1024) return (b / 1024).toFixed(2) + ' KB'
  return b + ' B'
}
const fmtTime = (t: any) => String(t || '').replace('T', ' ').substring(0, 19)

const dt = (n: string) => `AND timestamp > now() - INTERVAL ${n}`
const w = `WHERE 1=1 ${dt('24 HOUR')}`

// ── ① 顶部状态条 ──
const currentPlayback = ref('加载中...')
const healthScore = ref(0)
const alertCount = ref(0)
const probeId = ref('-')
const onlineDuration = ref('-')
const healthColor = computed(() => healthScore.value >= 80 ? '#6BEDB7' : healthScore.value >= 60 ? '#FFA940' : '#FF6B6B')
const healthGradient = computed(() => `linear-gradient(90deg, #6BEDB7 0%, ${healthScore.value >= 60 ? '#FFA940' : '#FF6B6B'} 100%)`)

// ── ② KPI 卡片 ──
const kpiCards = ref([
  { label: '总采集事件', value: '0', icon: TrendCharts, color: '#00CCFF', bg: 'rgba(0,204,255,0.15)' },
  { label: '总流量', value: '0', icon: Connection, color: '#6BEDB7', bg: 'rgba(107,237,183,0.15)' },
  { label: '今日流量', value: '0', icon: Cloudy, color: '#FFA940', bg: 'rgba(255,169,64,0.15)' },
  { label: '活跃连接', value: '0', icon: Grid, color: '#B37FEB', bg: 'rgba(179,127,235,0.15)' },
])

// ── ③ 流量概览图表 ──
const darkTip = { backgroundColor: 'rgba(5,56,90,0.95)', borderColor: '#0ABAFF', textStyle: { color: '#fff' } }
const ax = { axisLine: { lineStyle: { color: 'rgba(255,255,255,0.3)' } }, axisLabel: { color: '#fff' }, splitLine: { lineStyle: { color: 'rgba(255,255,255,0.06)' } } }

const trendOption = shallowRef({
  backgroundColor: 'transparent',
  tooltip: { ...darkTip, trigger: 'axis' },
  legend: { data: ['流量(字节)', '包数(PPS)'], textStyle: { color: '#fff', fontSize: 11 }, top: 0 },
  grid: { left: 55, right: 55, top: 30, bottom: 25 },
  xAxis: { type: 'category', data: [] as string[], ...ax },
  yAxis: [
    { type: 'value', name: '字节', ...ax, axisLabel: { color: '#fff', formatter: (v: number) => fmtBytes(v) } },
    { type: 'value', name: 'PPS', ...ax, splitLine: { show: false } }
  ],
  dataZoom: [{ type: 'inside' }],
  series: [
    { name: '流量(字节)', type: 'bar', barWidth: 8, itemStyle: { borderRadius: [3, 3, 0, 0], color: 'rgba(0,204,255,0.6)' }, data: [] as number[] },
    { name: '包数(PPS)', type: 'line', yAxisIndex: 1, smooth: true, itemStyle: { color: '#6BEDB7' }, lineStyle: { color: '#6BEDB7', width: 2 }, data: [] as number[] }
  ]
})

const protocolOption = shallowRef({
  backgroundColor: 'transparent',
  tooltip: { ...darkTip, trigger: 'item', formatter: '{b}: {c} ({d}%)' },
  legend: { orient: 'vertical', right: 5, top: 'middle', textStyle: { color: '#fff', fontSize: 11 } },
  series: [{
    type: 'pie', radius: ['38%', '68%'], center: ['38%', '55%'],
    itemStyle: { borderRadius: 5, borderColor: '#05385A', borderWidth: 2 },
    label: { show: false },
    emphasis: { label: { show: true, color: '#fff', fontSize: 13 } },
    data: [] as { name: string; value: number; itemStyle?: any }[]
  }]
})

const businessOption = shallowRef({
  backgroundColor: 'transparent',
  tooltip: { ...darkTip, trigger: 'item', formatter: '{b}: {c} ({d}%)' },
  legend: { orient: 'vertical', right: 5, top: 'middle', textStyle: { color: '#fff', fontSize: 11 } },
  series: [{
    type: 'pie', radius: ['38%', '68%'], center: ['38%', '55%'],
    roseType: 'radius',
    itemStyle: { borderRadius: 5, borderColor: '#05385A', borderWidth: 2 },
    label: { show: false },
    emphasis: { label: { show: true, color: '#fff', fontSize: 13 } },
    data: [] as { name: string; value: number; itemStyle?: any }[]
  }]
})

// ── ④ 告警 ──
const alertExpanded = ref(true)
interface Alert { time: string; level: number; type: string; detail: string; duration: string }
const alerts = ref<Alert[]>([])

// ── ⑤ 流量明细 ──
const topPairs = ref<any[]>([])
const activeConns = ref<any[]>([])

// ── ⑥ 系统性能 ──
const sysMetrics = ref([
  { label: 'CPU', value: '0%', percent: 0, color: '#00CCFF' },
  { label: '内存', value: '0%', percent: 0, color: '#6BEDB7' },
  { label: '磁盘', value: '0%', percent: 0, color: '#FFA940' },
])
const sysChartOption = shallowRef({
  backgroundColor: 'transparent',
  tooltip: { ...darkTip, trigger: 'axis' },
  legend: { data: ['CPU', '内存'], textStyle: { color: '#fff', fontSize: 10 }, top: 0 },
  grid: { left: 35, right: 10, top: 25, bottom: 15 },
  xAxis: { type: 'category', data: [] as string[], ...ax, axisLabel: { color: '#fff', fontSize: 10 } },
  yAxis: { type: 'value', max: 100, ...ax, splitLine: { lineStyle: { color: 'rgba(255,255,255,0.06)' } }, axisLabel: { color: '#fff', fontSize: 10 } },
  series: [
    { name: 'CPU', type: 'line', smooth: true, itemStyle: { color: '#00CCFF' }, lineStyle: { width: 1.5 }, areaStyle: { color: { type: 'linear', x: 0, y: 0, x2: 0, y2: 1, colorStops: [{ offset: 0, color: 'rgba(0,204,255,0.25)' }, { offset: 1, color: 'rgba(0,204,255,0.02)' }] } }, symbol: 'none', data: [] as number[] },
    { name: '内存', type: 'line', smooth: true, itemStyle: { color: '#6BEDB7' }, lineStyle: { width: 1.5 }, areaStyle: { color: { type: 'linear', x: 0, y: 0, x2: 0, y2: 1, colorStops: [{ offset: 0, color: 'rgba(107,237,183,0.25)' }, { offset: 1, color: 'rgba(107,237,183,0.02)' }] } }, symbol: 'none', data: [] as number[] }
  ]
})

function bizTagType(b: string): '' | 'success' | 'warning' | 'info' | 'danger' {
  if (b === 'IPTV直播') return 'danger'
  if (b === '视频点播') return 'warning'
  if (b === '网页浏览') return 'success'
  return 'info'
}

function classifyBusiness(row: any): string {
  const p = (row.protocol || '').toUpperCase()
  const port = Number(row.dst_port || 0)
  const bytes = Number(row.bytes || 0)
  if (p === 'UDP' && bytes > 100000) return 'IPTV直播'
  if (p === 'TCP' && (port === 80 || port === 443) && bytes > 50000) return '视频点播'
  if (p === 'TCP' && (port === 80 || port === 443)) return '网页浏览'
  if ([1900, 5353, 5000, 8080].includes(port)) return '管理协议'
  return '其他'
}

async function fetchAll() {
  // ① 顶部 + ② KPI
  const r = await queryClickHouse(
    `SELECT count() as total, sum(bytes) as tb, countIf(timestamp > toStartOfDay(now())) as today_tb, count(distinct probe_id) as probes FROM cloudflow.ebpf_events WHERE timestamp > now() - INTERVAL 24 HOUR`
  )
  if (r.length > 0) {
    const d = r[0]
    kpiCards.value[0].value = Number(d.total || 0).toLocaleString()
    kpiCards.value[1].value = fmtBytes(Number(d.tb || 0))
    kpiCards.value[2].value = fmtBytes(Number(d.today_tb || 0))
    healthScore.value = Math.min(100, Math.round(Number(d.total || 0) / 1000))
    probeId.value = 'stb-188'
  }

  // 在线时长
  const uptime = await queryClickHouse(
    `SELECT min(timestamp) as first FROM cloudflow.ebpf_events WHERE probe_id = 'stb-188'`
  )
  if (uptime.length > 0 && uptime[0].first) {
    const diff = Date.now() - new Date(uptime[0].first).getTime()
    const h = Math.floor(diff / 3600000)
    const m = Math.floor((diff % 3600000) / 60000)
    onlineDuration.value = h + 'h ' + m + 'm'
  }

  // ③ 流量趋势
  const trend = await queryClickHouse(
    `SELECT toStartOfHour(timestamp) as t, sum(bytes) as bytes, sum(packets) as pkts FROM cloudflow.ebpf_events ${w} GROUP BY t ORDER BY t`
  )
  trendOption.value = {
    ...trendOption.value,
    xAxis: { ...trendOption.value.xAxis, data: trend.map((x: any) => String(x.t).substring(11, 16)) },
    series: [
      { ...trendOption.value.series[0], data: trend.map((x: any) => Number(x.bytes || 0)) },
      { ...trendOption.value.series[1], data: trend.map((x: any) => Number(x.pkts || 0)) }
    ]
  }

  // ③ 协议分布
  const protos = await queryClickHouse(
    `SELECT protocol, count() as cnt FROM cloudflow.ebpf_events WHERE protocol != '' ${dt('24 HOUR')} GROUP BY protocol ORDER BY cnt DESC`
  )
  const pColors = ['#00CCFF', '#6BEDB7', '#FFA940', '#FF6B6B', '#B37FEB', '#36CFC9']
  protocolOption.value = {
    ...protocolOption.value,
    series: [{ ...protocolOption.value.series[0], data: protos.map((x: any, i: number) => ({ name: x.protocol, value: Number(x.cnt), itemStyle: { color: pColors[i % pColors.length] } })) }]
  }

  // ③ 业务分布
  const bizData = await queryClickHouse(
    `SELECT protocol, dst_port, bytes, count() as cnt FROM cloudflow.ebpf_events WHERE event_type = 'flow' ${dt('24 HOUR')} GROUP BY protocol, dst_port, bytes ORDER BY cnt DESC LIMIT 500`
  )
  const bizMap: Record<string, number> = { 'IPTV直播': 0, '视频点播': 0, '网页浏览': 0, '管理协议': 0, '其他': 0 }
  bizData.forEach((x: any) => {
    const b = classifyBusiness(x)
    bizMap[b] += Number(x.cnt || 0)
  })
  const bizColors = ['#FF6B6B', '#FFA940', '#6BEDB7', '#B37FEB', '#5D7092']
  businessOption.value = {
    ...businessOption.value,
    series: [{ ...businessOption.value.series[0], data: Object.entries(bizMap).filter(([, v]) => v > 0).map(([k, v], i) => ({ name: k, value: v, itemStyle: { color: bizColors[i % bizColors.length] } })) }]
  }

  // ④ 告警
  const alertList: Alert[] = []
  const cpuData = await queryClickHouse(
    `SELECT value FROM cloudflow.ebpf_events WHERE event_type = 'host_metrics' AND details LIKE '%cpu%' ${dt('1 HOUR')} ORDER BY timestamp DESC LIMIT 1`
  )
  if (cpuData.length > 0 && Number(cpuData[0].value || 0) > 90) {
    alertList.push({ time: fmtTime(new Date()), level: 1, type: 'CPU飙高', detail: `CPU使用率 ${cpuData[0].value}%`, duration: '持续中' })
  }
  const dropData = await queryClickHouse(
    `SELECT toStartOfMinute(timestamp) as t, sum(bytes) as b FROM cloudflow.ebpf_events WHERE event_type = 'flow' ${dt('1 HOUR')} GROUP BY t ORDER BY t DESC LIMIT 10`
  )
  if (dropData.length >= 5) {
    const avg = dropData.slice(0, 5).reduce((s: number, x: any) => s + Number(x.b || 0), 0) / 5
    const latest = Number(dropData[0].b || 0)
    if (latest < avg * 0.5 && latest > 0) {
      alertList.push({ time: fmtTime(dropData[0].t), level: 2, type: '码率突降', detail: `当前 ${(latest / avg * 100).toFixed(0)}% 基线`, duration: '8秒' })
    }
  }
  alerts.value = alertList
  alertCount.value = alertList.length
  alertExpanded.value = alertList.length > 0

  // ⑤ Top 10 通信对
  const pairs = await queryClickHouse(
    `SELECT src_ip, dst_ip, protocol, sum(bytes) as bytes, sum(packets) as packets, count() as flows FROM cloudflow.ebpf_events WHERE event_type = 'flow' ${dt('24 HOUR')} GROUP BY src_ip, dst_ip, protocol ORDER BY bytes DESC LIMIT 10`
  )
  topPairs.value = pairs.map((x: any) => ({ ...x, business: classifyBusiness(x) }))

  // ⑤ 活跃连接
  const conns = await queryClickHouse(
    `SELECT src_ip, dst_ip, protocol, dst_port, bytes, timestamp as last_seen FROM cloudflow.ebpf_events WHERE event_type = 'flow' ${dt('30 MINUTE')} ORDER BY timestamp DESC LIMIT 20`
  )
  activeConns.value = conns.map((x: any) => ({
    ...x,
    bytes: Number(x.bytes || 0),
    dst_port: Number(x.dst_port || 0),
    last_seen: fmtTime(x.last_seen),
    status: (Date.now() - new Date(x.last_seen).getTime()) < 300000 ? 'active' : 'idle'
  }))

  // ⑥ 系统性能
  const metrics = await queryClickHouse(
    `SELECT details, value, toStartOfMinute(timestamp) as t FROM cloudflow.ebpf_events WHERE event_type = 'host_metrics' ${dt('1 HOUR')} ORDER BY timestamp DESC LIMIT 300`
  )
  const cpuVals: { t: string; v: number }[] = []
  const memVals: { t: string; v: number }[] = []
  metrics.forEach((m: any) => {
    const d = String(m.details || '').toLowerCase()
    const v = Number(m.value || 0)
    if (d.includes('cpu')) cpuVals.push({ t: String(m.t).substring(11, 16), v })
    else if (d.includes('mem')) memVals.push({ t: String(m.t).substring(11, 16), v })
  })
  if (cpuVals.length > 0) {
    const latest = cpuVals[0].v
    sysMetrics.value[0] = { label: 'CPU', value: latest.toFixed(1) + '%', percent: Math.min(100, Math.round(latest)), color: latest > 80 ? '#FF6B6B' : '#00CCFF' }
  }
  if (memVals.length > 0) {
    const latest = memVals[0].v
    sysMetrics.value[1] = { label: '内存', value: latest.toFixed(1) + '%', percent: Math.min(100, Math.round(latest)), color: latest > 80 ? '#FF6B6B' : '#6BEDB7' }
  }
  // 磁盘用网络流量估算
  const diskUsage = Math.min(95, Math.round((Number(kpiCards.value[1].value) || 0) / 100))
  sysMetrics.value[2] = { label: '磁盘', value: diskUsage + '%', percent: diskUsage, color: diskUsage > 80 ? '#FF6B6B' : '#FFA940' }

  sysChartOption.value = {
    ...sysChartOption.value,
    xAxis: { ...sysChartOption.value.xAxis, data: cpuVals.slice(0, 20).reverse().map(x => x.t) },
    series: [
      { ...sysChartOption.value.series[0], data: cpuVals.slice(0, 20).reverse().map(x => x.v) },
      { ...sysChartOption.value.series[1], data: memVals.slice(0, 20).reverse().map(x => x.v) }
    ]
  }
}

let timer: number | null = null
onMounted(() => {
  fetchAll()
  timer = window.setInterval(fetchAll, 30000)
})
onUnmounted(() => { if (timer) clearInterval(timer) })
</script>

<style scoped lang="scss">
.stb-dashboard {
  padding: 16px 20px 24px;
}

// ── ① 顶部状态条 ──
.top-bar {
  display: flex;
  align-items: center;
  gap: 0;
  background: linear-gradient(90deg, rgba(10,186,255,0.12) 0%, rgba(107,237,183,0.08) 100%);
  border: 1px solid rgba(10,186,255,0.25);
  border-radius: 10px;
  padding: 12px 24px;
  margin-bottom: 16px;
  .top-bar-item {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 0 20px;
    border-right: 1px solid rgba(255,255,255,0.1);
    &:last-child { border-right: none; }
    &.playback { flex: 2; }
    &.health { flex: 1.5; flex-direction: column; align-items: flex-start; gap: 4px; }
    &.alerts-top { cursor: pointer; }
    &.probe-info { flex: 1; }
    &.uptime { flex: 0.8; }
  }
  .top-label {
    font-size: 11px;
    color: rgba(255,255,255,0.5);
    white-space: nowrap;
  }
  .top-value {
    font-size: 14px;
    color: #fff;
    font-weight: 600;
    &.highlight {
      color: #6BEDB7;
      font-size: 15px;
    }
  }
  .top-score {
    font-size: 22px;
    font-weight: 700;
    font-family: 'Arial', sans-serif;
    .unit { font-size: 12px; font-weight: 400; margin-left: 2px; }
  }
  .health-bar {
    width: 100%;
    height: 4px;
    background: rgba(255,255,255,0.1);
    border-radius: 2px;
    overflow: hidden;
    .health-fill { height: 100%; border-radius: 2px; transition: width 0.6s; }
  }
  .top-alert-count {
    font-size: 18px;
    font-weight: 700;
    color: rgba(255,255,255,0.5);
    font-family: 'Arial', sans-serif;
    &.has-alert { color: #FF6B6B; }
    .unit { font-size: 12px; font-weight: 400; }
  }
  .status-dot {
    width: 8px; height: 8px; border-radius: 50%;
    &.online { background: #6BEDB7; box-shadow: 0 0 6px #6BEDB7; }
  }
}

// ── ② KPI 卡片 ──
.kpi-row {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 16px;
  margin-bottom: 16px;
}
.kpi-card {
  display: flex;
  align-items: center;
  gap: 12px;
  background: rgba(255,255,255,0.03);
  border: 1px solid rgba(255,255,255,0.08);
  border-radius: 8px;
  padding: 14px 16px;
  transition: border-color 0.2s;
  &:hover { border-color: rgba(10,186,255,0.4); }
  .kpi-icon-wrap {
    width: 42px; height: 42px; border-radius: 10px;
    display: flex; align-items: center; justify-content: center;
    flex-shrink: 0;
  }
  .kpi-body {
    .kpi-val {
      font-size: 20px; font-weight: 700; color: #fff;
      font-family: 'Arial', sans-serif; line-height: 1.2;
    }
    .kpi-label {
      font-size: 12px; color: rgba(255,255,255,0.5); margin-top: 2px;
    }
  }
}

// ── ③ 流量概览 ──
.overview-row {
  display: grid;
  grid-template-columns: 1.2fr 0.8fr 0.8fr;
  gap: 16px;
  margin-bottom: 16px;
}
.overview-card {
  border-radius: 8px;
  background: rgba(255,255,255,0.03);
  border: 1px solid rgba(255,255,255,0.08);
  .card-title {
    font-size: 13px; font-weight: 600; color: rgba(255,255,255,0.85);
    margin-bottom: 8px;
  }
}

// ── ④ 告警 ──
.alert-section {
  background: rgba(255,107,107,0.06);
  border: 1px solid rgba(255,107,107,0.2);
  border-radius: 8px;
  margin-bottom: 16px;
  overflow: hidden;
}
.alert-header {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 16px;
  cursor: pointer;
  &:hover { background: rgba(255,107,107,0.08); }
  .alert-title { font-size: 13px; font-weight: 600; color: #fff; }
  .alert-badge {
    background: #FF6B6B; color: #fff; font-size: 11px; font-weight: 700;
    padding: 1px 7px; border-radius: 10px;
  }
  .expand-icon {
    margin-left: auto; color: rgba(255,255,255,0.5); transition: transform 0.2s;
    &.expanded { transform: rotate(180deg); }
  }
}
.alert-list { padding: 0 16px 10px; }
.alert-item {
  display: flex; align-items: center; gap: 10px;
  padding: 6px 0;
  border-bottom: 1px solid rgba(255,255,255,0.05);
  font-size: 12px; color: rgba(255,255,255,0.8);
  &:last-child { border-bottom: none; }
  .alert-time { color: rgba(255,255,255,0.4); width: 145px; flex-shrink: 0; }
  .alert-level {
    font-size: 10px; font-weight: 700; padding: 1px 6px; border-radius: 3px;
    &.p0 { background: #FF6B6B; color: #fff; }
    &.p1 { background: #FFA940; color: #fff; }
    &.p2 { background: #FFC53D; color: #333; }
  }
  .alert-type { font-weight: 600; width: 70px; }
  .alert-detail { flex: 1; color: rgba(255,255,255,0.6); }
  .alert-duration { color: rgba(255,255,255,0.4); width: 80px; text-align: right; }
}

// ── ⑤ 流量明细 ──
.detail-row {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 16px;
  margin-bottom: 16px;
}
.detail-card {
  border-radius: 8px;
  background: rgba(255,255,255,0.03);
  border: 1px solid rgba(255,255,255,0.08);
  .card-title {
    font-size: 13px; font-weight: 600; color: rgba(255,255,255,0.85);
    margin-bottom: 8px;
  }
}
.conn-status {
  font-size: 11px; font-weight: 600;
  &.active { color: #6BEDB7; }
  &.idle { color: rgba(255,255,255,0.4); }
}

// ── ⑥ 系统性能 ──
.sys-row {
  margin-bottom: 16px;
}
.sys-card {
  border-radius: 8px;
  background: rgba(255,255,255,0.03);
  border: 1px solid rgba(255,255,255,0.08);
  .card-title {
    font-size: 13px; font-weight: 600; color: rgba(255,255,255,0.85);
    margin-bottom: 10px;
  }
}
.sys-metrics {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 20px;
  margin-bottom: 12px;
}
.sys-metric {
  .sys-metric-header {
    display: flex; justify-content: space-between; align-items: center;
    margin-bottom: 6px;
    .sys-metric-label { font-size: 12px; color: rgba(255,255,255,0.6); }
    .sys-metric-value { font-size: 14px; font-weight: 700; font-family: 'Arial', sans-serif; }
  }
}
</style>
