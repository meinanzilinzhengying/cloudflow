<template>
  <div class="business-topo-page">
    <!-- 页面头部 -->
    <div class="topo-header">
      <div class="header-left">
        <span class="header-title">业务拓扑</span>
      </div>
      <div class="header-right">
        <span class="time-range">2026-02-04 00:00:00 - 2026-02-04 23:59:59</span>
        <el-icon :size="18" color="#fff"><Bell /></el-icon>
        <div class="admin-info">
          <el-avatar :size="24" style="background:#4A90D9;margin-right:6px;">管</el-avatar>
          <span>系统管理员</span>
          <span class="admin-name">admin</span>
        </div>
      </div>
    </div>

    <!-- 统计栏 -->
    <div class="stats-bar">
      <div class="stat-item" v-for="(stat, idx) in statsData" :key="idx">
        <div class="stat-label">{{ stat.label }}</div>
        <div class="stat-value">{{ stat.value }}</div>
        <div class="stat-icon" v-html="stat.icon"></div>
      </div>
    </div>

    <!-- 主内容区：左侧拓扑图 + 右侧业务详情 -->
    <div class="topo-body">
      <!-- 左侧：业务流向拓扑图 -->
      <div class="topo-canvas-wrap">
        <svg class="topo-svg" ref="svgRef">
          <defs>
            <marker id="arrowhead" markerWidth="10" markerHeight="7" refX="9" refY="3.5" orient="auto">
              <polygon points="0 0, 10 3.5, 0 7" fill="#7EB8FF" opacity="0.7"/>
            </marker>
          </defs>
          <path v-for="(link, i) in visibleLinks" :key="'link-'+i"
            :d="link.path"
            class="topo-link"
            :stroke-width="link.width"
          />
        </svg>
        <div class="topo-nodes-area" ref="nodesAreaRef">
          <div v-for="node in allNodes"
            :key="node.id"
            class="topo-node-card"
            :style="{ left: node.x + 'px', top: node.y + 'px' }"
            @click="selectNode(node)"
          >
            <!-- 节点头部 -->
            <div class="node-header">
              <div class="node-icon-row">
                <span :class="['node-type-icon', node.type === 'host' ? 'icon-host' : 'icon-service']"></span>
                <span class="node-ip">{{ node.ip }}</span>
                <el-badge :value="node.alertCount || 0" :max="99"
                  :hidden="!node.alertCount || node.alertCount === 0"
                  class="alert-badge">
                  <span></span>
                </el-badge>
              </div>
              <div class="node-name">{{ node.name }}</div>
            </div>
            <!-- 迷你趋势图 -->
            <div class="node-mini-chart">
              <svg width="80" height="20" viewBox="0 0 80 20">
                <polyline :points="node.chartPoints" fill="none" stroke="#4A90D9" stroke-width="1.5"/>
              </svg>
            </div>
            <!-- 指标行 -->
            <div class="node-metrics">
              <div class="metric-row"><span class="metric-label">每秒请求数:</span><span class="metric-val">{{ node.rps }}</span><span class="mini-bar"><span class="bar-fill" :style="{width: node.rpsBar}"></span></span></div>
              <div class="metric-row"><span class="metric-label">错误占比:</span><span class="metric-val">{{ node.errorRate }}</span><span class="mini-bar error"><span class="bar-fill" :style="{width: node.errorBar}"></span></span></div>
              <div class="metric-row"><span class="metric-label">平均延迟:</span><span class="metric-val">{{ node.avgLatency }}</span></div>
            </div>
          </div>
        </div>
      </div>

      <!-- 右侧：业务详情面板 -->
      <div class="business-panel">
        <div class="panel-header">
          <span class="panel-title">业务详情</span>
          <el-select v-model="selectedBusiness" size="small" placeholder="选择业务" style="width:120px;">
            <el-option label="全部" value="all" />
            <el-option v-for="b in businessList" :key="b.id" :label="b.name" :value="b.id" />
          </el-select>
          <el-button type="primary" size="small" @click="queryBusiness">查询</el-button>
        </div>
        <div class="business-list">
          <div v-for="biz in filteredBusinesses" :key="biz.id" class="biz-card">
            <div class="biz-header">
              <span :class="['biz-status-dot', biz.statusClass]"></span>
              <span class="biz-name">{{ biz.name }}</span>
              <el-tag v-if="biz.tag1" size="small" :type="biz.tagType1" effect="light" round>{{ biz.tag1 }}</el-tag>
              <el-tag v-if="biz.tag2" size="small" :type="biz.tagType2" effect="light" round>{{ biz.tag2 }}</el-tag>
              <el-badge :value="biz.alertCount || 0" :max="99"
                :hidden="!biz.alertCount || biz.alertCount === 0"
                class="biz-alert-badge">
                <span></span>
              </el-badge>
            </div>
            <div class="biz-throughput">
              <span class="throughput-label">网络流量</span>
              <span class="throughput-value">{{ biz.throughput }}</span>
            </div>
            <div class="biz-alerts" v-if="biz.alerts && biz.alerts.length > 0">
              <div v-for="(alert, ai) in biz.alerts" :key="ai" class="alert-item">
                <span :class="['alert-severity', alert.severity]"></span>
                <span class="alert-text">{{ alert.text }}</span>
                <span class="alert-time">{{ alert.time }}</span>
              </div>
            </div>
          </div>
          <div v-if="filteredBusinesses.length === 0" class="empty-businesses">
            暂无业务数据
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, nextTick } from 'vue'
import { Bell } from '@element-plus/icons-vue'
import axios from 'axios'
import { ElMessage } from 'element-plus'

// ===== 类型定义 =====
interface TopoNode {
  id: string
  ip: string
  name: string
  type: string // host | service | container
  column: number
  row: number
  rps: string
  errorRate: string
  avgLatency: string
  rpsBar: string
  errorBar: string
  alertCount?: number
  chartPoints: string
  x: number
  y: number
}

interface TopoLink {
  sourceId: string
  targetId: string
  path: string
  width: number
}

interface BusinessItem {
  id: string
  name: string
  status: string
  statusClass: string
  tag1?: string
  tagType1?: '' | 'success' | 'warning' | 'danger'
  tag2?: string
  tagType2?: '' | 'success' | 'warning' | 'danger'
  alertCount?: number
  throughput: string
  alerts?: { severity: string; text: string; time: string }[]
}

// ===== 响应式数据 =====
const svgRef = ref<SVGSVGElement | null>(null)
const nodesAreaRef = ref<HTMLDivElement | null>(null)
const selectedBusiness = ref('all')
const loading = ref(false)

// 节点卡片尺寸
const NODE_W = 200
const NODE_H = 110

// 统计数据
const statsData = ref([
  { label: '业务数量', value: 10, icon: '<svg viewBox="0 0 24 24" width="22" height="22" fill="#E6A23C"><path d="M19 3H5c-1.1 0-2 .9-2 2v14c0 1.1.9 2 2 2h14c1.1 0 2-.9 2-2V5c0-1.1-.9-2-2-2zm-5 14H7v-2h7v2zm3-4H7v-2h10v2zm0-4H7V7h10v2z"/></svg>' },
  { label: '服务数量', value: 5, icon: '<svg viewBox="0 0 24 24" width="22" height="22" fill="#409EFF"><circle cx="12" cy="12" r="10"/><path d="M12 6v6l4 2" stroke="#fff" stroke-width="1.5" fill="none"/></svg>' },
  { label: '云主机数', value: 5, icon: '<svg viewBox="0 0 24 24" width="22" height="22" fill="#67C23A"><path d="M21 16.5c0 .38-.21.71-.53.88l-7.9 4.44c-.16.12-.36.18-.57.18s-.41-.06-.57-.18l-7.9-4.44A.991.991 0 013 16.5v-9c0-.38.21-.71.53-.88l7.9-4.44c.16-.12.36-.18.57-.18s.41.06.57.18l7.9 4.44c.32.17.53.5.53.88v9z"/></svg>' },
  { label: '容器数', value: 20, icon: '<svg viewBox="0 0 24 24" width="22" height="22" fill="#409EFF"><path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm-2 15l-5-5 1.41-1.41L10 14.17l7.59-7.59L19 8l-9 9z"/></svg>' },
  { label: '健康率', value: '100%', icon: '<svg viewBox="0 0 24 24" width="22" height="22" fill="#67C23A"><path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm-2 15l-5-5 1.41-1.41L10 14.17l7.59-7.59L19 8l-9 9z"/></svg>' },
])

// 拓扑节点
const allNodes = ref<TopoNode[]>([])
const visibleLinks = ref<TopoLink[]>([])

// 业务列表
const businessList = ref<BusinessItem[]>([])

// 过滤后的业务
const filteredBusinesses = computed(() => {
  if (selectedBusiness.value === 'all') return businessList.value
  return businessList.value.filter(b => b.id === selectedBusiness.value)
})

// ===== 方法 =====

function generateChartPoints(): string {
  const pts: string[] = []
  for (let i = 0; i < 20; i++) {
    const x = (i / 19) * 78 + 1
    const h = Math.random() * 16 + 2
    pts.push(x + ',' + (20 - h))
  }
  return pts.join(' ')
}

function calcPath(sx: number, sy: number, tx: number, ty: number): string {
  var midX = sx + (tx - sx) / 2
  return 'M ' + sx + ' ' + sy + ' C ' + midX + ' ' + sy + ', ' + midX + ' ' + ty + ', ' + tx + ' ' + ty
}

async function loadTopology() {
  loading.value = true
  try {
    var topoData: any = null
    try {
      var res = await axios.get('/api/v1/network/topology', { timeout: 8000 })
      if (res.data && res.data.data) topoData = res.data.data
    } catch (e) {
      console.log('API获取拓扑失败，使用示例数据')
    }

    if (topoData && topoData.nodes && topoData.nodes.length > 0) {
      buildFromAPI(topoData)
    } else {
      buildDemoData()
    }

    await nextTick()
    layoutNodes()
  } catch (err) {
    console.error(err)
    buildDemoData()
    await nextTick()
    layoutNodes()
  } finally {
    loading.value = false
  }
}

function buildFromAPI(_data: any) {
  buildDemoData()
}

function buildDemoData() {
  var nodes: TopoNode[] = [
    // 列1: 入口/前端节点
    { id: 'n1', ip: '192.168.0.217', name: 'loadgenerator', type: 'host', column: 0, row: 0,
      rps: '0.24/s', errorRate: '0.00%', avgLatency: '18.02ms',
      rpsBar: '24%', errorBar: '0%', alertCount: 1, chartPoints: generateChartPoints(), x: 0, y: 0 },
    { id: 'n2', ip: '192.168.0.10', name: 'dashboard-scraper', type: 'host', column: 0, row: 1,
      rps: '0.00/s', errorRate: '0.00%', avgLatency: '16.10ms',
      rpsBar: '0%', errorBar: '0%', chartPoints: generateChartPoints(), x: 0, y: 0 },
    { id: 'n3', ip: '192.168.0.169', name: 'nginxsvcv1-1', type: 'service', column: 0, row: 2,
      rps: '0.41/s', errorRate: '0.00%', avgLatency: '13.27ms',
      rpsBar: '41%', errorBar: '0%', chartPoints: generateChartPoints(), x: 0, y: 0 },
    { id: 'n4', ip: '192.168.0.232', name: 'productpage-v1', type: 'service', column: 0, row: 3,
      rps: '0.41/s', errorRate: '0.00%', avgLatency: '11.30ms',
      rpsBar: '41%', errorBar: '0%', chartPoints: generateChartPoints(), x: 0, y: 0 },

    // 列2: 中间层服务
    { id: 'n5', ip: '10.108.215.210', name: 'coredns-11', type: 'service', column: 1, row: 0,
      rps: '0.00/s', errorRate: '0.00%', avgLatency: '4.57ms',
      rpsBar: '0%', errorBar: '0%', alertCount: 1, chartPoints: generateChartPoints(), x: 0, y: 0 },
    { id: 'n6', ip: '10.104.72.57', name: 'rds-user', type: 'service', column: 1, row: 1,
      rps: '0.44/s', errorRate: '0.00%', avgLatency: '3.56ms',
      rpsBar: '44%', errorBar: '0%', chartPoints: generateChartPoints(), x: 0, y: 0 },
    { id: 'n7', ip: '', name: 'test-monitor-rds-single-01', type: 'service', column: 1, row: 2,
      rps: '0.42/s', errorRate: '0.00%', avgLatency: '2.27ms',
      rpsBar: '42%', errorBar: '0%', chartPoints: generateChartPoints(), x: 0, y: 0 },
    { id: 'n8', ip: '', name: 'test-monitor-rds-gen-single-01', type: 'service', column: 1, row: 3,
      rps: '0.19/s', errorRate: '0.00%', avgLatency: '5.20ms',
      rpsBar: '19%', errorBar: '0%', chartPoints: generateChartPoints(), x: 0, y: 0 },

    // 列3: 后端存储
    { id: 'n9', ip: '172.30.172.161', name: 'mysql-order', type: 'service', column: 2, row: 0,
      rps: '0.14/s', errorRate: '0.00%', avgLatency: '7.03ms',
      rpsBar: '14%', errorBar: '0%', chartPoints: generateChartPoints(), x: 0, y: 0 },
    { id: 'n10', ip: '192.168.0.41', name: 'redis-cache', type: 'service', column: 2, row: 1,
      rps: '0.15/s', errorRate: '0.00%', avgLatency: '11.21ms',
      rpsBar: '15%', errorBar: '0%', chartPoints: generateChartPoints(), x: 0, y: 0 },
  ]

  allNodes.value = nodes

  businessList.value = [
    { id: 'b1', name: '业务-1', status: '正常', statusClass: 'status-normal',
      tag1: '运行正常', tagType1: 'success', throughput: '2.3GBps', alertCount: 0 },
    { id: 'b2', name: '业务-2', status: '警告', statusClass: 'status-warning',
      tag1: '运行正常', tagType1: 'success', tag2: '告警关注', tagType2: 'warning', throughput: '1.8GBps', alertCount: 3,
      alerts: [{ severity: 'warning', text: '连接数接近上限', time: '23分钟' }] },
    { id: 'b3', name: '业务-3', status: '异常', statusClass: 'status-abnormal',
      tag1: '需要关注', tagType1: 'warning', tag2: '告警异常', tagType2: 'danger', throughput: '1.2GBps', alertCount: 3,
      alerts: [{ severity: 'warning', text: '平均响应时间增高15%', time: '5分钟' }, { severity: 'info', text: 'http 404 状态码异常', time: '10分钟' }] },
    { id: 'b4', name: '业务-4', status: '正常', statusClass: 'status-normal',
      tag1: '运行正常', tagType1: 'success', throughput: '456MBps', alertCount: 0 },
    { id: 'b5', name: '业务-5', status: '异常', statusClass: 'status-abnormal',
      tag1: '需要关注', tagType1: 'warning', tag2: '告警异常', tagType2: 'danger', throughput: '234MBps', alertCount: 3,
      alerts: [{ severity: 'error', text: '连接激增', time: '1分钟' }, { severity: 'warning', text: 'CPU使用率 80%', time: '23分钟' }] },
    { id: 'b6', name: '业务-6', status: '正常', statusClass: 'status-normal',
      tag1: '运行正常', tagType1: 'success', throughput: '189MBps', alertCount: 1,
      alerts: [{ severity: 'info', text: '延迟告警 0.2s', time: '1分钟' }] },
  ]
}

function layoutNodes() {
  var colMap: Record<number, TopoNode[]> = {}
  for (var ni = 0; ni < allNodes.value.length; ni++) {
    var n = allNodes.value[ni]
    if (!colMap[n.column]) colMap[n.column] = []
    colMap[n.column].push(n)
  }

  var cols = Object.keys(colMap).map(Number).sort(function(a,b){return a-b})
  var startX = 20
  var startY = 20
  var COL_GAP = 120
  var ROW_GAP = 30

  for (var ci = 0; ci < cols.length; ci++) {
    var c = cols[ci]
    var colNodes = colMap[c]
    for (var ri = 0; ri < colNodes.length; ri++) {
      var node = colNodes[ri]
      node.x = startX + c * (NODE_W + COL_GAP)
      node.y = startY + ri * (NODE_H + ROW_GAP)
    }
  }

  var links: TopoLink[] = []
  for (var i = 0; i < allNodes.value.length; i++) {
    for (var j = i+1; j < allNodes.value.length; j++) {
      var a = allNodes.value[i]
      var b = allNodes.value[j]
      if (a.column + 1 === b.column) {
        var sx = a.x + NODE_W
        var sy = a.y + NODE_H / 2
        var tx = b.x
        var ty = b.y + NODE_H / 2
        links.push({
          sourceId: a.id,
          targetId: b.id,
          path: calcPath(sx, sy, tx, ty),
          width: 1.5
        })
      }
    }
  }
  visibleLinks.value = links
}

function selectNode(node: TopoNode) {
  console.log('选中节点:', node.name)
}

function queryBusiness() {
  ElMessage.info('查询业务: ' + selectedBusiness.value)
}

onMounted(function() {
  loadTopology()
})
</script>

<style scoped>
.business-topo-page {
  min-height: 100vh;
  background: #F5F7FA;
  display: flex;
  flex-direction: column;
}
.topo-header {
  height: 50px;
  background: linear-gradient(135deg, #1e3a5f 0%, #2d5a87 100%);
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 20px;
}
.header-title {
  color: #fff;
  font-size: 15px;
  font-weight: 600;
  padding: 4px 14px;
  border: 1px solid rgba(64,158,255,.5);
  border-radius: 3px;
  background: rgba(64,158,255,.15);
}
.header-right {
  display: flex;
  align-items: center;
  gap: 16px;
  color: rgba(255,255,255,.75);
  font-size: 12px;
}
.time-range { color: rgba(255,255,255,.65); }
.admin-info {
  display: flex;
  align-items: center;
  gap: 4px;
  color: rgba(255,255,255,.85);
}
.admin-name { color: rgba(255,255,255,.55); margin-left: 4px; }

.stats-bar {
  display: flex;
  gap: 0;
  padding: 0;
  margin: 0;
  background: #fff;
  border-bottom: 1px solid #E4E7ED;
}
.stat-item {
  flex: 1;
  position: relative;
  padding: 14px 16px 14px 52px;
  border-right: 1px solid #EBEEF5;
  text-align: left;
}
.stat-item:last-child { border-right: none; }
.stat-label { font-size: 13px; color: #606266; margin-bottom: 2px; }
.stat-value { font-size: 22px; font-weight: 700; color: #303133; font-family: Arial, Helvetica, sans-serif; }
.stat-icon {
  position: absolute;
  left: 16px;
  top: 50%;
  transform: translateY(-50%);
  opacity: 0.7;
}

.topo-body { display: flex; flex: 1; overflow: hidden; position: relative; }

/* 左侧拓扑画布 */
.topo-canvas-wrap {
  flex: 1;
  position: relative;
  overflow: auto;
  background: #fff;
}
.topo-svg {
  position: absolute;
  top: 0;
  left: 0;
  width: 100%;
  height: 100%;
  z-index: 1;
  pointer-events: none;
}
.topo-link {
  fill: none;
  stroke: #7EB8FF;
  opacity: 0.45;
  pointer-events: none;
}

.topo-nodes-area {
  position: relative;
  z-index: 2;
  padding: 20px;
  min-height: 500px;
  min-width: 900px;
}

/* 节点卡片 */
.topo-node-card {
  position: absolute;
  width: 200px;
  background: #fff;
  border: 1px solid #DCDFE6;
  border-radius: 6px;
  box-shadow: 0 2px 8px rgba(0,0,0,.06);
  cursor: pointer;
  transition: transform .2s, box-shadow .2s, border-color .2s;
  overflow: hidden;
}
.topo-node-card:hover {
  transform: translateY(-2px);
  box-shadow: 0 4px 16px rgba(0,0,0,.12);
  border-color: #409EFF;
}

.node-header { padding: 8px 10px 4px; }
.node-icon-row {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 2px;
}
.node-type-icon {
  display: inline-block;
  width: 16px;
  height: 16px;
  border-radius: 3px;
  vertical-align: middle;
}
.icon-host { background: linear-gradient(135deg, #74b9ff, #0984e3); }
.icon-service { background: linear-gradient(135deg, #a29bfe, #6c5ce7); }
.node-ip {
  font-size: 11px;
  color: #606266;
  font-family: Consolas, monospace;
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.node-name {
  font-size: 12px;
  color: #303133;
  font-weight: 600;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.node-mini-chart { padding: 0 10px; height: 20px; }

.node-metrics { padding: 4px 10px 8px; }
.metric-row {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: 11px;
  line-height: 18px;
  color: #909399;
}
.metric-label { flex-shrink: 0; color: #909399; font-size: 11px; }
.metric-val {
  flex-shrink: 0;
  font-size: 11px;
  color: #303133;
  font-family: Consolas, monospace;
  width: 56px;
  text-align: right;
}
.mini-bar {
  flex: 1;
  height: 4px;
  background: #EBEEF5;
  border-radius: 2px;
  overflow: hidden;
  max-width: 48px;
}
.mini-bar .bar-fill {
  display: block;
  height: 100%;
  background: #409EFF;
  border-radius: 2px;
  transition: width .3s;
}
.mini-bar.error .bar-fill { background: #67C23A; }
.alert-badge :deep(.el-badge__content) { font-size: 10px; }

/* 右侧业务面板 */
.business-panel {
  width: 300px;
  flex-shrink: 0;
  background: #fff;
  border-left: 1px solid #E4E7ED;
  display: flex;
  flex-direction: column;
}
.panel-header {
  display: flex;
  align-items: center;
  padding: 12px 14px;
  border-bottom: 1px solid #EBEEF5;
  gap: 8px;
  flex-shrink: 0;
}
.panel-title { font-size: 14px; font-weight: 600; color: #303133; flex-shrink: 0; }

.business-list {
  flex: 1;
  overflow-y: auto;
  padding: 8px 0;
}
.business-list::-webkit-scrollbar { width: 4px; }
.business-list::-webkit-scrollbar-thumb { background: #C0C4CC; border-radius: 2px; }

.biz-card {
  margin: 6px 10px;
  padding: 12px;
  border: 1px solid #EBEEF5;
  border-radius: 6px;
  transition: border-color .2s, box-shadow .2s;
  cursor: pointer;
}
.biz-card:hover {
  border-color: #409EFF;
  box-shadow: 0 2px 8px rgba(64,158,255,.1);
}
.biz-header {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
  margin-bottom: 8px;
}
.biz-status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  flex-shrink: 0;
}
.status-normal { background: #67C23A; }
.status-warning { background: #E6A23C; }
.status-abnormal { background: #F56C6C; }
.biz-name { font-size: 13px; font-weight: 600; color: #303133; flex-shrink: 0; }
.biz-alert-badge { margin-left: auto; }
.biz-alert-badge :deep(.el-badge__content) { font-size: 10px; }

.biz-throughput {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 6px;
}
.throughput-label { font-size: 11px; color: #909399; }
.throughput-value { font-size: 14px; font-weight: 700; color: #303133; font-family: Arial, sans-serif; }

.biz-alerts { border-top: 1px dashed #EBEEF5; padding-top: 6px; }
.alert-item { display: flex; align-items: center; gap: 6px; padding: 3px 0; font-size: 11px; }
.alert-severity { width: 6px; height: 6px; border-radius: 50%; flex-shrink: 0; }
.alert-severity.error { background: #F56C6C; }
.alert-severity.warning { background: #E6A23C; }
.alert-severity.info { background: #409EFF; }
.alert-text { color: #606266; flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.alert-time { color: #C0C4CC; flex-shrink: 0; font-size: 10px; }

.empty-businesses {
  text-align: center;
  padding: 40px 20px;
  color: #C0C4CC;
  font-size: 13px;
}
</style>
