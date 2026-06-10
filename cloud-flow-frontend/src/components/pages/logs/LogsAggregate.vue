<template>
  <div class="space-y-6">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold text-slate-900 dark:text-white">日志聚合</h1>
        <p class="text-sm text-slate-500 dark:text-slate-400 mt-1">聚合分析日志数据</p>
      </div>
    </div>

    <div class="card p-6">
      <h3 class="text-lg font-semibold text-slate-900 dark:text-white mb-4">日志级别分布</h3>
      <div class="h-64">
        <ECharts :option="logsOption" class="w-full h-full" />
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { use } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { PieChart } from 'echarts/charts'
import { TooltipComponent, LegendComponent } from 'echarts/components'
import ECharts from 'vue-echarts'

use([CanvasRenderer, PieChart, TooltipComponent, LegendComponent])

const logsOption = computed(() => ({
  tooltip: { trigger: 'item', backgroundColor: 'rgba(255,255,255,0.95)', textStyle: { color: '#1e293b' } },
  legend: { bottom: 0, textStyle: { color: '#64748b' } },
  series: [{
    type: 'pie',
    radius: ['40%', '70%'],
    itemStyle: { borderRadius: 8, borderColor: '#fff', borderWidth: 2 },
    label: { show: false },
    emphasis: { label: { show: true, fontSize: 14, fontWeight: 'bold' } },
    data: [
      { value: 75, name: 'INFO', itemStyle: { color: '#22c55e' } },
      { value: 15, name: 'WARN', itemStyle: { color: '#f59e0b' } },
      { value: 8, name: 'ERROR', itemStyle: { color: '#ef4444' } },
      { value: 2, name: 'DEBUG', itemStyle: { color: '#3b82f6' } },
    ],
  }],
}))
</script>
