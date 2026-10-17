<script setup>
import { ref, reactive } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { register } from '../api/index.js'

const router = useRouter()
const formRef = ref(null)
const loading = ref(false)

// 表单数据：注册比登录多昵称和确认密码两个字段
const form = reactive({
  username: '',
  nickname: '',
  password: '',
  confirmPassword: '',
})

// Element Plus 表单校验规则
const rules = {
  username: [
    { required: true, message: '用户名不能为空', trigger: 'blur' },
    { min: 2, max: 20, message: '用户名长度 2-20 个字符', trigger: 'blur' },
  ],
  nickname: [{ max: 20, message: '昵称最长 20 个字符', trigger: 'blur' }],
  password: [
    { required: true, message: '密码不能为空', trigger: 'blur' },
    { min: 6, message: '密码至少 6 位', trigger: 'blur' },
  ],
  confirmPassword: [
    { required: true, message: '请再次输入密码', trigger: 'blur' },
    {
      // 自定义校验：确认密码必须和密码一致
      validator: (rule, value, callback) => {
        if (value !== form.password) {
          callback(new Error('两次输入的密码不一致'))
        } else {
          callback()
        }
      },
      trigger: 'blur',
    },
  ],
}

async function handleRegister() {
  // 先触发 Element Plus 表单校验，没通过就直接返回，不发请求
  try {
    await formRef.value.validate()
  } catch {
    return
  }

  loading.value = true
  try {
    // 调用后端注册接口（后端目前只接收 username/password）
    const res = await register({
      username: form.username,
      password: form.password,
    })
    ElMessage.success(`注册成功，请登录：${res.data.username}`)
    router.push('/login')
  } catch (e) {
    // 后端返回错误时，msg 字段是错误信息（如用户名已存在）
    ElMessage.error(e.response?.data?.msg || '注册失败，请重试')
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="register-page">
    <el-card class="register-card">
      <h2 class="title">注册</h2>
      <el-form ref="formRef" :model="form" :rules="rules" label-width="76px">
        <el-form-item label="用户名" prop="username">
          <el-input v-model="form.username" placeholder="请输入用户名" />
        </el-form-item>
        <el-form-item label="昵称" prop="nickname">
          <el-input v-model="form.nickname" placeholder="请输入昵称（可选）" />
        </el-form-item>
        <el-form-item label="密码" prop="password">
          <el-input v-model="form.password" type="password" show-password placeholder="请输入密码（至少 6 位）" />
        </el-form-item>
        <el-form-item label="确认密码" prop="confirmPassword">
          <el-input v-model="form.confirmPassword" type="password" show-password placeholder="请再次输入密码" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="loading" style="width: 100%" @click="handleRegister">
            注册
          </el-button>
        </el-form-item>
      </el-form>
      <p class="tip">已有账号？<router-link to="/login">去登录</router-link></p>
    </el-card>
  </div>
</template>

<style scoped>
.register-page {
  display: flex;
  justify-content: center;
  align-items: center;
  min-height: 70vh;
  background: linear-gradient(160deg, #eef2ff 0%, #f5f7fa 60%);
  padding: 24px;
}

.register-card {
  width: 400px;
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
