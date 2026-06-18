<template>
  <div class="dashboard">
    <el-row :gutter="16" class="metric-row">
      <el-col :span="4">
        <el-card class="stat-card" :body-style="{ padding: '16px' }">
          <div class="stat-title">探针总数</div>
          <div class="stat-value">{{ overview.probeTotal }}</div>
        </el-card>
      </el-col>
      <el-col :span="4">
        <el-card class="stat-card" :body-style="{ padding: '16px' }">
          <div class="stat-title">在线探针</div>
          <div class="stat-value">{{ overview.probeOnline }}</div>
        </el-card>
      </el-col>
      <el-col :span="4">
        <el-card class="stat-card" :body-style="{ padding: '16px' }">
          <div class="stat-title">今日流量</div>
          <div class="stat-value">{{ overview.todayTraffic }}</div>
        </el-card>
      </el-col>
      <el-col :span="4">
        <el-card class="stat-card" :body-style="{ padding: '16px' }">
          <div class="stat-title">活跃告警</div>
          <div class="stat-value">{{ overview.activeAlerts }}</div>
        </el-card>
      </el-col>
      <el-col :span="4">
        <el-card class="stat-card" :body-style="{ padding: '16px' }">
          <div class="stat-title">监控主机</div>
          <div class="stat-value">{{ overview.monitoredHosts }}</div>
        </el-card>
      </el-col>
      <el-col :span="4">
        <el-card class="stat-card" :body-style="{ padding: '16px' }">
          <div class="stat-title">活跃连接</div>
          <div class="stat-value">{{ overview.activeConnections }}</div>
        </el-card>
      </el-col>
    </el-row>

    <el-row :gutter="16" class="status-row">
      <el-col :span="8">
        <el-card class="status-card" :body-style="{ padding: '20px' }">
          <div class="card-title">探针状态</div>
          <div class="status-grid">
            <div v-for="(probe, idx) in probeStatusList" :key="idx"
                 class="status-block"
                 :class="probe.status"
                 :title="probe.name"
            ></div>
          </div>
        </el-card>
      </el-col>
      <el-col :span="8">
        <el-card class="status-card" :body-style="{ padding: '20px' }">
          <div class="card-title">主机状态</div>
          <div class="status-grid">
            <div v-for="(host, idx) in hostStatusList" :key="idx"
                 class="status-block"
                 :class="host.status"
                 :title="host.name"
            ></div>
          </div>
        </el-card>
      </el-col>
      <el-col :span="8">
        <el-card class="status-card" :body-style="{ padding: '20px' }">
          <div class="card-title">服务状态</div>
          <div class="status-grid">
            <div v-for="(svc, idx) in serviceStatusList" :key="idx"
                 class="status-block"
                 :class="svc.status"
                 :title="svc.name"
            ></div>
          </div>
        </el-card>
      </el-col>
    </el-row>

    <el-row :gutter="16" class="bottom-row">
      <el-col :span="6">
        <el-card class="alert-card" :body-style="{ padding: '20px' }">
          <div class="card-title">主机告警</div>
          <el-table :data="hostAlerts" size="small" style="width:100%" :show-header="false">
            <el-table-column prop="name" label="主机" />
            <el-table-column prop="count" label="告警数" width="60">
              <template #default="{ row }">
                <el-tag :type="row.count > 5 ? 'danger' : 'warning'" size="small">{{ row.count }}</el-tag>
              </template>
            </el-table-column>
          </el-table>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card class="alert-card" :body-style="{ padding: '20px' }">
          <div class="card-title">服务告警</div>
          <el-table :data="serviceAlerts" size="small" style="width:100%" :show-header="false">
            <el-table-column prop="name" label="服务" />
            <el-table-column prop="count" label="告警数" width="60">
              <template #default="{ row }">
                <el-tag :type="row.count > 5 ? 'danger' : 'warning'" size="small">{{ row.count }}</el-tag>
              </template>
            </el-table-column>
          </el-table>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card class="top-card" :body-style="{ padding: '20px' }">
          <div class="card-title">Top 5 HTTP 延迟</div>
          <div v-for="(item, idx) in topHttpLatency" :key="idx" class="top-bar-item">
            <span class="top-name">{{ item.name }}</span>
            <el-progress :percentage="item.percent" :stroke-width="12" :color="'#409EFF'" :show-text="false" />
            <span class="top-value">{{ item.value }} ms</span>
          </div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card class="top-card" :body-style="{ padding: '20px' }">
          <div class="card-title">Top 5 TCP 延迟</div>
          <div v-for="(item, idx) in topTcpLatency" :key="idx" class="top-bar-item">
            <span class="top-name">{{ item.name }}</span>
            <el-progress :percentage="item.percent" :stroke-width="12" :color="'#409EFF'" :show-text="false" />
            <span class="top-value">{{ item.value }} ms</span>
          </div>
        </el-card>
      </el-col>
    </el-row>

    <el-row :gutter="16" class="bottom-row">
      <el-col :span="6">
        <el-card class="top-card" :body-style="{ padding: '20px' }">
          <div class="card-title">Top 5 CPU 主机</div>
          <div v-for="(item, idx) in topCpu" :key="idx" class="top-bar-item">
            <span class="top-name">{{ item.name }}</span>
            <el-progress :percentage="item.percent" :stroke-width="12" :color="'#E6A23C'" :show-text="false" />
            <span class="top-value">{{ item.value }}%</span>
          </div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card class="top-card" :body-style="{ padding: '20px' }">
          <div class="card-title">Top 5 内存 主机</div>
          <div v-for="(item, idx) in topMemory" :key="idx" class="top-bar-item">
            <span class="top-name">{{ item.name }}</span>
            <el-progress :percentage="item.percent" :stroke-width="12" :color="'#67C23A'" :show-text="false" />
            <span class="top-value">{{ item.value }}%</span>
          </div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card class="top-card" :body-style="{ padding: '20px' }">
          <div class="card-title">Top TCP 连接数</div>
          <div v-for="(item, idx) in topConnections" :key="idx" class="top-bar-item">
            <span class="top-name">{{ item.name }}</span>
            <el-progress :percentage="item.percent" :stroke-width="12" :color="'#409EFF'" :show-text="false" />
            <span class="top-value">{{ item.value }} 个</span>
          </div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card class="top-card" :body-style="{ padding: '20px' }">
          <div class="card-title">Top 5 流量主机</div>
          <div v-for="(item, idx) in topTraffic" :key="idx" class="top-bar-item">
            <span class="top-name">{{ item.name }}</span>
            <el-progress :percentage="item.percent" :stroke-width="12" :color="'#409EFF'" :show-text="false" />
            <span class="top-value">{{ item.value }}</span>
          </div>
        </el-card>
      </el-col>
    </el-row>

    <el-row :gutter="16" class="chart-row">
      <el-col :span="14">
        <el-card class="chart-card" :body-style="{ padding: '20px' }">
          <div class="card-title">流量趋势 (24小时)</div>
          <v-chart :option="flowOption" autoresize style="height: 280px" />
        </el-card>
      </el-col>
      <el-col :span="10">
        <el-card class="chart-card" :body-style="{ padding: '20px' }">
          <div class="card-title">协议分布</div>
          <v-chart :option="protocolOption" autoresize style="height: 280px" />
        </el-card>
      </el-col>
    </el-row>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted } from 'vue'
import { getOverview } from '@/api/dashboard'
import type { DashboardOverview } from '@/api/dashboard'
import { formatBytes } from '@/utils/format'

const overview = reactive<DashboardOverview & { activeConnections: number }>({
  probeOnline: 12, probeTotal: 15, todayTraffic: '1.2TB', trafficTrend: '+15%',
  activeAlerts: 3, alertTrend: '-1', monitoredHosts: 42, hostTrend: '+5',
  flowTrend: [], protocolDist: [], topHosts: [], recentAlerts: [],
  activeConnections: 104
})


const fetchData = async () => {
  try {
    const res = await getOverview()
    if (res.code === 0) Object.assign(overview, res.data)
  } catch (e) {}
}

const probeStatusList = ref([
  { name: 'node-01', status: 'healthy' }, { name: 'node-02', status: 'healthy' },
  { name: 'node-03', status: 'warning' }, { name: 'node-04', status: 'offline' },
  { name: 'node-05', status: 'healthy' }, { name: 'node-06', status: 'healthy' },
  { name: 'node-07', status: 'healthy' }, { name: 'node-08', status: 'warning' },
])
const hostStatusList = ref([
  { name: '192.168.1.101', status: 'healthy' }, { name: '192.168.1.102', status: 'healthy' },
  { name: '192.168.1.103', status: 'healthy' }, { name: '192.168.1.104', status: 'offline' },
  { name: '192.168.1.105', status: 'healthy' }, { name: '192.168.1.106', status: 'warning' },
  { name: '192.168.1.107', status: 'healthy' }, { name: '192.168.1.108', status: 'healthy' },
])
const serviceStatusList = ref([
  { name: 'nginx', status: 'healthy' }, { name: 'mysql', status: 'healthy' },
  { name: 'redis', status: 'healthy' }, { name: 'api-gateway', status: 'warning' },
  { name: 'kafka', status: 'healthy' }, { name: 'elasticsearch', status: 'healthy' },
  { name: 'prometheus', status: 'healthy' }, { name: 'grafana', status: 'offline' },
])
const hostAlerts = ref([
  { name: 'node-01', count: 2 }, { name: 'node-03', count: 5 }, { name: 'node-04', count: 1 },
])
const serviceAlerts = ref([
  { name: 'api-gateway', count: 3 }, { name: 'mysql', count: 1 }, { name: 'redis', count: 1 },
])
const topHttpLatency = ref([
  { name: 'loadgenerator', value: 39.92, percent: 100 },
  { name: 'nginxsvc-v1', value: 19.79, percent: 50 },
  { name: 'productpage', value: 15.44, percent: 39 },
  { name: 'reviews-v3', value: 3.96, percent: 10 },
  { name: 'reviews-v2', value: 3.55, percent: 9 },
])
const topTcpLatency = ref([
  { name: 'loadgenerator', value: 4.62, percent: 100 },
  { name: 'nginxsvc-v1', value: 4.19, percent: 91 },
  { name: 'reviews-v2', value: 2.86, percent: 62 },
  { name: 'productpage', value: 2.86, percent: 62 },
  { name: 'reviews-v3', value: 2.66, percent: 58 },
])
const topCpu = ref([
  { name: 'node-03', value: 78, percent: 78 },
  { name: 'node-01', value: 35, percent: 35 },
  { name: 'node-02', value: 28, percent: 28 },
  { name: 'node-05', value: 22, percent: 22 },
  { name: 'node-06', value: 15, percent: 15 },
])
const topMemory = ref([
  { name: 'node-03', value: 65, percent: 65 },
  { name: 'node-01', value: 42, percent: 42 },
  { name: 'node-02', value: 38, percent: 38 },
  { name: 'node-05', value: 30, percent: 30 },
  { name: 'node-06', value: 25, percent: 25 },
])
const topConnections = ref([
  { name: 'node-01', value: 104, percent: 100 },
  { name: 'node-02', value: 77, percent: 74 },
  { name: 'node-05', value: 56, percent: 54 },
  { name: 'node-03', value: 43, percent: 41 },
  { name: 'node-06', value: 32, percent: 31 },
])
const topTraffic = ref([
  { name: '192.168.1.101', value: '2.3GB', percent: 100 },
  { name: '192.168.1.102', value: '1.8GB', percent: 78 },
  { name: '192.168.1.105', value: '1.2GB', percent: 52 },
  { name: '192.168.1.103', value: '0.9GB', percent: 39 },
  { name: '192.168.1.106', value: '0.6GB', percent: 26 },
])

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

onMounted(() => { fetchData() })
</script>

<style scoped lang="scss">
.dashboard {
  .metric-row { margin-bottom: 16px; }
  .stat-card {
    border-radius: 8px; box-shadow: 0 2px 8px rgba(0,0,0,0.08);
    text-align: center;
    .stat-title { font-size: 12px; color: var(--el-text-color-secondary); margin-bottom: 8px; }
    .stat-value { font-size: 22px; font-weight: 600; color: var(--el-text-color-primary); }
  }
  .status-row { margin-bottom: 16px; }
  .status-card {
    border-radius: 8px; box-shadow: 0 2px 8px rgba(0,0,0,0.08);
    .card-title { font-size: 14px; font-weight: 600; color: var(--el-text-color-primary); margin-bottom: 12px; }
    .status-grid { display: flex; flex-wrap: wrap; gap: 6px; }
    .status-block {
      width: 28px; height: 28px; border-radius: 4px; cursor: pointer; transition: transform 0.2s;
      &:hover { transform: scale(1.1); }
      &.healthy { background: #67C23A; }
      &.warning { background: #E6A23C; }
      &.offline { background: #F56C6C; }
    }
  }
  .bottom-row { margin-bottom: 16px; }
  .alert-card, .top-card {
    border-radius: 8px; box-shadow: 0 2px 8px rgba(0,0,0,0.08);
    .card-title { font-size: 14px; font-weight: 600; color: var(--el-text-color-primary); margin-bottom: 12px; }
    .top-bar-item {
      display: flex; align-items: center; gap: 8px; margin-bottom: 8px;
      .top-name { width: 100px; font-size: 12px; color: var(--el-text-color-regular); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
      .top-value { width: 70px; font-size: 12px; color: var(--el-text-color-secondary); text-align: right; }
      :deep(.el-progress) { flex: 1; }
    }
  }
  .chart-row { margin-bottom: 16px; }
  .chart-card {
    border-radius: 8px; box-shadow: 0 2px 8px rgba(0,0,0,0.08);
    .card-title { font-size: 16px; font-weight: 600; color: var(--el-text-color-primary); margin-bottom: 16px; }
  }
}
</style>
