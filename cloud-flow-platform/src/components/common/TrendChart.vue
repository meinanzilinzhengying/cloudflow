<template>
  <div class="bg-dark-800 rounded-xl p-5 border border-dark-600">
    <div class="flex items-center justify-between mb-4">
      <div>
        <h3 class="font-semibold text-white">{{ title }}</h3>
        <p class="text-gray-400 text-sm">{{ subtitle }}</p>
      </div>
      <div class="flex gap-4 text-xs text-gray-400">
        <span v-for="legend in legends" :key="legend.label" class="flex items-center gap-1">
          <span class="w-3 h-3 rounded-full" :style="{ backgroundColor: legend.color }"></span>
          {{ legend.label }}
        </span>
      </div>
    </div>
    <div class="h-64">
      <canvas ref="chartRef"></canvas>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, watch, onUnmounted } from 'vue'
import { Chart, registerables } from 'chart.js'

Chart.register(...registerables)

const props = defineProps({
  title: { type: String, required: true },
  subtitle: { type: String, default: '' },
  type: { type: String, default: 'line' },
  data: { type: Object, default: () => ({}) },
  options: { type: Object, default: () => ({}) },
  legends: { type: Array, default: () => [] }
})

const chartRef = ref(null)
let chartInstance = null

const defaultOptions = {
  responsive: true,
  maintainAspectRatio: false,
  interaction: {
    mode: 'index',
    intersect: false
  },
  plugins: {
    legend: {
      display: false
    },
    tooltip: {
      backgroundColor: '#1f2937',
      titleColor: '#e0e6ed',
      bodyColor: '#9ca3af',
      borderColor: '#374151',
      borderWidth: 1,
      padding: 12,
      cornerRadius: 8
    }
  },
  scales: {
    x: {
      grid: {
        color: '#1f2937'
      },
      ticks: {
        color: '#6b7280',
        maxTicksLimit: 10
      }
    },
    y: {
      grid: {
        color: '#1f2937'
      },
      ticks: {
        color: '#6b7280'
      }
    }
  }
}

const createChart = () => {
  if (!chartRef.value) return
  
  const ctx = chartRef.value.getContext('2d')
  
  if (chartInstance) {
    chartInstance.destroy()
  }
  
  chartInstance = new Chart(ctx, {
    type: props.type,
    data: props.data,
    options: { ...defaultOptions, ...props.options }
  })
}

onMounted(() => {
  createChart()
})

watch(() => props.data, () => {
  createChart()
}, { deep: true })

onUnmounted(() => {
  if (chartInstance) {
    chartInstance.destroy()
  }
})
</script>
