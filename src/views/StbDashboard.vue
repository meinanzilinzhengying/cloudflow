<template>
  <div class="stb-dashboard">
    <div class="page-header">
      <h2 class="page-title">机顶盒总览</h2>
      <div class="probe-info">
        <el-tag type="info" effect="dark">探针: stb-188-bpf</el-tag>
        <el-tag v-if="lastHeartbeat" type="success" effect="dark" class="ml-2">
          最后心跳: {{ lastHeartbeat }}
        </el-tag>
      </div>
    </div>

    <!-- 指标卡片 -->
    <el-row :gutter="16" class="metric-row">
      <el-col :span="6">
        <div class="metric-card">
          <div class="metric-label">总采集事件</div>
          <div class="metric-value">{{ formatNumber(metrics.totalEvents) }}</div>
          <div class="metric-sub">采集速率: {{ metrics.collectRate }}/s</div>
        </div>
      </el-col>
      <el-col :span="6">
        <div class="metric-card">
          <div class="metric-label">总流量</div>
          <div class="metric-value">{{ formatMB(metrics.totalMB) }}</div>
          <div class="metric-sub">数据包: {{ formatNumber(metrics.totalPackets) }}</div>
        </div>
      </el-col>
      <el-col :span="6">
        <div class="metric-card">
          <div class="metric-label">活跃协议数</div>
          <div class="metric-value">{{ metrics.activeProtocols }}</div>
          <div class="metric-sub">目标IP: {{ metrics.uniqueDstIPs }} 个</div>
        </div>
      </el-col>
      <el-col :span="6">
        <div class="metric-card">
          <div class="metric-label">当前状态</div>
          <div class="metric-value" style="color: #67C23A">在线</div>
          <div class="metric-sub">数据持续采集</div>
        </div>
      </el-col>
    </el-row>

    <!-- 图表行 -->
    <el-row :gutter="16" class="chart-row">
      <el-col :span="12">
        <el-card class="chart-card">
          <template #header>协议分布</template>
          <v-chart :option="protocolOption" style="height: 320px" autoresize />
        </el-card>
      </el-col>
      <el-col :span="12">
        <el-card class="chart-card">
          <template #header>24H 流量趋势 (Mbps)</template>
          <v-chart :option="trendOption" style="height: 320px" autoresize />
        </el-card>
      </el-col>
    </el-row>

    <!-- Top通信对 & 活跃连接 -->
    <el-row :gutter="16" class="table-row">
      <el-col :span="12">
        <el-card class="table-card">
          <template #header>Top 10 通信对</template>
          <el-table :data="topTalkers" stripe size="small" style="width: 100%" v-loading="loadingTalkers">
            <el-table-column prop="dstIp" label="目标IP" min-width="140" />
            <el-table-column prop="protocol" label="协议" width="80" />
            <el-table-column prop="mb" label="流量(MB)" width="100" align="right">
              <template #default="{ row }">{{ row.mb.toFixed(2) }}</template>
            </el-table-column>
            <el-table-column prop="packets" label="包数" width="90" align="right" />
          </el-table>
        </el-card>
      </el-col>
      <el-col :span="12">
        <el-card class="table-card">
          <template #header>活跃连接</template>
          <el-table :data="connections" stripe size="small" style="width: 100%" v-loading="loadingConns">
            <el-table-column prop="dstIp" label="目标IP" min-width="140" />
            <el-table-column prop="port" label="端口" width="80" />
            <el-table-column prop="bytes" label="总字节" width="100" align="right">
              <template #default="{ row }">{{ formatBytes(row.bytes) }}</template>
            </el-table-column>
            <el-table-column prop="packets" label="包数" width="80" align="right" />
          </el-table>
        </el-card>
      </el-col>
    </el-row>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, reactive } from 'vue'
import VChart from 'vue-echarts'
import { use } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { PieChart, LineChart } from 'echarts/charts'
import { TooltipComponent, LegendComponent, GridComponent } from 'echarts/components'
import { getOverviewMetrics, getProtocolDistribution, getTopTalkers, getTrafficTrend, getActiveConnections } from '@/api/stb'

use([CanvasRenderer, PieChart, LineChart, TooltipComponent, LegendComponent, GridComponent])

const metrics = reactive({
  totalEvents: 0, totalMB: 0, totalPackets: 0,
  collectRate: 0, activeProtocols: 0, uniqueDstIPs: 0
})
const lastHeartbeat = ref('')
const topTalkers = ref<any[]>([])
const connections = ref<any[]>([])
const loadingTalkers = ref(false)
const loadingConns = ref(false)
const protocolOption = ref({})
const trendOption = ref({})

const formatNumber = (n: number) => n >= 10000 ? (n / 10000).toFixed(1) + '万' : n.toString()
const formatMB = (mb: number) => mb >= 1024 ? (mb / 1024).toFixed(2) + ' GB' : mb.toFixed(2) + ' MB'
const formatBytes = (b: number) => b >= 1073741824 ? (b / 1073741824).toFixed(2) + ' GB' : b >= 1048576 ? (b / 1048576).toFixed(2) + ' MB' : b.toFixed(0) + ' B'

const COLORS = ['#1887EE', '#00CCFF', '#0ABFA2', '#F56C6C', '#E6A23C', '#67C23A', '#909399']

async function loadAll() {
  try {
    const m = await getOverviewMetrics()
    Object.assign(metrics, m)
    lastHeartbeat.value = m.lastHeartbeat
  } catch (e) { console.error('load metrics error:', e) }

  try {
    const protocols = await getProtocolDistribution()
    protocolOption.value = {
      tooltip: { trigger: 'item', formatter: '{b}: {c} 包 ({d}%)' },
      legend: { bottom: 0, textStyle: { color: '#ccc' } },
      series: [{
        type: 'pie', radius: ['40%', '65%'], center: ['50%', '45%'],
        data: protocols.map(p => ({ name: p.protocol, value: p.packets })),
        label: { color: '#ccc', formatter: '{b}: {d}%' },
        itemStyle: { color: (params: any) => COLORS[params.dataIndex % COLORS.length] },
        emphasis: { itemStyle: { shadowBlur: 10, shadowOffsetX: 0, shadowColor: 'rgba(0,0,0,0.5)' } }
      }]
    }
  } catch (e) { console.error('load protocols error:', e) }

  try {
    const trend = await getTrafficTrend()
    trendOption.value = {
      tooltip: { trigger: 'axis' },
      grid: { left: '3%', right: '4%', bottom: '3%', containLabel: true },
      xAxis: { type: 'category', data: trend.map(t => t.time), axisLabel: { color: '#999', fontSize: 10 } },
      yAxis: { type: 'value', axisLabel: { color: '#999' }, splitLine: { lineStyle: { color: 'rgba(255,255,255,0.1)' } } },
      series: [{
        type: 'line', smooth: true, data: trend.map(t => t.mbps),
        lineStyle: { color: '#00CCFF', width: 2 },
        areaStyle: { color: 'rgba(0, 204, 255, 0.15)' },
        itemStyle: { color: '#00CCFF' }
      }]
    }
  } catch (e) { console.error('load trend error:', e) }

  loadingTalkers.value = true
  try { topTalkers.value = await getTopTalkers(10) }
  catch (e) { console.error('load talkers error:', e) }
  finally { loadingTalkers.value = false }

  loadingConns.value = true
  try { connections.value = await getActiveConnections() }
  catch (e) { console.error('load connections error:', e) }
  finally { loadingConns.value = false }
}

onMounted(loadAll)
</script>

<style scoped>
.stb-dashboard { padding: 16px; }
.page-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 16px; }
.page-title { font-size: 20px; color: #e0e0e0; margin: 0; }
.probe-info { display: flex; gap: 8px; }
.ml-2 { margin-left: 8px; }
.metric-row { margin-bottom: 16px; }
.metric-card {
  background: linear-gradient(135deg, rgba(38, 36, 68, 0.9), rgba(24, 135, 238, 0.15));
  border: 1px solid rgba(24, 135, 238, 0.3);
  border-radius: 8px;
  padding: 20px;
  text-align: center;
}
.metric-label { font-size: 13px; color: #999; margin-bottom: 8px; }
.metric-value { font-size: 28px; font-weight: 600; color: #00CCFF; margin-bottom: 4px; }
.metric-sub { font-size: 12px; color: #888; }
.chart-row { margin-bottom: 16px; }
.chart-card { background: rgba(38, 36, 68, 0.8); border: 1px solid rgba(24, 135, 238, 0.2); }
:deep(.el-card__header) { color: #ccc; border-bottom: 1px solid rgba(255,255,255,0.05); }
.table-card { background: rgba(38, 36, 68, 0.8); border: 1px solid rgba(24, 135, 238, 0.2); }
:deep(.el-table) { background: transparent; }
:deep(.el-table th) { background: rgba(24, 135, 238, 0.1); color: #ccc; }
:deep(.el-table tr) { background: transparent; }
:deep(.el-table td) { color: #ccc; }
:deep(.el-table--striped .el-table__body tr.el-table__row--striped td) { background: rgba(255,255,255,0.02); }
</style>
