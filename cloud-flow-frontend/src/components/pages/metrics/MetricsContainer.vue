<template>
  <div class="space-y-6">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold text-slate-900 dark:text-white">容器指标</h1>
        <p class="text-sm text-slate-500 dark:text-slate-400 mt-1">监控容器性能指标</p>
      </div>
    </div>

    <div class="card p-6">
      <h3 class="text-lg font-semibold text-slate-900 dark:text-white mb-4">容器资源使用</h3>
      <div class="h-64">
        <ECharts :option="containerOption" class="w-full h-full" />
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { use } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { BarChart } from 'echarts/charts'
import { TooltipComponent, GridComponent, LegendComponent } from 'echarts/components'
import ECharts from 'vue-echarts'

use([CanvasRenderer, BarChart, TooltipComponent, GridComponent, LegendComponent])

const containerOption = computed(() => ({
  tooltip: { trigger: 'axis', backgroundColor: 'rgba(255,255,255,0.95)', textStyle: { color: '#1e293b' } },
  legend: { bottom: 0, textStyle: { color: '#64748b' } },
  grid: { left: '3%', right: '4%', bottom: '15%', top: '10%', containLabel: true },
  xAxis: { type: 'category', data: [], axisLabel: { color: '#64748b' } },
  yAxis: { type: 'value', axisLabel: { color: '#64748b' }, splitLine: { lineStyle: { color: '#f1f5f9' } } },
  series: [
    { name: 'CPU(cores)', type: 'bar', itemStyle: { color: '#2563eb' }, data: [] },
    { name: 'Memory(GB)', type: 'bar', itemStyle: { color: '#14b8a6' }, data: [] },
  ],
}))
</script>
