import { ref } from 'vue'

// 共享的时间范围状态 - 导出 ref 使其在多个组件间保持响应式
export const timeRange = ref('5m')
