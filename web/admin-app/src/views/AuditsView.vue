<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import * as adminApi from '@/api/admin'
import type { AuditRow } from '@/api/types'

const list = ref<AuditRow[]>([])
const loading = ref(false)

async function load() {
  loading.value = true
  try {
    list.value = await adminApi.fetchAudits()
  } catch (e: unknown) {
    ElMessage.error(String(e))
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>

<template>
  <div>
    <el-table v-loading="loading" :data="list" stripe border max-height="560">
      <el-table-column prop="id" label="ID" width="80" />
      <el-table-column prop="action" label="动作" width="140" />
      <el-table-column prop="object_type" label="对象类型" width="120" />
      <el-table-column prop="object_id" label="对象 ID" width="100" />
      <el-table-column prop="detail" label="详情" min-width="200" show-overflow-tooltip />
      <el-table-column prop="created_at" label="时间" min-width="180" />
    </el-table>
    <el-button style="margin-top: 12px" @click="load">刷新</el-button>
  </div>
</template>
