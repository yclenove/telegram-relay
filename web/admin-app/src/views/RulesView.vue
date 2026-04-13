<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import * as adminApi from '@/api/admin'
import type { RuleRow } from '@/api/types'

const list = ref<RuleRow[]>([])
const loading = ref(false)
const form = ref({
  name: '',
  priority: 100,
  match_source: '',
  match_level: '',
  destination_id: 0,
})

async function load() {
  loading.value = true
  try {
    list.value = await adminApi.fetchRules()
  } catch (e: unknown) {
    ElMessage.error(String(e))
  } finally {
    loading.value = false
  }
}

async function onCreate() {
  if (!form.value.name || !form.value.destination_id) {
    ElMessage.warning('请填写规则名与目标 Destination ID')
    return
  }
  try {
    await adminApi.createRule({
      name: form.value.name,
      priority: Number(form.value.priority),
      match_source: form.value.match_source,
      match_level: form.value.match_level,
      destination_id: Number(form.value.destination_id),
    })
    ElMessage.success('已创建')
    form.value = { name: '', priority: 100, match_source: '', match_level: '', destination_id: 0 }
    await load()
  } catch (e: unknown) {
    ElMessage.error(String(e))
  }
}

onMounted(load)
</script>

<template>
  <div>
    <el-card shadow="never" class="mb">
      <el-form :inline="true" label-width="120px">
        <el-form-item label="规则名"><el-input v-model="form.name" /></el-form-item>
        <el-form-item label="优先级"><el-input-number v-model="form.priority" :min="0" /></el-form-item>
        <el-form-item label="匹配来源"><el-input v-model="form.match_source" placeholder="留空=全部" /></el-form-item>
        <el-form-item label="匹配级别"><el-input v-model="form.match_level" placeholder="留空=全部" /></el-form-item>
        <el-form-item label="Destination ID"><el-input-number v-model="form.destination_id" :min="1" /></el-form-item>
        <el-form-item><el-button type="primary" @click="onCreate">创建</el-button></el-form-item>
      </el-form>
    </el-card>
    <el-table v-loading="loading" :data="list" stripe border>
      <el-table-column prop="id" label="ID" width="80" />
      <el-table-column prop="name" label="名称" />
      <el-table-column prop="priority" label="优先级" width="100" />
      <el-table-column prop="match_source" label="来源" />
      <el-table-column prop="match_level" label="级别" />
      <el-table-column prop="destination_id" label="目标 ID" width="100" />
    </el-table>
    <el-button style="margin-top: 12px" @click="load">刷新</el-button>
  </div>
</template>

<style scoped>
.mb {
  margin-bottom: 16px;
}
</style>
