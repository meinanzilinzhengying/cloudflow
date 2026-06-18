<template>
  <div class="dashboard">
    <div class="page-header">
      <h2 class="page-title">总览仪表盘</h2>
      <el-tag type="info" size="small">自动刷新 {{ countdown }}s</el-tag>
    </div>
    <el-row :gutter="24" class="metric-row">
      <el-col :span="6">
        <MetricCard title="探针在线" :value="`${overview.probeOnline}/${overview.probeTotal}`" :trend="2.3" />
      </el-col>
      <el-col :span="6">
        <MetricCard title="今日流量" :value="overview.todayTraffic" :trend="15" />
      </el-col>
      <el-col :span="6">
        <MetricCard title="活跃告警" :value="String(overview.activeAlerts)" :trend="-1" />
      </el-col>
      <el-col :span="6">
        <MetricCard title="监控主机" :value="String(overview.monitoredHosts)" :trend="5" />
      </el-col>
    </el-row>
    <el-row :gutter="24" class="chart-row">
      <el-col :span="14">
        <el-card class="chart-card" :body-style="{ padding: '20px' }">
          <div class="card-title">流量趋势 (24小时)</div>
          <v-chart :option="flowOption" autoresize style="height: 300px" />
        </el-card>
      </el-col>
      <el-col :span="10">
        <el-card class="chart-card" :body-style="{ padding: '20px' }">
          <div class="card-title">协议分布</div>
          <v-chart :option="protocolOption" autoresize style="height: 300px" />
        </el-card>
      </el-col>
    </el-row>
    <el-row :gutter="24" class="table-row">
      <el-col :span="12">
        <el-card class="table-card" :body-style="{ padding: '20px' }">
          <div class="card-title">TOP 5 流量主机</div>
          <el-table :data="overview.topHosts" size="small" style="width: 100%">
            <el-table-column prop="ip" label="IP地址" />
            <el-table-column prop="bytes" label="流量" :formatter="(row: any) => formatBytes(row.bytes)" />
            <el-table-column prop="percent" label="占比" />
          </el-table>
        </el-card>
      </el-col>
      <el-col :span="12">
        <el-card class="table-card" :body-style="{ padding: '20px' }">
          <div class="card-title">最近告警</div>
          <el-table :data="overview.recentAlerts" size="small" style="width: 100%">
            <el-table-column prop="time" label="时间" width="160" />
            <el-table-column prop="level" label="级别" width="80">
              <template #default="{ row }">
                <StatusTag :status="row.level" />
              </template>
            </el-table-column>
            <el-table-column prop="message" label="描述" show-overflow-tooltip />
          </el-table>
        </el-card>
      </el-col>
    </el-row>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted, onUnmounted } from 'vue'
import MetricCard from '@/components/MetricCard.vue'
import StatusTag from '@/components/StatusTag.vue'
import { getOverview } from '@/api/dashboard'
import type { DashboardOverview } from '@/api/dashboard'
import { formatBytes } from '@/utils/format'

const overview = reactive<DashboardOverview>({
  probeOnline: 12, probeTotal: 15, todayTraffic: '1.2TB', trafficTrend: '+15%',
  activeAlerts: 3, alertTrend: '-1', monitoredHosts: 42, hostTrend: '+5',
  flowTrend: [], protocolDist: [], topHosts: [], recentAlerts: []
})

const countdown = ref(30)
let timer: ReturnType<typeof setInterval> | null = null
let countTimer: ReturnType<typeof setInterval> | null = null

const fetchData = async () => {
  try {
    const res = await getOverview()
    if (res.code === 0) {
      Object.assign(overview, res.data)
    }
  } catch (e) { /* ignore */ }
}

const flowOption = computed(() => ({
  tooltip: { trigger: 'axis' },
  legend: { data: ['上行', '下行'] },
  xAxis: { type: 'category', data: overview.flowTrend.map((i: any) => i.time) },
  yAxis: { type: 'value', axisLabel: { formatter: (v: number) => formatBytes(v) } },
  series: [
    { name: '上行', type: 'line', areaStyle: { opacity: 0.3 }, data: overview.flowTrend.map((i: any) => i.tx), smooth: true },
    { name: '下行', type: 'line', areaStyle: { opacity: 0.3 }, data: overview.flowTrend.map((i: any) => i.rx), smooth: true }
  ]
}))

const protocolOption = computed(() => ({
  tooltip: { trigger: 'item' },
  legend: { bottom: 0 },
  series: [{
    type: 'pie', radius: ['40%', '70%'], avoidLabelOverlap: false,
    itemStyle: { borderRadius: 6, borderColor: '#fff', borderWidth: 2 },
    label: { show: true, formatter: '{b}: {d}%' },
    data: overview.protocolDist
  }]
}))

onMounted(() => {
  fetchData()
  timer = setInterval(fetchData, 30000)
  countTimer = setInterval(() => { countdown.value = countdown.value > 1 ? countdown.value - 1 : 30 }, 1000)
})

onUnmounted(() => {
  if (timer) clearInterval(timer)
  if (countTimer) clearInterval(countTimer)
})
</script>

<style scoped lang="scss">
.dashboard {
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
  }
  .metric-row { margin-bottom: 24px; }
  .chart-row { margin-bottom: 24px; }
  .chart-card, .table-card {
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
