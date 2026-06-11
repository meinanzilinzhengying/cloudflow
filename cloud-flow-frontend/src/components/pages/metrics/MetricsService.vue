<template>
  <div class="space-y-6">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold text-slate-900 dark:text-white">服务指标</h1>
        <p class="text-sm text-slate-500 dark:text-slate-400 mt-1">监控服务性能指标</p>
      </div>
    </div>

    <div class="card p-6">
      <h3 class="text-lg font-semibold text-slate-900 dark:text-white mb-4">服务QPS趋势</h3>
      <div class="h-64">
        <ECharts :option="serviceOption" class="w-full h-full" />
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { use } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { LineChart } from 'echarts/charts'
import { TooltipComponent, GridComponent, LegendComponent } from 'echarts/components'
import ECharts from 'vue-echarts'

use([CanvasRenderer, LineChart, TooltipComponent, GridComponent, LegendComponent])

const serviceOption = computed(() => ({
  tooltip: { trigger: 'axis', backgroundColor: 'rgba(255,255,255,0.95)', textStyle: { color: '#1e293b' } },
  legend: { bottom: 0, textStyle: { color: '#64748b' } },
  grid: { left: '3%', right: '4%', bottom: '15%', top: '10%', containLabel: true },
  xAxis: { type: 'category', data: ['00:00', '06:00', '12:00', '18:00', '24:00'], axisLabel: { color: '#64748b' } },
  yAxis: { type: 'value', axisLabel: { color: '#64748b' }, splitLine: { lineStyle: { color: '#f1f5f9' } } },
  series: [
    { name: 'API Gateway', type: 'line', smooth: true, lineStyle: { color: '#2563eb', width: 2 }, data: [12000, 8000, 18000, 15000, 10000] },
    { name: 'Order Service', type: 'line', smooth: true, lineStyle: { color: '#f59e0b', width: 2 }, data: [6000, 4000, 9000, 7500, 5000] },
    { name: 'User Service', type: 'line', smooth: true, lineStyle: { color: '#22c55e', width: 2 }, data: [4000, 3000, 6000, 5000, 3500] },
  ],
}))
</script>
