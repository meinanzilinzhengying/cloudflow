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
          <el-option label="业务-1" value="biz1" />
          <el-option label="业务-2" value="biz2" />
          <el-option label="业务-3" value="biz3" />
        </el-select>
      </div>
      <div class="filter-item">
        <span class="filter-label">选择服务:</span>
        <el-select v-model="selectedService" placeholder="请选择" size="small" class="filter-select">
          <el-option label="业务服务-1" value="svc1" />
          <el-option label="业务服务-2" value="svc2" />
          <el-option label="RDS服务" value="rds" />
        </el-select>
      </div>
      <div class="filter-item">
        <span class="filter-label">选择虚拟机/Pod:</span>
        <el-select v-model="selectedPod" placeholder="请选择" size="small" class="filter-select">
          <el-option label="product-1" value="p1" />
          <el-option label="product-2" value="p2" />
          <el-option label="MySQL" value="mysql" />
          <el-option label="Redis" value="redis" />
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
import { ref, computed } from 'vue'
import {
  OfficeBuilding, Collection, Box, ArrowLeft, ArrowRight,
  Warning, CircleCheck, TrendCharts, Timer, Bell, Cloudy, Grid, SetUp
} from '@element-plus/icons-vue'

// KPI 数据
const kpiData = ref([
  { name: '总请求量', value: '1967.5 K', compare: '12.5% VS 昨日', trend: 'up', icon: TrendCharts, iconSize: 44, iconColor: '#00CCFF' },
  { name: '总流量', value: '2.8 TB', compare: '12.5% VS 昨日', trend: 'up', icon: TrendCharts, iconSize: 44, iconColor: '#00CCFF' },
  { name: '平均延迟', value: '23ms', compare: '8% VS 昨日', trend: 'down', icon: Timer, iconSize: 44, iconColor: '#00CCFF' },
  { name: '告警数量', value: '15', compare: '', trend: '', icon: Bell, iconSize: 44, iconColor: '#FF745A' },
  { name: '云主机节点/异常数', value: '15/0', compare: '', trend: '', icon: Cloudy, iconSize: 44, iconColor: '#61DDAA' },
  { name: '容器节点数/异常数', value: '20/1', compare: '', trend: '', icon: Grid, iconSize: 44, iconColor: '#61DDAA' },
  { name: '业务节点数/异常数', value: '15/1', compare: '', trend: '', icon: SetUp, iconSize: 44, iconColor: '#61DDAA' },
])

// 筛选器
const selectedBusiness = ref('')
const selectedService = ref('')
const selectedPod = ref('')

const handleQuery = () => {
  console.log('Query:', selectedBusiness.value, selectedService.value, selectedPod.value)
}

// 仅显示异常开关
const onlyAbnormalBiz = ref(false)
const onlyAbnormalSvc = ref(false)
const onlyAbnormalPod = ref(false)

// 业务层数据
const businessData = ref([
  { name: '业务-1', value: '2.8 Gbps', status: '正常', selected: true },
  { name: '业务-2', value: '1.5 Gbps', status: '正常', selected: false },
  { name: '业务-3', value: '2.8 Gbps', status: '正常', selected: false },
  { name: '业务-4', value: '1.5 Gbps', status: '正常', selected: false },
  { name: '业务-5', value: '2.8 Gbps', status: '正常', selected: false },
  { name: '业务-6', value: '1.5 Gbps', status: '正常', selected: false },
  { name: '业务-7', value: '1.5 Gbps', status: '正常', selected: false },
  { name: '业务-8', value: '1.5 Gbps', status: '异常', selected: false },
  { name: '业务-9', value: '1.5 Gbps', status: '正常', selected: false },
  { name: '业务-10', value: '1.5 Gbps', status: '正常', selected: false },
])

const filteredBusiness = computed(() => {
  if (onlyAbnormalBiz.value) return businessData.value.filter(b => b.status === '异常')
  return businessData.value
})

const selectBusiness = (biz: any) => {
  businessData.value.forEach(b => b.selected = false)
  biz.selected = true
}

// 服务层数据
const servicesData = ref([
  { name: '业务服务-1', value: '4.5 Gbps', delay: '45ms', status: '正常', selected: false },
  { name: '业务服务-2', value: '1.2 Gbps', delay: '32ms', status: '正常', selected: false },
  { name: 'RDS服务', value: '0.8 Gbps', delay: '28ms', status: '正常', selected: false },
  { name: '业务服务-3', value: '4.5 Gbps', delay: '45ms', status: '正常', selected: false },
  { name: '业务服务-4', value: '1.2 Gbps', delay: '32ms', status: '正常', selected: false },
  { name: '业务服务-5', value: '0.8 Gbps', delay: '28ms', status: '正常', selected: false },
  { name: '业务服务-6', value: '4.5 Gbps', delay: '45ms', status: '正常', selected: false },
  { name: '业务服务-7', value: '4.5 Gbps', delay: '45ms', status: '正常', selected: false },
])

const filteredServices = computed(() => {
  if (onlyAbnormalSvc.value) return servicesData.value.filter(s => s.status === '异常')
  return servicesData.value
})

const selectService = (svc: any) => {
  servicesData.value.forEach(s => s.selected = false)
  svc.selected = true
}

// 虚拟机/Pod层数据
const podsData = ref([
  { name: 'product-1', value: '456 Mbps', delay: '18ms', status: '正常', selected: false },
  { name: 'product-2', value: '312 Mbps', delay: '45ms', status: '正常', selected: false },
  { name: 'product-3', value: '389 Mbps', delay: '22ms', status: '正常', selected: false },
  { name: 'product-4', value: '234 Mbps', delay: '12ms', status: '正常', selected: false },
  { name: 'MySQL', value: '234 Mbps', delay: '12ms', status: '正常', selected: false },
  { name: 'Redis', value: '189 Mbps', delay: '8ms', status: '正常', selected: false },
  { name: 'product-5', value: '156 Mbps', delay: '14ms', status: '正常', selected: false },
  { name: 'product-6', value: '456 Mbps', delay: '18ms', status: '正常', selected: false },
  { name: 'product-7', value: '312 Mbps', delay: '45ms', status: '正常', selected: false },
  { name: 'product-8', value: '389 Mbps', delay: '22ms', status: '正常', selected: false },
  { name: 'product-9', value: '234 Mbps', delay: '12ms', status: '正常', selected: false },
])

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
    data: [
      { name: '网络指标告警', value: 3, itemStyle: { color: '#2391FF' }, label: { formatter: '{d}%' } },
      { name: '事件告警', value: 5, itemStyle: { color: '#FF745A' }, label: { formatter: '{d}%' } },
      { name: '连通性告警', value: 4, itemStyle: { color: '#FFC328' }, label: { formatter: '{d}%' } },
      { name: '系统告警', value: 2, itemStyle: { color: '#5D7092' }, label: { formatter: '{d}%' } },
    ]
  }]
}))

// 请求数 TOP 5
const requestTop5 = ref([
  { ip: '192.168.1.45', name: 'order-service-7d8f9c-xk2m9', status: 'normal', statusText: '正常', value: '453次/秒', percent: 90 },
  { ip: '10.244.3.12', name: 'payment-service-5b6c8d-p9k4n', status: 'normal', statusText: '正常', value: '389次/秒', percent: 78 },
  { ip: '192.168.0.99', name: 'user-service-9a2b3c-m7h5j', status: 'warning', statusText: '需关注', value: '349次/秒', percent: 70 },
  { ip: '192.168.1.45', name: 'production-service-4e5f6g-q3w8r', status: 'normal', statusText: '正常', value: '320次/秒', percent: 64 },
  { ip: '192.168.0.41', name: 'search-service-8h9i0j-t6y2u', status: 'normal', statusText: '正常', value: '290次/秒', percent: 58 },
])

// 流量 TOP 5
const trafficTop5 = ref([
  { name: '业务-1', status: 'normal', statusText: '正常', value: '1.2 Gbps', percent: 85 },
  { name: '业务-2', status: 'normal', statusText: '正常', value: '567 Mbps', percent: 60 },
  { name: '业务-3', status: 'warning', statusText: '需关注', value: '234 Mbps', percent: 45 },
  { name: '业务-4', status: 'normal', statusText: '正常', value: '189 Mbps', percent: 35 },
  { name: '业务-5', status: 'normal', statusText: '正常', value: '145 Mbps', percent: 28 },
])
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
