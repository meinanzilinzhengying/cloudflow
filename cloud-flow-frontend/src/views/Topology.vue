<template>
  <div class="topology-page">
    <!-- 页面头部 -->
    <div class="page-header">
      <h2 class="page-title">业务拓扑</h2>
      <div class="header-actions">
        <el-select
          v-model="viewMode"
          placeholder="视图"
          size="small"
          class="dark-select"
          @change="handleViewChange"
        >
          <el-option label="服务拓扑" value="service" />
          <el-option label="主机拓扑" value="host" />
          <el-option label="Pod 拓扑" value="pod" />
        </el-select>
        <el-button type="primary" size="small" class="dark-btn" @click="refresh" :loading="loading">
          刷新
        </el-button>
      </div>
    </div>

    <!-- 主内容区 -->
    <div class="topology-content">
      <!-- 左侧拓扑图 -->
      <div class="topology-chart" ref="topologyRef">
        <!-- 左下角图例 -->
        <div class="legend" v-if="nodes.length > 0">
          <div class="legend-item">
            <span class="legend-dot" style="background:#61DDAA"></span>
            <span>正常</span>
          </div>
          <div class="legend-item">
            <span class="legend-dot" style="background:#FF745A"></span>
            <span>异常</span>
          </div>
          <div class="legend-item">
            <span class="legend-dot" style="background:#FFC328"></span>
            <span>警告</span>
          </div>
          <div class="legend-item">
            <span class="legend-line"></span>
            <span>流量连接</span>
          </div>
        </div>
      </div>

      <!-- 右侧详情面板 -->
      <div class="detail-panel">
        <div class="panel-header">
          <div class="panel-title">
            <div class="title-bar"></div>
            <span>节点详情</span>
          </div>
        </div>
        <div class="panel-body">
          <!-- 未选中状态 -->
          <div class="empty-state" v-if="!selectedNode">
            <el-icon :size="48" color="rgba(255,255,255,0.3)"><DataAnalysis /></el-icon>
            <p>请点击拓扑图中的节点查看详情</p>
          </div>

          <!-- 选中节点后显示 -->
          <div class="detail-content" v-else>
            <!-- 基本信息区 -->
            <div class="node-basic">
              <div class="node-name">{{ selectedNode.name }}</div>
              <div class="node-tags">
                <span class="tag tag-type">{{ typeLabel(selectedNode.type) }}</span>
                <span
                  class="tag"
                  :class="selectedNode.status === 'normal' ? 'tag-normal' : selectedNode.status === 'abnormal' ? 'tag-abnormal' : 'tag-warning'"
                >
                  {{ statusLabel(selectedNode.status) }}
                </span>
              </div>
              <div class="node-ip" v-if="selectedNode.ip">
                <el-icon :size="12"><Monitor /></el-icon>
                <span>{{ selectedNode.ip }}</span>
              </div>
            </div>

            <!-- 性能指标区 -->
            <div class="metrics-grid">
              <div class="metric-item">
                <div class="metric-label">请求数</div>
                <div class="metric-value">{{ selectedNode.requests?.toLocaleString() || '-' }}</div>
              </div>
              <div class="metric-item">
                <div class="metric-label">流量</div>
                <div class="metric-value">{{ selectedNode.traffic || '-' }}</div>
              </div>
              <div class="metric-item">
                <div class="metric-label">平均延迟</div>
                <div class="metric-value">{{ selectedNode.latency ?? '-' }}<span class="metric-unit" v-if="selectedNode.latency !== undefined">ms</span></div>
              </div>
              <div class="metric-item">
                <div class="metric-label">错误率</div>
                <div class="metric-value" :class="(selectedNode.errorRate ?? 0) >= 5 ? 'value-warning' : ''">
                  {{ selectedNode.errorRate ?? '-' }}<span class="metric-unit" v-if="selectedNode.errorRate !== undefined">%</span>
                </div>
              </div>
              <div class="metric-item">
                <div class="metric-label">异常数</div>
                <div class="metric-value" :class="(selectedNode.alertCount ?? 0) > 0 ? 'value-abnormal' : ''">
                  {{ selectedNode.alertCount ?? '-' }}
                </div>
              </div>
              <div class="metric-item">
                <div class="metric-label">在线时长</div>
                <div class="metric-value">{{ selectedNode.uptime || '-' }}</div>
              </div>
            </div>

            <!-- 流量趋势图 -->
            <div class="trend-section">
              <div class="section-title">流量趋势（24h）</div>
              <div class="trend-chart" ref="trendRef"></div>
            </div>

            <!-- 关联节点列表 -->
            <div class="related-section">
              <div class="section-title">关联节点</div>
              <div class="related-list">
                <div
                  class="related-item"
                  v-for="(rel, idx) in relatedNodes"
                  :key="idx"
                  @click="handleRelatedClick(rel)"
                >
                  <div class="related-name">
                    <span class="direction-arrow" :class="'dir-' + rel.direction">{{ rel.direction === 'in' ? '←' : '→' }}</span>
                    <span>{{ rel.name }}</span>
                  </div>
                  <div class="related-traffic">{{ rel.traffic }}</div>
                </div>
                <div class="related-empty" v-if="relatedNodes.length === 0">暂无关联节点</div>
              </div>
            </div>

            <!-- 操作按钮区 -->
            <div class="action-buttons">
              <el-button size="small" class="action-btn" @click="viewDetail">查看详情</el-button>
              <el-button size="small" class="action-btn" @click="viewLog">查看日志</el-button>
              <el-button size="small" class="action-btn warn" @click="setAlert">设置告警</el-button>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, nextTick } from 'vue'
import * as echarts from 'echarts'
import axios from 'axios'
import { DataAnalysis, Monitor } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'

// 类型定义
interface TopologyNode {
  id: string
  name: string
  type: string
  category?: string
  value?: number
  status: string
  symbolSize?: number
  x?: number
  y?: number
  ip?: string
  latency?: number
  errorRate?: number
  alertCount?: number
  requests?: number
  traffic?: string
  uptime?: string
  trafficTrend?: { time: string; value: number }[]
}

interface TopologyLink {
  source: string
  target: string
  value?: number
  lineStyle?: { width?: number }
}

interface RelatedNode {
  id: string
  name: string
  direction: 'in' | 'out'
  traffic: string
}

// 响应式变量
const viewMode = ref<'service' | 'host' | 'pod'>('service')
const topologyRef = ref<HTMLDivElement | null>(null)
const trendRef = ref<HTMLDivElement | null>(null)
const chartInstance = ref<echarts.ECharts | null>(null)
const trendChartInstance = ref<echarts.ECharts | null>(null)
const loading = ref(false)
const nodes = ref<TopologyNode[]>([])
const links = ref<TopologyLink[]>([])
const selectedNode = ref<TopologyNode | null>(null)
const relatedNodes = ref<RelatedNode[]>([])

// API 实例
const api = axios.create({ baseURL: '/api', timeout: 30000 })

// 类型标签
const typeLabel = (type: string) => {
  const map: Record<string, string> = { service: '服务', host: '主机', pod: 'Pod', business: '业务' }
  return map[type] || type
}

// 状态标签
const statusLabel = (status: string) => {
  const map: Record<string, string> = { normal: '正常', abnormal: '异常', warning: '警告' }
  return map[status] || status
}

// 状态颜色
const statusColor = (status: string) => {
  const map: Record<string, string> = { normal: '#61DDAA', abnormal: '#FF745A', warning: '#FFC328' }
  return map[status] || '#FFFFFF'
}

// 加载拓扑数据
const refresh = async () => {
  loading.value = true
  try {
    const res = await api.get('/topology', { params: { type: viewMode.value } })
    const data = res.data?.data || res.data || {}
    nodes.value = data.nodes || []
    links.value = data.links || []

    renderTopology()
  } catch (err: any) {
    console.error('获取拓扑数据失败:', err)
    ElMessage.error('加载拓扑数据失败：' + (err.message || '网络错误'))
    renderEmpty('加载失败')
  } finally {
    loading.value = false
  }
}

// 渲染拓扑图
const renderTopology = () => {
  if (!topologyRef.value) return

  let chart = chartInstance.value
  if (!chart) {
    chart = echarts.init(topologyRef.value)
    chartInstance.value = chart
  }

  if (nodes.value.length === 0) {
    chart.setOption({
      backgroundColor: 'transparent',
      title: { text: '暂无拓扑数据', left: 'center', top: 'center', textStyle: { color: '#fff', fontSize: 16 } },
      series: []
    }, true)
    return
  }

  // 构建 ECharts graph 数据
  const chartNodes = nodes.value.map(n => ({
    id: n.id,
    name: n.name,
    category: n.type === 'service' ? 0 : n.type === 'host' ? 1 : n.type === 'pod' ? 2 : 3,
    value: n.value,
    symbolSize: n.symbolSize || calcNodeSize(n.value),
    itemStyle: {
      color: statusColor(n.status),
      borderColor: '#fff',
      borderWidth: selectedNode.value?.id === n.id ? 3 : 1,
      shadowColor: statusColor(n.status),
      shadowBlur: selectedNode.value?.id === n.id ? 20 : 10
    },
    label: { show: true, color: '#fff', fontSize: 12 },
    // 附加数据供 tooltip 使用
    _status: n.status,
    _ip: n.ip,
    _latency: n.latency,
    _alertCount: n.alertCount
  }))

  const chartLinks = links.value.map(l => ({
    source: l.source,
    target: l.target,
    value: l.value,
    lineStyle: {
      color: 'rgba(10, 186, 255, 0.6)',
      width: l.lineStyle?.width || calcLineWidth(l.value),
      curveness: 0.1,
      opacity: 0.8
    }
  }))

  chart.setOption({
    backgroundColor: 'transparent',
    tooltip: {
      backgroundColor: 'rgba(5, 56, 90, 0.9)',
      borderColor: '#0ABAFF',
      borderWidth: 1,
      textStyle: { color: '#fff', fontSize: 12 },
      formatter: (params: any) => {
        if (params.dataType === 'node') {
          const n = nodes.value.find(n => n.id === params.data.id)
          if (!n) return params.data.name
          let html = `<div style="font-weight:600;margin-bottom:8px">${n.name}</div>`
          html += `<div>类型：${typeLabel(n.type)}</div>`
          html += `<div>状态：<span style="color:${statusColor(n.status)}">${statusLabel(n.status)}</span></div>`
          if (n.ip) html += `<div>IP：${n.ip}</div>`
          if (n.value) html += `<div>流量：${n.value}</div>`
          if (n.latency !== undefined) html += `<div>延迟：${n.latency}ms</div>`
          if (n.alertCount) html += `<div>告警：${n.alertCount}个</div>`
          return html
        }
        if (params.dataType === 'edge') {
          return `<div>流量：${params.data.value || '-'}</div>`
        }
        return ''
      }
    },
    legend: { show: false },
    series: [{
      type: 'graph',
      layout: 'force',
      roam: true,
      draggable: true,
      force: {
        repulsion: 300,
        edgeLength: [80, 200],
        gravity: 0.1
      },
      label: { show: true, color: '#fff', fontSize: 12 },
      edgeSymbol: ['none', 'arrow'],
      edgeSymbolSize: [0, 8],
      data: chartNodes,
      links: chartLinks,
      emphasis: {
        focus: 'adjacency',
        lineStyle: { width: 4, color: '#0ABAFF' }
      }
    }]
  }, true)

  // 绑定节点点击事件
  chart.off('click')
  chart.on('click', (params: any) => {
    if (params.dataType === 'node') {
      const node = nodes.value.find(n => n.id === params.data.id)
      if (node) {
        selectedNode.value = node
        updateNodeHighlight()
        loadRelatedNodes(node)
        nextTick(() => renderTrendChart())
      }
    }
  })
}

// 计算节点大小
const calcNodeSize = (value?: number) => {
  if (!value) return 40
  if (value > 10000) return 80
  if (value > 5000) return 60
  return 40
}

// 计算连线宽度
const calcLineWidth = (value?: number) => {
  if (!value) return 1
  if (value > 10000) return 3
  if (value > 1000) return 2
  return 1
}

// 更新节点高亮状态
const updateNodeHighlight = () => {
  if (!chartInstance.value || !selectedNode.value) return
  const option = chartInstance.value.getOption()
  const series = option.series as any[]
  if (!series?.[0]?.data) return

  series[0].data = series[0].data.map((n: any) => ({
    ...n,
    itemStyle: {
      ...n.itemStyle,
      borderWidth: selectedNode.value?.id === n.id ? 3 : 1,
      shadowBlur: selectedNode.value?.id === n.id ? 20 : 10
    }
  }))

  chartInstance.value.setOption({ series }, false)
}

// 加载关联节点
const loadRelatedNodes = (node: TopologyNode) => {
  const related: RelatedNode[] = []

  links.value.forEach(l => {
    if (l.source === node.id) {
      const target = nodes.value.find(n => n.id === l.target)
      if (target) related.push({ id: target.id, name: target.name, direction: 'out', traffic: formatTraffic(l.value) })
    }
    if (l.target === node.id) {
      const source = nodes.value.find(n => n.id === l.source)
      if (source) related.push({ id: source.id, name: source.name, direction: 'in', traffic: formatTraffic(l.value) })
    }
  })

  relatedNodes.value = related.slice(0, 5)
}

// 格式化流量
const formatTraffic = (value?: number) => {
  if (!value) return '-'
  if (value > 1000000) return (value / 1000000).toFixed(1) + ' MB'
  if (value > 1000) return (value / 1000).toFixed(1) + ' KB'
  return value + ' B'
}

// 渲染流量趋势图
const renderTrendChart = () => {
  if (!trendRef.value || !selectedNode.value) return

  let chart = trendChartInstance.value
  if (!chart) {
    chart = echarts.init(trendRef.value)
    trendChartInstance.value = chart
  }

  // 生成模拟趋势数据（实际应从 API 获取）
  const trendData = selectedNode.value.trafficTrend ||
    Array.from({ length: 24 }, (_, i) => ({
      time: `${String(i).padStart(2, '0')}:00`,
      value: Math.floor(Math.random() * 1000) + 100
    }))

  chart.setOption({
    backgroundColor: 'transparent',
    grid: { top: 10, right: 10, bottom: 20, left: 40 },
    xAxis: {
      type: 'category',
      data: trendData.map(d => d.time),
      axisLine: { lineStyle: { color: 'rgba(255,255,255,0.2)' } },
      axisLabel: { color: 'rgba(255,255,255,0.5)', fontSize: 10, interval: 3 }
    },
    yAxis: {
      type: 'value',
      splitLine: { lineStyle: { color: 'rgba(255,255,255,0.1)' } },
      axisLine: { lineStyle: { color: 'rgba(255,255,255,0.2)' } },
      axisLabel: { color: 'rgba(255,255,255,0.5)', fontSize: 10 }
    },
    series: [{
      type: 'line',
      data: trendData.map(d => d.value),
      smooth: true,
      symbol: 'none',
      lineStyle: { color: '#0ABAFF', width: 2 },
      areaStyle: {
        color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [
          { offset: 0, color: 'rgba(10, 186, 255, 0.3)' },
          { offset: 1, color: 'rgba(10, 186, 255, 0.05)' }
        ])
      }
    }]
  })
}

// 视图切换
const handleViewChange = () => {
  selectedNode.value = null
  relatedNodes.value = []
  refresh()
}

// 点击关联节点
const handleRelatedClick = (rel: RelatedNode) => {
  const node = nodes.value.find(n => n.id === rel.id)
  if (node) {
    selectedNode.value = node
    updateNodeHighlight()
    loadRelatedNodes(node)
    nextTick(() => renderTrendChart())
  }
}

// 操作按钮
const viewDetail = () => ElMessage.info('查看详情功能开发中')
const viewLog = () => ElMessage.info('查看日志功能开发中')
const setAlert = () => ElMessage.info('设置告警功能开发中')

// 渲染空状态
const renderEmpty = (text: string) => {
  if (!chartInstance.value) return
  chartInstance.value.setOption({
    backgroundColor: 'transparent',
    title: { text, left: 'center', top: 'center', textStyle: { color: text === '加载失败' ? '#FF745A' : '#fff', fontSize: 16 } },
    series: []
  }, true)
}

// 窗口 resize 处理
const handleResize = () => {
  chartInstance.value?.resize()
  trendChartInstance.value?.resize()
}

onMounted(() => {
  refresh()
  window.addEventListener('resize', handleResize)
})

onUnmounted(() => {
  window.removeEventListener('resize', handleResize)
  chartInstance.value?.dispose()
  trendChartInstance.value?.dispose()
  chartInstance.value = null
  trendChartInstance.value = null
})
</script>

<style scoped lang="scss">
.topology-page {
  min-height: 100vh;
  padding: 20px 24px;
  background: #0a1628;

  .page-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 16px;

    .page-title {
      font-size: 18px;
      font-weight: 600;
      color: #FFFFFF;
    }

    .header-actions {
      display: flex;
      gap: 12px;
      align-items: center;

      .dark-select {
        :deep(.el-select__wrapper) {
          background: rgba(10, 186, 255, 0.15);
          border: 1px solid rgba(10, 186, 255, 0.3);
          box-shadow: none;
          .el-select__placeholder { color: rgba(255, 255, 255, 0.7); }
          .el-select__selected-item { color: #FFFFFF; }
        }
      }

      .dark-btn {
        background: rgba(10, 186, 255, 0.3) !important;
        border: 1px solid rgba(10, 186, 255, 0.5) !important;
        color: #FFFFFF !important;
        &:hover { background: rgba(10, 186, 255, 0.5) !important; }
      }
    }
  }

  .topology-content {
    display: flex;
    gap: 16px;
    height: calc(100vh - 140px);
    min-height: 500px;

    .topology-chart {
      flex: 1;
      height: 100%;
      background: rgba(10, 186, 255, 0.08);
      border: 1px solid rgba(10, 186, 255, 0.3);
      border-radius: 8px;
      position: relative;

      .legend {
        position: absolute;
        left: 16px;
        bottom: 16px;
        display: flex;
        gap: 16px;
        padding: 8px 12px;
        background: rgba(10, 22, 40, 0.8);
        border-radius: 4px;
        z-index: 10;

        .legend-item {
          display: flex;
          align-items: center;
          gap: 6px;
          font-size: 12px;
          color: rgba(255, 255, 255, 0.7);

          .legend-dot {
            width: 10px;
            height: 10px;
            border-radius: 50%;
          }

          .legend-line {
            width: 16px;
            height: 2px;
            background: rgba(10, 186, 255, 0.6);
          }
        }
      }
    }

    .detail-panel {
      width: 320px;
      flex-shrink: 0;
      background: rgba(10, 186, 255, 0.08);
      border: 1px solid rgba(10, 186, 255, 0.3);
      border-radius: 8px;
      display: flex;
      flex-direction: column;
      overflow: hidden;

      .panel-header {
        padding: 16px;
        border-bottom: 1px solid rgba(10, 186, 255, 0.15);

        .panel-title {
          display: flex;
          align-items: center;
          gap: 8px;
          font-size: 14px;
          font-weight: 600;
          color: #FFFFFF;

          .title-bar {
            width: 4px;
            height: 16px;
            background: #0ABAFF;
            border-radius: 2px;
          }
        }
      }

      .panel-body {
        flex: 1;
        overflow-y: auto;
        padding: 16px;

        &::-webkit-scrollbar { width: 4px; }
        &::-webkit-scrollbar-track { background: transparent; }
        &::-webkit-scrollbar-thumb { background: rgba(10, 186, 255, 0.3); border-radius: 2px; }

        .empty-state {
          display: flex;
          flex-direction: column;
          align-items: center;
          justify-content: center;
          height: 300px;
          gap: 16px;
          color: rgba(255, 255, 255, 0.5);
          font-size: 13px;
        }

        .detail-content {
          .node-basic {
            margin-bottom: 16px;

            .node-name {
              font-size: 16px;
              font-weight: 600;
              color: #FFFFFF;
              margin-bottom: 8px;
            }

            .node-tags {
              display: flex;
              gap: 8px;
              margin-bottom: 8px;

              .tag {
                padding: 2px 8px;
                border-radius: 4px;
                font-size: 12px;

                &.tag-type {
                  background: rgba(10, 186, 255, 0.2);
                  color: #0ABAFF;
                }
                &.tag-normal {
                  background: rgba(97, 221, 170, 0.2);
                  color: #61DDAA;
                }
                &.tag-abnormal {
                  background: rgba(255, 116, 90, 0.2);
                  color: #FF745A;
                }
                &.tag-warning {
                  background: rgba(255, 195, 40, 0.2);
                  color: #FFC328;
                }
              }
            }

            .node-ip {
              display: flex;
              align-items: center;
              gap: 6px;
              font-size: 12px;
              color: rgba(255, 255, 255, 0.5);
            }
          }

          .metrics-grid {
            display: grid;
            grid-template-columns: 1fr 1fr;
            gap: 12px;
            margin-bottom: 16px;

            .metric-item {
              background: rgba(10, 186, 255, 0.05);
              border: 1px solid rgba(10, 186, 255, 0.15);
              border-radius: 6px;
              padding: 10px;

              .metric-label {
                font-size: 11px;
                color: rgba(255, 255, 255, 0.5);
                margin-bottom: 4px;
              }

              .metric-value {
                font-size: 16px;
                font-weight: 600;
                color: #FFFFFF;
                font-family: Arial, sans-serif;

                .metric-unit {
                  font-size: 11px;
                  font-weight: 400;
                  color: rgba(255, 255, 255, 0.5);
                  margin-left: 2px;
                }

                &.value-warning { color: #FFC328; }
                &.value-abnormal { color: #FF745A; }
              }
            }
          }

          .trend-section {
            margin-bottom: 16px;

            .section-title {
              font-size: 13px;
              font-weight: 600;
              color: rgba(255, 255, 255, 0.7);
              margin-bottom: 8px;
            }

            .trend-chart {
              height: 120px;
            }
          }

          .related-section {
            margin-bottom: 16px;

            .section-title {
              font-size: 13px;
              font-weight: 600;
              color: rgba(255, 255, 255, 0.7);
              margin-bottom: 8px;
            }

            .related-list {
              .related-item {
                display: flex;
                justify-content: space-between;
                align-items: center;
                padding: 8px;
                border-radius: 4px;
                cursor: pointer;
                transition: background 0.2s;

                &:hover { background: rgba(10, 186, 255, 0.1); }

                .related-name {
                  display: flex;
                  align-items: center;
                  gap: 6px;
                  font-size: 12px;
                  color: rgba(255, 255, 255, 0.8);

                  .direction-arrow {
                    font-size: 14px;
                    &.dir-in { color: #61DDAA; }
                    &.dir-out { color: #FF745A; }
                  }
                }

                .related-traffic {
                  font-size: 11px;
                  color: rgba(255, 255, 255, 0.5);
                  font-family: Arial, sans-serif;
                }
              }

              .related-empty {
                text-align: center;
                padding: 16px;
                font-size: 12px;
                color: rgba(255, 255, 255, 0.3);
              }
            }
          }

          .action-buttons {
            display: flex;
            gap: 8px;

            .action-btn {
              flex: 1;
              background: rgba(10, 186, 255, 0.15) !important;
              border: 1px solid rgba(10, 186, 255, 0.3) !important;
              color: #FFFFFF !important;
              font-size: 12px;

              &:hover { background: rgba(10, 186, 255, 0.3) !important; }
              &.warn { border-color: rgba(255, 116, 90, 0.5) !important; color: #FF745A !important; }
            }
          }
        }
      }
    }
  }
}
</style>
