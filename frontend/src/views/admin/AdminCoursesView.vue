<script setup>
import { ref, onMounted, computed } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import axios from 'axios'

const token = localStorage.getItem('token')
const auth = { headers: { Authorization: 'Bearer ' + token } }

const courses = ref([])
const loading = ref(false)
const keyword = ref('')

const showCreate = ref(false)
const showEdit = ref(false)

const form = ref({ name: '', intro: '', teacher: '' })
const editForm = ref({ id: 0, name: '', intro: '', teacher: '' })

const filtered = computed(() => {
  const kw = keyword.value.trim().toLowerCase()
  if (!kw) return courses.value
  return courses.value.filter(c =>
    (c.name || '').toLowerCase().includes(kw) ||
    (c.teacher || '').toLowerCase().includes(kw)
  )
})

async function load() {
  loading.value = true
  try {
    const r = await axios.get('/api/course/list', auth)
    courses.value = r.data.courses || []
  } catch (e) {
    ElMessage.error('加载课程列表失败')
  } finally {
    loading.value = false
  }
}

async function createCourse() {
  if (!form.value.name) {
    ElMessage.warning('课程名不能为空')
    return
  }
  try {
    await axios.post('/api/course/create', form.value, auth)
    ElMessage.success('课程创建成功')
    showCreate.value = false
    form.value = { name: '', intro: '', teacher: '' }
    load()
  } catch (e) {
    ElMessage.error(e.response?.data?.message || '创建失败')
  }
}

function openEdit(c) {
  editForm.value = { id: c.id, name: c.name, intro: c.intro, teacher: c.teacher }
  showEdit.value = true
}

async function saveEdit() {
  try {
    await axios.put('/api/course/edit', editForm.value, auth)
    ElMessage.success('已保存')
    showEdit.value = false
    load()
  } catch (e) {
    ElMessage.error(e.response?.data?.message || '保存失败')
  }
}

async function removeCourse(c) {
  try {
    await ElMessageBox.confirm(`确定删除课程「${c.name}」？相关视频和资源将一并删除。`, '警告', { type: 'warning' })
  } catch { return }
  try {
    await axios.delete(`/api/course/delete/${c.id}`, auth)
    ElMessage.success('已删除')
    load()
  } catch (e) {
    ElMessage.error(e.response?.data?.message || '删除失败')
  }
}

onMounted(load)
</script>

<template>
  <div class="admin-courses-page">
    <div class="page-header">
      <div>
        <h1>课程管理</h1>
        <p class="sub">共 {{ courses.length }} 门课程</p>
      </div>
      <div class="actions">
        <el-input v-model="keyword" placeholder="搜索课程名/教师" style="width:220px" clearable />
        <el-button type="primary" @click="showCreate = true">新增课程</el-button>
      </div>
    </div>

    <el-card shadow="never">
      <el-table :data="filtered" v-loading="loading" stripe>
        <el-table-column prop="id" label="ID" width="70" />
        <el-table-column prop="name" label="课程名" min-width="180" />
        <el-table-column prop="teacher" label="教师" width="120" />
        <el-table-column prop="intro" label="简介" min-width="200" show-overflow-tooltip />
        <el-table-column label="操作" width="220" fixed="right">
          <template #default="{ row }">
            <el-button size="small" @click="openEdit(row)">编辑</el-button>
            <el-button size="small" type="danger" @click="removeCourse(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <!-- 新增课程 -->
    <el-dialog v-model="showCreate" title="新增课程" width="480px">
      <el-form :model="form" label-width="70px">
        <el-form-item label="课程名"><el-input v-model="form.name" /></el-form-item>
        <el-form-item label="教师"><el-input v-model="form.teacher" /></el-form-item>
        <el-form-item label="简介"><el-input v-model="form.intro" type="textarea" :rows="3" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showCreate = false">取消</el-button>
        <el-button type="primary" @click="createCourse">创建</el-button>
      </template>
    </el-dialog>

    <!-- 编辑课程 -->
    <el-dialog v-model="showEdit" title="编辑课程" width="480px">
      <el-form :model="editForm" label-width="70px">
        <el-form-item label="课程名"><el-input v-model="editForm.name" /></el-form-item>
        <el-form-item label="教师"><el-input v-model="editForm.teacher" /></el-form-item>
        <el-form-item label="简介"><el-input v-model="editForm.intro" type="textarea" :rows="3" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showEdit = false">取消</el-button>
        <el-button type="primary" @click="saveEdit">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.admin-courses-page { padding: 24px; max-width: 1200px; margin: 0 auto; }
.page-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 16px; }
.page-header h1 { margin: 0; font-size: 22px; color: #1f2937; }
.page-header .sub { margin: 4px 0 0; color: #9ca3af; font-size: 13px; }
.actions { display: flex; gap: 10px; }
</style>
