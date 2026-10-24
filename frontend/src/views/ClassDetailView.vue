<template>
  <div class="class-detail" v-loading="loading">
    <template v-if="detail">
      <el-button @click="$router.push('/classes')" size="small">← 返回班级列表</el-button>
      <h1>{{ detail.class.name }}</h1>
      <p class="sub">{{ detail.class.grade }} · {{ detail.class.major }}
        <el-button v-if="isTeacher" size="small" @click="genInvite">生成邀请码</el-button>
        <span v-if="inviteCode" class="invite-code">邀请码：{{ inviteCode }}</span>
      </p>

      <el-tabs v-model="tab">
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
import { ref, onMounted, computed } from 'vue'
import { useRoute } from 'vue-router'
import { ElMessage } from 'element-plus'
import axios from 'axios'

import NoticeTab from '../components/class/NoticeTab.vue'
import AssignmentTab from '../components/class/AssignmentTab.vue'
import DiscussTab from '../components/class/DiscussTab.vue'
import GradeTab from '../components/class/GradeTab.vue'
import SigninTab from '../components/class/SigninTab.vue'

const route = useRoute()
const classId = route.params.id
const token = localStorage.getItem('token')
const auth = { headers: { Authorization: 'Bearer ' + token } }

const loading = ref(false)
const detail = ref(null)
const tab = ref('notice')
const inviteCode = ref('')

const isTeacher = computed(() => detail.value?.myRole === 'teacher' || detail.value?.myRole === 'admin')
const isMember = computed(() => !!detail.value?.myRole)

async function load() {
  loading.value = true
  try {
    const r = await axios.get(`/api/class/${classId}`, auth)
    detail.value = r.data
    inviteCode.value = r.data.inviteCode || ''
    // 查当前用户在班角色
    const me = localStorage.getItem('username')
    const myMember = r.data.members.find(m => m.username === me)
    detail.value.myRole = myMember ? myMember.role : ''
  } catch (e) {
    ElMessage.error('加载失败')
  } finally {
    loading.value = false
  }
}

async function genInvite() {
  const r = await axios.post(`/api/class/${classId}/invite`, {}, auth)
  inviteCode.value = r.data.code
  ElMessage.success('邀请码：' + r.data.code)
}

onMounted(load)
</script>

<style scoped>
.class-detail { padding: 24px; }
.sub { color: #666; }
.invite-code { margin-left: 10px; color: #e6a23c; font-weight: bold; }
</style>
