<template>
  <div class="dashboard">
    <!-- 顶部统计卡 -->
    <el-row :gutter="16" class="metric-row">
      <el-col :span="4">
        <el-card class="stat-card" :body-style="{ padding: '16px' }">
          <div class="stat-title">探针总数</div>
          <div class="stat-value">{{ overview.probeTotal }}</div>
          <div class="stat-sub">在线 {{ overview.probeOnline }}</div>
        </el-card>
      </el-col>
      <el-col :span="4">
        <el-card class="stat-card" :body-style="{ padding: '16px' }">
          <div class="stat-title">在线探针</div>
          <div class="stat-value green">{{ overview.probeOnline }}</div>
          <div class="stat-sub">共 {{ overview.probeTotal }} 个</div>
        </el-card>
      </el-col>
      <el-col :span="4">
        <el-card class="stat-card" :body-style="{ padding: '16px' }">
          <div class="stat-title">今日流量</div>
          <div class="stat-value">{{ computedTodayTraffic }}</div>
          <div class="stat-sub trend">{{ overview.trafficTrend }}</div>
        </el-card>
      </el-col>
      <el-col :span="4">
        <el-card class="stat-card" :body-style="{ padding: '16px' }">
          <div class="stat-title">活跃告警</div>
          <div class="stat-value" :class="overview.activeAlerts > 0 ? 'red' : 'green'">{{ overview.activeAlerts }}</div>
          <div class="stat-sub">{{ overview.alertTrend }}</div>
        </el-card>
      </el-col>
      <el-col :span="4">
        <el-card class="stat-card" :body-style="{ padding: '16px' }">
          <div class="stat-title">监控主机</div>
          <div class="stat-value">{{ overview.monitoredHosts }}</div>
          <div class="stat-sub">{{ overview.hostTrend }}</div>
        </el-card>
      </el-col>
      <el-col :span="4">
        <el-card class="stat-card" :body-style="{ padding: '16px' }">
          <div class="stat-title">协议类型</div>
          <div class="stat-value">{{ protocolCount }}</div>
          <div class="stat-sub">已识别协议</div>
        </el-card>
      </el-col>
    </el-row>

    <!-- 流量趋势 + 协议分布 -->
    <el-row :gutter="16" class="chart-row">
      <el-col :span="15">
        <el-card class="chart-card" :body-style="{ padding: '20px' }">
          <div class="card-title">
            流量趋势
            <el-tag size="small" type="success" style="margin-left:8px">实时</el-tag>
          </div>
          <v-chart :option="flowOption" autoresize style="height: 260px" />
        </el-card>
      </el-col>
      <el-col :span="9">
        <el-card class="chart-card" :body-style="{ padding: '20px' }">
          <div class="card-title">协议分布</div>
          <v-chart :option="protocolOption" autoresize style="height: 260px" />
        </el-card>
      </el-col>
    </el-row>

    <!-- Top 排行 + 告警 -->
    <el-row :gutter="16" class="bottom-row">
      <el-col :span="6">
        <el-card class="top-card" :body-style="{ padding: '20px' }">
          <div class="card-title">Top CPU 主机</div>
          <div v-if="topCpu.length > 0">
            <div v-for="(item, idx) in topCpu" :key="idx" class="top-bar-item">
              <span class="top-name">{{ item.name }}</span>
              <el-progress :percentage="item.percent" :stroke-width="10" :color="cpuColor(item.percent)" :show-text="false" />
              <span class="top-value">{{ item.value.toFixed(1) }}%</span>
            </div>
          </div>
          <el-empty v-else description="暂无数据" :image-size="40" />
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card class="top-card" :body-style="{ padding: '20px' }">
          <div class="card-title">Top 内存主机</div>
          <div v-if="topMemory.length > 0">
            <div v-for="(item, idx) in topMemory" :key="idx" class="top-bar-item">
              <span class="top-name">{{ item.name }}</span>
              <el-progress :percentage="item.percent" :stroke-width="10" :color="'#67C23A'" :show-text="false" />
              <span class="top-value">{{ item.value.toFixed(1) }}%</span>
            </div>
          </div>
          <el-empty v-else description="暂无数据" :image-size="40" />
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card class="top-card" :body-style="{ padding: '20px' }">
          <div class="card-title">Top 流量主机</div>
          <div v-if="(overview.topHosts || []).length > 0">
            <div v-for="(item, idx) in overview.topHosts.slice(0,5)" :key="idx" class="top-bar-item">
              <span class="top-name">{{ item.ip }}</span>
              <el-progress :percentage="Number(item.percent) || 0" :stroke-width="10" :color="'#409EFF'" :show-text="false" />
              <span class="top-value">{{ formatBytes(item.bytes) }}</span>
            </div>
          </div>
          <el-empty v-else description="暂无数据" :image-size="40" />
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card class="top-card" :body-style="{ padding: '20px' }">
          <div class="card-title">最近告警</div>
          <div v-if="(overview.recentAlerts || []).length > 0">
            <div v-for="(item, idx) in overview.recentAlerts.slice(0,5)" :key="idx" class="alert-item">
              <el-tag :type="item.level === 'high' ? 'danger' : item.level === 'medium' ? 'warning' : 'info'" size="small">{{ item.level }}</el-tag>
              <span class="alert-msg">{{ item.message }}</span>
            </div>
          </div>
          <el-empty v-else description="暂无告警" :image-size="40" />
        </el-card>
      </el-col>
    </el-row>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted, onUnmounted } from 'vue'
import { getOverview } from '@/api/dashboard'
import type { DashboardOverview } from '@/api/dashboard'
import { formatBytes } from '@/utils/format'
import { getPerformanceCpu, getPerformanceMemory } from '@/api/performance'

const overview = reactive<DashboardOverview>({
  probeOnline: 0, probeTotal: 0, todayTraffic: '', trafficTrend: '',
  activeAlerts: 0, alertTrend: '', monitoredHosts: 0, hostTrend: '',
  flowTrend: [], protocolDist: [], topHosts: [], recentAlerts: []
})

const topCpu = ref<{ name: string; value: number; percent: number }[]>([])
const topMemory = ref<{ name: string; value: number; percent: number }[]>([])

const cpuColor = (pct: number) => pct > 80 ? '#F56C6C' : pct > 50 ? '#E6A23C' : '#67C23A'

const computedTodayTraffic = computed(() => {
  if (overview.todayTraffic) return overview.todayTraffic
  if (!overview.flowTrend || !overview.flowTrend.length) return '0 B'
  const total = overview.flowTrend.reduce((s: number, i: any) => s + (i.rx || 0) + (i.tx || 0), 0)
  return formatBytes(total)
})

const protocolCount = computed(() => {
  if (!overview.protocolDist) return 0
  return overview.protocolDist.filter((p: any) => p.name && p.name !== '').length
})

const flowOption = computed(() => ({
  tooltip: {
    trigger: 'axis',
    formatter: (params: any[]) => params.map((p: any) => `${p.seriesName}: ${formatBytes(p.value)}`).join('<br>')
  },
  legend: { data: ['上行', '下行'], right: 0 },
  grid: { left: '3%', right: '4%', bottom: '3%', containLabel: true },
  xAxis: { type: 'category', data: (overview.flowTrend || []).map((i: any) => i.time), boundaryGap: false },
  yAxis: { type: 'value', axisLabel: { formatter: (v: number) => formatBytes(v) } },
  series: [
    {
      name: '上行', type: 'line', areaStyle: { opacity: 0.25, color: '#409EFF' },
      data: (overview.flowTrend || []).map((i: any) => i.tx || 0), smooth: true,
      lineStyle: { color: '#409EFF' }, itemStyle: { color: '#409EFF' }
    },
    {
      name: '下行', type: 'line', areaStyle: { opacity: 0.25, color: '#67C23A' },
      data: (overview.flowTrend || []).map((i: any) => i.rx || 0), smooth: true,
      lineStyle: { color: '#67C23A' }, itemStyle: { color: '#67C23A' }
    }
  ]
}))

const protocolOption = computed(() => ({
  tooltip: { trigger: 'item', formatter: '{b}: {c} ({d}%)' },
  legend: { bottom: 0, type: 'scroll' },
  series: [{
    type: 'pie', radius: ['35%', '65%'], avoidLabelOverlap: false,
    itemStyle: { borderRadius: 6, borderColor: '#fff', borderWidth: 2 },
    label: { show: true, formatter: '{b}: {d}%', fontSize: 11 },
    data: (overview.protocolDist || [])
      .filter((p: any) => p.name !== '' && p.value > 0)
      .map((p: any) => ({ name: p.name || 'Other', value: p.value }))
  }]
}))

const fetchData = async () => {
  try {
    const res = await getOverview()
    if (res.code === 0 && res.data) {
      Object.assign(overview, res.data)
    }
  } catch (e) { console.error('Dashboard fetch error:', e) }

  try {
    const cpuRes = await getPerformanceCpu()
    if (cpuRes.code === 0 && Array.isArray(cpuRes.data) && cpuRes.data.length > 0) {
      const latest = cpuRes.data[cpuRes.data.length - 1] as any
      const usage = Number(latest.usage) || 0
      topCpu.value = [{ name: 'vm2 (192.168.58.131)', value: usage, percent: Math.min(100, usage) }]
    }
  } catch (e) {}

  try {
    const memRes = await getPerformanceMemory()
    if (memRes.code === 0 && memRes.data) {
      const data = memRes.data as any
      const total = data.total || 1
      const used = total - (data.free || 0)
      const pct = Math.min(100, Math.max(0, (used / total) * 100))
      topMemory.value = [{ name: 'vm2 (192.168.58.131)', value: pct, percent: pct }]
    }
  } catch (e) {}
}

let timer: ReturnType<typeof setInterval> | null = null
onMounted(() => {
  fetchData()
  timer = setInterval(fetchData, 30000)
})
onUnmounted(() => { if (timer) clearInterval(timer) })
</script>

<style scoped lang="scss">
.dashboard {
  .metric-row { margin-bottom: 16px; }
  .stat-card {
    border-radius: 8px; box-shadow: 0 2px 8px rgba(0,0,0,0.08); text-align: center;
    .stat-title { font-size: 12px; color: var(--el-text-color-secondary); margin-bottom: 6px; }
    .stat-value {
      font-size: 24px; font-weight: 700; color: var(--el-text-color-primary);
      &.green { color: #67C23A; }
      &.red { color: #F56C6C; }
    }
    .stat-sub { font-size: 11px; color: var(--el-text-color-placeholder); margin-top: 4px; }
  }
  .chart-row { margin-bottom: 16px; }
  .chart-card {
    border-radius: 8px; box-shadow: 0 2px 8px rgba(0,0,0,0.08);
    .card-title { font-size: 14px; font-weight: 600; color: var(--el-text-color-primary); margin-bottom: 12px; display: flex; align-items: center; }
  }
  .bottom-row { margin-bottom: 16px; }
  .top-card {
    border-radius: 8px; box-shadow: 0 2px 8px rgba(0,0,0,0.08); min-height: 200px;
    .card-title { font-size: 14px; font-weight: 600; color: var(--el-text-color-primary); margin-bottom: 12px; }
    .top-bar-item {
      display: flex; align-items: center; gap: 8px; margin-bottom: 8px;
      .top-name { width: 120px; font-size: 12px; color: var(--el-text-color-regular); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
      .top-value { width: 55px; font-size: 12px; color: var(--el-text-color-secondary); text-align: right; flex-shrink: 0; }
      :deep(.el-progress) { flex: 1; }
    }
    .alert-item {
      display: flex; align-items: flex-start; gap: 8px; margin-bottom: 8px;
      .alert-msg { font-size: 12px; color: var(--el-text-color-regular); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; flex: 1; }
    }
  }
}
</style>
