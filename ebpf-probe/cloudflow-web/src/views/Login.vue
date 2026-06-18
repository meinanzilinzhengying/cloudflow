<template>
  <div class="login-page">
    <div class="login-container">
      <div class="login-left">
        <div class="brand">
          <el-icon size="64" color="#165DFF"><Connection /></el-icon>
          <h1 class="brand-title">CloudFlow</h1>
          <p class="brand-subtitle">网络可观测平台</p>
          <p class="brand-desc">新一代 eBPF 全栈流量分析系统</p>
        </div>
        <div class="features">
          <div class="feature-item">
            <el-icon size="20" color="#165DFF"><Monitor /></el-icon>
            <span>实时内核级流量采集</span>
          </div>
          <div class="feature-item">
            <el-icon size="20" color="#165DFF"><DataAnalysis /></el-icon>
            <span>智能协议解析与异常检测</span>
          </div>
          <div class="feature-item">
            <el-icon size="20" color="#165DFF"><Warning /></el-icon>
            <span>安全审计与威胁感知</span>
          </div>
        </div>
      </div>
      <div class="login-right">
        <el-card class="login-card" shadow="never">
          <h2 class="login-title">欢迎登录</h2>
          <el-form
            ref="formRef"
            :model="form"
            :rules="rules"
            size="large"
            @submit.prevent="handleLogin"
          >
            <el-form-item prop="username">
              <el-input
                v-model="form.username"
                placeholder="用户名"
                :prefix-icon="User"
                clearable
              />
            </el-form-item>
            <el-form-item prop="password">
              <el-input
                v-model="form.password"
                type="password"
                placeholder="密码"
                :prefix-icon="Lock"
                show-password
                clearable
              />
            </el-form-item>
            <el-form-item>
              <el-checkbox v-model="form.remember">记住我</el-checkbox>
            </el-form-item>
            <el-form-item>
              <el-button
                type="primary"
                class="login-btn"
                :loading="loading"
                @click="handleLogin"
              >
                登录
              </el-button>
            </el-form-item>
          </el-form>
        </el-card>
      </div>
    </div>
    <div class="login-footer">
      <p>© 2025 CloudFlow Team. All Rights Reserved.</p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import type { FormInstance, FormRules } from 'element-plus'
import { useUserStore } from '@/stores/user'
import { Connection, Monitor, DataAnalysis, Warning, User, Lock } from '@element-plus/icons-vue'
import { login } from '@/api/auth'

const router = useRouter()
const userStore = useUserStore()
const formRef = ref<FormInstance>()
const loading = ref(false)

const form = reactive({
  username: '',
  password: '',
  remember: false,
})

const rules: FormRules = {
  username: [{ required: true, message: '请输入用户名', trigger: 'blur' }],
  password: [{ required: true, message: '请输入密码', trigger: 'blur' }],
}

const handleLogin = async () => {
  if (!formRef.value) return
  await formRef.value.validate()
  loading.value = true
  try {
    const res = await login({ username: form.username, password: form.password })
    if (res.code === 0) {
      userStore.setToken(res.data.token, res.data.username)
      ElMessage.success('登录成功')
      router.push('/dashboard')
    } else {
      ElMessage.error(res.message || '登录失败')
    }
  } catch (e) {
    ElMessage.error('网络错误，请稍后重试')
  } finally {
    loading.value = false
  }
}
</script>

<style scoped lang="scss">
.login-page {
  min-height: 100vh;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  background: linear-gradient(135deg, #0a1f44 0%, #0e3a8a 50%, #165DFF 100%);
  .login-container {
    display: flex;
    width: 900px;
    background: #fff;
    border-radius: 12px;
    overflow: hidden;
    box-shadow: 0 8px 32px rgba(0, 0, 0, 0.2);
    .login-left {
      flex: 1;
      padding: 48px;
      background: linear-gradient(180deg, #f7f8fa 0%, #e8ecf3 100%);
      display: flex;
      flex-direction: column;
      justify-content: center;
      .brand {
        margin-bottom: 40px;
        .brand-title {
          font-size: 32px;
          font-weight: 700;
          color: #165DFF;
          margin-top: 16px;
        }
        .brand-subtitle {
          font-size: 20px;
          font-weight: 600;
          color: var(--el-text-color-primary);
          margin-top: 8px;
        }
        .brand-desc {
          font-size: 14px;
          color: var(--el-text-color-secondary);
          margin-top: 8px;
        }
      }
      .features {
        display: flex;
        flex-direction: column;
        gap: 16px;
        .feature-item {
          display: flex;
          align-items: center;
          gap: 10px;
          font-size: 14px;
          color: var(--el-text-color-regular);
        }
      }
    }
    .login-right {
      width: 360px;
      padding: 48px;
      display: flex;
      flex-direction: column;
      justify-content: center;
      .login-card {
        border: none;
        .login-title {
          font-size: 24px;
          font-weight: 600;
          color: var(--el-text-color-primary);
          margin-bottom: 24px;
          text-align: center;
        }
      }
      .login-btn {
        width: 100%;
        height: 44px;
        font-size: 16px;
        border-radius: 6px;
      }
    }
  }
  .login-footer {
    margin-top: 24px;
    color: rgba(255, 255, 255, 0.6);
    font-size: 12px;
  }
}
</style>
