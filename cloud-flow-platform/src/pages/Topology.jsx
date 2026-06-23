import { useState, useEffect, useRef, useCallback } from 'react';
import * as echarts from 'echarts';
import { fetchServiceHealth } from '../api';

const LINK_API = '/api/link/';

// ============================================================
// 左到右布局（坐标范围 X:0~950, Y:0~450）
// 使用 center:['45%','50%'] 百分比居中，确保节点正确显示
//
// 列1      列2        列3     列4        列5        列6        列7
// 探针    Redis       Nginx   Edge/AI    Alert等    支撑服务    前端
// ingest  ClickHouse          Control    Sysstats              ...
// ============================================================

const NODES_TEMPLATE = [
  // ===== 第1列：数据采集 =====
  { id: 'probe',      name: 'eBPF探针',    x: 30,  y: 160, group: 'source' },
  { id: 'ingest',     name: 'data-ingest',  x: 30,  y: 300, group: 'ingest' },

  // ===== 第2列：数据存储 =====
  { id: 'redis',      name: 'Redis',        x: 175, y: 100, group: 'buffer' },
  { id: 'clickhouse', name: 'ClickHouse',   x: 175, y: 260, group: 'storage' },

  // ===== 第3列：代理入口 =====
  { id: 'nginx', name: 'Nginx', x: 330, y: 180, group: 'proxy' },

  // ===== 第4列：核心服务左组 =====
  { id: 'edge',    name: 'Edge节点',   x: 480, y: 70,  group: 'edge' },
  { id: 'ai',      name: 'AI服务',     x: 480, y: 180, group: 'control' },
  { id: 'control', name: 'Control',    x: 480, y: 290, group: 'control' },

  // ===== 第5列：核心服务右组 =====
  { id: 'alert',    name: 'Alert引擎',  x: 620, y: 70,  group: 'control' },
  { id: 'sysstats', name: '系统采集',   x: 620, y: 180, group: 'control' },
  { id: 'config',   name: '配置服务',   x: 620, y: 290, group: 'support' },

  // ===== 第6列：支撑服务 =====
  { id: 'log',         name: '日志服务',   x: 760, y: 70,  group: 'support' },
  { id: 'link',        name: '链路监控',   x: 760, y: 180, group: 'support' },
  { id: 'edge_health', name: '边缘健康',   x: 760, y: 290, group: 'support' },
  { id: 'cluster',     name: '集群API',    x: 760, y: 400, group: 'support' },

  // ===== 第7列：展示层 =====
  { id: 'frontend', name: '前端', x: 900, y: 180, group: 'display' },
];

const GROUP_COLOR = {
  source:  '#a78bfa',
  ingest:  '#f59e0b',
  buffer:  '#8b5cf6',
  storage: '#10b981',
  proxy:   '#ef4444',
  edge:    '#22d3ee',
  display: '#06b6d4',
  control: '#f97316',
  support: '#64748b',
};

const GROUP_LABEL = {
  source:  '数据源', ingest: '接入层', buffer: '缓冲层',
  storage: '存储层', proxy: '代理层', edge: 'Edge',
  display: '展示层', control: '控制面', support: '支撑服务',
};

function fmtBytes(b) {
  if (b < 0) return '-';
  if (b < 1024) return b + ' B/s';
  if (b < 1024 * 1024) return (b / 1024).toFixed(1) + ' KB/s';
  return (b / 1024 / 1024).toFixed(1) + ' MB/s';
}

export default function Topology() {
  const chartRef = useRef(null);
  const chartInst = useRef(null);
  const [linkData, setLinkData] = useState(null);
  const [services, setServices] = useState([]);
  const [lastUpdate, setLastUpdate] = useState(null);
  const [loading, setLoading] = useState(true);

  const buildNodes = () => {
    return NODES_TEMPLATE.map(n => {
      let status = 'unknown';
      let metrics = {};

      if (n.id === 'frontend') status = 'up';
      else if (n.id === 'probe') {
        const s = services.find(s => s.name?.includes('ebpf-probe') || s.port === 9090);
        status = s?.status === 'up' ? 'up' : (s ? 'down' : 'unknown');
      } else if (n.id === 'ingest') {
        const s = services.find(s => s.port === 9104 || s.name?.includes('data-ingest'));
        status = s?.status === 'up' ? 'up' : (s ? 'down' : 'unknown');
      } else if (n.id === 'redis') {
        const s = services.find(s => s.name?.includes('redis'));
        status = s?.status === 'up' ? 'up' : (s ? 'down' : 'unknown');
        if (linkData?.nodes?.['redis-vm1']?.ops_per_sec !== undefined) {
          metrics.ops = linkData.nodes['redis-vm1'].ops_per_sec;
        }
      } else if (n.id === 'clickhouse') {
        const s = services.find(s => s.port === 8123 || s.name?.includes('clickhouse'));
        status = s?.status === 'up' ? 'up' : (s ? 'down' : 'unknown');
        if (linkData?.nodes?.['clickhouse-vm1']?.qps !== undefined) {
          metrics.qps = linkData.nodes['clickhouse-vm1'].qps;
        }
      } else if (n.id === 'nginx') {
        const s = services.find(s => s.port === 8080 || s.name?.includes('nginx'));
        status = s?.status === 'up' ? 'up' : (s ? 'down' : 'unknown');
        if (linkData?.nodes?.['nginx-vm1']?.req_per_sec !== undefined) {
          metrics.rps = linkData.nodes['nginx-vm1'].req_per_sec;
        }
      } else if (['config','log','link','edge_health','cluster'].includes(n.id)) {
        const pm = { config:9108, log:9106, link:9105, edge_health:8081, cluster:8083 };
        const s = services.find(s => s.port === pm[n.id]);
        status = s?.status === 'up' ? 'up' : (s ? 'down' : 'unknown');
      } else if (n.id === 'edge') {
        const s = services.find(s => s.port === 9102 || s.name?.includes('data-plane') || s.name?.includes('edge'));
        status = s?.status === 'up' ? 'up' : (s ? 'down' : 'unknown');
      } else {
        const pm = { ai:8082, control:8001, alert:9010, sysstats:9099 };
        const s = services.find(s => s.port === pm[n.id] || s.name?.includes(n.id));
        status = s?.status === 'up' ? 'up' : (s ? 'down' : 'unknown');
      }

      const isDown = status === 'down';

      return {
        ...n,
        status, metrics,
        symbolSize: n.group === 'source' ? 46 : 48,
        itemStyle: {
          color: isDown ? '#334155' : GROUP_COLOR[n.group] || '#64748b',
          borderColor: isDown ? '#ef4444' : '#22c55e',
          borderWidth: 2.5,
          opacity: isDown ? 0.55 : 1,
          shadowBlur: !isDown ? 12 : 0,
          shadowColor: !isDown ? (GROUP_COLOR[n.group]||'#22c55e')+'40' : 'transparent',
        },
        label: {
          show: true,
          formatter: n.name + (isDown ? '\n[DOWN]' : ''),
          fontSize: 11,
          fontWeight: isDown ? 700 : 500,
          color: isDown ? '#ef4444' : '#e2e8f0',
          backgroundColor: isDown ? 'rgba(239,68,68,0.18)' : 'rgba(15,23,42,0.88)',
          padding: [4,7],
          borderRadius: 5,
          lineHeight: 15,
        },
      };
    });
  };

  const buildEdges = () => {
    if (!linkData?.links) return [];

    const lm = {};
    Object.entries(linkData.links).forEach(([k,v]) => { lm[k]=v; });

    const defs = [
      { k:'probe_ingest',s:'probe',t:'ingest' },
      { k:'ingest_redis',s:'ingest',t:'redis' },
      { k:'redis_clickhouse',s:'redis',t:'clickhouse' },
      { k:'clickhouse_nginx',s:'clickhouse',t:'nginx' },
      { k:'nginx_ai',s:'nginx',t:'ai' }, { k:'nginx_control',s:'nginx',t:'control' },
      { k:'nginx_alert',s:'nginx',t:'alert' }, { k:'nginx_edge',s:'nginx',t:'edge' },
      { k:'nginx_sysstats',s:'nginx',t:'sysstats' }, { k:'nginx_config',s:'nginx',t:'config' },
      { k:'nginx_log',s:'nginx',t:'log' }, { k:'nginx_link',s:'nginx',t:'link' },
      { k:'nginx_edge_health',s:'nginx',t:'edge_health' }, { k:'nginx_cluster',s:'nginx',t:'cluster' },
      { k:'nginx_frontend',s:'nginx',t:'frontend' },
    ];

    return defs.map(d => {
      const ld = lm[d.k];
      const lat = ld?.latency_ms || 0;
      const rps = ld?.req_per_sec || 0;
      const bps = ld?.bytes_per_sec || 0;
      const ep = ld?.error_pct || 0;
      const st = ld?.status || 'unknown';
      const dn = st==='down'||st==='unknown';
      const sl=!dn&&lat>500;

      let lc='#22c55e'; if(dn)lc='#475569'; else if(sl)lc='#f59e0b';

      let lt=''; if(dn){lt='✕';}else if(lat>0){
        lt=[lat.toFixed(lat<10?1:0)+'ms',(rps>0.1?rps.toFixed(1)+'/s':'')].filter(Boolean).join(' ');
      }

      return {
        source:d.s, target:d.t,
        lineStyle:{color:lc,width:dn?1:(sl?2.5:2),curveness:0,type:dn?'dashed':'solid',opacity:dn?0.4:0.8},
        label:{show:!!lt,formatter:lt,fontSize:9,color:dn?'#ef4444':'#86efac',
               backgroundColor:dn?'rgba(239,68,68,0.12)':'rgba(34,197,94,0.08)',
               padding:[1,4],borderRadius:3,position:'middle'},
        status:st, latency:lat, reqPerSec:rps, bytesPerSec:bps, errorPct:ep,
      };
    });
  };

  const loadData = useCallback(async () => {
    try {
      const [lr,sr] = await Promise.all([
        fetch(LINK_API).then(r=>r.json()).catch(()=>null),
        fetchServiceHealth().catch(()=>[]),
      ]);
      setLinkData(lr); setServices(Array.isArray(sr)?sr:[]);
      setLastUpdate(new Date().toLocaleTimeString());
    } catch(e) { console.error(e); }
    finally{ setLoading(false); }
  }, []);

  useEffect(()=>{loadData();const iv=setInterval(loadData,15000);return()=>clearInterval(iv);},[loadData]);

  const [ch,setCh]=useState(700);
  useEffect(()=>{
    const f=()=>setCh(Math.max(window.innerHeight-180,550));
    f();window.addEventListener('resize',f);return()=>window.removeEventListener('resize',f);
  },[]);

  useEffect(()=>{
    if(!chartRef.current)return;
    if(!chartInst.current) chartInst.current=echarts.init(chartRef.current,null,{renderer:'canvas'});
    const nodes=buildNodes(), edges=buildEdges();

    chartInst.current.setOption({
      tooltip:{trigger:'item',confine:true,enterable:false,
        extraCssText:'box-shadow:0 8px 32px rgba(0,0,0,.35);border-radius:8px;max-width:300px;',
        textStyle:{fontSize:13},backgroundColor:'rgba(15,23,42,.96)',
        borderColor:'rgba(100,116,139,.3)',borderWidth:1,padding:[12,16],
        formatter:p=>{
          if(p.dataType==='node'){
            const n=p.data;let h='<div style="margin-bottom:6px"><span style="font-size:15px;font-weight:bold;color:#f1f5f9">'+n.name+'</span></div>';
            const sc=n.status==='up'?'#22c55e':(n.status==='down'?'#ef4444':'#f59e0b');
            const st=n.status==='up'?'● 运行中':(n.status==='down'?'● 异常':'● 未知');
            h+='<div style="display:flex;align-items:center;gap:8px;margin-bottom:4px"><span style="color:#94a3b8;font-size:12px">状态</span><span style="color:'+sc+';font-weight:600;font-size:13px">'+st+'</span></div>';
            h+='<div style="display:flex;align-items:center;gap:8px;margin-bottom:4px"><span style="color:#94a3b8;font-size:12px">组别</span><span style="color:#cbd5e1;font-size:13px">'+(GROUP_LABEL[n.group]||n.group)+'</span></div>';
            if(n.metrics&&Object.keys(n.metrics).length>0){
              h+='<div style="margin-top:8px;padding-top:7px;border-top:1px solid rgba(100,116,139,.18)">';
              const L={ops:'OPS',qps:'QPS',rps:'RPS'};
              Object.entries(n.metrics).forEach(([k,v])=>{
                h+='<div style="display:flex;justify-content:space-between;gap:16px;margin-bottom:2px">';
                h+='<span style="color:#94a3b8;font-size:12px">'+(L[k]||k)+'</span>';
                h+='<span style="color:#38bdf8;font-weight:600;font-size:12px">'+v+'</span></div>';});
              h+='</div>';}
            return h;
          }
          if(p.dataType==='edge'){
            const e=p.data,sn=NODES_TEMPLATE.find(t=>t.id===e.source)?.name||e.source,tn=NODES_TEMPLATE.find(t=>t.id===e.target)?.name||e.target;
            let h='<div style="margin-bottom:7px"><span style="font-size:13px;color:#94a3b8">链路</span> <span style="font-size:14px;font-weight:bold;color:#f1f5f9">'+sn+' → '+tn+'</span></div>';
            const sc=(e.status==='up'||e.status==='active')?'#22c55e':(e.status==='down'?'#ef4444':'#94a3b8');
            const st=(e.status==='up'||e.status==='active')?'正常':(e.status==='down'?'断开':e.status||'-');
            h+='<div style="display:flex;align-items:center;gap:8px;margin-bottom:6px"><span style="color:#94a3b8;font-size:12px">状态</span><span style="color:'+sc+';font-weight:600;font-size:13px">● '+st+'</span></div>';
            const hm=e.latency>0||e.reqPerSec>0||e.bytesPerSec>0||e.errorPct>0;
            if(hm){h+='<table style="width:100%;border-collapse:collapse;margin-top:4px;font-size:12px">';
              if(e.latency>0){const c=e.latency>500?'#f59e0b':(e.latency>200?'#fbbf24':'#22c55e');h+='<tr><td style="color:#94a3b8;padding:3px 0;border-bottom:1px solid rgba(100,116,139,.13)">延迟</td><td style="text-align:right;color:'+c+';font-weight:600;padding:3px 0;border-bottom:1px solid rgba(100,116,139,.13)">'+e.latency.toFixed(1)+' ms</td></tr>';}
              if(e.reqPerSec>0)h+='<tr><td style="color:#94a3b8;padding:3px 0;border-bottom:1px solid rgba(100,116,139,.13)">请求速率</td><td style="text-align:right;color:#38bdf8;font-weight:600;padding:3px 0;border-bottom:1px solid rgba(100,116,139,.13)">'+e.reqPerSec.toFixed(2)+' req/s</td></tr>';
              if(e.bytesPerSec>0)h+='<tr><td style="color:#94a3b8;padding:3px 0;border-bottom:1px solid rgba(100,116,139,.13)">流量</td><td style="text-align:right;color:#a78bfa;font-weight:600;padding:3px 0;border-bottom:1px solid rgba(100,116,139,.13)">'+fmtBytes(e.bytesPerSec)+'</td></tr>';
              if(e.errorPct>0)h+='<tr><td style="color:#94a3b8;padding:3px 0">错误率</td><td style="text-align:right;color:#ef4444;font-weight:600;padding:3px 0">'+e.errorPct.toFixed(1)+'%</td></tr>';
              h+='</table>';}
            return h;
          }
          return '';
        }},
      animationDurationUpdate:500,
      series:[{
        type:'graph',layout:'none',data:nodes,edges:edges,
        roam:true,zoom:1,center:['45%','50%'],
        edgeSymbol:['none','arrow'],edgeSymbolSize:[0,7],
        label:{show:false},lineStyle:{width:2,curveness:0},
        emphasis:{focus:'adjacency',lineStyle:{width:3.5,color:'#38bdf8'},
          itemStyle:{borderColor:'#38bdf8',borderWidth:3,shadowBlur:18,shadowColor:'rgba(56,189,248,.45)'}},
      }],
    },true);

    const hr=()=>chartInst.current?.resize();
    window.addEventListener('resize',hr);
    return()=>window.removeEventListener('resize',hr);
  },[linkData,services,ch]);

  return (
    <div style={{height:'100%'}}>
      <div style={{display:'flex',justifyContent:'space-between',alignItems:'center',marginBottom:10}}>
        <h2 style={{margin:0}}>服务拓扑</h2>
        <div style={{display:'flex',gap:16,alignItems:'center'}}>
          {lastUpdate&&<span style={{color:'#64748b',fontSize:13}}>更新于 {lastUpdate}</span>}
          <button onClick={loadData} className="btn-refresh">↻ 刷新</button>
        </div>
      </div>

      <div className="card" style={{marginBottom:10,padding:'8px 16px',display:'flex',gap:20,alignItems:'center',flexWrap:'wrap'}}>
        <span style={{fontSize:12,color:'#94a3b8'}}>图例：</span>
        {[{l:'正常',c:'#22c55e'},{l:'断开/异常',c:'#ef4444'},{l:'慢链路>500ms',c:'#f59e0b'}].map((it,i)=>(
          <div key={i} style={{display:'flex',alignItems:'center',gap:5,fontSize:12}}>
            <span style={{display:'inline-block',width:18,height:3,backgroundColor:it.c,borderRadius:2}}/>
            <span style={{color:'#cbd5e1'}}>{it.l}</span>
          </div>
        ))}
        <span style={{marginLeft:'auto',fontSize:11,color:'#475569'}}>💡 鼠标悬停查看详情</span>
      </div>

      <div className="card" style={{padding:0,overflow:'hidden'}}>
        {loading?<div className="empty-state" style={{height:ch}}>加载中...</div>:<div ref={chartRef} style={{width:'100%',height:ch}}/>}
      </div>
    </div>
  );
}
