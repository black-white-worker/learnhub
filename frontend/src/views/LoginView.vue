<script setup>
import { ref, reactive } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { login } from '../api/index.js'

const router = useRouter()
const formRef = ref(null)
const loading = ref(false)

// 表单数据
const form = reactive({
  username: '',
  password: '',
})

// Element Plus 表单校验规则：必填（错误文案与 placeholder 区分，避免视觉重复）
const rules = {
  username: [{ required: true, message: '用户名不能为空', trigger: 'blur' }],
  password: [{ required: true, message: '密码不能为空', trigger: 'blur' }],
}

async function handleLogin() {
  // 先触发 Element Plus 表单校验，没通过就直接返回，不发请求
  try {
    await formRef.value.validate()
  } catch {
    return
  }

  loading.value = true
  try {
    // 调用后端登录接口
    const res = await login({ username: form.username, password: form.password })
    // 登录接口返回裸 data（{token,username,role}），但 rawClient/其他调用方
    // 可能带回 AxiosResponse，这里两种形状都兼容，避免"200 了却没存 token"。
    const body = res?.data ?? res ?? {}
    if (!body.token) {
      throw new Error('登录响应缺少 token')
    }
    // 登录成功：token / username / role 存进浏览器 localStorage
    localStorage.setItem('token', body.token)
    localStorage.setItem('username', body.username || form.username)
    localStorage.setItem('role', body.role || 'student')
    if (body.mustChangePassword) {
      // 初始账号 / 被重置过密码：直接引导到个人中心改密
      ElMessage.warning('请先修改初始密码')
      router.push({ path: '/profile', query: { forcePassword: '1' } })
      return
    }
    ElMessage.success('登录成功')
    router.push('/')
  } catch (e) {
    // 后端统一错误是 {code,message}；网络/解析异常则退回通用提示
    ElMessage.error(e.response?.data?.message || e.response?.data?.msg || e.message || '账号或密码错误')
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="login-page">
    <el-card class="login-card">
      <h2 class="title">登录</h2>
      <el-form ref="formRef" :model="form" :rules="rules" label-width="76px">
        <el-form-item label="用户名" prop="username">
          <el-input v-model="form.username" placeholder="请输入用户名" />
        </el-form-item>
        <el-form-item label="密码" prop="password">
          <el-input v-model="form.password" type="password" show-password placeholder="请输入密码" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="loading" style="width: 100%" @click="handleLogin">
            登录
          </el-button>
        </el-form-item>
      </el-form>
      <p class="tip">演示账号：admin / 123456（首次登录需修改密码）</p>
      <p class="tip">没有账号？<router-link to="/register">去注册</router-link></p>
    </el-card>
  </div>
</template>

<style scoped>
.login-page {
  display: flex;
  justify-content: center;
  align-items: center;
  min-height: 70vh;
  background: linear-gradient(160deg, #eef2ff 0%, #f5f7fa 60%);
  padding: 24px;
}

.login-card {
  width: 380px;
  padding: 12px 8px;
  border-radius: 14px;
  box-shadow: 0 12px 32px rgba(37, 99, 235, 0.12);
}

.title {
  text-align: center;
  margin: 0 0 16px;
}

.tip {
  color: #999;
  font-size: 13px;
  text-align: center;
}

.tip a {
  color: #2563eb;
  text-decoration: none;
}
</style>
