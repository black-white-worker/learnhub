<template>
  <div class="class-list-page">
    <div class="header">
      <h1>我的班级</h1>
      <div class="actions">
        <el-input v-model="joinCode" placeholder="输入邀请码加入" style="width:200px" @keyup.enter="doJoin" />
        <el-button type="success" @click="doJoin">加入班级</el-button>
        <!-- 只有老师/管理员能创建班级，学生账号不显示入口（后端 /api/class/create 也会拦截） -->
        <el-button v-if="canCreateClass" type="primary" @click="showCreate = true">创建班级</el-button>
      </div>
    </div>

    <el-row :gutter="16" v-loading="loading">
      <el-col :span="8" v-for="c in classes" :key="c.id" style="margin-bottom:16px">
        <el-card shadow="hover" @click="$router.push(`/class/${c.id}`)" class="class-card">
          <h3>{{ c.name }}</h3>
          <p>年级：{{ c.grade || '未设置' }}</p>
          <p>专业：{{ c.major || '未设置' }}</p>
        </el-card>
      </el-col>
    </el-row>

    <el-dialog v-model="showCreate" title="创建班级" width="400px">
      <el-form :model="form" label-width="60px">
        <el-form-item label="班名"><el-input v-model="form.name" /></el-form-item>
        <el-form-item label="年级"><el-input v-model="form.grade" placeholder="如：2024级" /></el-form-item>
        <el-form-item label="专业"><el-input v-model="form.major" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showCreate = false">取消</el-button>
        <el-button type="primary" @click="doCreate">创建</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import axios from 'axios'

const classes = ref([])
const loading = ref(false)
const joinCode = ref('')
const showCreate = ref(false)
const form = ref({ name: '', grade: '', major: '' })

// 只有老师/管理员能创建班级（与后端 /api/class/create 的角色校验保持一致）
const canCreateClass = computed(() => {
  const role = localStorage.getItem('role')
  return role === 'teacher' || role === 'admin'
})

async function load() {
  loading.value = true
  try {
    const token = localStorage.getItem('token')
    const r = await axios.get('/api/class/my', { headers: { Authorization: 'Bearer ' + token } })
    classes.value = r.data || []
  } catch (e) {
    ElMessage.error('加载失败：' + (e.response?.data?.message || e.message))
  } finally {
    loading.value = false
  }
}

async function doCreate() {
  const token = localStorage.getItem('token')
  try {
    const r = await axios.post('/api/class/create', form.value, { headers: { Authorization: 'Bearer ' + token } })
    showCreate.value = false
    // 创建成功后把邀请码弹窗展示给老师，方便发给学生加入
    if (r.data?.inviteCode) {
      ElMessageBox.alert(
        `班级创建成功，邀请码：${r.data.inviteCode}（发给学生用于加入班级）`,
        '创建成功',
        { confirmButtonText: '知道了' }
      )
    } else {
      ElMessage.success('创建成功')
    }
    load()
  } catch (e) {
    ElMessage.error(e.response?.data?.message || '创建失败')
  }
}

async function doJoin() {
  if (!joinCode.value.trim()) return
  const token = localStorage.getItem('token')
  try {
    const r = await axios.post('/api/class/join', { code: joinCode.value.trim() }, { headers: { Authorization: 'Bearer ' + token } })
    ElMessage.success('加入成功')
    joinCode.value = ''
    load()
  } catch (e) {
    ElMessage.error(e.response?.data?.message || '加入失败')
  }
}

onMounted(load)
</script>

<style scoped>
.class-list-page { padding: 24px; }
.header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 20px; }
.actions { display: flex; gap: 10px; }
.class-card { cursor: pointer; }
</style>
