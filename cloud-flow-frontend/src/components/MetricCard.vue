<template>
  <el-card class="metric-card" :body-style="{ padding: '20px' }">
    <div class="metric-header">{{ title }}</div>
    <div class="metric-value">{{ value }}</div>
    <div class="metric-trend">
      <el-icon :class="trendClass">
        <ArrowUp v-if="trend > 0" />
        <ArrowDown v-else-if="trend < 0" />
        <Minus v-else />
      </el-icon>
      <span>{{ Math.abs(trend) }}%</span>
    </div>
  </el-card>
</template>

<script setup lang="ts">
import { ArrowUp, ArrowDown, Minus } from '@element-plus/icons-vue'
import { computed } from 'vue'

const props = defineProps<{
  title: string
  value: string
  trend: number
}>()

const trendClass = computed(() => {
  if (props.trend > 0) return 'trend-up'
  if (props.trend < 0) return 'trend-down'
  return 'trend-flat'
})
</script>

<style scoped lang="scss">
.metric-card {
  border-radius: 8px;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.08);
}
.metric-header {
  font-size: 14px;
  color: var(--el-text-color-secondary);
  margin-bottom: 8px;
}
.metric-value {
  font-size: 28px;
  font-weight: 600;
  color: var(--el-text-color-primary);
  margin-bottom: 8px;
}
.metric-trend {
  font-size: 12px;
  display: flex;
  align-items: center;
  gap: 4px;
}
.trend-up { color: var(--el-color-danger); }
.trend-down { color: var(--el-color-success); }
.trend-flat { color: var(--el-text-color-secondary); }
</style>
