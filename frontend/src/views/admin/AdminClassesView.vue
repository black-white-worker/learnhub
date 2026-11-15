<script setup>
import { ref, onMounted, computed } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import axios from 'axios'

const token = localStorage.getItem('token')
const auth = { headers: { Authorization: 'Bearer ' + token } }

const classes = ref([])
const loading = ref(false)
const keyword = ref('')

const showEdit = ref(false)
const editForm = ref({ id: 0, name: '', grade: '', major: '' })

// 成员管理弹窗
const showMembers = ref(false)
const memberClass = ref(null)
const members = ref([])
const membersLoading = ref(false)

const filtered = computed(() => {
  const kw = keyword.value.trim().toLowerCase()
  if (!kw) return classes.value
  return classes.value.filter(
    (c) =>
      (c.name || '').toLowerCase().includes(kw) ||
      (c.headTeacherName || '').toLowerCase().includes(kw) ||
      (c.grade || '').toLowerCase().includes(kw) ||
      (c.major || '').toLowerCase().includes(kw)
  )
})

// 顶部汇总：让管理员一眼看出平台规模，而不是自己心算表格
const summary = computed(() => {
  const list = classes.value
  return {
    classes: list.length,
    students: list.reduce((n, c) => n + (c.studentCount || 0), 0),
    courses: list.reduce((n, c) => n + (c.courseCount || 0), 0),
  }
})

function fmtTime(v) {
  if (!v) return '-'
  const d = new Date(v)
  return Number.isNaN(d.getTime()) ? String(v) : d.toLocaleDateString()
}

async function load() {
  loading.value = true
  try {
    const r = await axios.get('/api/admin/classes', auth)
    classes.value = r.data || []
  } catch (e) {
    ElMessage.error(e.response?.data?.message || '加载班级列表失败')
  } finally {
    loading.value = false
  }
}

function openEdit(c) {
  editForm.value = { id: c.id, name: c.name, grade: c.grade || '', major: c.major || '' }
  showEdit.value = true
}

async function saveEdit() {
  if (!editForm.value.name.trim()) {
    ElMessage.warning('班级名不能为空')
    return
  }
  try {
    await axios.put('/api/admin/classes', editForm.value, auth)
    ElMessage.success('已保存')
    showEdit.value = false
    load()
  } catch (e) {
    ElMessage.error(e.response?.data?.message || '保存失败')
  }
}

async function removeClass(c) {
  const detail = `${c.memberCount || 0} 名成员、${c.assignmentCount || 0} 份作业、${c.examCount || 0} 场考试`
  try {
    await ElMessageBox.confirm(
      `删除「${c.name}」会同时清除它的作业与提交、考试与答卷、成绩、讨论、签到和公告，且无法恢复。\n\n` +
        `当前规模：${detail}。`,
      '删除班级',
      {
        type: 'warning',
        confirmButtonText: '确认删除',
        confirmButtonClass: 'el-button--danger',
        cancelButtonText: '取消',
        customClass: 'danger-confirm',
      }
    )
  } catch {
    return
  }
  try {
    const r = await axios.delete(`/api/class/${c.id}`, auth)
    ElMessage.success(r.data?.stats ? `已删除，清理 ${r.data.stats.members || 0} 名成员` : '已删除')
    load()
  } catch (e) {
    ElMessage.error(e.response?.data?.message || '删除失败')
  }
}

async function openMembers(c) {
  memberClass.value = c
  showMembers.value = true
  membersLoading.value = true
  try {
    const r = await axios.get(`/api/class/${c.id}/members`, auth)
    members.value = r.data || []
  } catch (e) {
    ElMessage.error(e.response?.data?.message || '加载成员失败')
    members.value = []
  } finally {
    membersLoading.value = false
  }
}

async function removeMember(m) {
  const label = m.nickname || m.username
  try {
    await ElMessageBox.confirm(
      m.role === 'teacher'
        ? `「${label}」是这个班的老师，移除后 TA 将无法管理该班。确定移除吗？`
        : `确定把「${label}」移出班级吗？TA 已提交的作业与成绩会保留。`,
      '移除成员',
      { type: 'warning', confirmButtonText: '确认移除', cancelButtonText: '取消' }
    )
  } catch {
    return
  }
  try {
    await axios.delete(`/api/class/${memberClass.value.id}/members/${m.userId}`, auth)
    ElMessage.success('已移除')
    openMembers(memberClass.value)
    load()
  } catch (e) {
    ElMessage.error(e.response?.data?.message || '移除失败')
  }
}

onMounted(load)
</script>

<template>
  <div class="admin-classes-page">
    <div class="page-header">
      <div>
        <h1>班级管理</h1>
        <p class="sub">
          共 {{ summary.classes }} 个班级 · {{ summary.students }} 名学生 · 绑定 {{ summary.courses }} 门课程
        </p>
      </div>
      <div class="actions">
        <el-input v-model="keyword" placeholder="搜索班级名 / 班主任 / 年级" style="width:260px" clearable />
        <el-button @click="load">刷新</el-button>
      </div>
    </div>

    <el-card shadow="never">
      <el-table :data="filtered" v-loading="loading" stripe>
        <el-table-column prop="id" label="ID" width="70" />
        <el-table-column prop="name" label="班级名" min-width="160" />
        <el-table-column prop="grade" label="年级" width="100" />
        <el-table-column prop="major" label="专业" min-width="140" show-overflow-tooltip />
        <el-table-column label="班主任" width="120">
          <template #default="{ row }">
            {{ row.headTeacherName || row.createdBy || '未设置' }}
          </template>
        </el-table-column>
        <el-table-column label="成员" width="100">
          <template #default="{ row }">
            {{ row.memberCount || 0 }}
            <span class="muted">（学生 {{ row.studentCount || 0 }}）</span>
          </template>
        </el-table-column>
        <el-table-column prop="courseCount" label="课程" width="70" />
        <el-table-column prop="assignmentCount" label="作业" width="70" />
        <el-table-column prop="examCount" label="考试" width="70" />
        <el-table-column label="创建时间" width="110">
          <template #default="{ row }">{{ fmtTime(row.createdAt) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="230" fixed="right">
          <template #default="{ row }">
            <el-button size="small" @click="openMembers(row)">成员</el-button>
            <el-button size="small" @click="openEdit(row)">编辑</el-button>
            <el-button size="small" type="danger" @click="removeClass(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
      <el-empty v-if="!loading && !filtered.length" description="没有匹配的班级" />
    </el-card>

    <!-- 编辑班级基本信息 -->
    <el-dialog v-model="showEdit" title="编辑班级" width="460px">
      <el-form :model="editForm" label-width="70px">
        <el-form-item label="班级名"><el-input v-model="editForm.name" /></el-form-item>
        <el-form-item label="年级"><el-input v-model="editForm.grade" placeholder="如：2024级" /></el-form-item>
        <el-form-item label="专业"><el-input v-model="editForm.major" /></el-form-item>
      </el-form>
      <p class="hint">班主任变更请在「成员」里处理：直接把某人设为老师还需要后端支持，当前仅支持移除成员。</p>
      <template #footer>
        <el-button @click="showEdit = false">取消</el-button>
        <el-button type="primary" @click="saveEdit">保存</el-button>
      </template>
    </el-dialog>

    <!-- 成员管理 -->
    <el-dialog v-model="showMembers" :title="`成员管理 · ${memberClass?.name || ''}`" width="560px">
      <el-table :data="members" v-loading="membersLoading" max-height="420">
        <el-table-column prop="userId" label="用户 ID" width="90" />
        <el-table-column label="姓名" min-width="140">
          <template #default="{ row }">{{ row.nickname || row.username }}</template>
        </el-table-column>
        <el-table-column prop="username" label="账号" width="130" />
        <el-table-column label="角色" width="90">
          <template #default="{ row }">
            <el-tag :type="row.role === 'teacher' ? 'warning' : 'info'" size="small">
              {{ row.role === 'teacher' ? '老师' : '学生' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="90">
          <template #default="{ row }">
            <el-button size="small" type="danger" link @click="removeMember(row)">移除</el-button>
          </template>
        </el-table-column>
      </el-table>
      <el-empty v-if="!membersLoading && !members.length" description="这个班还没有成员" />
      <template #footer>
        <el-button @click="showMembers = false">关闭</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.admin-classes-page { padding: 24px; max-width: 1280px; margin: 0 auto; }
.page-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 16px; }
.page-header h1 { margin: 0; font-size: 22px; color: #1f2937; }
.page-header .sub { margin: 4px 0 0; color: #9ca3af; font-size: 13px; }
.actions { display: flex; gap: 10px; }
.muted { color: #9ca3af; font-size: 12px; }
.hint { margin: 0; color: #9ca3af; font-size: 12px; line-height: 1.6; }
</style>

<style>
.danger-confirm .el-message-box__message { white-space: pre-line; line-height: 1.7; }
</style>
