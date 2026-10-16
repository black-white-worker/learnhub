<script setup>
import { RouterLink, RouterView } from 'vue-router'
</script>

<template>
  <el-container class="layout">
    <el-header class="header">
      <div class="logo">学习平台</div>
      <nav>
        <RouterLink to="/">首页</RouterLink>
        <RouterLink to="/about">关于</RouterLink>
      </nav>
    </el-header>
    <el-main>
      <RouterView />
    </el-main>
  </el-container>
</template>

<style scoped>
.layout {
  min-height: 100vh;
}

.header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  border-bottom: 1px solid var(--el-border-color);
  background: #fff;
}

.logo {
  font-size: 18px;
  font-weight: bold;
}

nav a {
  margin-left: 16px;
  color: #333;
  text-decoration: none;
}

nav a.router-link-exact-active {
  color: var(--el-color-primary);
}
</style>
