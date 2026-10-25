<script setup>
import { ref, onMounted, computed } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import axios from 'axios'

const token = localStorage.getItem('token')
const auth = { headers: { Authorization: 'Bearer ' + token } }

const users = ref([])
const loading = ref(false)
const keyword = ref('')

const showCreate = ref(false)
const showEdit = ref(false)
const showReset = ref(false)

const form = ref({ username: '', password: '', nickname: '', role: 'student' })
const editForm = ref({ username: '', nickname: '', role: 'student' })
const resetForm = ref({ username: '', password: '' })

const filtered = computed(() => {
  const kw = keyword.value.trim().toLowerCase()
  if (!kw) return users.value
  return users.value.filter(u =>
    (u.username || '').toLowerCase().includes(kw) ||
    (u.nickname || '').toLowerCase().includes(kw)
  )
})

async function load() {
  loading.value = true
  try {
    const r = await axios.get('/api/admin/users', auth)
    users.value = r.data || []
  } catch (e) {
    ElMessage.error('加载用户列表失败：' + (e.response?.data?.message || e.message))
  } finally {
    loading.value = false
  }
}

async function createUser() {
  if (!form.value.username || !form.value.password) {
    ElMessage.warning('用户名和密码不能为空')
    return
  }
  try {
    await axios.post('/api/admin/users', form.value, auth)
    ElMessage.success('用户创建成功')
    showCreate.value = false
    form.value = { username: '', password: '', nickname: '', role: 'student' }
    load()
  } catch (e) {
    ElMessage.error(e.response?.data?.message || '创建失败')
  }
}

function openEdit(u) {
  editForm.value = { username: u.username, nickname: u.nickname, role: u.role }
  showEdit.value = true
}

async function saveEdit() {
  try {
    await axios.put('/api/admin/users', editForm.value, auth)
    ElMessage.success('已保存')
    showEdit.value = false
    load()
  } catch (e) {
    ElMessage.error(e.response?.data?.message || '保存失败')
  }
}

async function removeUser(u) {
  try {
    await ElMessageBox.confirm(`确定删除用户「${u.username}」？此操作不可恢复。`, '警告', { type: 'warning' })
  } catch { return }
  try {
    await axios.delete('/api/admin/users', { ...auth, params: { username: u.username } })
    ElMessage.success('已删除')
    load()
  } catch (e) {
    ElMessage.error(e.response?.data?.message || '删除失败')
  }
}

function openReset(u) {
  resetForm.value = { username: u.username, password: '123456' }
  showReset.value = true
}

async function saveReset() {
  if (!resetForm.value.password) {
    ElMessage.warning('请输入新密码')
    return
  }
  try {
    await axios.post('/api/admin/users/reset-password', resetForm.value, auth)
    ElMessage.success('密码已重置')
    showReset.value = false
  } catch (e) {
    ElMessage.error(e.response?.data?.message || '重置失败')
  }
}

function roleTag(role) {
  return { admin: 'danger', teacher: 'success', student: 'info' }[role] || 'info'
}

onMounted(load)
</script>

<template>
  <div class="admin-users-page">
    <div class="page-header">
      <div>
        <h1>用户管理</h1>
        <p class="sub">共 {{ users.length }} 个账号</p>
      </div>
      <div class="actions">
        <el-input v-model="keyword" placeholder="搜索用户名/昵称" style="width:220px" clearable />
        <el-button type="primary" @click="showCreate = true">新增用户</el-button>
      </div>
    </div>

    <el-card shadow="never">
      <el-table :data="filtered" v-loading="loading" stripe>
        <el-table-column prop="username" label="用户名" width="140" />
        <el-table-column prop="nickname" label="昵称" width="160" />
        <el-table-column label="角色" width="100">
          <template #default="{ row }">
            <el-tag :type="roleTag(row.role)" size="small">{{ row.role }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="申请老师" width="100">
          <template #default="{ row }">
            <el-tag v-if="row.applyTeacher" type="warning" size="small">待审批</el-tag>
            <span v-else class="muted">—</span>
          </template>
        </el-table-column>
        <el-table-column label="操作" min-width="260" fixed="right">
          <template #default="{ row }">
            <el-button size="small" @click="openEdit(row)">编辑</el-button>
            <el-button size="small" type="warning" @click="openReset(row)">重置密码</el-button>
            <el-button size="small" type="danger" @click="removeUser(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <!-- 新增用户 -->
    <el-dialog v-model="showCreate" title="新增用户" width="420px">
      <el-form :model="form" label-width="70px">
        <el-form-item label="用户名"><el-input v-model="form.username" /></el-form-item>
        <el-form-item label="密码"><el-input v-model="form.password" type="password" show-password /></el-form-item>
        <el-form-item label="昵称"><el-input v-model="form.nickname" /></el-form-item>
        <el-form-item label="角色">
          <el-select v-model="form.role">
            <el-option label="学生" value="student" />
            <el-option label="老师" value="teacher" />
            <el-option label="管理员" value="admin" />
          </el-select>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showCreate = false">取消</el-button>
        <el-button type="primary" @click="createUser">创建</el-button>
      </template>
    </el-dialog>

    <!-- 编辑用户 -->
    <el-dialog v-model="showEdit" title="编辑用户" width="420px">
      <el-form :model="editForm" label-width="70px">
        <el-form-item label="用户名"><el-input v-model="editForm.username" disabled /></el-form-item>
        <el-form-item label="昵称"><el-input v-model="editForm.nickname" /></el-form-item>
        <el-form-item label="角色">
          <el-select v-model="editForm.role">
            <el-option label="学生" value="student" />
            <el-option label="老师" value="teacher" />
            <el-option label="管理员" value="admin" />
          </el-select>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showEdit = false">取消</el-button>
        <el-button type="primary" @click="saveEdit">保存</el-button>
      </template>
    </el-dialog>

    <!-- 重置密码 -->
    <el-dialog v-model="showReset" title="重置密码" width="400px">
      <el-form :model="resetForm" label-width="80px">
        <el-form-item label="用户名"><el-input v-model="resetForm.username" disabled /></el-form-item>
        <el-form-item label="新密码"><el-input v-model="resetForm.password" type="password" show-password /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showReset = false">取消</el-button>
        <el-button type="primary" @click="saveReset">确定</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.admin-users-page { padding: 24px; max-width: 1200px; margin: 0 auto; }
.page-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 16px; }
.page-header h1 { margin: 0; font-size: 22px; color: #1f2937; }
.page-header .sub { margin: 4px 0 0; color: #9ca3af; font-size: 13px; }
.actions { display: flex; gap: 10px; }
.muted { color: #c0c4cc; }
</style>
