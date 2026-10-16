<script setup>
import { ref } from 'vue'
import request from '../api/index.js'

const result = ref('')
const loading = ref(false)

// 调用后端接口测试前后端是否联通
async function testBackend() {
  loading.value = true
  result.value = ''
  try {
    const res = await request.get('/from/me')
    result.value = JSON.stringify(res.data)
  } catch (e) {
    result.value = '请求失败：' + (e.response?.data?.msg || e.message)
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div>
    <h2>学习平台首页</h2>
    <p>前端框架（Vue3 + Vite + Element Plus）已就绪，后端为 Go + go-zero（端口 8888）。</p>
    <el-button type="primary" :loading="loading" @click="testBackend">
      测试后端连接
    </el-button>
    <p v-if="result" class="result">后端返回：{{ result }}</p>
  </div>
</template>

<style scoped>
.result {
  margin-top: 16px;
  color: var(--el-color-primary);
}
</style>
