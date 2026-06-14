<template>
  <div class="space-y-6">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold text-slate-900 dark:text-white">业务系统</h1>
        <p class="text-sm text-slate-500 dark:text-slate-400 mt-1">模拟业务系统 - 产生流量供探针采集展示</p>
      </div>
      <div class="flex items-center gap-2">
        <span class="w-2 h-2 rounded-full" :class="health.status==='ok'?'bg-green-500':'bg-red-500'"></span>
        <span class="text-xs text-slate-500">{{ health.status==='ok' ? '运行中' : '离线' }}</span>
        <button class="btn-secondary" @click="fetchAll"><RefreshCw class="w-4 h-4" />刷新</button>
      </div>
    </div>

    <div class="grid grid-cols-4 gap-4">
      <div class="card p-4">
        <p class="text-sm text-slate-500">用户数</p>
        <p class="text-2xl font-bold text-slate-900 dark:text-white mt-1">{{ users.length }}</p>
      </div>
      <div class="card p-4">
        <p class="text-sm text-slate-500">订单数</p>
        <p class="text-2xl font-bold text-primary-500 mt-1">{{ orders.length }}</p>
      </div>
      <div class="card p-4">
        <p class="text-sm text-slate-500">订单总额</p>
        <p class="text-2xl font-bold text-accent-500 mt-1">{{ totalAmount }}</p>
      </div>
      <div class="card p-4">
        <p class="text-sm text-slate-500">产品数</p>
        <p class="text-2xl font-bold text-emerald-500 mt-1">{{ products.length }}</p>
      </div>
    </div>

    <div class="card p-6">
      <h3 class="text-lg font-semibold text-slate-900 dark:text-white mb-4">流量数据分析</h3>
      <div class="h-64">
        <ECharts :option="trafficChart" class="w-full h-full" />
      </div>
    </div>

    <div class="grid grid-cols-2 gap-6">
      <div class="card p-6">
        <h3 class="text-lg font-semibold text-slate-900 dark:text-white mb-4">用户列表</h3>
        <div class="space-y-2">
          <div v-for="u in users" :key="u.id" class="p-3 rounded-lg bg-slate-50 dark:bg-dark-700/50 flex items-center justify-between">
            <div>
              <span class="text-sm font-medium">{{ u.name }}</span>
              <span class="text-xs text-slate-500 ml-2">{{ u.email }}</span>
            </div>
            <span class="text-xs px-2 py-0.5 rounded" :class="u.role==='admin'?'bg-purple-100 text-purple-600':u.role==='user'?'bg-blue-100 text-blue-600':'bg-gray-100 text-gray-600'">{{ u.role }}</span>
          </div>
        </div>
      </div>
      <div class="card p-6">
        <h3 class="text-lg font-semibold text-slate-900 dark:text-white mb-4">订单列表</h3>
        <div class="space-y-2">
          <div v-for="o in orders" :key="o.id" class="p-3 rounded-lg bg-slate-50 dark:bg-dark-700/50">
            <div class="flex items-center justify-between">
              <span class="text-sm font-medium">#{{ o.id }}</span>
              <span class="text-sm font-bold">¥{{ o.amount.toFixed(2) }}</span>
            </div>
            <div class="flex items-center justify-between mt-1">
              <span class="text-xs text-slate-500">{{ o.items.join(', ') }}</span>
              <span class="text-xs px-2 py-0.5 rounded" :class="o.status==='completed'?'bg-green-100 text-green-600':o.status==='processing'?'bg-yellow-100 text-yellow-600':'bg-gray-100 text-gray-600'">{{ o.status }}</span>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { use } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { BarChart } from 'echarts/charts'
import { TooltipComponent, GridComponent } from 'echarts/components'
import VChart from 'vue-echarts'
import { RefreshCw } from 'lucide-vue-next'

use([CanvasRenderer, BarChart, TooltipComponent, GridComponent])
const ECharts = VChart

const health = ref({})
const users = ref([])
const orders = ref([])
const products = ref([])

const fetchAll = async () => {
  try {
    const [h, u, o, p] = await Promise.all([
      fetch('http://192.168.58.131:5000/api/health').then(r => r.json()).catch(() => ({status:'error'})),
      fetch('http://192.168.58.131:5000/api/users').then(r => r.json()).catch(() => ({users:[]})),
      fetch('http://192.168.58.131:5000/api/orders').then(r => r.json()).catch(() => ({orders:[]})),
      fetch('http://192.168.58.131:5000/api/products').then(r => r.json()).catch(() => ({products:[]})),
    ])
    health.value = h
    users.value = u.users || []
    orders.value = o.orders || []
    products.value = p.products || []
  } catch(e) { console.error(e) }
}

const totalAmount = computed(() => {
  const t = orders.value.reduce((s, o) => s + o.amount, 0)
  return '¥' + t.toFixed(2)
})

const trafficChart = computed(() => ({
  tooltip: { trigger: 'axis' },
  grid: { left: '3%', right: '4%', bottom: '10%', top: '5%' },
  xAxis: { type: 'category', data: ['用户API','订单API','产品API','健康检查'] },
  yAxis: { type: 'value', name: '请求数' },
  series: [{ type: 'bar', data: [users.length*5, orders.length*3, products.length*4, 10], itemStyle: { color: '#6366f1', borderRadius: [4,4,0,0] } }]
}))

onMounted(() => { fetchAll(); setInterval(fetchAll, 10000) })
</script>
