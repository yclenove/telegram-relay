<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import * as adminApi from '@/api/admin'
import type { EventRow } from '@/api/types'

const list = ref<EventRow[]>([])
const loading = ref(false)

async function load() {
  loading.value = true
  try {
    list.value = await adminApi.fetchEvents()
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
      <el-table-column prop="event_id" label="EventID" min-width="160" />
      <el-table-column prop="source" label="来源" width="100" />
      <el-table-column prop="level" label="级别" width="100" />
      <el-table-column prop="status" label="状态" width="100" />
      <el-table-column prop="created_at" label="时间" min-width="180" />
    </el-table>
    <el-button style="margin-top: 12px" type="primary" @click="load">刷新</el-button>
  </div>
</template>
