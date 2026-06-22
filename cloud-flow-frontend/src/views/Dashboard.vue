<template>
  <div class="dashboard-page" style="background-image: url('/bg-dashboard.jpg'); background-size: cover; background-position: center; background-repeat: no-repeat; background-color: #0a1628;">
    <!-- KPI 指标区域 -->
    <div class="kpi-section">
      <div v-for="kpi in kpiData" :key="kpi.name" class="kpi-card">
        <div class="kpi-icon">
          <el-icon :size="kpi.iconSize || 44" :color="kpi.iconColor || '#00CCFF'"><component :is="kpi.icon" /></el-icon>
        </div>
        <div class="kpi-info">
          <div class="kpi-name">{{ kpi.name }}</div>
          <div class="kpi-value">{{ kpi.value }}</div>
          <div v-if="kpi.compare" class="kpi-compare" :class="kpi.trend">
            <span class="trend-icon">{{ kpi.trend === 'up' ? '↑' : '↓' }}</span>
            {{ kpi.compare }}
          </div>
        </div>
      </div>
    </div>

    <!-- 选择器区域 -->
    <div class="filter-section">
      <div class="filter-item">
        <span class="filter-label">选择业务:</span>
        <el-select v-model="selectedBusiness" placeholder="请选择" size="small" class="filter-select">
          <el-option v-for="b in businessOptions" :key="b.value" :label="b.label" :value="b.value" />
        </el-select>
      </div>
      <div class="filter-item">
        <span class="filter-label">选择服务:</span>
        <el-select v-model="selectedService" placeholder="请选择" size="small" class="filter-select">
          <el-option v-for="s in serviceOptions" :key="s.value" :label="s.label" :value="s.value" />
        </el-select>
      </div>
      <div class="filter-item">
        <span class="filter-label">选择虚拟机/Pod:</span>
        <el-select v-model="selectedPod" placeholder="请选择" size="small" class="filter-select">
          <el-option v-for="p in podOptions" :key="p.value" :label="p.label" :value="p.value" />
        </el-select>
      </div>
      <el-button type="primary" size="small" class="query-btn" @click="handleQuery">查询</el-button>
    </div>

    <!-- 主内容区域：左侧三层 + 右侧统计 -->
    <div class="main-section">
      <!-- 左侧三层架构 -->
      <div class="left-panel">
        <!-- 业务层 -->
        <div class="layer-section">
          <div class="layer-header">
            <div class="layer-title-bar">业务层</div>
            <div class="layer-toggle" :class="{ active: onlyAbnormalBiz }" @click="onlyAbnormalBiz = !onlyAbnormalBiz">
              <span class="toggle-dot" :class="{ warning: onlyAbnormalBiz }"></span>
              <span>仅显示异常</span>
            </div>
          </div>
          <div class="layer-divider"></div>
          <div class="cards-container">
            <el-icon class="arrow-btn left" @click="scrollBiz(-1)"><ArrowLeft /></el-icon>
            <div class="cards-scroll" ref="bizScrollRef">
              <div
                v-for="biz in filteredBusiness"
                :key="biz.name"
                class="biz-card"
                :class="{ abnormal: biz.status === '异常', selected: biz.selected }"
                @click="selectBusiness(biz)"
              >
                <div class="status-icon" :class="biz.status">
                  <el-icon :size="16"><Warning v-if="biz.status === '异常'" /><CircleCheck v-else /></el-icon>
                </div>
                <div class="card-icon">
                  <el-icon :size="68" :color="biz.status === '异常' ? '#FF745A' : '#0ABAFF'"><OfficeBuilding /></el-icon>
                </div>
                <div class="card-name">{{ biz.name }}</div>
                <div class="card-value">{{ biz.value }}</div>
              </div>
            </div>
            <el-icon class="arrow-btn right" @click="scrollBiz(1)"><ArrowRight /></el-icon>
          </div>
        </div>

        <!-- 服务层 -->
        <div class="layer-section">
          <div class="layer-header">
            <div class="layer-title-bar">服务层</div>
            <div class="layer-toggle" :class="{ active: onlyAbnormalSvc }" @click="onlyAbnormalSvc = !onlyAbnormalSvc">
              <span class="toggle-dot" :class="{ warning: onlyAbnormalSvc }"></span>
              <span>仅显示异常</span>
            </div>
          </div>
          <div class="layer-divider"></div>
          <div class="cards-container">
            <el-icon class="arrow-btn left" @click="scrollSvc(-1)"><ArrowLeft /></el-icon>
            <div class="cards-scroll" ref="svcScrollRef">
              <div
                v-for="svc in filteredServices"
                :key="svc.name"
                class="svc-card"
                :class="{ abnormal: svc.status === '异常', selected: svc.selected }"
                @click="selectService(svc)"
              >
                <div class="status-icon" :class="svc.status">
                  <el-icon :size="16"><Warning v-if="svc.status === '异常'" /><CircleCheck v-else /></el-icon>
                </div>
                <div class="card-icon">
                  <el-icon :size="68" :color="svc.status === '异常' ? '#FF745A' : '#61DDAA'"><Collection /></el-icon>
                </div>
                <div class="card-name">{{ svc.name }}</div>
                <div class="card-value">{{ svc.value }}</div>
                <div class="card-delay">延迟: {{ svc.delay }}</div>
              </div>
            </div>
            <el-icon class="arrow-btn right" @click="scrollSvc(1)"><ArrowRight /></el-icon>
          </div>
        </div>

        <!-- 虚拟机/Pod层 -->
        <div class="layer-section">
          <div class="layer-header">
            <div class="layer-title-bar">虚拟机/Pod层</div>
            <div class="layer-toggle" :class="{ active: onlyAbnormalPod }" @click="onlyAbnormalPod = !onlyAbnormalPod">
              <span class="toggle-dot" :class="{ warning: onlyAbnormalPod }"></span>
              <span>仅显示异常</span>
            </div>
          </div>
          <div class="layer-divider"></div>
          <div class="cards-container">
            <el-icon class="arrow-btn left" @click="scrollPod(-1)"><ArrowLeft /></el-icon>
            <div class="cards-scroll" ref="podScrollRef">
              <div
                v-for="pod in filteredPods"
                :key="pod.name"
                class="pod-card"
                :class="{ abnormal: pod.status === '异常', selected: pod.selected }"
                @click="selectPod(pod)"
              >
                <div class="status-icon" :class="pod.status">
                  <el-icon :size="11"><Warning v-if="pod.status === '异常'" /><CircleCheck v-else /></el-icon>
                </div>
                <div class="card-icon">
                  <el-icon :size="48" :color="pod.status === '异常' ? '#FF745A' : '#FF745A'"><Box /></el-icon>
                </div>
                <div class="card-name">{{ pod.name }}</div>
                <div class="card-value">{{ pod.value }}</div>
                <div class="card-delay">延迟: {{ pod.delay }}</div>
              </div>
            </div>
            <el-icon class="arrow-btn right" @click="scrollPod(1)"><ArrowRight /></el-icon>
          </div>
        </div>
      </div>

      <!-- 右侧统计面板 -->
      <div class="right-panel">
        <!-- 告警类型统计 -->
        <div class="stat-panel">
          <div class="panel-header">
            <div class="panel-title">
              <div class="title-bar"></div>
              <span>告警类型统计</span>
            </div>
            <span class="detail-link">查看详情>>></span>
          </div>
          <div class="pie-chart-container">
            <v-chart :option="alertPieOption" autoresize style="height: 220px; width: 100%" />
          </div>
        </div>

        <!-- 云主机/容器请求数排行 TOP 5 -->
        <div class="stat-panel">
          <div class="panel-header">
            <div class="panel-title">
              <div class="title-bar"></div>
              <span>云主机/容器请求数排行 TOP 5</span>
            </div>
            <span class="detail-link">查看详情>>></span>
          </div>
          <div class="rank-list">
            <div v-for="(item, idx) in requestTop5" :key="idx" class="rank-item">
              <div class="rank-num">{{ idx + 1 }}</div>
              <div class="rank-info">
                <div class="rank-ip">{{ item.ip }}</div>
                <div class="rank-name-row">
                  <span class="status-dot" :class="item.status"></span>
                  <span class="rank-status">{{ item.statusText }}</span>
                  <span class="rank-name">{{ item.name }}</span>
                </div>
              </div>
              <div class="rank-bar">
                <div class="bar-bg">
                  <div class="bar-fill" :style="{ width: item.percent + '%' }"></div>
                </div>
              </div>
              <div class="rank-value">{{ item.value }}</div>
            </div>
          </div>
        </div>

        <!-- 业务网络流量排行 TOP 5 -->
        <div class="stat-panel">
          <div class="panel-header">
            <div class="panel-title">
              <div class="title-bar"></div>
              <span>业务网络流量排行 TOP 5</span>
            </div>
            <span class="detail-link">查看详情>>></span>
          </div>
          <div class="rank-list">
            <div v-for="(item, idx) in trafficTop5" :key="idx" class="rank-item">
              <div class="rank-num">{{ idx + 1 }}</div>
              <div class="rank-info">
                <div class="rank-name-row">
                  <span class="rank-name">{{ item.name }}</span>
                  <span class="status-dot" :class="item.status"></span>
                  <span class="rank-status">{{ item.statusText }}</span>
                </div>
              </div>
              <div class="rank-bar">
                <div class="bar-bg">
                  <div class="bar-fill" :style="{ width: item.percent + '%' }"></div>
                </div>
              </div>
              <div class="rank-value">{{ item.value }}</div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import {
  OfficeBuilding, Collection, Box, ArrowLeft, ArrowRight,
  Warning, CircleCheck, TrendCharts, Timer, Bell, Cloudy, Grid, SetUp
} from '@element-plus/icons-vue'
import axios from 'axios'

const api = axios.create({ baseURL: '/api', timeout: 30000 })

// KPI 数据
const kpiData = ref([
  { name: '总请求量', value: '0', compare: '', trend: '', icon: TrendCharts, iconSize: 44, iconColor: '#00CCFF' },
  { name: '总流量', value: '0', compare: '', trend: '', icon: TrendCharts, iconSize: 44, iconColor: '#00CCFF' },
  { name: '平均延迟', value: '0ms', compare: '', trend: '', icon: Timer, iconSize: 44, iconColor: '#00CCFF' },
  { name: '告警数量', value: '0', compare: '', trend: '', icon: Bell, iconSize: 44, iconColor: '#FF745A' },
  { name: '云主机节点/异常数', value: '0/0', compare: '', trend: '', icon: Cloudy, iconSize: 44, iconColor: '#61DDAA' },
  { name: '容器节点数/异常数', value: '0/0', compare: '', trend: '', icon: Grid, iconSize: 44, iconColor: '#61DDAA' },
  { name: '业务节点数/异常数', value: '0/0', compare: '', trend: '', icon: SetUp, iconSize: 44, iconColor: '#61DDAA' },
])

// 选择器
const selectedBusiness = ref('')
const selectedService = ref('')
const selectedPod = ref('')
const businessOptions = ref<{label: string, value: string}[]>([])
const serviceOptions = ref<{label: string, value: string}[]>([])
const podOptions = ref<{label: string, value: string}[]>([])

const handleQuery = () => {
  console.log('Query:', selectedBusiness.value, selectedService.value, selectedPod.value)
}

// 仅显示异常开关
const onlyAbnormalBiz = ref(false)
const onlyAbnormalSvc = ref(false)
const onlyAbnormalPod = ref(false)

// 业务层数据
const businessData = ref<any[]>([])
const filteredBusiness = computed(() => {
  if (onlyAbnormalBiz.value) return businessData.value.filter(b => b.status === '异常')
  return businessData.value
})
const selectBusiness = (biz: any) => {
  businessData.value.forEach(b => b.selected = false)
  biz.selected = true
}

// 服务层数据
const servicesData = ref<any[]>([])
const filteredServices = computed(() => {
  if (onlyAbnormalSvc.value) return servicesData.value.filter(s => s.status === '异常')
  return servicesData.value
})
const selectService = (svc: any) => {
  servicesData.value.forEach(s => s.selected = false)
  svc.selected = true
}

// 虚拟机/Pod层数据
const podsData = ref<any[]>([])
const filteredPods = computed(() => {
  if (onlyAbnormalPod.value) return podsData.value.filter(p => p.status === '异常')
  return podsData.value
})
const selectPod = (pod: any) => {
  podsData.value.forEach(p => p.selected = false)
  pod.selected = true
}

// 滚动控制
const bizScrollRef = ref<HTMLElement>()
const svcScrollRef = ref<HTMLElement>()
const podScrollRef = ref<HTMLElement>()

const scrollBiz = (dir: number) => {
  if (bizScrollRef.value) bizScrollRef.value.scrollLeft += dir * 200
}
const scrollSvc = (dir: number) => {
  if (svcScrollRef.value) svcScrollRef.value.scrollLeft += dir * 200
}
const scrollPod = (dir: number) => {
  if (podScrollRef.value) podScrollRef.value.scrollLeft += dir * 200
}

// 告警饼图配置
const alertPieData = ref([
  { name: '网络指标告警', value: 0, itemStyle: { color: '#2391FF' } },
  { name: '事件告警', value: 0, itemStyle: { color: '#FF745A' } },
  { name: '连通性告警', value: 0, itemStyle: { color: '#FFC328' } },
  { name: '系统告警', value: 0, itemStyle: { color: '#5D7092' } },
])
const alertPieOption = computed(() => ({
  backgroundColor: 'transparent',
  tooltip: { trigger: 'item', backgroundColor: 'rgba(5, 56, 90, 0.9)', borderColor: '#0ABAFF', textStyle: { color: '#fff' } },
  series: [{
    type: 'pie',
    radius: ['35%', '65%'],
    center: ['50%', '50%'],
    avoidLabelOverlap: true,
    itemStyle: { borderRadius: 4, borderColor: 'rgba(38, 36, 68, 0.8)', borderWidth: 2 },
    label: { show: true, color: '#fff', fontSize: 12, formatter: '{d}%' },
    labelLine: { lineStyle: { color: 'rgba(255,255,255,0.45)' } },
    data: alertPieData.value.filter(d => d.value > 0)
  }]
}))

// 请求数 TOP 5
const requestTop5 = ref<any[]>([])
// 流量 TOP 5
const trafficTop5 = ref<any[]>([])

const loadDashboard = async () => {
  try {
    // 1. Overview
    const overviewRes = await api.get('/overview')
    const ov = overviewRes.data?.data || overviewRes.data || {}
    kpiData.value[0].value = ov.total_requests ? ov.total_requests.toString() : '0'
    kpiData.value[1].value = ov.total_traffic ? ov.total_traffic.toString() : '0'
    kpiData.value[2].value = ov.avg_latency ? (ov.avg_latency + 'ms') : '0ms'
    kpiData.value[3].value = ov.alert_count ? ov.alert_count.toString() : '0'
    const nodes = ov.nodes || {}
    kpiData.value[4].value = `${nodes.total || 0}/${nodes.abnormal || 0}`
    kpiData.value[5].value = `${nodes.container_total || 0}/${nodes.container_abnormal || 0}`
    kpiData.value[6].value = `${nodes.business_total || 0}/${nodes.business_abnormal || 0}`
  } catch (e) { console.error('overview error:', e) }

  try {
    // 2. Business
    const bizRes = await api.get('/business')
    const bizList = bizRes.data?.data || bizRes.data || []
    const list = Array.isArray(bizList) ? bizList : (bizList.list || [])
    businessData.value = list.map((b: any, i: number) => ({
      name: b.name || b.id || `业务-${i+1}`,
      value: b.traffic || b.value || '0 Gbps',
      status: b.status === 'abnormal' ? '异常' : '正常',
      selected: i === 0
    }))
    businessOptions.value = list.map((b: any, i: number) => ({ label: b.name || b.id || `业务-${i+1}`, value: b.id || `biz${i+1}` }))
    trafficTop5.value = list.slice(0, 5).map((b: any, i: number) => {
      const val = parseFloat(b.traffic || b.value || '0')
      const max = Math.max(...list.slice(0, 5).map((x: any) => parseFloat(x.traffic || x.value || '0')))
      return {
        name: b.name || b.id || `业务-${i+1}`,
        status: b.status === 'abnormal' ? 'warning' : 'normal',
        statusText: b.status === 'abnormal' ? '需关注' : '正常',
        value: b.traffic || b.value || '0',
        percent: max > 0 ? Math.round((val / max) * 100) : 0
      }
    })
  } catch (e) { console.error('business error:', e) }

  try {
    // 3. Service
    const svcRes = await api.get('/service')
    const svcList = svcRes.data?.data || svcRes.data || []
    const list = Array.isArray(svcList) ? svcList : (svcList.list || [])
    servicesData.value = list.map((s: any, i: number) => ({
      name: s.name || s.id || `服务-${i+1}`,
      value: s.traffic || s.value || '0 Gbps',
      delay: s.latency ? (s.latency + 'ms') : '0ms',
      status: s.status === 'abnormal' ? '异常' : '正常',
      selected: false
    }))
    serviceOptions.value = list.map((s: any, i: number) => ({ label: s.name || s.id || `服务-${i+1}`, value: s.id || `svc${i+1}` }))
  } catch (e) { console.error('service error:', e) }

  try {
    // 4. Nodes (Pod)
    const nodeRes = await api.get('/nodes')
    const nodeList = nodeRes.data?.data || nodeRes.data || []
    const list = Array.isArray(nodeList) ? nodeList : (nodeList.list || [])
    podsData.value = list.map((n: any, i: number) => ({
      name: n.name || n.hostname || n.id || `node-${i+1}`,
      value: n.traffic || n.value || '0 Mbps',
      delay: n.latency ? (n.latency + 'ms') : '0ms',
      status: n.status === 'abnormal' || n.status === 'unhealthy' ? '异常' : '正常',
      selected: false
    }))
    podOptions.value = list.map((n: any, i: number) => ({ label: n.name || n.hostname || n.id || `node-${i+1}`, value: n.id || `p${i+1}` }))
    requestTop5.value = list.slice(0, 5).map((n: any, i: number) => {
      const req = n.requests || n.req_count || 0
      const maxReq = Math.max(...list.slice(0, 5).map((x: any) => x.requests || x.req_count || 0))
      return {
        ip: n.ip || n.address || '0.0.0.0',
        name: n.name || n.hostname || n.id || `node-${i+1}`,
        status: n.status === 'abnormal' || n.status === 'unhealthy' ? 'warning' : 'normal',
        statusText: n.status === 'abnormal' || n.status === 'unhealthy' ? '需关注' : '正常',
        value: (req ? (req + '次/秒') : '0次/秒'),
        percent: maxReq > 0 ? Math.round((req / maxReq) * 100) : 0
      }
    })
  } catch (e) { console.error('nodes error:', e) }

  try {
    // 5. Alerts for pie
    const alertRes = await api.get('/alert/list')
    const alertData = alertRes.data?.data || alertRes.data || []
    const alerts = Array.isArray(alertData) ? alertData : (alertData.list || [])
    const counts: Record<string, number> = { '网络指标告警': 0, '事件告警': 0, '连通性告警': 0, '系统告警': 0 }
    alerts.forEach((a: any) => {
      const type = (a.rule_name || (a.labels && a.labels.alertname) || '').toLowerCase()
      if (type.includes('network') || type.includes('网络')) counts['网络指标告警']++
      else if (type.includes('connect') || type.includes('连通') || type.includes('ping')) counts['连通性告警']++
      else if (type.includes('system') || type.includes('系统') || type.includes('cpu') || type.includes('memory') || type.includes('disk')) counts['系统告警']++
      else counts['事件告警']++
    })
    alertPieData.value = [
      { name: '网络指标告警', value: counts['网络指标告警'], itemStyle: { color: '#2391FF' } },
      { name: '事件告警', value: counts['事件告警'], itemStyle: { color: '#FF745A' } },
      { name: '连通性告警', value: counts['连通性告警'], itemStyle: { color: '#FFC328' } },
      { name: '系统告警', value: counts['系统告警'], itemStyle: { color: '#5D7092' } },
    ]
  } catch (e) { console.error('alert error:', e) }
}

onMounted(loadDashboard)
</script>

<style scoped lang="scss">
.dashboard-page {
  position: relative;
  min-height: 100vh;
  overflow-x: hidden;
}

// KPI 区域
.kpi-section {
  position: relative;
  z-index: 1;
  display: flex;
  gap: 20px;
  padding: 16px 24px;
  justify-content: space-between;
  .kpi-card {
    flex: 1;
    min-width: 120px;
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 12px 16px;
    background: rgba(10, 186, 255, 0.08);
    border: 1px solid rgba(10, 186, 255, 0.2);
    border-radius: 8px;
    .kpi-icon {
      flex-shrink: 0;
    }
    .kpi-info {
      .kpi-name {
        font-size: 12px;
        color: rgba(255, 255, 255, 0.7);
        margin-bottom: 4px;
      }
      .kpi-value {
        font-size: 18px;
        font-weight: 700;
        color: #FFFFFF;
        font-family: 'Arial', sans-serif;
      }
      .kpi-compare {
        font-size: 11px;
        margin-top: 2px;
        &.up { color: #6BEDB7; }
        &.down { color: #FF745A; }
        .trend-icon {
          margin-right: 2px;
        }
      }
    }
  }
}

// 筛选器区域
.filter-section {
  position: relative;
  z-index: 1;
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 8px 24px;
  .filter-item {
    display: flex;
    align-items: center;
    gap: 8px;
    .filter-label {
      font-size: 13px;
      color: rgba(255, 255, 255, 0.8);
      white-space: nowrap;
    }
    .filter-select {
      width: 140px;
    }
  }
  .query-btn {
    background: rgba(10, 186, 255, 0.3);
    border: 1px solid rgba(10, 186, 255, 0.5);
    color: #FFFFFF;
  }
}

// 主内容区域
.main-section {
  position: relative;
  z-index: 1;
  display: flex;
  gap: 16px;
  padding: 8px 24px 24px;
  .left-panel {
    flex: 1;
    display: flex;
    flex-direction: column;
    gap: 16px;
  }
  .right-panel {
    width: 520px;
    display: flex;
    flex-direction: column;
    gap: 16px;
  }
}

// 层区域
.layer-section {
  .layer-header {
    display: flex;
    align-items: center;
    gap: 16px;
    margin-bottom: 8px;
    .layer-title-bar {
      padding: 6px 20px;
      background: rgba(10, 186, 255, 0.15);
      border: 1px solid rgba(10, 186, 255, 0.3);
      border-radius: 4px;
      font-size: 14px;
      font-weight: 600;
      color: #FFFFFF;
    }
    .layer-toggle {
      display: flex;
      align-items: center;
      gap: 6px;
      padding: 6px 14px;
      background: rgba(102, 187, 249, 0.2);
      border-radius: 42px;
      font-size: 13px;
      color: #FFFEFE;
      cursor: pointer;
      transition: all 0.2s;
      .toggle-dot {
        width: 8px;
        height: 8px;
        border-radius: 50%;
        background: rgba(255, 255, 255, 0.5);
        &.warning {
          background: #FB8737;
        }
      }
      &.active {
        background: rgba(23, 101, 149, 1);
      }
    }
  }
  .layer-divider {
    width: 5px;
    height: 41px;
    background: linear-gradient(180deg, rgba(35, 228, 171, 1) 0%, rgba(9, 174, 233, 1) 100%);
    border-radius: 10px;
    margin-bottom: 8px;
  }
  .cards-container {
    display: flex;
    align-items: center;
    gap: 8px;
    .arrow-btn {
      width: 32px;
      height: 86px;
      display: flex;
      align-items: center;
      justify-content: center;
      color: rgba(255, 255, 255, 0.5);
      cursor: pointer;
      transition: color 0.2s;
      &:hover {
        color: #FFFFFF;
      }
      flex-shrink: 0;
    }
    .cards-scroll {
      flex: 1;
      display: flex;
      gap: 12px;
      overflow-x: auto;
      scroll-behavior: smooth;
      scrollbar-width: none;
      &::-webkit-scrollbar {
        display: none;
      }
    }
  }
}

// 业务卡片
.biz-card {
  width: 150px;
  height: 150px;
  flex-shrink: 0;
  background: rgba(10, 186, 255, 0.2);
  border: 1px solid rgba(10, 186, 255, 1);
  border-radius: 5px;
  padding: 8px;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 4px;
  position: relative;
  cursor: pointer;
  transition: all 0.3s;
  &:hover {
    background: rgba(10, 186, 255, 0.3);
  }
  &.selected {
    background: rgba(23, 101, 148, 1);
    box-shadow: 3px 3px 6px 0px rgba(9, 60, 83, 1) inset;
  }
  &.abnormal {
    border-color: #FF745A;
    background: rgba(255, 116, 90, 0.2);
  }
  .status-icon {
    position: absolute;
    top: 8px;
    right: 8px;
    width: 16px;
    height: 16px;
    display: flex;
    align-items: center;
    justify-content: center;
    &.异常 {
      color: #FB8737;
    }
    &.正常 {
      color: #61DDAA;
    }
  }
  .card-icon {
    opacity: 0.76;
  }
  .card-name {
    font-size: 16px;
    font-weight: 700;
    color: #FFFFFF;
    font-family: '微软雅黑', sans-serif;
  }
  .card-value {
    font-size: 14px;
    color: #FFFFFF;
    font-family: 'Arial', sans-serif;
  }
}

// 服务卡片
.svc-card {
  width: 150px;
  height: 150px;
  flex-shrink: 0;
  background: rgba(97, 221, 170, 0.36);
  border: 1px solid rgba(97, 221, 170, 1);
  border-radius: 5px;
  padding: 8px;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 2px;
  position: relative;
  cursor: pointer;
  transition: all 0.3s;
  &:hover {
    background: rgba(97, 221, 170, 0.5);
  }
  &.selected {
    background: rgba(53, 174, 152, 1);
    box-shadow: inset 3px 3px 6px rgba(41, 98, 86, 0.73);
  }
  &.abnormal {
    border-color: #FF745A;
  }
  .status-icon {
    position: absolute;
    top: 8px;
    right: 8px;
    width: 16px;
    height: 16px;
    display: flex;
    align-items: center;
    justify-content: center;
    &.异常 {
      color: #FB8737;
    }
    &.正常 {
      color: #61DDAA;
    }
  }
  .card-icon {
    opacity: 0.93;
  }
  .card-name {
    font-size: 16px;
    font-weight: 700;
    color: #FFFFFF;
    font-family: '微软雅黑', sans-serif;
    letter-spacing: -0.6px;
  }
  .card-value {
    font-size: 14px;
    color: #FFFFFF;
    font-family: 'Arial', sans-serif;
  }
  .card-delay {
    font-size: 14px;
    color: rgba(255, 255, 255, 0.8);
    font-family: 'Arial', sans-serif;
  }
}

// Pod卡片
.pod-card {
  width: 120px;
  height: 120px;
  flex-shrink: 0;
  background: rgba(255, 116, 90, 0.2);
  border: 1px solid rgba(255, 116, 90, 1);
  border-radius: 5px;
  padding: 6px;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 2px;
  position: relative;
  cursor: pointer;
  transition: all 0.3s;
  &:hover {
    background: rgba(255, 116, 90, 0.3);
  }
  &.selected {
    background: rgba(180, 80, 60, 1);
  }
  &.abnormal {
    border-color: #FF745A;
  }
  .status-icon {
    position: absolute;
    top: 6px;
    right: 6px;
    width: 11px;
    height: 11px;
    display: flex;
    align-items: center;
    justify-content: center;
    &.异常 {
      color: #FB8737;
    }
    &.正常 {
      color: #61DDAA;
    }
  }
  .card-name {
    font-size: 14px;
    font-weight: 700;
    color: #FFFFFF;
    font-family: '微软雅黑', sans-serif;
  }
  .card-value {
    font-size: 12px;
    color: #FFFFFF;
    font-family: 'Arial', sans-serif;
  }
  .card-delay {
    font-size: 12px;
    color: rgba(255, 255, 255, 0.8);
    font-family: 'Arial', sans-serif;
  }
}

// 右侧面板
.stat-panel {
  background: rgba(10, 186, 255, 0.06);
  border: 1px solid rgba(10, 186, 255, 0.15);
  border-radius: 8px;
  padding: 16px;
  .panel-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 12px;
    .panel-title {
      display: flex;
      align-items: center;
      gap: 8px;
      font-size: 14px;
      font-weight: 600;
      color: #FFFFFF;
      font-family: '微软雅黑', sans-serif;
      .title-bar {
        width: 4px;
        height: 16px;
        background: linear-gradient(180deg, rgba(35, 228, 171, 1) 0%, rgba(9, 174, 233, 1) 100%);
        border-radius: 2px;
      }
    }
    .detail-link {
      font-size: 12px;
      color: rgba(0, 204, 255, 0.8);
      cursor: pointer;
      &:hover {
        color: #00CCFF;
      }
    }
  }
}

// 排行列表
.rank-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
  .rank-item {
    display: flex;
    align-items: center;
    gap: 10px;
    .rank-num {
      width: 20px;
      font-size: 14px;
      font-weight: 700;
      color: rgba(255, 255, 255, 0.8);
      font-family: 'DIN Bold', 'Arial', sans-serif;
      text-align: center;
    }
    .rank-info {
      flex: 1;
      min-width: 0;
      .rank-ip {
        font-size: 13px;
        color: #FFFFFF;
        margin-bottom: 2px;
      }
      .rank-name-row {
        display: flex;
        align-items: center;
        gap: 6px;
        .rank-name {
          font-size: 12px;
          color: rgba(255, 255, 255, 0.7);
        }
        .status-dot {
          width: 8px;
          height: 8px;
          border-radius: 50%;
          flex-shrink: 0;
          &.normal {
            background: #61DDAA;
          }
          &.warning {
            background: #FB8737;
          }
        }
        .rank-status {
          font-size: 12px;
          color: #FFFFFF;
        }
      }
    }
    .rank-bar {
      width: 120px;
      flex-shrink: 0;
      .bar-bg {
        height: 8px;
        background: rgba(0, 204, 255, 0.098);
        border-radius: 140px;
        overflow: hidden;
        .bar-fill {
          height: 100%;
          background: rgba(0, 204, 255, 1);
          border-radius: 125px;
          transition: width 0.5s ease;
        }
      }
    }
    .rank-value {
      width: 70px;
      font-size: 14px;
      font-weight: 700;
      color: #6BEDB7;
      font-family: 'Arial', sans-serif;
      text-align: right;
      flex-shrink: 0;
    }
  }
}

.pie-chart-container {
  width: 100%;
}
</style>
