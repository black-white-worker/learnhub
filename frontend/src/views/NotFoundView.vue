<script setup>
// 404 页面：改造前没有兜底路由，访问错地址会得到一片空白。
import { useRouter } from 'vue-router'

const router = useRouter()
</script>

<template>
  <div class="not-found">
    <div class="code">404</div>
    <p class="tip">这个页面不存在，可能是链接过期或者地址写错了。</p>
    <div class="actions">
      <el-button type="primary" @click="router.push('/')">回到首页</el-button>
      <el-button @click="router.back()">返回上一页</el-button>
    </div>
  </div>
</template>

<style scoped>
.not-found {
  min-height: 60vh;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 12px;
}
.code {
  font-size: 72px;
  font-weight: 800;
  color: #cbd5e1;
  letter-spacing: 2px;
}
.tip {
  color: #64748b;
  margin: 0;
}
.actions {
  margin-top: 8px;
  display: flex;
  gap: 10px;
}
</style>
