<template>
  <div class="stb-channel">
    <div class="page-header">
      <h2 class="page-title">换台行为分析</h2>
      <el-tag type="info" effect="dark">基于HTTP请求分析</el-tag>
    </div>

    <el-row :gutter="16" class="metric-row">
      <el-col :span="8">
        <div class="metric-card">
          <div class="metric-label">HTTP 事件数</div>
          <div class="metric-value">{{ httpEvents }}</div>
        </div>
      </el-col>
      <el-col :span="8">
        <div class="metric-card">
          <div class="metric-label">换台频次(次/5分钟)</div>
          <div class="metric-value">{{ avgRate }}</div>
        </div>
      </el-col>
      <el-col :span="8">
        <div class="metric-card">
          <div class="metric-label">活跃时间段</div>
          <div class="metric-value" style="font-size:18px">{{ peakTime }}</div>
        </div>
      </el-col>
    </el-row>

    <el-card class="chart-card">
      <template #header>换台频率趋势 (5分钟粒度)</template>
      <v-chart :option="chartOption" style="height: 350px" autoresize />
      <div v-if="httpEvents === 0" class="empty-tip">
        当前24小时内未检测到HTTP请求事件，换台行为分析需要HTTP协议数据支撑
      </div>
    </el-card>

    <el-card class="info-card" style="margin-top:16px">
      <template #header>说明</template>
      <div class="info-content">
        <p>换台行为分析基于机顶盒发出的HTTP请求事件进行识别。当机顶盒切换频道时，通常会触发对频道URL的HTTP请求。</p>
        <p>当前数据仅包含 <strong>{{ httpEvents }}</strong> 条HTTP事件，需要更多采集数据才能形成有意义的换台模式分析。</p>
        <p>建议：确保eBPF探针的HTTP协议解析功能正常启用，并持续采集数据。</p>
      </div>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import VChart from 'vue-echarts'
import { use } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { BarChart } from 'echarts/charts'
import { TooltipComponent, GridComponent } from 'echarts/components'
import { getHTTPChannelChanges } from '@/api/stb'

use([CanvasRenderer, BarChart, TooltipComponent, GridComponent])

const httpEvents = ref(0)
const avgRate = ref(0)
const peakTime = ref('--')
const chartOption = ref({})

onMounted(async () => {
  try {
    const data = await getHTTPChannelChanges()
    httpEvents.value = data.reduce((s, x) => s + x.count, 0)
    avgRate.value = data.length > 0 ? parseFloat((httpEvents.value / data.length).toFixed(1)) : 0
    if (data.length > 0) {
      const peak = data.reduce((a, b) => a.count > b.count ? a : b)
      peakTime.value = peak.time
    }
    chartOption.value = {
      tooltip: { trigger: 'axis' },
      grid: { left: '3%', right: '4%', bottom: '3%', containLabel: true },
      xAxis: { type: 'category', data: data.map(x => x.time), axisLabel: { color: '#999', fontSize: 10 } },
      yAxis: { type: 'value', name: '事件数', axisLabel: { color: '#999' }, splitLine: { lineStyle: { color: 'rgba(255,255,255,0.1)' } } },
      series: [{
        type: 'bar', data: data.map(x => x.count),
        itemStyle: { color: '#E6A23C', borderRadius: [4, 4, 0, 0] }
      }]
    }
  } catch {}
})
</script>
<style scoped>
.stb-channel { padding: 16px; }
.page-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 16px; }
.page-title { font-size: 20px; color: #e0e0e0; margin: 0; }
.metric-row { margin-bottom: 16px; }
.metric-card { background: linear-gradient(135deg, rgba(38,36,68,0.9), rgba(24,135,238,0.15)); border: 1px solid rgba(24,135,238,0.3); border-radius: 8px; padding: 20px; text-align: center; }
.metric-label { font-size: 13px; color: #999; margin-bottom: 8px; }
.metric-value { font-size: 28px; font-weight: 600; color: #E6A23C; }
.chart-card, .info-card { background: rgba(38,36,68,0.8); border: 1px solid rgba(24,135,238,0.2); }
:deep(.el-card__header) { color: #ccc; border-bottom: 1px solid rgba(255,255,255,0.05); }
.info-content { color: #aaa; font-size: 13px; line-height: 1.8; }
.info-content p { margin: 4px 0; }
.empty-tip { text-align: center; padding: 20px; color: #999; }
</style>
