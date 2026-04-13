<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import * as adminApi from '@/api/admin'

const stats = ref<Record<string, number>>({})
const loading = ref(false)

async function load() {
  loading.value = true
  try {
    stats.value = await adminApi.fetchDashboard()
  } catch (e: unknown) {
    ElMessage.error(String(e))
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>

<template>
  <div v-loading="loading">
    <el-row :gutter="16">
      <el-col v-for="(v, k) in stats" :key="k" :xs="24" :sm="12" :md="8" :lg="6">
        <el-card shadow="hover">
          <div class="stat-label">{{ k }}</div>
          <div class="stat-value">{{ v }}</div>
        </el-card>
      </el-col>
    </el-row>
    <el-button type="primary" link style="margin-top: 12px" @click="load">刷新</el-button>
  </div>
</template>

<style scoped>
.stat-label {
  font-size: 13px;
  color: var(--el-text-color-secondary);
}
.stat-value {
  font-size: 28px;
  font-weight: 600;
  margin-top: 8px;
}
</style>
