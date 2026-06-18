import { createRouter, createWebHashHistory } from "vue-router"

const routes = [
  { path: "/login", name: "Login", component: () => import("@/views/Login.vue"), meta: { public: true } },
  { path: "/", name: "Layout", component: () => import("@/views/Layout.vue"), redirect: "/dashboard", children: [
    { path: "/dashboard", name: "Dashboard", component: () => import("@/views/Dashboard.vue") },
    { path: "/probes", name: "Probes", component: () => import("@/views/Probes.vue") },
    { path: "/network", name: "Network", component: () => import("@/views/Network.vue") },
    { path: "/protocol", name: "Protocol", component: () => import("@/views/Protocol.vue") },
    { path: "/performance", name: "Performance", component: () => import("@/views/Performance.vue") },
    { path: "/logs", name: "Logs", component: () => import("@/views/Logs.vue") },
    { path: "/topology", name: "Topology", component: () => import("@/views/Topology.vue") },
    { path: "/alerts", name: "Alerts", component: () => import("@/views/Alerts.vue") },
    { path: "/settings", name: "Settings", component: () => import("@/views/Settings.vue") },
  ]},
]

const router = createRouter({
  history: createWebHashHistory(),
  routes,
})

router.beforeEach((_to, _from, next) => {
  next()
})

export default router
