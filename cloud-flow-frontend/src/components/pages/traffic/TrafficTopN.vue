<template>
  <div class="space-y-6">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold text-slate-900 dark:text-white">TopN排行</h1>
        <p class="text-sm text-slate-500 dark:text-slate-400 mt-1">流量排行分析</p>
      </div>
      <div class="flex items-center gap-3">
        <button class="btn-secondary" @click="fetchData"><RefreshCw class="w-4 h-4" />刷新</button>
      </div>
    </div>

    <div class="grid grid-cols-2 gap-6">
      <div class="card p-6">
        <h3 class="text-lg font-semibold text-slate-900 dark:text-white mb-4">Top 源 IP</h3>
        <div class="space-y-3" v-if="topSrcIPs.length">
          <div v-for="(item, i) in topSrcIPs" :key="item.ip" class="flex items-center gap-3">
            <span class="w-6 text-xs font-medium text-slate-400">{{ i + 1 }}</span>
            <div class="flex-1">
              <div class="flex items-center justify-between mb-1">
                <span class="text-sm font-medium text-slate-700 dark:text-slate-200">{{ item.ip }}</span>
                <span class="text-xs text-slate-500">{{ item.value }}</span>
              </div>
              <div class="h-2 bg-slate-100 dark:bg-dark-700 rounded-full overflow-hidden">
                <div class="h-full bg-gradient-to-r from-primary-500 to-accent-500 rounded-full" :style="{'width': item.pct + '%'}"></div>
              </div>
            </div>
          </div>
        </div>
        <div v-else class="flex justify-center py-12 text-slate-400">暂无数据</div>
      </div>

      <div class="card p-6">
        <h3 class="text-lg font-semibold text-slate-900 dark:text-white mb-4">Top 目的 IP</h3>
        <div class="space-y-3" v-if="topDstIPs.length">
          <div v-for="(item, i) in topDstIPs" :key="item.ip" class="flex items-center gap-3">
            <span class="w-6 text-xs font-medium text-slate-400">{{ i + 1 }}</span>
            <div class="flex-1">
              <div class="flex items-center justify-between mb-1">
                <span class="text-sm font-medium text-slate-700 dark:text-slate-200">{{ item.ip }}</span>
                <span class="text-xs text-slate-500">{{ item.value }}</span>
              </div>
              <div class="h-2 bg-slate-100 dark:bg-dark-700 rounded-full overflow-hidden">
                <div class="h-full bg-gradient-to-r from-accent-500 to-emerald-500 rounded-full" :style="{'width': item.pct + '%'}"></div>
              </div>
            </div>
          </div>
        </div>
        <div v-else class="flex justify-center py-12 text-slate-400">暂无数据</div>
      </div>
    </div>

    <div class="card p-6">
      <h3 class="text-lg font-semibold text-slate-900 dark:text-white mb-4">Top 端口</h3>
      <div class="space-y-3" v-if="topPorts.length">
        <div v-for="(item, i) in topPorts" :key="item.port" class="flex items-center gap-3">
          <span class="w-6 text-xs font-medium text-slate-400">{{ i + 1 }}</span>
          <div class="flex-1">
            <div class="flex items-center justify-between mb-1">
              <span class="text-sm font-medium text-slate-700 dark:text-slate-200">{{ item.protocol }}/{{ item.port }}</span>
              <span class="text-xs text-slate-500">{{ item.value }}</span>
            </div>
            <div class="h-2 bg-slate-100 dark:bg-dark-700 rounded-full overflow-hidden">
              <div class="h-full bg-gradient-to-r from-violet-500 to-pink-500 rounded-full" :style="{'width': item.pct + '%'}"></div>
            </div>
          </div>
        </div>
      </div>
      <div v-else class="flex justify-center py-8 text-slate-400">暂无数据</div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { RefreshCw } from 'lucide-vue-next'

const flows = ref([])
const topSrcIPs = ref([])
const topDstIPs = ref([])
const topPorts = ref([])

const PROTOCOL_MAP = {
  80:'HTTP',443:'HTTPS',8080:'HTTP',8443:'HTTPS',3000:'Grafana',
  3306:'MySQL',5432:'PostgreSQL',6379:'Redis',27017:'MongoDB',8123:'ClickHouse',
  22:'SSH',21:'FTP',53:'DNS',123:'NTP',161:'SNMP',389:'LDAP',
  2379:'etcd',6443:'K8s API',9092:'Kafka',9093:'Alertmanager',
  9090:'Prometheus',16686:'Jaeger',4318:'OTel',
  9002:'DataPlane',8007:'QuerySvc',8009:'AlertSvc',8006:'AuthSvc',
  8001:'ControlPlane',8010:'TenantSvc',8082:'AISvc',
  10808:'Proxy',137:'NetBIOS',5353:'mDNS',1900:'UPnP',
}
const appProto = (p, port) => { return (port && PROTOCOL_MAP[port]) ? PROTOCOL_MAP[port] : p }

const formatBytes = (b) => {
  if (!b) return '0 B'
  if (b >= 1e9) return (b / 1e9).toFixed(2) + ' GB'
  if (b >= 1e6) return (b / 1e6).toFixed(2) + ' MB'
  if (b >= 1e3) return (b / 1e3).toFixed(2) + ' KB'
  return b + ' B'
}

const buildTop = (list, key) => {
  const map = {}
  flows.value.forEach(f => { const k = f[key]; if (k) map[k] = (map[k] || 0) + (f.bytes || 0) })
  const sorted = Object.entries(map).sort((a, b) => b[1] - a[1]).slice(0, 10)
  if (!sorted.length) return []
  const max = sorted[0][1]
  return sorted.map(([ip, bytes]) => ({ ip, value: formatBytes(bytes), pct: ((bytes / max) * 100).toFixed(0) }))
}

const fetchData = async () => {
  try {
    const res = await fetch('/api/query/flows?limit=2000')
    if (res.ok) {
      const data = await res.json()
      flows.value = data.records || []
    }
  } catch(e) { console.error(e) }
  
  topSrcIPs.value = buildTop(flows.value, 'src_ip')
  topDstIPs.value = buildTop(flows.value, 'dst_ip')
  
  // Top ports
  const pmap = {}
  flows.value.forEach(f => {
    if (f.dst_port) {
      const proto = appProto(f.protocol, f.dst_port)
      const k = proto + '/' + f.dst_port
      pmap[k] = (pmap[k] || 0) + (f.bytes || 0)
    }
  })
  const sorted = Object.entries(pmap).sort((a, b) => b[1] - a[1]).slice(0, 10)
  if (sorted.length) {
    const max = sorted[0][1]
    topPorts.value = sorted.map(([k, bytes]) => {
      const [proto, port] = k.split('/')
      return { protocol: proto, port, value: formatBytes(bytes), pct: ((bytes / max) * 100).toFixed(0) }
    })
  }
}

onMounted(() => { fetchData(); setInterval(fetchData, 30000) })
</script>
