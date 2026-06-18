import { createRouter, createWebHashHistory } from 'vue-router'
import { useUserStore } from '@/stores/user'

const routes = [
  { path: '/login', name: 'Login', component: () => import('@/views/Login.vue'), meta: { public: true } },
  { path: '/', name: 'Layout', component: () => import('@/views/Layout.vue'), redirect: '/dashboard', children: [
    { path: '/dashboard', name: 'Dashboard', component: () => import('@/views/Dashboard.vue') },
    { path: '/probes', name: 'Probes', component: () => import('@/views/Probes.vue') },
    { path: '/network', name: 'Network', component: () => import('@/views/Network.vue') },
    { path: '/protocol', name: 'Protocol', component: () => import('@/views/Protocol.vue') },
    { path: '/performance', name: 'Performance', component: () => import('@/views/Performance.vue') },
    { path: '/security', name: 'Security', component: () => import('@/views/Security.vue') },
  ]},
]

const router = createRouter({
  history: createWebHashHistory(),
  routes,
})

router.beforeEach((to, _from, next) => {
  const userStore = useUserStore()
  if (!to.meta.public && !userStore.token) {
    next('/login')
  } else {
    next()
  }
})

export default router
