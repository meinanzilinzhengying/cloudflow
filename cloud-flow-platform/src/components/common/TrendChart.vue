<template>
  <div class="bg-dark-200 rounded-xl p-5 border border-dark-100">
    <div class="flex items-center justify-between mb-4">
      <span class="text-sm text-gray-400">{{ title }}</span>
      <span class="text-xs text-gray-500">{{ subtitle }}</span>
    </div>
    <Line :data="chartData" :options="chartOptions" />
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { Line } from 'vue-chartjs'
import {
  Chart as ChartJS,
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  Title,
  Tooltip,
  Legend,
  Filler
} from 'chart.js'

ChartJS.register(CategoryScale, LinearScale, PointElement, LineElement, Title, Tooltip, Legend, Filler)

const props = defineProps({
  title: { type: String, required: true },
  subtitle: { type: String, default: '' },
  labels: { type: Array, default: () => [] },
  data: { type: Array, default: () => [] },
  color: { type: String, default: '#89b4fa' }
})

const chartData = computed(() => ({
  labels: props.labels,
  datasets: [{
    data: props.data,
    borderColor: props.color,
    backgroundColor: props.color + '20',
    fill: true,
    tension: 0.4,
    pointRadius: 0,
    pointHoverRadius: 4
  }]
}))

const chartOptions = {
  responsive: true,
  maintainAspectRatio: false,
  plugins: {
    legend: { display: false },
    tooltip: {
      backgroundColor: '#1e1e2e',
      borderColor: '#313244',
      borderWidth: 1,
      titleColor: '#cdd6f4',
      bodyColor: '#cdd6f4',
      padding: 10,
      displayColors: false
    }
  },
  scales: {
    x: {
      grid: { color: '#181825' },
      ticks: { color: '#6c7086', font: { size: 10 } }
    },
    y: {
      grid: { color: '#181825' },
      ticks: { color: '#6c7086', font: { size: 10 } }
    }
  }
}
</script>
