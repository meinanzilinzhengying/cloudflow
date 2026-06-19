<template>
  <div class="app-layout">
    <!-- 左侧导航栏 -->
    <aside class="sidebar" :style="{ width: sidebarWidth + 'px' }">
      <div class="logo">
        <el-icon size="28" color="#00CCFF"><Connection /></el-icon>
        <span class="logo-text">CloudFlow</span>
      </div>
      <nav class="nav-menu">
        <div
          v-for="item in menuItems"
          :key="item.path"
          class="nav-item"
          :class="{ active: isActive(item.path), expanded: item.expanded }"
          @click="handleMenuClick(item)"
        >
          <div class="nav-item-main">
            <el-icon :size="16" class="nav-icon"><component :is="item.icon" /></el-icon>
            <span class="nav-text">{{ item.name }}</span>
            <el-icon v-if="item.children" size="12" class="nav-arrow" :class="{ rotated: item.expanded }"><ArrowRight /></el-icon>
          </div>
          <div v-if="item.children && item.expanded" class="nav-sub">
            <div
              v-for="sub in item.children"
              :key="sub.path"
              class="nav-sub-item"
              :class="{ active: isActive(sub.path) }"
              @click.stop="navigate(sub.path)"
            >
              {{ sub.name }}
            </div>
          </div>
        </div>
      </nav>
    </aside>

    <!-- 主内容区 -->
    <main class="main-content" :style="{ marginLeft: sidebarWidth + 'px' }">
      <!-- 顶部标题栏 -->
      <header class="top-header" :style="headerBgStyle">
        <div class="header-left">
          <span class="date-text">{{ currentDate }}</span>
        </div>
        <div class="header-center">
          <span class="page-title">云流量检测态势概览</span>
        </div>
        <div class="header-right">
          <el-icon size="20" class="header-icon"><Bell /></el-icon>
          <div class="user-avatar">A</div>
          <div class="user-info">
            <span class="user-role">系统管理员</span>
            <span class="user-name">admin</span>
          </div>
        </div>
      </header>

      <!-- 路由内容 -->
      <div class="content-area">
        <router-view />
      </div>
    </main>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import {
  Connection, Monitor, Odometer, Timer, Document,
  Cpu, Setting, MapLocation, Share, ArrowRight, Bell
} from '@element-plus/icons-vue'

const router = useRouter()
const route = useRoute()
const sidebarWidth = 200

const currentDate = ref('')

const updateDate = () => {
  const now = new Date()
  const year = now.getFullYear()
  const month = String(now.getMonth() + 1).padStart(2, '0')
  const day = String(now.getDate()).padStart(2, '0')
  const weekdays = ['星期日', '星期一', '星期二', '星期三', '星期四', '星期五', '星期六']
  const weekday = weekdays[now.getDay()]
  currentDate.value = `${year}年${month}月${day}日 ${weekday}`
}

onMounted(() => {
  updateDate()
  setInterval(updateDate, 60000)
})

interface MenuItem {
  name: string
  path: string
  icon: any
  children?: { name: string; path: string }[]
  expanded?: boolean
}

const menuItems = ref<MenuItem[]>([
  { name: '态势概览', path: '/dashboard', icon: Monitor },
  { name: '网络拓扑', path: '/topology', icon: Share, children: [
    { name: '业务拓扑', path: '/topology' },
    { name: '容器拓扑', path: '/topology-container' },
    { name: 'IP 拓扑', path: '/topology-ip' },
  ]},
  { name: '运行监控', path: '/monitor', icon: Odometer, children: [
    { name: '监控与异常告警', path: '/alerts' },
    { name: '告警配置', path: '/alert-config' },
  ]},
  { name: '跟踪分析', path: '/trace', icon: MapLocation, children: [
    { name: '访问路径跟踪', path: '/trace-path' },
    { name: '端到端跟踪', path: '/trace-e2e' },
    { name: 'Dubbo跟踪', path: '/trace-dubbo' },
    { name: 'DNS跟踪', path: '/trace-dns' },
    { name: '节点间连通性跟踪', path: '/trace-connectivity' },
  ]},
  { name: '资源指标分析', path: '/performance', icon: Timer },
  { name: '流量回溯与日志', path: '/logs', icon: Document },
  { name: '探针管理与采集', path: '/probes', icon: Cpu, children: [
    { name: '探针集群管理', path: '/probe-cluster' },
    { name: '探针管理与采集', path: '/probes' },
    { name: '探针版本管理', path: '/probe-version' },
  ]},
  { name: '系统管理', path: '/system', icon: Setting, children: [
    { name: '系统状态', path: '/settings' },
    { name: '用户管理', path: '/users' },
  ]},
])

const isActive = (path: string) => route.path === path || route.path.startsWith(path + '/')

const handleMenuClick = (item: any) => {
  if (item.children) {
    item.expanded = !item.expanded
  } else {
    navigate(item.path)
  }
}

const navigate = (path: string) => {
  router.push(path)
}

const headerBgStyle = computed(() => {
  return {
    background: 'linear-gradient(90deg, rgba(20, 40, 80, 0.95) 0%, rgba(30, 80, 100, 0.9) 50%, rgba(20, 40, 80, 0.95) 100%)',
  }
})
</script>

<style scoped lang="scss">
.app-layout {
  min-height: 100vh;
  display: flex;
  .sidebar {
    position: fixed;
    left: 0;
    top: 0;
    height: 100vh;
    background: rgba(38, 36, 68, 1);
    z-index: 100;
    display: flex;
    flex-direction: column;
    .logo {
      height: 61px;
      display: flex;
      align-items: center;
      padding: 0 20px;
      gap: 12px;
      border-bottom: 1px solid rgba(255, 255, 255, 0.1);
      .logo-text {
        font-size: 20px;
        font-weight: 600;
        color: #00CCFF;
        font-family: '微软雅黑', sans-serif;
      }
    }
    .nav-menu {
      flex: 1;
      overflow-y: auto;
      padding: 8px 0;
      .nav-item {
        cursor: pointer;
        .nav-item-main {
          display: flex;
          align-items: center;
          padding: 0 16px;
          height: 40px;
          gap: 12px;
          color: rgba(255, 255, 255, 0.996);
          font-size: 14px;
          font-family: '微软雅黑', sans-serif;
          transition: all 0.2s;
          .nav-icon {
            flex-shrink: 0;
          }
          .nav-text {
            flex: 1;
          }
          .nav-arrow {
            transition: transform 0.2s;
            &.rotated {
              transform: rotate(90deg);
            }
          }
        }
        &:hover .nav-item-main {
          background: rgba(255, 255, 255, 0.05);
        }
        &.active > .nav-item-main {
          background: rgba(24, 135, 238, 1);
          color: #FFFFFF;
        }
        .nav-sub {
          .nav-sub-item {
            padding: 0 16px 0 48px;
            height: 36px;
            display: flex;
            align-items: center;
            color: rgba(255, 255, 255, 0.8);
            font-size: 13px;
            font-family: '微软雅黑', sans-serif;
            cursor: pointer;
            transition: all 0.2s;
            &:hover {
              background: rgba(255, 255, 255, 0.05);
              color: #FFFFFF;
            }
            &.active {
              background: rgba(14, 154, 141, 1);
              color: #FFFFFF;
            }
          }
        }
      }
    }
  }
  .main-content {
    flex: 1;
    min-height: 100vh;
    .top-header {
      height: 66px;
      display: flex;
      align-items: center;
      justify-content: space-between;
      padding: 0 24px;
      border-bottom: 1px solid rgba(10, 186, 255, 0.2);
      .header-left {
        .date-text {
          font-size: 16px;
          color: #FFFFFF;
          font-family: 'Arial', sans-serif;
        }
      }
      .header-center {
        .page-title {
          font-size: 28px;
          font-weight: 400;
          color: #FFFFFF;
          font-family: '微软雅黑', sans-serif;
        }
      }
      .header-right {
        display: flex;
        align-items: center;
        gap: 16px;
        .header-icon {
          color: #FFFFFF;
          cursor: pointer;
        }
        .user-avatar {
          width: 27px;
          height: 27px;
          border-radius: 50%;
          background: rgba(10, 186, 255, 0.3);
          display: flex;
          align-items: center;
          justify-content: center;
          color: #FFFFFF;
          font-size: 12px;
        }
        .user-info {
          display: flex;
          flex-direction: column;
          align-items: flex-start;
          .user-role {
            font-size: 14px;
            color: #FFFFFF;
            font-family: '微软雅黑', sans-serif;
          }
          .user-name {
            font-size: 12px;
            color: #A4A8AE;
            font-family: 'Arial', sans-serif;
          }
        }
      }
    }
    .content-area {
      min-height: calc(100vh - 66px);
      background: #05385A;
      padding: 0;
    }
  }
}
</style>
