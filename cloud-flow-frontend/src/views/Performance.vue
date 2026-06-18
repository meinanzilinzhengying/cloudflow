<template>
  <div class="performance">
    <div class="page-header">
      <h2 class="page-title">系统性能</h2>
      <div class="header-actions">
        <el-select v-model="selectedHost" placeholder="选择主机" size="small" style="width: 160px">
          <el-option label="node-01 (192.168.1.101)" value="node-01" />
          <el-option label="node-02 (192.168.1.102)" value="node-02" />
        </el-select>
        <TimePicker v-model="timeRange" @change="fetchData" />
      </div>
    </div>
    <el-row :gutter="24" class="chart-row">
      <el-col :span="12">
        <el-card class="chart-card" :body-style="{ padding: '20px' }">
          <div class="card-title">CPU 调度延迟</div>
          <v-chart :option="cpuOption" autoresize style="height: 280px" />
        </el-card>
      </el-col>
      <el-col :span="12">
        <el-card class="chart-card" :body-style="{ padding: '20px' }">
          <div class="card-title">内存分配大小分布</div>
          <v-chart :option="memOption" autoresize style="height: 280px" />
        </el-card>
      </el-col>
    </el-row>
    <el-row :gutter="24" class="chart-row">
      <el-col :span="12">
        <el-card class="chart-card" :body-style="{ padding: '20px' }">
          <div class="card-title">块设备 IO 时延</div>
          <v-chart :option="blockOption" autoresize style="height: 280px" />
        </el-card>
      </el-col>
      <el-col :span="12">
        <el-card class="chart-card" :body-style="{ padding: '20px' }">
          <div class="card-title">进程资源排行 TOP 10</div>
          <el-table :data="topProcesses" size="small" style="width: 100%" max-height="280">
            <el-table-column prop="comm" label="进程名" />
            <el-table-column prop="pid" label="PID" width="80" />
            <el-table-column prop="cpu" label="CPU%" width="80">
              <template #default="{ row }">
                <el-progress :percentage="row.cpu" :stroke-width="6" :show-text="false" />
              </template>
            </el-table-column>
            <el-table-column prop="mem" label="内存" width="100" :formatter="(row: any) => formatBytes(row.mem)" />
          </el-table>
        </el-card>
      </el-col>
    </el-row>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import TimePicker from '@/components/TimePicker.vue'
import { getPerformance } from '@/api/performance'
import { formatBytes } from '@/utils/format'

const selectedHost = ref('node-01')
const timeRange = ref('1h')

const topProcesses = ref([
  { comm: 'nginx', pid: 1234, cpu: 35, mem: 104857600 },
  { comm: 'mysqld', pid: 2345, cpu: 28, mem: 209715200 },
  { comm: 'redis-server', pid: 3456, cpu: 15, mem: 52428800 },
  { comm: 'cloudflow-ebpf', pid: 4567, cpu: 12, mem: 94371840 },
  { comm: 'python3', pid: 5678, cpu: 8, mem: 67108864 },
  { comm: 'java', pid: 6789, cpu: 6, mem: 536870912 },
  { comm: 'kubelet', pid: 7890, cpu: 5, mem: 83886080 },
  { comm: 'dockerd', pid: 8901, cpu: 4, mem: 125829120 },
  { comm: 'sshd', pid: 9012, cpu: 2, mem: 16777216 },
  { comm: 'systemd', pid: 1, cpu: 1, mem: 25165824 },
])

const cpuOption = computed(() => ({
  tooltip: { trigger: 'axis' },
  legend: { data: ['P50', 'P95', 'P99'] },
  xAxis: { type: 'category', data: ['00:00', '04:00', '08:00', '12:00', '16:00', '20:00'] },
  yAxis: { type: 'value', name: 'μs' },
  series: [
    { name: 'P50', type: 'line', data: [50, 55, 60, 58, 62, 55], smooth: true },
    { name: 'P95', type: 'line', data: [120, 130, 150, 140, 160, 135], smooth: true },
    { name: 'P99', type: 'line', data: [300, 320, 380, 360, 400, 340], smooth: true },
  ]
}))

const memOption = computed(() => ({
  tooltip: { trigger: 'axis' },
  xAxis: { type: 'category', data: ['<64B', '64-256B', '256-1K', '1K-4K', '4K-16K', '16K-64K', '>64K'] },
  yAxis: { type: 'value' },
  series: [{
    type: 'bar', data: [5000, 3200, 1800, 900, 400, 150, 60],
    itemStyle: { borderRadius: [4, 4, 0, 0], color: '#165DFF' }
  }]
}))

const blockOption = computed(() => ({
  tooltip: { trigger: 'axis' },
  xAxis: { type: 'category', data: ['<1ms', '1-5ms', '5-10ms', '10-50ms', '50-100ms', '>100ms'] },
  yAxis: { type: 'value' },
  series: [{
    type: 'bar', data: [8000, 3500, 1200, 300, 50, 10],
    itemStyle: { borderRadius: [4, 4, 0, 0], color: '#00B42A' }
  }]
}))

const fetchData = async () => {
  try {
    const res = await getPerformance(selectedHost.value, { range: timeRange.value })
    if (res.code === 0) {
      topProcesses.value = res.data.topProcesses
    }
  } catch (e) { /* ignore */ }
}

onMounted(fetchData)
</script>

<style scoped lang="scss">
.performance {
  .page-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 20px;
    .page-title {
      font-size: 18px;
      font-weight: 600;
      color: var(--el-text-color-primary);
    }
    .header-actions {
      display: flex;
      gap: 12px;
      align-items: center;
    }
  }
  .chart-row {
    margin-bottom: 24px;
  }
  .chart-card {
    border-radius: 8px;
    box-shadow: 0 2px 8px rgba(0, 0, 0, 0.08);
    .card-title {
      font-size: 16px;
      font-weight: 600;
      color: var(--el-text-color-primary);
      margin-bottom: 16px;
    }
  }
}
</style>
