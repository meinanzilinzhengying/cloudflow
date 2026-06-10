<template>
  <div class="space-y-6">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold text-slate-900 dark:text-white">Agent 管理</h1>
        <p class="text-sm text-slate-500 dark:text-slate-400 mt-1">管理 eBPF Agent 与 Edge 节点</p>
      </div>
    </div>

    <!-- Agent Stats -->
    <div class="grid grid-cols-3 gap-4">
      <div class="card p-4">
        <p class="text-sm text-slate-500">在线 Agent</p>
        <p class="text-2xl font-bold text-green-500">0</p>
      </div>
      <div class="card p-4">
        <p class="text-sm text-slate-500">离线 Agent</p>
        <p class="text-2xl font-bold text-slate-400">0</p>
      </div>
      <div class="card p-4">
        <p class="text-sm text-slate-500">Edge 节点</p>
        <p class="text-2xl font-bold text-blue-500">0</p>
      </div>
    </div>

    <!-- Agent List -->
    <div class="card">
      <div class="p-8 text-center text-slate-500">
        <Server class="w-12 h-12 mx-auto mb-4 opacity-50" />
        <p>暂无 Agent 数据</p>
      </div>
    </div>
  </div>
</template>

<script setup>
import { Server } from 'lucide-vue-next'
</script>
