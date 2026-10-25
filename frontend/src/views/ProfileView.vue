<script setup>
import { ref, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import axios from 'axios'

const profile = ref({ username: '', nickname: '', role: '' })
const applying = ref(false)
const applied = ref(false)

async function load() {
  const token = localStorage.getItem('token')
  try {
    const r = await axios.get('/api/user/info', { headers: { Authorization: 'Bearer ' + token } })
    profile.value = r.data || {}
  } catch (e) {
    ElMessage.error('加载用户信息失败')
  }
}

async function applyTeacher() {
  applying.value = true
  try {
    const token = localStorage.getItem('token')
    await axios.post('/api/user/apply-teacher', {}, { headers: { Authorization: 'Bearer ' + token } })
    applied.value = true
    ElMessage.success('申请已提交，请等待管理员审批')
  } catch (e) {
    ElMessage.error(e.response?.data?.message || '申请失败')
  } finally {
    applying.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="profile-page">
    <el-card class="profile-card">
      <h2>个人主页</h2>
      <el-descriptions :column="1" border>
        <el-descriptions-item label="用户名">{{ profile.username }}</el-descriptions-item>
        <el-descriptions-item label="昵称">{{ profile.nickname || '未设置' }}</el-descriptions-item>
        <el-descriptions-item label="角色">
          <el-tag v-if="profile.role === 'admin'" type="danger">管理员</el-tag>
          <el-tag v-else-if="profile.role === 'teacher'" type="success">老师</el-tag>
          <el-tag v-else type="info">学生</el-tag>
        </el-descriptions-item>
      </el-descriptions>

      <div v-if="profile.role === 'student'" class="apply-box">
        <el-divider />
        <p>想发布课程、创建班级、布置作业？申请成为老师：</p>
        <el-button v-if="!applied" type="primary" :loading="applying" @click="applyTeacher">
          申请成为老师
        </el-button>
        <el-alert v-else type="success" :closable="false" title="申请已提交，请等待管理员审批" />
      </div>
    </el-card>
  </div>
</template>

<style scoped>
.profile-page { padding: 24px; max-width: 640px; margin: 0 auto; }
.profile-card { padding: 12px; }
.apply-box { margin-top: 16px; }
</style>
