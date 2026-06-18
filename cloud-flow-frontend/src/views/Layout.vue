<template>
  <el-container class="layout">
    <el-aside width="200px" class="sidebar">
      <div class="logo">
        <el-icon size="28" color="#00CCFF"><Connection /></el-icon>
        <span class="logo-text">CloudFlow</span>
      </div>
      <el-menu
        :default-active="$route.path"
        router
        class="el-menu-vertical"
        background-color="transparent"
        text-color="#A4A8AE"
        active-text-color="#00CCFF"
      >
        <el-menu-item index="/dashboard">
          <el-icon><Monitor /></el-icon>
          <span>态势概览</span>
        </el-menu-item>
        <el-sub-menu index="/monitor">
          <template #title>
            <el-icon><Odometer /></el-icon>
            <span>运行监控</span>
          </template>
          <el-menu-item index="/alerts">监控与异常告警</el-menu-item>
          <el-menu-item index="/alert-config">告警配置</el-menu-item>
        </el-sub-menu>
        <el-menu-item index="/performance">
          <el-icon><Timer /></el-icon>
          <span>资源指标分析</span>
        </el-menu-item>
        <el-menu-item index="/logs">
          <el-icon><Document /></el-icon>
          <span>流量回溯与日志</span>
        </el-menu-item>
        <el-sub-menu index="/probes">
          <template #title>
            <el-icon><Cpu /></el-icon>
            <span>探针管理与采集</span>
          </template>
          <el-menu-item index="/probes">探针管理与采集</el-menu-item>
          <el-menu-item index="/probe-cluster">探针集群管理</el-menu-item>
          <el-menu-item index="/probe-version">探针版本管理</el-menu-item>
        </el-sub-menu>
        <el-sub-menu index="/system">
          <template #title>
            <el-icon><Setting /></el-icon>
            <span>系统管理</span>
          </template>
          <el-menu-item index="/settings">系统状态</el-menu-item>
          <el-menu-item index="/users">用户管理</el-menu-item>
        </el-sub-menu>
        <el-sub-menu index="/trace">
          <template #title>
            <el-icon><MapLocation /></el-icon>
            <span>跟踪分析</span>
          </template>
          <el-menu-item index="/trace-path">访问路径跟踪</el-menu-item>
          <el-menu-item index="/trace-e2e">端到端跟踪</el-menu-item>
          <el-menu-item index="/trace-dubbo">Dubbo跟踪</el-menu-item>
          <el-menu-item index="/trace-dns">DNS跟踪</el-menu-item>
          <el-menu-item index="/trace-connectivity">节点间连通性跟踪</el-menu-item>
        </el-sub-menu>
        <el-sub-menu index="/topology">
          <template #title>
            <el-icon><Share /></el-icon>
            <span>网络拓扑</span>
          </template>
          <el-menu-item index="/topology">业务拓扑</el-menu-item>
          <el-menu-item index="/topology-container">容器拓扑</el-menu-item>
          <el-menu-item index="/topology-ip">IP 拓扑</el-menu-item>
        </el-sub-menu>
      </el-menu>
    </el-aside>
    <el-container>
      <el-header class="header">
        <div class="header-left">
          <span class="page-title">云流量检测态势概览</span>
          <span class="page-subtitle">云内流量检测与分析工具</span>
        </div>
        <div class="header-center">
          <span class="date-text">{{ currentDate }}</span>
        </div>
        <div class="header-right">
          <span class="user-role">系统管理员</span>
          <span class="user-name">admin</span>
          <el-dropdown>
            <span class="user-info">
              <el-icon><User /></el-icon>
              <el-icon><ArrowDown /></el-icon>
            </span>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item @click="handleLogout">退出登录</el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </div>
      </el-header>
      <el-main class="main">
        <router-view />
      </el-main>
    </el-container>
  </el-container>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { useUserStore } from '@/stores/user'
import {
  Connection, Monitor, Cpu, Share, Odometer, Timer,
  Document, MapLocation, Setting,
  User, ArrowDown
} from '@element-plus/icons-vue'

const router = useRouter()
const userStore = useUserStore()
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

const handleLogout = () => {
  userStore.logout()
  router.push('/login')
}
</script>

<style scoped lang="scss">
.layout {
  height: 100vh;
  background: #05385A;
  .sidebar {
    background: #001529;
    border-right: 1px solid rgba(10, 186, 255, 0.2);
    .logo {
      height: 61px;
      display: flex;
      align-items: center;
      padding: 0 20px;
      gap: 12px;
      border-bottom: 1px solid rgba(10, 186, 255, 0.2);
      .logo-text {
        font-size: 20px;
        font-weight: 600;
        color: #00CCFF;
        font-family: '微软雅黑', sans-serif;
      }
    }
    .el-menu-vertical {
      border-right: none;
      background: transparent;
      :deep(.el-menu-item) {
        color: #A4A8AE;
        &:hover {
          background: rgba(10, 186, 255, 0.1);
          color: #00CCFF;
        }
        &.is-active {
          background: rgba(10, 186, 255, 0.15);
          color: #00CCFF;
          border-right: 3px solid #00CCFF;
        }
      }
      :deep(.el-sub-menu__title) {
        color: #A4A8AE;
        &:hover {
          background: rgba(10, 186, 255, 0.1);
          color: #00CCFF;
        }
      }
    }
  }
  .header {
    height: 61px;
    background: rgba(0, 160, 150, 0.925);
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0 24px;
    border-bottom: 1px solid rgba(10, 186, 255, 0.3);
    .header-left {
      display: flex;
      align-items: center;
      gap: 16px;
      .page-title {
        font-size: 20px;
        font-weight: 400;
        color: #FFFFFF;
        font-family: '微软雅黑', sans-serif;
        text-shadow: 0 0 10px rgba(7, 213, 192, 1);
      }
      .page-subtitle {
        font-size: 32px;
        font-weight: 400;
        color: #FFFFFF;
        font-family: '微软雅黑', sans-serif;
      }
    }
    .header-center {
      .date-text {
        font-size: 16px;
        color: #FFFFFF;
        font-family: 'Arial', sans-serif;
      }
    }
    .header-right {
      display: flex;
      align-items: center;
      gap: 12px;
      .user-role {
        font-size: 14px;
        color: #FFFFFF;
        font-family: '微软雅黑', sans-serif;
      }
      .user-name {
        font-size: 14px;
        color: #FFFFFF;
        font-family: 'Arial', sans-serif;
      }
      .user-info {
        display: flex;
        align-items: center;
        gap: 6px;
        cursor: pointer;
        font-size: 14px;
        color: #FFFFFF;
      }
    }
  }
  .main {
    padding: 20px 24px;
    background: #05385A;
    overflow-y: auto;
  }
}
</style>
