<template>
  <div class="dashboard">
    <!-- 业务卡片区域 -->
    <el-row :gutter="12" class="card-row">
      <el-col :span="2" v-for="biz in businessCards" :key="biz.name">
        <div class="biz-card" :class="{ abnormal: biz.status === '异常' }">
          <div class="biz-icon">
            <el-icon :size="32" :color="biz.status === '异常' ? '#FF745A' : '#0ABAFF'"><OfficeBuilding /></el-icon>
          </div>
          <div class="biz-name">{{ biz.name }}</div>
          <div class="biz-value">{{ biz.value }}</div>
        </div>
      </el-col>
    </el-row>

    <!-- 服务卡片区域 -->
    <el-row :gutter="12" class="card-row">
      <el-col :span="4" v-for="svc in serviceCards" :key="svc.name">
        <div class="service-card" :class="{ abnormal: svc.status === '异常' }">
          <div class="svc-icon">
            <el-icon :size="40" :color="svc.status === '异常' ? '#FF745A' : '#0ABAFF'"><Collection /></el-icon>
          </div>
          <div class="svc-name">{{ svc.name }}</div>
          <div class="svc-status" :class="svc.status">{{ svc.status }}</div>
        </div>
      </el-col>
    </el-row>

    <el-row :gutter="16" class="content-row">
      <!-- 左侧图表区域 -->
      <el-col :span="14">
        <div class="panel chart-panel">
          <div class="panel-title">业务网络流量排行 TOP 5</div>
          <div class="traffic-list">
            <div v-for="item in trafficTop5" :key="item.name" class="traffic-item">
              <div class="traffic-rank">{{ item.rank }}</div>
              <div class="traffic-info">
                <div class="traffic-name">{{ item.name }}</div>
                <div class="traffic-bar-bg">
                  <div class="traffic-bar-fill" :style="{ width: item.percent + '%' }"></div>
                </div>
              </div>
              <div class="traffic-value">{{ item.value }}</div>
            </div>
          </div>
        </div>

        <div class="panel chart-panel">
          <div class="panel-title">告警类型统计</div>
          <v-chart :option="alertChartOption" autoresize style="height: 200px" />
        </div>
      </el-col>

      <!-- 右侧服务列表 -->
      <el-col :span="10">
        <div class="panel service-panel">
          <div class="panel-header">
            <div class="panel-title">服务运行状态</div>
            <el-checkbox v-model="onlyAbnormal" label="仅显示异常" size="small" />
          </div>
          <div class="service-list">
            <div v-for="svc in filteredServices" :key="svc.name" class="service-item">
              <div class="svc-rank">{{ svc.rank }}</div>
              <div class="svc-dot" :class="svc.status"></div>
              <div class="svc-info">
                <div class="svc-name">{{ svc.name }}</div>
                <div class="svc-ip">{{ svc.ip }}</div>
              </div>
              <div class="svc-qps">{{ svc.qps }}</div>
              <div class="svc-status-tag" :class="svc.status">{{ svc.statusText }}</div>
            </div>
          </div>
        </div>

        <div class="panel">
          <div class="panel-title">资源利用率</div>
          <div class="resource-list">
            <div v-for="res in resources" :key="res.name" class="resource-item">
              <div class="res-label">{{ res.name }}</div>
              <div class="res-bar-bg">
                <div class="res-bar-fill" :style="{ width: res.value + '%' }"></div>
              </div>
              <div class="res-value" :class="{ high: res.value > 80 }">{{ res.value }}%</div>
            </div>
          </div>
        </div>
      </el-col>
    </el-row>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { OfficeBuilding, Collection } from '@element-plus/icons-vue'

const onlyAbnormal = ref(false)

const businessCards = ref([
  { name: '业务-1', value: '2.8 Gbps', status: '正常' },
  { name: '业务-2', value: '1.5 Gbps', status: '正常' },
  { name: '业务-3', value: '2.8 Gbps', status: '正常' },
  { name: '业务-4', value: '1.5 Gbps', status: '正常' },
  { name: '业务-5', value: '2.8 Gbps', status: '正常' },
  { name: '业务-6', value: '1.5 Gbps', status: '正常' },
  { name: '业务-7', value: '1.5 Gbps', status: '正常' },
  { name: '业务-8', value: '1.5 Gbps', status: '异常' },
  { name: '业务-9', value: '1.5 Gbps', status: '正常' },
  { name: '业务-10', value: '1.5 Gbps', status: '正常' },
])

const serviceCards = ref([
  { name: 'order-service', status: '正常' },
  { name: 'user-service', status: '需关注' },
  { name: 'production-service', status: '正常' },
  { name: 'search-service', status: '正常' },
  { name: 'rds-service', status: '正常' },
  { name: 'redis-service', status: '正常' },
])

const services = ref([
  { name: 'order-service-7d8f9c-xk2m9', ip: '192.168.1.45', qps: '453次/秒', rank: 1, status: 'normal', statusText: '正常' },
  { name: 'user-service-9a2b3c-m7h5j', ip: '192.168.0.99', qps: '349次/秒', rank: 3, status: 'warning', statusText: '需关注' },
  { name: 'production-service-4e5f6g-q3w8r', ip: '192.168.0.41', qps: '320次/秒', rank: 4, status: 'normal', statusText: '正常' },
  { name: 'search-service-8h9i0j-t6y2u', ip: '10.244.3.12', qps: '290次/秒', rank: 5, status: 'normal', statusText: '正常' },
  { name: 'rds-service-1a2b3c-d4e5f6', ip: '192.168.1.100', qps: '234次/秒', rank: 6, status: 'normal', statusText: '正常' },
  { name: 'redis-service-7g8h9i-j0k1l2', ip: '192.168.1.101', qps: '189次/秒', rank: 7, status: 'normal', statusText: '正常' },
  { name: 'kafka-service-3m4n5o-p6q7r8', ip: '192.168.1.102', qps: '145次/秒', rank: 8, status: 'normal', statusText: '正常' },
])

const filteredServices = computed(() => {
  if (onlyAbnormal.value) {
    return services.value.filter(s => s.status === 'warning' || s.status === 'error')
  }
  return services.value
})

const trafficTop5 = ref([
  { name: '业务-1', value: '2.8 Gbps', percent: 85, rank: 1 },
  { name: '业务-3', value: '2.8 Gbps', percent: 80, rank: 2 },
  { name: '业务-5', value: '2.8 Gbps', percent: 75, rank: 3 },
  { name: '业务-2', value: '1.5 Gbps', percent: 45, rank: 4 },
  { name: '业务-4', value: '1.5 Gbps', percent: 40, rank: 5 },
])

const resources = ref([
  { name: 'CPU', value: 15 },
  { name: '内存', value: 55 },
  { name: '磁盘', value: 25 },
])

const alertChartOption = computed(() => ({
  tooltip: { trigger: 'item' },
  legend: { bottom: 0, textStyle: { color: '#fff' } },
  series: [{
    type: 'pie',
    radius: ['40%', '70%'],
    avoidLabelOverlap: false,
    itemStyle: { borderRadius: 6, borderColor: '#05385A', borderWidth: 2 },
    label: { show: true, color: '#fff', formatter: '{b}: {d}%' },
    data: [
      { name: '网络异常', value: 35, itemStyle: { color: '#FF745A' } },
      { name: '性能告警', value: 25, itemStyle: { color: '#00CCFF' } },
      { name: '安全事件', value: 20, itemStyle: { color: '#6BEDB7' } },
      { name: '配置变更', value: 20, itemStyle: { color: '#31A7FF' } },
    ]
  }]
}))
</script>

<style scoped lang="scss">
.dashboard {
  .card-row {
    margin-bottom: 16px;
  }
  .biz-card {
    background: rgba(10, 186, 255, 0.2);
    border: 1px solid rgba(10, 186, 255, 1);
    border-radius: 5px;
    padding: 12px;
    text-align: center;
    height: 150px;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 8px;
    transition: all 0.3s;
    cursor: pointer;
    &:hover {
      background: rgba(10, 186, 255, 0.3);
      transform: translateY(-2px);
    }
    &.abnormal {
      background: rgba(255, 116, 90, 0.2);
      border-color: #FF745A;
      &:hover {
        background: rgba(255, 116, 90, 0.3);
      }
    }
    .biz-name {
      font-size: 12px;
      color: #FFFFFF;
      font-family: '微软雅黑', sans-serif;
    }
    .biz-value {
      font-size: 14px;
      color: #00CCFF;
      font-weight: 700;
      font-family: 'DIN Bold', 'Arial', sans-serif;
    }
  }
  .service-card {
    background: rgba(10, 186, 255, 0.2);
    border: 1px solid rgba(10, 186, 255, 1);
    border-radius: 5px;
    padding: 16px;
    text-align: center;
    height: 154px;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 10px;
    transition: all 0.3s;
    cursor: pointer;
    &:hover {
      background: rgba(10, 186, 255, 0.3);
    }
    &.abnormal {
      background: rgba(255, 116, 90, 0.2);
      border-color: #FF745A;
    }
    .svc-name {
      font-size: 14px;
      color: #FFFFFF;
      font-family: '微软雅黑', sans-serif;
    }
    .svc-status {
      font-size: 12px;
      padding: 2px 12px;
      border-radius: 12px;
      &.正常 {
        color: #6BEDB7;
        background: rgba(107, 237, 183, 0.15);
      }
      &.需关注 {
        color: #FF745A;
        background: rgba(255, 116, 90, 0.15);
      }
      &.异常 {
        color: #FF745A;
        background: rgba(255, 116, 90, 0.15);
      }
    }
  }
  .content-row {
    margin-bottom: 16px;
  }
  .panel {
    background: rgba(10, 186, 255, 0.08);
    border: 1px solid rgba(10, 186, 255, 0.3);
    border-radius: 5px;
    padding: 16px;
    margin-bottom: 16px;
    .panel-header {
      display: flex;
      justify-content: space-between;
      align-items: center;
      margin-bottom: 12px;
    }
    .panel-title {
      font-size: 16px;
      font-weight: 600;
      color: #FFFFFF;
      font-family: '微软雅黑', sans-serif;
      margin-bottom: 12px;
    }
  }
  .traffic-list {
    display: flex;
    flex-direction: column;
    gap: 12px;
    .traffic-item {
      display: flex;
      align-items: center;
      gap: 12px;
      .traffic-rank {
        width: 24px;
        height: 24px;
        display: flex;
        align-items: center;
        justify-content: center;
        background: rgba(0, 204, 255, 0.2);
        border-radius: 50%;
        font-size: 12px;
        color: #00CCFF;
        font-weight: 700;
        font-family: 'DIN Bold', 'Arial', sans-serif;
      }
      .traffic-info {
        flex: 1;
        .traffic-name {
          font-size: 14px;
          color: #FFFFFF;
          margin-bottom: 4px;
        }
        .traffic-bar-bg {
          height: 8px;
          background: rgba(0, 204, 255, 0.1);
          border-radius: 140px;
          overflow: hidden;
          .traffic-bar-fill {
            height: 100%;
            background: rgba(0, 204, 255, 1);
            border-radius: 125px;
            transition: width 0.5s ease;
          }
        }
      }
      .traffic-value {
        font-size: 14px;
        color: #00CCFF;
        font-weight: 700;
        font-family: 'DIN Bold', 'Arial', sans-serif;
        min-width: 80px;
        text-align: right;
      }
    }
  }
  .service-list {
    display: flex;
    flex-direction: column;
    gap: 8px;
    .service-item {
      display: flex;
      align-items: center;
      gap: 10px;
      padding: 8px 12px;
      background: rgba(10, 186, 255, 0.05);
      border-radius: 4px;
      transition: background 0.2s;
      &:hover {
        background: rgba(10, 186, 255, 0.15);
      }
      .svc-rank {
        width: 20px;
        font-size: 12px;
        color: #00CCFF;
        font-weight: 700;
        font-family: 'DIN Bold', 'Arial', sans-serif;
      }
      .svc-dot {
        width: 8px;
        height: 8px;
        border-radius: 50%;
        &.normal {
          background: #6BEDB7;
        }
        &.warning {
          background: #FF745A;
        }
        &.error {
          background: #FF745A;
        }
      }
      .svc-info {
        flex: 1;
        .svc-name {
          font-size: 13px;
          color: #FFFFFF;
        }
        .svc-ip {
          font-size: 11px;
          color: #A4A8AE;
        }
      }
      .svc-qps {
        font-size: 12px;
        color: #00CCFF;
        font-weight: 700;
        font-family: 'DIN Bold', 'Arial', sans-serif;
      }
      .svc-status-tag {
        font-size: 12px;
        padding: 2px 8px;
        border-radius: 4px;
        &.正常 {
          color: #6BEDB7;
          background: rgba(107, 237, 183, 0.15);
        }
        &.需关注 {
          color: #FF745A;
          background: rgba(255, 116, 90, 0.15);
        }
        &.异常 {
          color: #FF745A;
          background: rgba(255, 116, 90, 0.15);
        }
      }
    }
  }
  .resource-list {
    display: flex;
    flex-direction: column;
    gap: 12px;
    .resource-item {
      display: flex;
      align-items: center;
      gap: 12px;
      .res-label {
        width: 50px;
        font-size: 13px;
        color: #FFFFFF;
      }
      .res-bar-bg {
        flex: 1;
        height: 8px;
        background: rgba(0, 204, 255, 0.1);
        border-radius: 140px;
        overflow: hidden;
        .res-bar-fill {
          height: 100%;
          background: rgba(0, 204, 255, 1);
          border-radius: 125px;
          transition: width 0.5s ease;
        }
      }
      .res-value {
        width: 50px;
        font-size: 13px;
        color: #00CCFF;
        font-weight: 700;
        font-family: 'DIN Bold', 'Arial', sans-serif;
        text-align: right;
        &.high {
          color: #FF745A;
        }
      }
    }
  }
}
</style>
