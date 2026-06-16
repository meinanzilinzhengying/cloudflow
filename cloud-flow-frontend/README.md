# CloudFlow 前端

CloudFlow 云原生网络流量分析平台前端

## 技术栈

- Vue 3.4.21
- Vite 5.2.8
- TailwindCSS 3.4.3
- ECharts 5.5.0
- Axios 1.6.8
- Element Plus

## 开发环境

```bash
# 安装依赖
npm install

# 启动开发服务器
npm run dev
```

## 生产环境部署

```bash
# 安装依赖
npm install

# 构建生产版本
npm run build

# 部署到nginx
cp -r dist/* /usr/share/nginx/html/
```

## Nginx 反向代理配置

```nginx
server {
    listen 80;
    server_name cloudflow.example.com;
    root /usr/share/nginx/html;
    index index.html;

    # 前端静态资源
    location / {
        try_files $uri $uri/ /index.html;
    }

    # API 反向代理
    location /api/ {
        proxy_pass http://cloud-flow-center:8080/;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    # WebSocket 支持
    location /ws/ {
        proxy_pass http://cloud-flow-center:8080/ws/;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
    }
}
```

## 环境变量配置

复制 `.env.example` 为 `.env` 并根据实际环境修改：

```bash
cp .env.example .env
```

主要配置项：

| 变量名 | 说明 | 默认值 |
|--------|------|--------|
| VITE_API_BASE_URL | API 基础路径 | /api |
| VITE_APP_TITLE | 应用标题 | CloudFlow |
| VITE_APP_VERSION | 应用版本 | 1.0.0 |
| VITE_PROMETHEUS_URL | Prometheus 地址 | http://prometheus:9090 |
| VITE_GRAFANA_URL | Grafana 地址 | http://grafana:3000 |

## 项目结构

```
cloud-flow-frontend/
├── src/
│   ├── api/              # API 接口封装
│   │   ├── index.js      # 服务导出
│   │   └── request.js    # Axios 封装
│   ├── components/       # 组件
│   │   ├── common/       # 通用组件
│   │   └── pages/        # 页面组件
│   ├── router/           # 路由
│   ├── store/            # 状态管理
│   ├── utils/            # 工具函数
│   ├── App.vue
│   └── main.js
├── public/               # 静态资源
├── .env                  # 环境变量
├── .env.example          # 环境变量示例
├── package.json
├── vite.config.js
└── README.md
```

## 主要功能页面

1. **Dashboard** - 运营驾驶舱
   - 流量总览
   - 探针状态
   - 告警统计
   - 系统健康度

2. **Traffic** - 流量分析
   - 流量概览
   - 会话分析
   - TopN 排行
   - 流量地图
   - 流量回放

3. **Topology** - 网络拓扑
   - 服务拓扑
   - Pod 拓扑
   - 进程拓扑
   - 命名空间拓扑
   - 拓扑对比

4. **Tracing** - 链路追踪
   - 链路概览
   - 慢请求分析
   - 错误分析
   - 调用链详情

5. **Metrics** - 指标监控
   - 主机指标
   - 容器指标
   - 服务指标
   - 自定义指标

6. **Logs** - 日志分析
   - 日志搜索
   - 日志聚合
   - 日志关联

7. **Alerts** - 告警中心
   - 告警事件
   - 告警规则
   - 通知配置
   - 告警统计

8. **RCA** - 根因分析
   - 异常分析
   - 关联分析
   - 时间线

9. **Management** - 管理页面
   - 探针管理
   - 用户管理
   - 租户管理
   - API Key 管理
   - 系统设置

## API 服务列表

- `authService` - 认证服务
- `tenantService` - 租户服务
- `controlPlaneService` - 控制平面服务
- `queryService` - 查询服务
- `alertService` - 告警服务
- `dataPlaneService` - 数据平面服务

## 常见问题

### 1. API 请求跨域问题

开发环境可在 `vite.config.js` 中配置代理：

```javascript
export default defineConfig({
  server: {
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
    },
  },
})
```

### 2. 探针状态不更新

检查控制平面 API 是否正常运行，确认 `/api/control-plane/agents/status` 接口可访问。

### 3. 图表数据不显示

检查查询服务 API 是否正常，确认 `/api/query/flows` 接口返回数据格式正确。
