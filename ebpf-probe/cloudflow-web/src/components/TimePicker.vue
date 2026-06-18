<template>
  <div class="time-picker">
    <el-radio-group v-model="innerValue" size="small">
      <el-radio-button label="15m">15分钟</el-radio-button>
      <el-radio-button label="1h">1小时</el-radio-button>
      <el-radio-button label="6h">6小时</el-radio-button>
      <el-radio-button label="24h">24小时</el-radio-button>
    </el-radio-group>
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'

const props = defineProps<{
  modelValue: string
}>()
const emit = defineEmits<{
  (e: 'update:modelValue', v: string): void
  (e: 'change', v: string): void
}>()

const innerValue = ref(props.modelValue)

watch(innerValue, (v) => {
  emit('update:modelValue', v)
  emit('change', v)
})
</script>

<style scoped>
.time-picker {
  display: inline-flex;
}
</style>
