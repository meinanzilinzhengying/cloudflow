<template>
  <div class="space-y-6">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold text-slate-900 dark:text-white">慢请求分析</h1>
        <p class="text-sm text-slate-500 dark:text-slate-400 mt-1">识别和分析慢请求</p>
      </div>
    </div>

    <div class="card p-6">
      <div class="flex items-center justify-between mb-6">
        <h3 class="text-lg font-semibold text-slate-900 dark:text-white">慢请求分布</h3>
        <select class="input w-32">
          <option>按服务</option>
          <option>按端点</option>
          <option>按延迟</option>
        </select>
      </div>
      <div class="h-64">
        <ECharts :option="slowRequestOption" class="w-full h-full" />
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { use } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { BarChart } from 'echarts/charts'
import { TooltipComponent, GridComponent } from 'echarts/components'
import ECharts from 'vue-echarts'

use([CanvasRenderer, BarChart, TooltipComponent, GridComponent])

const slowRequestOption = computed(() => ({
  tooltip: { trigger: 'axis', backgroundColor: 'rgba(255,255,255,0.95)', textStyle: { color: '#1e293b' } },
  grid: { left: '3%', right: '4%', bottom: '3%', top: '10%', containLabel: true },
  xAxis: { type: 'category', data: ['API Gateway', 'User Service', 'Order Service', 'Payment Service', 'Product Service'], axisLabel: { color: '#64748b', fontSize: 11 } },
  yAxis: { type: 'value', name: '平均延迟(ms)', axisLabel: { color: '#64748b', fontSize: 11 }, splitLine: { lineStyle: { color: '#f1f5f9' } } },
  series: [{
    type: 'bar',
    itemStyle: { color: '#ef4444', borderRadius: [4, 4, 0, 0] },
    data: [156, 234, 456, 123, 89],
  }],
}))
</script>
