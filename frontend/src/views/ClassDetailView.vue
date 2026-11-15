<template>
  <div class="class-detail" v-loading="loading">
    <template v-if="detail">
      <div class="page-head">
        <div>
          <el-button @click="$router.push('/classes')" size="small">← 返回班级列表</el-button>
          <h1>{{ detail.class.name }}</h1>
          <p class="sub">
            {{ detail.class.grade }} · {{ detail.class.major }}
            <span v-if="inviteCode" class="invite-code">邀请码：{{ inviteCode }}</span>
          </p>
        </div>
        <div class="head-actions">
          <!-- 邀请码：班内老师与管理员都能生成（管理员在班主任失联时兜底） -->
          <el-button v-if="isTeacher" size="small" @click="genInvite">生成邀请码</el-button>
          <!-- 删除班级：后端 canDelete 已按 authz 算好（建班老师 / 管理员），前端不重复判 -->
          <el-button v-if="detail.canDelete" size="small" type="danger" plain @click="removeClass">
            删除班级
          </el-button>
          <!-- 退出班级：只有学生成员能看到 -->
          <el-button v-if="detail.canQuit" size="small" type="warning" plain @click="quitClass">
            退出班级
          </el-button>
        </div>
      </div>

      <el-tabs v-model="tab" lazy>
        <el-tab-pane label="公告" name="notice">
          <NoticeTab :class-id="classId" :is-teacher="isTeacher" />
        </el-tab-pane>

        <el-tab-pane label="作业" name="hw">
          <AssignmentTab :class-id="classId" :is-teacher="isTeacher" />
        </el-tab-pane>

        <el-tab-pane label="讨论" name="discuss">
          <DiscussTab :class-id="classId" :is-member="isMember" />
        </el-tab-pane>

        <el-tab-pane v-if="isTeacher" label="成绩" name="grade">
          <GradeTab :class-id="classId" />
        </el-tab-pane>

        <el-tab-pane label="签到" name="signin">
          <SigninTab :class-id="classId" :is-teacher="isTeacher" />
        </el-tab-pane>
      </el-tabs>
    </template>
  </div>
</template>

<script setup>
import { ref, onMounted, computed, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import axios from 'axios'

import NoticeTab from '../components/class/NoticeTab.vue'
import AssignmentTab from '../components/class/AssignmentTab.vue'
import DiscussTab from '../components/class/DiscussTab.vue'
import GradeTab from '../components/class/GradeTab.vue'
import SigninTab from '../components/class/SigninTab.vue'

const route = useRoute()
const router = useRouter()
const classId = route.params.id
const token = localStorage.getItem('token')
const auth = { headers: { Authorization: 'Bearer ' + token } }

const loading = ref(false)
const detail = ref(null)
const inviteCode = ref('')

// ===== 页签与 URL 同步 =====
//
// 改造前 tab = ref('notice')：只要刷新，组件重挂载就回到"公告"，
// 正在看作业/签到的老师会被强行弹走。现在把页签放进 URL 查询参数
// （?tab=hw），刷新、分享链接、前进后退都能保持当前页签。
const TABS = ['notice', 'hw', 'discuss', 'grade', 'signin']
const tab = ref(TABS.includes(route.query.tab) ? route.query.tab : 'notice')

watch(tab, (val) => {
  // 用 replace 而不是 push，避免每切一次页签就往历史里塞一条记录
  if (route.query.tab === val) return
  router.replace({ query: { ...route.query, tab: val } })
})

// 浏览器前进/后退时同步回来
watch(() => route.query.tab, (val) => {
  if (TABS.includes(val) && val !== tab.value) tab.value = val
})

const isTeacher = computed(() => detail.value?.myRole === 'teacher' || detail.value?.myRole === 'admin')
const isMember = computed(() => !!detail.value?.myRole)

async function load() {
  loading.value = true
  try {
    const r = await axios.get(`/api/class/${classId}`, auth)
    detail.value = r.data
    inviteCode.value = r.data.inviteCode || ''
    // myRole 由后端 RequireClassMember 直接返回（teacher/student/admin），
    // 不再前端自己去 members 里 find（admin 不在 members 表里会误判成学生）。
    detail.value.myRole = r.data.myRole || ''
  } catch (e) {
    ElMessage.error('加载失败：' + (e.response?.data?.message || e.message))
  } finally {
    loading.value = false
  }
}

async function genInvite() {
  try {
    const r = await axios.post(`/api/class/${classId}/invite`, {}, auth)
    inviteCode.value = r.data.code
    ElMessage.success('邀请码：' + r.data.code)
  } catch (e) {
    ElMessage.error(e.response?.data?.message || '生成邀请码失败')
  }
}

// 删除班级：不可逆，二次确认里必须把"会连带删掉什么"说清楚。
// 成员数与绑定课程数从详情里能直接拿到，作业/考试/成绩的条数只有删完后端才回，
// 所以文案用"全部"而不是编一个数字。
async function removeClass() {
  const name = detail.value?.class?.name || '该班级'
  const members = detail.value?.members?.length || 0
  const courses = detail.value?.courseIds?.length || 0
  try {
    await ElMessageBox.confirm(
      `删除「${name}」会同时清除它的作业与提交、考试与答卷、成绩、讨论、签到、公告和成员关系，且无法恢复。\n\n` +
        `当前规模：${members} 名成员、${courses} 门绑定课程。`,
      '删除班级',
      {
        type: 'warning',
        confirmButtonText: '确认删除',
        confirmButtonClass: 'el-button--danger',
        cancelButtonText: '再想想',
        // 换行在 MessageBox 里默认不保留，加 white-space 才像人话
        customClass: 'danger-confirm',
      }
    )
  } catch {
    return // 用户取消
  }
  try {
    const r = await axios.delete(`/api/class/${classId}`, auth)
    const s = r.data?.stats || {}
    const parts = []
    if (s.members) parts.push(`${s.members} 名成员`)
    if (s.assignments) parts.push(`${s.assignments} 份作业`)
    if (s.submissions) parts.push(`${s.submissions} 份提交`)
    if (s.exams) parts.push(`${s.exams} 场考试`)
    ElMessage.success(parts.length ? `班级已删除，清理了 ${parts.join('、')}` : '班级已删除')
    router.push('/classes')
  } catch (e) {
    ElMessage.error(e.response?.data?.message || '删除失败')
  }
}

// 退出班级：只解除成员关系，已交作业与成绩保留（后端同口径）
async function quitClass() {
  const name = detail.value?.class?.name || '该班级'
  try {
    await ElMessageBox.confirm(
      `确定退出「${name}」吗？退出后你看不到该班的公告、作业与考试；` +
        `已经提交的作业和成绩会保留在班级里。`,
      '退出班级',
      { type: 'warning', confirmButtonText: '确认退出', cancelButtonText: '取消' }
    )
  } catch {
    return
  }
  try {
    await axios.post(`/api/class/${classId}/quit`, {}, auth)
    ElMessage.success('已退出班级')
    router.push('/classes')
  } catch (e) {
    ElMessage.error(e.response?.data?.message || '退出失败')
  }
}

onMounted(load)
</script>

<style scoped>
.class-detail { padding: 24px; }
.sub { color: #666; }
.invite-code { margin-left: 10px; color: #e6a23c; font-weight: bold; }
.page-head { display: flex; justify-content: space-between; align-items: flex-start; gap: 16px; }
.page-head h1 { margin: 12px 0 0; }
.head-actions { display: flex; gap: 8px; flex-shrink: 0; padding-top: 4px; }
</style>

<style>
/* MessageBox 渲染在 body 下，scoped 样式够不着 */
.danger-confirm .el-message-box__message { white-space: pre-line; line-height: 1.7; }
</style>
