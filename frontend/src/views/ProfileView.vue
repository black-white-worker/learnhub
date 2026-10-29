<script setup>
import { ref, reactive, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { useRouter } from 'vue-router'
import axios from 'axios'
import { changePassword } from '../api/index.js'

const router = useRouter()
const profile = ref({ username: '', nickname: '', role: '' })
const applying = ref(false)
const applied = ref(false)

// 消息通知（问题3）
const notifications = ref([])
const unreadCount = ref(0)

async function load() {
  const token = localStorage.getItem('token')
  const auth = { headers: { Authorization: 'Bearer ' + token } }
  try {
    const r = await axios.get('/api/user/info', auth)
    profile.value = r.data || {}
  } catch (e) {
    ElMessage.error('加载用户信息失败')
  }
  // 加载消息
  try {
    const n = await axios.get('/api/notifications', auth)
    notifications.value = n.data?.list || []
    unreadCount.value = n.data?.unread || 0
  } catch (e) {
    notifications.value = []
  }
}

// 我的作业（/api/my/assignments：一次返回我所有班级的作业 + 提交状态）
const assignments = ref([])
const assignmentLoading = ref(false)

const ASSIGN_STATUS = {
  submitted: { text: '已提交', type: 'success' },
  overdue: { text: '已截止未交', type: 'danger' },
  pending: { text: '待完成', type: 'warning' },
}

async function loadAssignments() {
  assignmentLoading.value = true
  const token = localStorage.getItem('token')
  try {
    const r = await axios.get('/api/my/assignments', {
      headers: { Authorization: 'Bearer ' + token },
    })
    assignments.value = r.data?.list || []
  } catch (e) {
    assignments.value = []
  } finally {
    assignmentLoading.value = false
  }
}

function assignmentStatus(a) {
  return ASSIGN_STATUS[a.myStatus] || ASSIGN_STATUS.pending
}

function fmtDeadline(t) {
  if (!t) return '不限时'
  return String(t).replace('T', ' ').slice(0, 16)
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

// 点击消息：跳转到对应页面并标记已读
async function openNotification(n) {
  const token = localStorage.getItem('token')
  if (!n.isRead) {
    try {
      await axios.post(`/api/notifications/${n.id}/read`, {}, { headers: { Authorization: 'Bearer ' + token } })
      n.isRead = true
      unreadCount.value = Math.max(0, unreadCount.value - 1)
    } catch (e) {}
  }
  if (n.linkUrl) {
    router.push(n.linkUrl)
  }
}

async function markAllRead() {
  const token = localStorage.getItem('token')
  try {
    await axios.post('/api/notifications/read-all', {}, { headers: { Authorization: 'Bearer ' + token } })
    notifications.value.forEach(n => n.isRead = true)
    unreadCount.value = 0
    ElMessage.success('已全部标为已读')
  } catch (e) {}
}

onMounted(() => {
  load()
  loadAssignments()
})

// ===== 修改密码 =====
// 初始账号 / 管理员重置密码后，顶栏会常驻提示引导到本页。
const pwdForm = reactive({ oldPassword: '', newPassword: '', confirm: '' })
const pwdSaving = ref(false)

async function submitPassword() {
  if (pwdForm.newPassword.length < 6) {
    ElMessage.warning('新密码至少 6 位')
    return
  }
  if (pwdForm.newPassword !== pwdForm.confirm) {
    ElMessage.warning('两次输入的新密码不一致')
    return
  }
  pwdSaving.value = true
  try {
    await changePassword({
      oldPassword: pwdForm.oldPassword,
      newPassword: pwdForm.newPassword,
    })
    ElMessage.success('密码已更新')
    pwdForm.oldPassword = ''
    pwdForm.newPassword = ''
    pwdForm.confirm = ''
    // 通知 App.vue 重新拉一次会话，收起"必须改密"提示
    window.dispatchEvent(new Event('dsh:session-updated'))
    load()
  } catch (e) {
    ElMessage.error(e.response?.data?.message || '修改失败，请检查原密码')
  } finally {
    pwdSaving.value = false
  }
}
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

    <!-- 修改密码：必须提供原密码，避免 token 泄露后被直接改密 -->
    <el-card class="pwd-card">
      <template #header>
        <div class="notify-head">
          <b>修改密码</b>
          <el-tag v-if="profile.mustChangePassword" type="warning" size="small">建议立即修改</el-tag>
        </div>
      </template>
      <el-form label-width="88px" @submit.prevent>
        <el-form-item label="原密码">
          <el-input v-model="pwdForm.oldPassword" type="password" show-password autocomplete="current-password" />
        </el-form-item>
        <el-form-item label="新密码">
          <el-input v-model="pwdForm.newPassword" type="password" show-password autocomplete="new-password" placeholder="至少 6 位" />
        </el-form-item>
        <el-form-item label="确认新密码">
          <el-input v-model="pwdForm.confirm" type="password" show-password autocomplete="new-password" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="pwdSaving" @click="submitPassword">保存新密码</el-button>
        </el-form-item>
      </el-form>
    </el-card>

    <!-- 我的作业：跨班级聚合，按状态一眼看出哪些还没交 -->
    <el-card class="assign-card" v-loading="assignmentLoading">
      <template #header>
        <div class="notify-head">
          <b>我的作业</b>
          <span class="all-read">共 {{ assignments.length }} 份</span>
        </div>
      </template>
      <el-empty v-if="!assignments.length" description="暂无作业" :image-size="60" />
      <div v-else class="assign-list">
        <div
          v-for="a in assignments"
          :key="a.id"
          class="assign-item"
          @click="router.push(`/assignment/${a.id}`)"
        >
          <div class="assign-main">
            <b>{{ a.title }}</b>
            <small>{{ a.className }} · 截止 {{ fmtDeadline(a.deadline) }}</small>
          </div>
          <div class="assign-right">
            <el-tag size="small" :type="assignmentStatus(a).type">{{ assignmentStatus(a).text }}</el-tag>
            <span v-if="a.myScore !== null && a.myScore !== undefined" class="score">
              {{ a.myScore }} / {{ a.totalScore }}
            </span>
          </div>
        </div>
      </div>
    </el-card>

    <!-- 消息中心（问题3：审批结果 + 班级动态提醒） -->
    <el-card class="notify-card">
      <template #header>
        <div class="notify-head">
          <b>消息中心</b>
          <el-badge v-if="unreadCount > 0" :value="unreadCount" type="danger">
            <el-button size="small" text @click="markAllRead">全部已读</el-button>
          </el-badge>
          <span v-else class="all-read">暂无未读消息</span>
        </div>
      </template>
      <el-empty v-if="!notifications.length" description="暂无消息" :image-size="60" />
      <div v-else class="notify-list">
        <div
          v-for="n in notifications"
          :key="n.id"
          class="notify-item"
          :class="{ unread: !n.isRead }"
          @click="openNotification(n)"
        >
          <el-badge is-dot :hidden="n.isRead" />
          <div class="notify-body">
            <b>{{ n.title }}</b>
            <p>{{ n.content }}</p>
            <small>{{ n.createdAt }}</small>
          </div>
        </div>
      </div>
    </el-card>
  </div>
</template>

<style scoped>
.profile-page { padding: 24px; max-width: 640px; margin: 0 auto; }
.profile-card { padding: 12px; margin-bottom: 16px; }
.apply-box { margin-top: 16px; }
.pwd-card { padding: 12px; margin-bottom: 16px; }
.notify-card { padding: 12px; }
.assign-card { padding: 12px; margin-bottom: 16px; }
.assign-list { display: flex; flex-direction: column; gap: 8px; }
.assign-item {
  display: flex; align-items: center; justify-content: space-between; gap: 12px;
  padding: 10px; border-radius: 8px; cursor: pointer; border: 1px solid #f1f5f9;
}
.assign-item:hover { background: #f8fafc; }
.assign-main b { font-size: 14px; color: #1f2937; display: block; }
.assign-main small { color: #9ca3af; font-size: 12px; }
.assign-right { display: flex; align-items: center; gap: 8px; }
.assign-right .score { font-size: 13px; color: #475569; }
.notify-head { display: flex; justify-content: space-between; align-items: center; }
.all-read { color: #9ca3af; font-size: 13px; }
.notify-list { display: flex; flex-direction: column; gap: 8px; }
.notify-item {
  display: flex; gap: 8px; padding: 10px; border-radius: 8px; cursor: pointer;
}
.notify-item:hover { background: #f5f7fa; }
.notify-item.unread { background: #eff6ff; }
.notify-body b { font-size: 14px; color: #1f2937; }
.notify-body p { margin: 4px 0; font-size: 13px; color: #6b7280; }
.notify-body small { color: #9ca3af; font-size: 12px; }
</style>
