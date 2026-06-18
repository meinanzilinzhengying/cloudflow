# CloudFlow Web - eBPF 网络可观测平台前端

> Vue 3 + TypeScript + Vite + Element Plus + ECharts

---

## 快速开始

```bash
# 安装依赖
npm install

# 开发模式（端口8080，代理到后端9090）
npm run dev

# 生产构建
npm run build

# 预览构建产物
npm run preview
```

---

## 技术栈

| 技术 | 版本 | 用途 |
|------|------|------|
| Vue 3 | ^3.4 | 核心框架 |
| TypeScript | ^5.4 | 类型安全 |
| Vite | ^5.2 | 构建工具 |
| Element Plus | ^2.7 | UI 组件库 |
| Pinia | ^2.1 | 状态管理 |
| Vue Router | ^4.2 | 路由 |
| Axios | ^1.7 | HTTP 请求 |
| ECharts | ^5.4 | 数据可视化 |
| vue-echarts | ^6.6 | ECharts Vue 封装 |

---

## 页面清单

| 页面 | 路由 | 说明 |
|------|------|------|
| 登录 | `/login` | 深蓝渐变背景，品牌展示 + 表单 |
| 总览仪表盘 | `/dashboard` | 4指标卡片 + 流量趋势 + 协议分布 + TOP主机 + 告警 |
| 探针管理 | `/probes` | 探针表格 + 详情弹窗(5Tab) |
| 网络流量 | `/network` | 流量趋势 + 通信矩阵 + 拓扑 + 流日志 |
| L7协议 | `/protocol` | HTTP/DNS Tab + 指标卡片 + 日志表格 |
| 系统性能 | `/performance` | CPU/内存/IO/进程 2×2网格 |
| 安全审计 | `/security` | 事件时间线 + 详情面板 |

---

## 目录结构

```
cloudflow-web/
├── src/
│   ├── api/          # API 接口封装
│   ├── components/   # 公共组件
│   ├── views/        # 页面组件
│   ├── router/       # 路由配置
│   ├── stores/       # Pinia 状态
│   ├── utils/        # 工具函数
│   ├── styles/       # 全局样式
│   ├── App.vue
│   └── main.ts
├── index.html
├── package.json
├── vite.config.ts
├── tsconfig.json
└── README.md
```

---

## 部署

### 独立部署（Nginx）

```bash
npm run build
# 将 dist/ 目录复制到 nginx 的 html 目录
cp -r dist/* /usr/share/nginx/html/
```

### 嵌入 Go 后端（go:embed）

```go
//go:embed dist
var webFS embed.FS

func serveWeb() {
    sub, _ := fs.Sub(webFS, "dist")
    http.Handle("/", http.FileServer(http.FS(sub)))
    http.ListenAndServe(":8080", nil)
}
```

---

## 后端 API 对接

所有接口前缀 `/api/v1`，代理到 `http://localhost:9090`（开发模式）。

核心接口：
- `POST /api/v1/auth/login` - 登录
- `GET /api/v1/probes` - 探针列表
- `GET /api/v1/dashboard/overview` - 仪表盘数据
- `GET /api/v1/network/flows` - 网络流
- `GET /api/v1/protocol/http` - HTTP 日志
- `GET /api/v1/protocol/dns` - DNS 日志
- `GET /api/v1/performance/:host` - 性能数据
- `GET /api/v1/security/events` - 安全事件

---

## 设计规范

- 主色: `#165DFF`
- 成功: `#00B42A`, 警告: `#FF7D00`, 危险: `#F53F3F`
- 卡片圆角: `8px`, 阴影: `0 2px 8px rgba(0,0,0,0.08)`
- 页面边距: `20px 24px`
- 字体: 标题 `18px 600`, 卡片标题 `16px 600`, 正文 `14px 400`

---

## 响应式适配

- 1920×1080: 完整布局
- 1366×768: 表格横向滚动，图表自适应

---

## 许可证

MIT © 2025 CloudFlow Team
