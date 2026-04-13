<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import * as adminApi from '@/api/admin'
import type { BotRow } from '@/api/types'

const list = ref<BotRow[]>([])
const loading = ref(false)
const form = ref({ name: '', bot_token: '', remark: '', is_default: false })

async function load() {
  loading.value = true
  try {
    list.value = await adminApi.fetchBots()
  } catch (e: unknown) {
    ElMessage.error(String(e))
  } finally {
    loading.value = false
  }
}

async function onCreate() {
  if (!form.value.name || !form.value.bot_token) {
    ElMessage.warning('请填写名称与 Bot Token')
    return
  }
  try {
    await adminApi.createBot(form.value)
    ElMessage.success('已创建')
    form.value = { name: '', bot_token: '', remark: '', is_default: false }
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
      <el-form :inline="true" label-width="100px">
        <el-form-item label="名称"><el-input v-model="form.name" placeholder="机器人名称" /></el-form-item>
        <el-form-item label="Bot Token"><el-input v-model="form.bot_token" placeholder="从 BotFather 获取" show-password /></el-form-item>
        <el-form-item label="备注"><el-input v-model="form.remark" /></el-form-item>
        <el-form-item label="默认"><el-switch v-model="form.is_default" /></el-form-item>
        <el-form-item><el-button type="primary" @click="onCreate">创建</el-button></el-form-item>
      </el-form>
    </el-card>
    <el-table v-loading="loading" :data="list" stripe border>
      <el-table-column prop="id" label="ID" width="80" />
      <el-table-column prop="name" label="名称" />
      <el-table-column prop="is_default" label="默认" width="80">
        <template #default="{ row }">
          <el-tag :type="row.is_default ? 'success' : 'info'" size="small">{{ row.is_default }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="is_enabled" label="启用" width="80" />
      <el-table-column prop="remark" label="备注" />
    </el-table>
    <el-button style="margin-top: 12px" @click="load">刷新</el-button>
  </div>
</template>

<style scoped>
.mb {
  margin-bottom: 16px;
}
</style>
