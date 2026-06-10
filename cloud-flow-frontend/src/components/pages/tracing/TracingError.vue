<template>
  <div class="space-y-6">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold text-slate-900 dark:text-white">错误请求分析</h1>
        <p class="text-sm text-slate-500 dark:text-slate-400 mt-1">识别和分析错误请求</p>
      </div>
    </div>

    <div class="grid grid-cols-3 gap-4">
      <div class="card p-4">
        <p class="text-sm text-slate-500">总错误数</p>
        <p class="text-2xl font-bold text-red-500 mt-1">156</p>
      </div>
      <div class="card p-4">
        <p class="text-sm text-slate-500">4xx错误</p>
        <p class="text-2xl font-bold text-amber-500 mt-1">89</p>
      </div>
      <div class="card p-4">
        <p class="text-sm text-slate-500">5xx错误</p>
        <p class="text-2xl font-bold text-red-500 mt-1">67</p>
      </div>
    </div>

    <div class="card p-6">
      <h3 class="text-lg font-semibold text-slate-900 dark:text-white mb-6">错误分布</h3>
      <div class="h-64">
        <ECharts :option="errorOption" class="w-full h-full" />
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

const errorOption = computed(() => ({
  tooltip: { trigger: 'item', backgroundColor: 'rgba(255,255,255,0.95)', textStyle: { color: '#1e293b' } },
  legend: { bottom: 0, textStyle: { color: '#64748b' } },
  series: [{
    type: 'pie',
    radius: ['40%', '70%'],
    itemStyle: { borderRadius: 8, borderColor: '#fff', borderWidth: 2 },
    label: { show: false },
    emphasis: { label: { show: true, fontSize: 14, fontWeight: 'bold' } },
    data: [
      { value: 45, name: '500 内部错误', itemStyle: { color: '#ef4444' } },
      { value: 30, name: '400 请求错误', itemStyle: { color: '#f59e0b' } },
      { value: 15, name: '401 未授权', itemStyle: { color: '#8b5cf6' } },
      { value: 10, name: '其他', itemStyle: { color: '#94a3b8' } },
    ],
  }],
}))
</script>
