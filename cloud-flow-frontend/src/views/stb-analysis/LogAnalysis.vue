<template>
  <div class=stb-logs>
    <div class=page-header>
      <h2 class=page-title>STB 原始日志分析</h2>
      <div class=header-actions>
        <el-select v-model="protocolFilter" placeholder="协议" size="small" clearable style="width: 120px" @change="fetchLogs">
          <el-option label="全部" value="" />
          <el-option label="TCP" value="TCP" />
          <el-option label="UDP" value="UDP" />
          <el-option label="ICMP" value="ICMP" />
        </el-select>
        <el-input v-model="srcIpFilter" placeholder="源IP过滤" size="small" clearable style="width: 180px" @keyup.enter="fetchLogs" />
        <el-input v-model="dstIpFilter" placeholder="目标IP过滤" size="small" clearable style="width: 180px" @keyup.enter="fetchLogs" />
        <el-button type="primary" size="small" @click="fetchLogs">查询</el-button>
        <el-switch v-model="autoRefresh" active-text="自动刷新" size="small" style="margin-left: 12px" />
      </div>
    </div>
    <el-row :gutter="16" class="stats-row">
      <el-col :span="6">
        <el-card shadow="hover" class="stats-card">
          <div class="stat-value">{{ stats.totalRows.toLocaleString() }}</div>
          <div class="stat-label">总数据行数</div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card shadow="hover" class="stats-card">
          <div class="stat-value">{{ formatBytes(stats.totalBytes) }}</div>
          <div class="stat-label">总流量</div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card shadow="hover" class="stats-card">
          <div class="stat-value">{{ stats.tcpCount.toLocaleString() }}</div>
          <div class="stat-label">TCP 连接数</div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card shadow="hover" class="stats-card">
          <div class="stat-value">{{ stats.udpCount.toLocaleString() }}</div>
          <div class="stat-label">UDP 连接数</div>
        </el-card>
      </el-col>
    </el-row>
    <el-card class="table-card" :body-style="{ padding: '12px' }">
      <el-table :data="logs" size="small" style="width: 100%" height="520" stripe>
        <el-table-column prop="timestamp" label="时间戳" width="170" />
        <el-table-column prop="src_ip" label="源IP" width="140" />
        <el-table-column prop="dst_ip" label="目标IP" width="140" />
        <el-table-column prop="protocol" label="协议" width="70" />
        <el-table-column prop="dst_port" label="端口" width="70" />
        <el-table-column prop="bytes" label="字节数" width="90" />
        <el-table-column prop="packets" label="包数" width="70" />
        <el-table-column prop="details" label="详情" min-width="200" show-overflow-tooltip />
        <el-table-column prop="event_type" label="事件类型" width="100" />
      </el-table>
      <el-pagination class="pagination" background layout="total, sizes, prev, pager, next, jumper" :total="total" :page-size="pageSize" :page-sizes="[20, 50, 100]" @size-change="onSizeChange" @current-change="onPageChange" />
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, watch } from "vue"
import { queryClickHouse, PROBE_FILTER } from "@/api/stb"
interface FlowRow {
  timestamp: string; src_ip: string; dst_ip: string; protocol: string
  dst_port: number; bytes: number; packets: number; details: string; event_type: string
}
const logs = ref<FlowRow[]>([])
const total = ref(0)
const pageSize = ref(50)
const currentPage = ref(1)
const protocolFilter = ref("")
const srcIpFilter = ref("")
const dstIpFilter = ref("")
const autoRefresh = ref(false)
let refreshTimer: number | null = null
const stats = ref({ totalRows: 0, totalBytes: 0, tcpCount: 0, udpCount: 0 })
function formatBytes(b: number): string {
  if (b >= 1073741824) return (b / 1073741824).toFixed(2) + " GB"
  if (b >= 1048576) return (b / 1048576).toFixed(2) + " MB"
  if (b >= 1024) return (b / 1024).toFixed(2) + " KB"
  return b + " B"
}
function buildWhereClause(): string {
  const c: string[] = ["event_type = 'flow'", PROBE_FILTER]
  if (protocolFilter.value) c.push("protocol = '" + protocolFilter.value + "'")
  if (srcIpFilter.value) c.push("src_ip LIKE '%" + srcIpFilter.value + "%'")
  if (dstIpFilter.value) c.push("dst_ip LIKE '%" + dstIpFilter.value + "%'")
  return c.length > 0 ? " WHERE " + c.join(" AND ") : ""
}
async function fetchStats() {
  try {
    const w = buildWhereClause()
    const r = await queryClickHouse("SELECT count() as total, sum(bytes) as tb, countIf(protocol='TCP') as tcp, countIf(protocol='UDP') as udp FROM cloudflow.ebpf_events" + w)
    if (r.length > 0) stats.value = { totalRows: Number(r[0].total)||0, totalBytes: Number(r[0].tb)||0, tcpCount: Number(r[0].tcp)||0, udpCount: Number(r[0].udp)||0 }
  } catch(e) { console.error(e) }
}
async function fetchLogs() {
  try {
    const w = buildWhereClause()
    const off = (currentPage.value-1) * pageSize.value
    const r = await queryClickHouse("SELECT timestamp,src_ip,dst_ip,protocol,dst_port,bytes,packets,details,event_type FROM cloudflow.ebpf_events" + w + " ORDER BY timestamp DESC LIMIT " + pageSize.value + " OFFSET " + off)
    logs.value = r.map((x:any) => ({ timestamp: x.timestamp, src_ip: x.src_ip, dst_ip: x.dst_ip, protocol: x.protocol, dst_port: x.dst_port, bytes: Number(x.bytes)||0, packets: Number(x.packets)||0, details: x.details||"", event_type: x.event_type||"" }))
    const cr = await queryClickHouse("SELECT count() as cnt FROM cloudflow.ebpf_events" + w)
    total.value = cr.length > 0 ? Number(cr[0].cnt) : 0
    await fetchStats()
  } catch(e) { console.error(e) }
}
function onSizeChange(s: number) { pageSize.value=s; currentPage.value=1; fetchLogs() }
function onPageChange(p: number) { currentPage.value=p; fetchLogs() }
watch(autoRefresh, v => { if (v) refreshTimer=window.setInterval(fetchLogs,15000); else { if(refreshTimer!==null){clearInterval(refreshTimer);refreshTimer=null} } })
onMounted(() => fetchLogs())
onUnmounted(() => { if(refreshTimer!==null) clearInterval(refreshTimer) })
</script>

<style scoped>
.stb-logs { padding: 20px; }
.page-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 20px; }
.page-title { font-size: 20px; font-weight: 600; margin: 0; }
.header-actions { display: flex; align-items: center; gap: 8px; }
.stats-row { margin-bottom: 16px; }
.stats-card { text-align: center; }
.stat-value { font-size: 24px; font-weight: 700; color: #409eff; }
.stat-label { font-size: 13px; color: #909399; margin-top: 4px; }
.table-card { margin-bottom: 16px; }
.pagination { margin-top: 12px; justify-content: flex-end; }
</style>
