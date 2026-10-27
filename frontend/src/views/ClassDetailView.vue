<template>
  <div class="class-detail" v-loading="loading">
    <template v-if="detail">
      <el-button @click="$router.push('/classes')" size="small">← 返回班级列表</el-button>
      <h1>{{ detail.class.name }}</h1>
      <p class="sub">{{ detail.class.grade }} · {{ detail.class.major }}
        <el-button v-if="isTeacher" size="small" @click="genInvite">生成邀请码</el-button>
        <span v-if="inviteCode" class="invite-code">邀请码：{{ inviteCode }}</span>
      </p>

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
import { ElMessage } from 'element-plus'
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

onMounted(load)
</script>

<style scoped>
.class-detail { padding: 24px; }
.sub { color: #666; }
.invite-code { margin-left: 10px; color: #e6a23c; font-weight: bold; }
</style>
