<template>
  <div class="settings">
    <div class="page-header">
      <h2 class="page-title">系统设置</h2>
    </div>
    <el-row :gutter="24">
      <el-col :span="12">
        <el-card class="setting-card" :body-style="{ padding: '24px' }">
          <div class="card-title">告警配置</div>
          <el-form :model="alertConfig" label-width="120px">
            <el-form-item label="告警开关">
              <el-switch v-model="alertConfig.enabled" />
            </el-form-item>
            <el-form-item label="通知方式">
              <el-checkbox-group v-model="alertConfig.channels">
                <el-checkbox label="email">邮件</el-checkbox>
                <el-checkbox label="webhook">Webhook</el-checkbox>
                <el-checkbox label="sms">短信</el-checkbox>
              </el-checkbox-group>
            </el-form-item>
            <el-form-item label="告警间隔">
              <el-input-number v-model="alertConfig.interval" :min="1" :max="60" size="small" />
              <span style="margin-left: 8px">分钟</span>
            </el-form-item>
          </el-form>
        </el-card>
      </el-col>
      <el-col :span="12">
        <el-card class="setting-card" :body-style="{ padding: '24px' }">
          <div class="card-title">数据采集配置</div>
          <el-form :model="collectConfig" label-width="120px">
            <el-form-item label="采集周期">
              <el-input-number v-model="collectConfig.interval" :min="1" :max="300" size="small" />
              <span style="margin-left: 8px">秒</span>
            </el-form-item>
            <el-form-item label="采集器">
              <el-checkbox-group v-model="collectConfig.collectors">
                <el-checkbox label="network">网络流量</el-checkbox>
                <el-checkbox label="process">进程</el-checkbox>
                <el-checkbox label="file">文件</el-checkbox>
                <el-checkbox label="syscall">系统调用</el-checkbox>
              </el-checkbox-group>
            </el-form-item>
            <el-form-item label="数据保留">
              <el-input-number v-model="collectConfig.retention" :min="1" :max="365" size="small" />
              <span style="margin-left: 8px">天</span>
            </el-form-item>
          </el-form>
        </el-card>
      </el-col>
    </el-row>
    <el-row :gutter="24" style="margin-top: 24px">
      <el-col :span="24">
        <el-card class="setting-card" :body-style="{ padding: '24px' }">
          <div class="card-title">平台信息</div>
          <el-descriptions :column="2" border>
            <el-descriptions-item label="版本">v3.1.0</el-descriptions-item>
            <el-descriptions-item label="构建时间">2026-06-18</el-descriptions-item>
            <el-descriptions-item label="探针版本">v3.1.0</el-descriptions-item>
            <el-descriptions-item label="数据库">ClickHouse 23.8</el-descriptions-item>
            <el-descriptions-item label="后端框架">Go 1.24</el-descriptions-item>
            <el-descriptions-item label="前端框架">Vue 3 + Element Plus</el-descriptions-item>
          </el-descriptions>
        </el-card>
      </el-col>
    </el-row>
  </div>
</template>

<script setup lang="ts">
import { reactive } from 'vue'

const alertConfig = reactive({
  enabled: true,
  channels: ['email', 'webhook'],
  interval: 5,
})

const collectConfig = reactive({
  interval: 10,
  collectors: ['network', 'process'],
  retention: 30,
})
</script>

<style scoped lang="scss">
.settings {
  .page-header {
    display: flex; justify-content: space-between; align-items: center; margin-bottom: 20px;
    .page-title { font-size: 18px; font-weight: 600; color: #FFFFFF; }
  }
  .setting-card {
    border-radius: 8px; box-shadow: 0 2px 8px rgba(0,0,0,0.08);
    .card-title { font-size: 16px; font-weight: 600; color: #FFFFFF; margin-bottom: 20px; }
  }
}
</style>
