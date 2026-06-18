<template>
  <div class="performance">
    <div class="page-header">
      <h2 class="page-title">系统性能</h2>
      <div class="header-actions">
        <el-tag type="success" size="small">vm2 (192.168.58.131)</el-tag>
        <TimePicker v-model="timeRange" @change="fetchData" />
      </div>
    </div>

    <!-- CPU 使用率趋势 -->
    <el-row :gutter="24" class="chart-row">
      <el-col :span="12">
        <el-card class="chart-card" :body-style="{ padding: '20px' }">
          <div class="card-title">CPU 使用率趋势</div>
          <v-chart :option="cpuOption" autoresize style="height: 260px" />
        </el-card>
      </el-col>
      <el-col :span="12">
        <el-card class="chart-card" :body-style="{ padding: '20px' }">
          <div class="card-title">内存使用趋势 (MB)</div>
          <v-chart :option="memOption" autoresize style="height: 260px" />
        </el-card>
      </el-col>
    </el-row>

    <!-- 当前内存状态 + 进程表 -->
    <el-row :gutter="24" class="chart-row">
      <el-col :span="8">
        <el-card class="chart-card" :body-style="{ padding: '20px' }">
          <div class="card-title">内存使用详情</div>
          <v-chart :option="memPieOption" autoresize style="height: 260px" />
        </el-card>
      </el-col>
      <el-col :span="16">
        <el-card class="chart-card" :body-style="{ padding: '20px' }">
          <div class="card-title">进程列表 (Top {{ processes.length }})</div>
          <el-table :data="processes" size="small" style="width: 100%" max-height="260">
            <el-table-column prop="name" label="进程名" />
            <el-table-column prop="pid" label="PID" width="80" />
            <el-table-column prop="status" label="状态" width="90">
              <template #default="{ row }">
                <el-tag :type="row.status === 'execve' ? 'success' : row.status === 'exit' ? 'info' : 'warning'" size="small">{{ row.status }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="cpu" label="CPU%" width="80">
              <template #default="{ row }">
                <el-progress :percentage="row.cpu" :stroke-width="6" :show-text="false" :color="row.cpu > 80 ? '#F56C6C' : '#409EFF'" />
              </template>
            </el-table-column>
            <el-table-column prop="threads" label="线程数" width="70" />
          </el-table>
        </el-card>
      </el-col>
    </el-row>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import TimePicker from '@/components/TimePicker.vue'
import { getPerformanceCpu, getPerformanceMemory, getPerformanceProcess } from '@/api/performance'
import type { CpuDataPoint, MemoryData, ProcessRecord } from '@/api/performance'

const timeRange = ref('1h')
const cpuData = ref<CpuDataPoint[]>([])
const memData = ref<MemoryData>({ total: 0, free: 0, buffers: 0, cache: 0, trend: [] })
const processes = ref<ProcessRecord[]>([])

const cpuOption = computed(() => ({
  tooltip: { trigger: 'axis', formatter: (p: any[]) => p.map(i => `${i.seriesName}: ${i.value.toFixed(2)}%`).join('<br>') },
  legend: { data: ['总使用率', 'User', 'System', 'IOWait'], right: 0, textStyle: { fontSize: 11 } },
  grid: { left: '3%', right: '4%', bottom: '3%', containLabel: true },
  xAxis: { type: 'category', data: cpuData.value.map(i => i.time), boundaryGap: false },
  yAxis: { type: 'value', name: '%', max: 100 },
  series: [
    { name: '总使用率', type: 'line', data: cpuData.value.map(i => Number(i.usage.toFixed(2))), smooth: true, lineStyle: { color: '#409EFF' }, areaStyle: { opacity: 0.2 } },
    { name: 'User', type: 'line', data: cpuData.value.map(i => Number(i.user.toFixed(2))), smooth: true, lineStyle: { color: '#67C23A' } },
    { name: 'System', type: 'line', data: cpuData.value.map(i => Number(i.system.toFixed(2))), smooth: true, lineStyle: { color: '#E6A23C' } },
    { name: 'IOWait', type: 'line', data: cpuData.value.map(i => Number(i.iowait.toFixed(2))), smooth: true, lineStyle: { color: '#F56C6C' } },
  ]
}))

const memOption = computed(() => ({
  tooltip: { trigger: 'axis', formatter: (p: any[]) => p.map(i => `${i.seriesName}: ${i.value.toFixed(0)} MB`).join('<br>') },
  legend: { data: ['已使用', '空闲'], right: 0 },
  grid: { left: '3%', right: '4%', bottom: '3%', containLabel: true },
  xAxis: { type: 'category', data: (memData.value.trend || []).map(i => i.time), boundaryGap: false },
  yAxis: { type: 'value', name: 'MB' },
  series: [
    { name: '已使用', type: 'line', data: (memData.value.trend || []).map(i => Number(i.used.toFixed(0))), smooth: true, areaStyle: { opacity: 0.3, color: '#E6A23C' }, lineStyle: { color: '#E6A23C' } },
    { name: '空闲', type: 'line', data: (memData.value.trend || []).map(i => Number(i.free.toFixed(0))), smooth: true, areaStyle: { opacity: 0.3, color: '#67C23A' }, lineStyle: { color: '#67C23A' } },
  ]
}))

const memPieOption = computed(() => {
  const used = memData.value.total - memData.value.free - (memData.value.buffers || 0) - (memData.value.cache || 0)
  return {
    tooltip: { trigger: 'item', formatter: '{b}: {c} MB ({d}%)' },
    legend: { bottom: 0 },
    series: [{
      type: 'pie', radius: ['40%', '68%'],
      itemStyle: { borderRadius: 6, borderColor: '#fff', borderWidth: 2 },
      label: { show: true, formatter: '{b}\n{d}%' },
      data: [
        { name: '已使用', value: Math.max(0, Math.round(used)) },
        { name: 'Buffer/Cache', value: Math.round((memData.value.buffers || 0) + (memData.value.cache || 0)) },
        { name: '空闲', value: Math.round(memData.value.free || 0) },
      ].filter(i => i.value > 0)
    }]
  }
})

const fetchData = async () => {
  try {
    const cpuRes = await getPerformanceCpu()
    if (cpuRes.code === 0 && Array.isArray(cpuRes.data)) cpuData.value = cpuRes.data
  } catch (e) {}
  try {
    const memRes = await getPerformanceMemory()
    if (memRes.code === 0 && memRes.data) memData.value = memRes.data
  } catch (e) {}
  try {
    const procRes = await getPerformanceProcess()
    if (procRes.code === 0 && Array.isArray(procRes.data)) processes.value = procRes.data.slice(0, 20)
  } catch (e) {}
}

let timer: ReturnType<typeof setInterval> | null = null
onMounted(() => { fetchData(); timer = setInterval(fetchData, 30000) })
onUnmounted(() => { if (timer) clearInterval(timer) })
</script>

<style scoped lang="scss">
.performance {
  .page-header {
    display: flex; justify-content: space-between; align-items: center; margin-bottom: 20px;
    .page-title { font-size: 18px; font-weight: 600; color: var(--el-text-color-primary); }
    .header-actions { display: flex; gap: 12px; align-items: center; }
  }
  .chart-row { margin-bottom: 24px; }
  .chart-card {
    border-radius: 8px; box-shadow: 0 2px 8px rgba(0,0,0,0.08);
    .card-title { font-size: 14px; font-weight: 600; color: var(--el-text-color-primary); margin-bottom: 16px; }
  }
}
</style>
