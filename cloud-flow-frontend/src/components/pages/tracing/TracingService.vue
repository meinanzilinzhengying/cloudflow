<template>
  <div class="space-y-6">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold text-slate-900 dark:text-white">服务调用分析</h1>
        <p class="text-sm text-slate-500 dark:text-slate-400 mt-1">分析服务间调用关系</p>
      </div>
    </div>

    <div class="card p-6">
      <div class="h-80">
        <ECharts :option="callGraphOption" class="w-full h-full" autoresize />
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { use } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { GraphChart } from 'echarts/charts'
import { TooltipComponent } from 'echarts/components'
import ECharts from 'vue-echarts'

use([CanvasRenderer, GraphChart, TooltipComponent])

const callGraphOption = computed(() => ({
  tooltip: {
    trigger: 'item',
    backgroundColor: 'rgba(255,255,255,0.95)',
    textStyle: { color: '#1e293b' },
    formatter: (params) => {
      if (params.dataType === 'edge') {
        return `<div class="font-medium">${params.data.callCount} calls</div>`
      }
      return params.name
    },
  },
  series: [{
    type: 'graph',
    layout: 'circular',
    symbolSize: (data) => Math.sqrt(data.callCount) * 3,
    roam: true,
    label: { show: true, fontSize: 12, color: '#64748b' },
    lineStyle: { width: (data) => data.callCount / 50, color: '#cbd5e1' },
    emphasis: { focus: 'adjacency' },
    data: [
      { name: 'API Gateway', callCount: 10000 },
      { name: 'User Service', callCount: 5000 },
      { name: 'Order Service', callCount: 4000 },
      { name: 'Payment Service', callCount: 3000 },
      { name: 'Product Service', callCount: 2500 },
    ],
    links: [
      { source: 'API Gateway', target: 'User Service', callCount: 5000 },
      { source: 'API Gateway', target: 'Order Service', callCount: 4000 },
      { source: 'API Gateway', target: 'Product Service', callCount: 1000 },
      { source: 'Order Service', target: 'Payment Service', callCount: 3000 },
    ],
  }],
}))
</script>
