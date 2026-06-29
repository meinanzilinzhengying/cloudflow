<template>
  <div class="stb-iptv">
    <div class="page-header">
      <h2 class="page-title">IPTV 质量监控</h2>
      <el-tag type="info" effect="dark">基于UDP组播分析</el-tag>
    </div>

    <el-card class="chart-card">
      <template #header>UDP 组播流量趋势 (Mbps)</template>
      <v-chart :option="trendOption" style="height: 300px" autoresize />
    </el-card>

    <el-card class="table-card" style="margin-top:16px">
      <template #header>组播频道列表 (Top 20)</template>
      <el-table :data="channels" stripe size="small" style="width: 100%" v-loading="loading">
        <el-table-column prop="multicastAddr" label="组播地址" min-width="160" />
        <el-table-column prop="channelPort" label="端口" width="80" />
        <el-table-column prop="totalMB" label="流量(MB)" width="120" align="right">
          <template #default="{ row }">{{ row.totalMB.toFixed(2) }}</template>
        </el-table-column>
        <el-table-column prop="pktCount" label="包数" width="100" align="right" />
      </el-table>
      <div v-if="!loading && channels.length === 0" class="empty-tip">
        当前24小时内未检测到组播流量，无IPTV频道数据
      </div>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import VChart from 'vue-echarts'
import { use } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { LineChart } from 'echarts/charts'
import { TooltipComponent, GridComponent } from 'echarts/components'
import { getIPTVChannels, getUDPTrafficTrend } from '@/api/stb'

use([CanvasRenderer, LineChart, TooltipComponent, GridComponent])

const channels = ref<any[]>([])
const loading = ref(false)
const trendOption = ref({})

onMounted(async () => {
  try {
    const t = await getUDPTrafficTrend()
    trendOption.value = {
      tooltip: { trigger: 'axis' },
      grid: { left: '3%', right: '4%', bottom: '3%', containLabel: true },
      xAxis: { type: 'category', data: t.map(x => x.time), axisLabel: { color: '#999', fontSize: 10 } },
      yAxis: { type: 'value', axisLabel: { color: '#999' }, splitLine: { lineStyle: { color: 'rgba(255,255,255,0.1)' } } },
      series: [{
        type: 'line', smooth: true, data: t.map(x => x.mbps),
        lineStyle: { color: '#0ABFA2', width: 2 },
        areaStyle: { color: 'rgba(10,191,162,0.15)' }, itemStyle: { color: '#0ABFA2' }
      }]
    }
  } catch {}
  loading.value = true
  try { channels.value = await getIPTVChannels() } catch {}
  finally { loading.value = false }
})
</script>
<style scoped>
.stb-iptv { padding: 16px; }
.page-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 16px; }
.page-title { font-size: 20px; color: #e0e0e0; margin: 0; }
.chart-card, .table-card { background: rgba(38,36,68,0.8); border: 1px solid rgba(24,135,238,0.2); }
:deep(.el-card__header) { color: #ccc; border-bottom: 1px solid rgba(255,255,255,0.05); }
:deep(.el-table) { background: transparent; }
:deep(.el-table th) { background: rgba(24,135,238,0.1); color: #ccc; }
:deep(.el-table td) { color: #ccc; }
.empty-tip { text-align: center; padding: 40px; color: #999; }
</style>
