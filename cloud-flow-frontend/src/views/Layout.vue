<template>
  <el-container class="layout">
    <el-aside width="220px" class="sidebar">
      <div class="logo">
        <el-icon size="28" color="#165DFF"><Connection /></el-icon>
        <span class="logo-text">CloudFlow</span>
      </div>
      <el-menu
        :default-active="$route.path"
        router
        class="el-menu-vertical"
        background-color="#001529"
        text-color="#b3b9c4"
        active-text-color="#165DFF"
      >
        <el-menu-item index="/dashboard">
          <el-icon><Monitor /></el-icon>
          <span>总览仪表盘</span>
        </el-menu-item>
        <el-menu-item index="/probes">
          <el-icon><Cpu /></el-icon>
          <span>探针管理</span>
        </el-menu-item>
        <el-menu-item index="/network">
          <el-icon><Share /></el-icon>
          <span>网络流量</span>
        </el-menu-item>
        <el-menu-item index="/protocol">
          <el-icon><DataAnalysis /></el-icon>
          <span>L7协议分析</span>
        </el-menu-item>
        <el-menu-item index="/performance">
          <el-icon><Timer /></el-icon>
          <span>系统性能</span>
        </el-menu-item>
        <el-menu-item index="/security">
          <el-icon><Warning /></el-icon>
          <span>安全审计</span>
        </el-menu-item>
      </el-menu>
    </el-aside>
    <el-container>
      <el-header class="header">
        <div class="header-left">
          <el-icon size="20"><Fold /></el-icon>
          <span class="breadcrumb">CloudFlow eBPF 可观测平台</span>
        </div>
        <div class="header-right">
          <el-dropdown>
            <span class="user-info">
              <el-icon><User /></el-icon>
              {{ userStore.username }}
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
import { useRouter } from 'vue-router'
import { useUserStore } from '@/stores/user'
import {
  Connection, Monitor, Cpu, Share, DataAnalysis, Timer, Warning,
  Fold, User, ArrowDown
} from '@element-plus/icons-vue'

const router = useRouter()
const userStore = useUserStore()

const handleLogout = () => {
  userStore.logout()
  router.push('/login')
}
</script>

<style scoped lang="scss">
.layout {
  height: 100vh;
  .sidebar {
    background: #001529;
    .logo {
      height: 64px;
      display: flex;
      align-items: center;
      padding: 0 20px;
      gap: 12px;
      border-bottom: 1px solid rgba(255,255,255,0.1);
      .logo-text {
        font-size: 20px;
        font-weight: 600;
        color: #fff;
      }
    }
    .el-menu-vertical {
      border-right: none;
    }
  }
  .header {
    height: 64px;
    background: #fff;
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0 24px;
    box-shadow: 0 1px 4px rgba(0,0,0,0.08);
    .header-left {
      display: flex;
      align-items: center;
      gap: 12px;
      .breadcrumb {
        font-size: 16px;
        font-weight: 600;
        color: var(--el-text-color-primary);
      }
    }
    .header-right {
      .user-info {
        display: flex;
        align-items: center;
        gap: 6px;
        cursor: pointer;
        font-size: 14px;
        color: var(--el-text-color-regular);
      }
    }
  }
  .main {
    padding: 20px 24px;
    background: var(--el-bg-color-page);
    overflow-y: auto;
  }
}
</style>
