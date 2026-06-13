<template>
  <div class="space-y-6">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold text-slate-900 dark:text-white">主机指标</h1>
        <p class="text-sm text-slate-500 dark:text-slate-400 mt-1">监控主机性能指标</p>
      </div>
    </div>

    <div class="grid grid-cols-4 gap-4">
      <div class="card p-4">
        <p class="text-sm text-slate-500">CPU使用率</p>
        <p class="text-2xl font-bold text-primary-500 mt-1">--</p>
      </div>
      <div class="card p-4">
        <p class="text-sm text-slate-500">内存使用</p>
        <p class="text-2xl font-bold text-accent-500 mt-1">--</p>
      </div>
      <div class="card p-4">
        <p class="text-sm text-slate-500">磁盘I/O</p>
        <p class="text-2xl font-bold text-emerald-500 mt-1">-- MB/s</p>
      </div>
      <div class="card p-4">
        <p class="text-sm text-slate-500">网络带宽</p>
        <p class="text-2xl font-bold text-amber-500 mt-1">-- Mbps</p>
      </div>
    </div>

    <div class="grid grid-cols-2 gap-6">
      <div class="card p-6">
        <h3 class="text-lg font-semibold text-slate-900 dark:text-white mb-4">CPU趋势</h3>
        <div class="h-64">
          <ECharts :option="cpuOption" class="w-full h-full" />
        </div>
      </div>
      <div class="card p-6">
        <h3 class="text-lg font-semibold text-slate-900 dark:text-white mb-4">内存趋势</h3>
        <div class="h-64">
          <ECharts :option="memoryOption" class="w-full h-full" />
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { use } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { LineChart } from 'echarts/charts'
import { TooltipComponent, GridComponent } from 'echarts/components'
import ECharts from 'vue-echarts'

use([CanvasRenderer, LineChart, TooltipComponent, GridComponent])

const cpuOption = computed(() => ({
  tooltip: { trigger: 'axis', backgroundColor: 'rgba(255,255,255,0.95)', textStyle: { color: '#1e293b' } },
  grid: { left: '3%', right: '4%', bottom: '3%', top: '10%', containLabel: true },
  xAxis: { type: 'category', data: [], axisLabel: {} },
  yAxis: { type: 'value', max: 100, axisLabel: { color: '#64748b', formatter: '{value}%' }, splitLine: { lineStyle: { color: '#f1f5f9' } } },
  series: [{ 
    type: 'line', 
    smooth: true, 
    lineStyle: { color: '#2563eb', width: 2 }, 
    areaStyle: { 
      color: { 
        type: 'linear', 
        x: 0, y: 0, x2: 0, y2: 1, 
        colorStops: [{ offset: 0, color: 'rgba(37,99,235,0.15)' }, { offset: 1, color: 'rgba(37,99,235,0)' }] 
      } 
    }, 
    data: [] 
  }],
}))

const memoryOption = computed(() => ({
  tooltip: { trigger: 'axis', backgroundColor: 'rgba(255,255,255,0.95)', textStyle: { color: '#1e293b' } },
  grid: { left: '3%', right: '4%', bottom: '3%', top: '10%', containLabel: true },
  xAxis: { type: 'category', data: [], axisLabel: {} },
  yAxis: { type: 'value', max: 16, axisLabel: { color: '#64748b', formatter: '{value} GB' }, splitLine: { lineStyle: { color: '#f1f5f9' } } },
  series: [{ 
    type: 'line', 
    smooth: true, 
    lineStyle: { color: '#14b8a6', width: 2 }, 
    areaStyle: { 
      color: { 
        type: 'linear', 
        x: 0, y: 0, x2: 0, y2: 1, 
        colorStops: [{ offset: 0, color: 'rgba(20,184,166,0.15)' }, { offset: 1, color: 'rgba(20,184,166,0)' }] 
      } 
    }, 
    data: [] 
  }],
}))
</script>
