<template>
  <div class="space-y-6">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold text-slate-900 dark:text-white">会话分析</h1>
        <p class="text-sm text-slate-500 dark:text-slate-400 mt-1">深入分析网络会话数据</p>
      </div>
    </div>

    <div class="grid grid-cols-4 gap-4">
      <div class="card p-4">
        <p class="text-sm text-slate-500">活跃会话</p>
        <p class="text-2xl font-bold text-slate-900 dark:text-white mt-1">0</p>
      </div>
      <div class="card p-4">
        <p class="text-sm text-slate-500">TCP会话</p>
        <p class="text-2xl font-bold text-primary-500 mt-1">0</p>
      </div>
      <div class="card p-4">
        <p class="text-sm text-slate-500">UDP会话</p>
        <p class="text-2xl font-bold text-accent-500 mt-1">0</p>
      </div>
      <div class="card p-4">
        <p class="text-sm text-slate-500">平均时长</p>
        <p class="text-2xl font-bold text-emerald-500 mt-1">0s</p>
      </div>
    </div>

    <div class="card p-6">
      <h3 class="text-lg font-semibold text-slate-900 dark:text-white mb-6">会话状态分布</h3>
      <div class="h-64">
        <ECharts :option="sessionStatusOption" class="w-full h-full" />
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

const sessionStatusOption = computed(() => ({
  tooltip: { trigger: 'item', backgroundColor: 'rgba(255,255,255,0.95)', textStyle: { color: '#1e293b' } },
  legend: { bottom: 0, textStyle: { color: '#64748b' } },
  series: [{
    type: 'pie',
    radius: ['40%', '70%'],
    avoidLabelOverlap: false,
    itemStyle: { borderRadius: 8, borderColor: '#fff', borderWidth: 2 },
    label: { show: false },
    emphasis: { label: { show: true, fontSize: 14, fontWeight: 'bold' } },
    data: [
      { value: 45, name: 'ESTABLISHED', itemStyle: { color: '#22c55e' } },
      { value: 30, name: 'TIME_WAIT', itemStyle: { color: '#f59e0b' } },
      { value: 15, name: 'CLOSE_WAIT', itemStyle: { color: '#ef4444' } },
      { value: 10, name: 'SYN_SENT', itemStyle: { color: '#8b5cf6' } },
    ],
  }],
}))
</script>
