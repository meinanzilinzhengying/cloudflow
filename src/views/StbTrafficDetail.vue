<template>
  <div class="stb-traffic">
    <div class="page-header">
      <h2 class="page-title">流量分布明细</h2>
      <el-tag type="info" effect="dark">24小时数据</el-tag>
    </div>

    <el-row :gutter="16" class="metric-row">
      <el-col :span="6">
        <div class="metric-card"><div class="metric-label">总流量</div>
          <div class="metric-value">{{ formatMB(summary.totalMB) }}</div></div>
      </el-col>
      <el-col :span="6">
        <div class="metric-card"><div class="metric-label">总数据包</div>
          <div class="metric-value">{{ formatNumber(summary.totalPackets) }}</div></div>
      </el-col>
      <el-col :span="6">
        <div class="metric-card"><div class="metric-label">TCP 流量</div>
          <div class="metric-value" style="color:#1887EE">{{ formatMB(summary.tcpBytes) }}</div></div>
      </el-col>
      <el-col :span="6">
        <div class="metric-card"><div class="metric-label">UDP 流量</div>
          <div class="metric-value" style="color:#00CCFF">{{ formatMB(summary.udpBytes) }}</div></div>
      </el-col>
    </el-row>

    <el-row :gutter="16" class="chart-row">
      <el-col :span="8">
        <el-card class="chart-card"><template #header>协议分布(包量)</template>
          <v-chart :option="barOption" style="height: 300px" autoresize /></el-card>
      </el-col>
      <el-col :span="16">
        <el-card class="chart-card"><template #header>24H 流量趋势(Mbps)</template>
          <v-chart :option="trendOption" style="height: 300px" autoresize /></el-card>
      </el-col>
    </el-row>

    <el-card class="table-card">
      <template #header>Top 20 通信对</template>
      <el-table :data="talkers" stripe size="small" style="width: 100%" v-loading="loading">
        <el-table-column prop="dstIp" label="目标IP" min-width="140" />
        <el-table-column prop="protocol" label="协议" width="70" />
        <el-table-column prop="mb" label="流量(MB)" width="110" align="right">
          <template #default="{ row }">{{ row.mb.toFixed(2) }}</template>
        </el-table-column>
        <el-table-column prop="bytes" label="总字节" width="120" align="right">
          <template #default="{ row }">{{ formatBytes(row.bytes) }}</template>
        </el-table-column>
        <el-table-column prop="packets" label="包数" width="90" align="right" />
        <el-table-column prop="events" label="事件数" width="80" align="right" />
      </el-table>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, reactive } from 'vue'
import VChart from 'vue-echarts'
import { use } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { BarChart, LineChart } from 'echarts/charts'
import { TooltipComponent, LegendComponent, GridComponent } from 'echarts/components'
import { getTrafficSummary, getProtocolDistribution, getTopTalkers, getTrafficTrend } from '@/api/stb'

use([CanvasRenderer, BarChart, LineChart, TooltipComponent, LegendComponent, GridComponent])

const summary = reactive({ totalMB: 0, totalPackets: 0, tcpBytes: 0, udpBytes: 0 })
const talkers = ref<any[]>([])
const loading = ref(false)
const barOption = ref({})
const trendOption = ref({})
const formatNumber = (n: number) => n >= 10000 ? (n / 10000).toFixed(1) + '万' : n.toString()
const formatMB = (mb: number) => mb >= 1024 ? (mb / 1024).toFixed(2) + ' GB' : mb.toFixed(2) + ' MB'
const formatBytes = (b: number) => b >= 1073741824 ? (b / 1073741824).toFixed(2) + ' GB' : b >= 1048576 ? (b / 1048576).toFixed(2) + ' MB' : b.toFixed(0) + ' B'
const COLORS = ['#1887EE', '#00CCFF', '#0ABFA2', '#F56C6C']

onMounted(async () => {
  try { Object.assign(summary, await getTrafficSummary()) } catch {}
  try {
    const p = await getProtocolDistribution()
    barOption.value = {
      tooltip: { trigger: 'axis' },
      grid: { left: '3%', right: '4%', bottom: '3%', containLabel: true },
      xAxis: { type: 'category', data: p.map(x => x.protocol), axisLabel: { color: '#ccc' } },
      yAxis: { type: 'value', name: '包数', axisLabel: { color: '#999' }, splitLine: { lineStyle: { color: 'rgba(255,255,255,0.1)' } } },
      series: [{ type: 'bar', data: p.map(x => x.packets), itemStyle: { color: (params: any) => COLORS[params.dataIndex % COLORS.length] } }]
    }
  } catch {}
  try {
    const t = await getTrafficTrend()
    trendOption.value = {
      tooltip: { trigger: 'axis' },
      grid: { left: '3%', right: '4%', bottom: '3%', containLabel: true },
      xAxis: { type: 'category', data: t.map(x => x.time), axisLabel: { color: '#999', fontSize: 10 } },
      yAxis: { type: 'value', axisLabel: { color: '#999' }, splitLine: { lineStyle: { color: 'rgba(255,255,255,0.1)' } } },
      series: [{
        type: 'line', smooth: true, data: t.map(x => x.mbps),
        lineStyle: { color: '#00CCFF', width: 2 },
        areaStyle: { color: 'rgba(0,204,255,0.15)' }, itemStyle: { color: '#00CCFF' }
      }]
    }
  } catch {}
  loading.value = true
  try { talkers.value = await getTopTalkers(20) } catch {}
  finally { loading.value = false }
})
</script>
<style scoped>
.stb-traffic { padding: 16px; }
.page-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 16px; }
.page-title { font-size: 20px; color: #e0e0e0; margin: 0; }
.metric-row { margin-bottom: 16px; }
.metric-card { background: linear-gradient(135deg, rgba(38,36,68,0.9), rgba(24,135,238,0.15)); border: 1px solid rgba(24,135,238,0.3); border-radius: 8px; padding: 20px; text-align: center; }
.metric-label { font-size: 13px; color: #999; margin-bottom: 8px; }
.metric-value { font-size: 28px; font-weight: 600; color: #00CCFF; }
.chart-row { margin-bottom: 16px; }
.chart-card, .table-card { background: rgba(38,36,68,0.8); border: 1px solid rgba(24,135,238,0.2); }
:deep(.el-card__header) { color: #ccc; border-bottom: 1px solid rgba(255,255,255,0.05); }
:deep(.el-table) { background: transparent; }
:deep(.el-table th) { background: rgba(24,135,238,0.1); color: #ccc; }
:deep(.el-table td) { color: #ccc; }
:deep(.el-table--striped .el-table__body tr.el-table__row--striped td) { background: rgba(255,255,255,0.02); }
</style>
