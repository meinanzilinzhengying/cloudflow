<template>
  <el-tag :type="color" size="small" round>
    {{ text }}
  </el-tag>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { statusColor, statusText } from '@/utils/format'

const props = defineProps<{
  status: string
}>()

const color = computed(() => statusColor(props.status))
const text = computed(() => statusText(props.status))
</script>
