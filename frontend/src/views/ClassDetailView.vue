<template>
  <div class="class-detail" v-loading="loading">
    <template v-if="detail">
      <h1>{{ detail.class.name }}</h1>
      <p class="sub">{{ detail.class.grade }} · {{ detail.class.major }}
        <el-button v-if="isTeacher" size="small" @click="genInvite">生成邀请码</el-button>
        <span v-if="inviteCode" class="invite-code">邀请码：{{ inviteCode }}</span>
      </p>

      <el-tabs v-model="tab">
        <el-tab-pane label="公告" name="notice">
          <div v-if="isTeacher" class="toolbar">
            <el-input v-model="newNotice.title" placeholder="公告标题" style="width:200px" />
            <el-input v-model="newNotice.content" type="textarea" placeholder="公告内容" style="width:400px" />
            <el-button type="primary" @click="postNotice">发布</el-button>
          </div>
          <div v-for="n in notices" :key="n.id" class="notice-item">
            <el-tag v-if="n.isTop" type="warning" size="small">置顶</el-tag>
            <b>{{ n.title }}</b>
            <p>{{ n.content }}</p>
            <small>{{ n.createdAt }}</small>
          </div>
        </el-tab-pane>

        <el-tab-pane label="作业" name="hw">
          <div v-if="isTeacher" class="toolbar">
            <el-input v-model="newHw.title" placeholder="作业标题" style="width:200px" />
            <el-input v-model="newHw.description" placeholder="作业描述" style="width:300px" />
            <el-button type="primary" @click="postHw">布置</el-button>
          </div>
          <div v-for="a in assignments" :key="a.id" class="hw-item" @click="openHw(a)">
            <b>{{ a.title }}</b>
            <p>截止：{{ a.deadline || '无截止时间' }} · 总分：{{ a.totalScore }}</p>
          </div>
        </el-tab-pane>

        <el-tab-pane label="讨论" name="discuss">
          <div v-if="isMember" class="toolbar">
            <el-input v-model="newPost.title" placeholder="帖子标题" style="width:200px" />
            <el-input v-model="newPost.content" placeholder="内容" style="width:300px" />
            <el-button type="primary" @click="postDiscussion">发帖</el-button>
          </div>
          <div v-for="d in discussions" :key="d.id" class="discuss-item" @click="openDiscussion(d)">
            <b>{{ d.title }}</b>
            <p>{{ d.content.substring(0, 60) }}</p>
          </div>
        </el-tab-pane>
      </el-tabs>

      <!-- 作业详情弹窗 -->
      <el-dialog v-model="hwDialog" :title="currentHw?.title" width="600px">
        <p>{{ currentHw?.description }}</p>
        <el-divider />
        <el-input v-model="mySubmission" type="textarea" placeholder="提交作业内容" />
        <el-button type="primary" style="margin-top:10px" @click="submitHw">提交</el-button>
        <div v-if="myScore !== null" style="margin-top:10px">得分：{{ myScore }} 评语：{{ myComment }}</div>
      </el-dialog>

      <!-- 讨论详情弹窗 -->
      <el-dialog v-model="discussDialog" :title="currentDiscussion?.title" width="600px">
        <p>{{ currentDiscussion?.content }}</p>
        <el-divider />
        <div v-for="r in replies" :key="r.id" class="reply">{{ r.content }}</div>
        <el-input v-model="replyText" placeholder="回复" style="margin-top:10px" />
        <el-button type="primary" style="margin-top:10px" @click="doReply">回复</el-button>
      </el-dialog>
    </template>
  </div>
</template>

<script setup>
import { ref, onMounted, computed } from 'vue'
import { useRoute } from 'vue-router'
import { ElMessage } from 'element-plus'
import axios from 'axios'

const route = useRoute()
const classId = route.params.id
const token = localStorage.getItem('token')
const auth = { headers: { Authorization: 'Bearer ' + token } }

const loading = ref(false)
const detail = ref(null)
const tab = ref('notice')
const notices = ref([])
const assignments = ref([])
const discussions = ref([])
const inviteCode = ref('')

const newNotice = ref({ title: '', content: '' })
const newHw = ref({ title: '', description: '' })
const newPost = ref({ title: '', content: '' })

const hwDialog = ref(false)
const currentHw = ref(null)
const mySubmission = ref('')
const myScore = ref(null)
const myComment = ref('')

const discussDialog = ref(false)
const currentDiscussion = ref(null)
const replies = ref([])
const replyText = ref('')

const isTeacher = computed(() => detail.value?.myRole === 'teacher' || detail.value?.myRole === 'admin')
const isMember = computed(() => !!detail.value?.myRole)

async function load() {
  loading.value = true
  try {
    const r = await axios.get(`/api/class/${classId}`, auth)
    detail.value = r.data
    // 查当前用户在班角色
    const me = localStorage.getItem('username')
    const myMember = r.data.members.find(m => m.username === me)
    detail.value.myRole = myMember ? myMember.role : ''
    loadNotices()
    loadHw()
    loadDiscuss()
  } catch (e) {
    ElMessage.error('加载失败')
  } finally {
    loading.value = false
  }
}

async function loadNotices() {
  const r = await axios.get(`/api/class/${classId}/announcements`, auth)
  notices.value = r.data || []
}
async function loadHw() {
  const r = await axios.get(`/api/class/${classId}/assignments`, auth)
  assignments.value = r.data || []
}
async function loadDiscuss() {
  const r = await axios.get(`/api/class/${classId}/discussions`, auth)
  discussions.value = r.data || []
}

async function postNotice() {
  await axios.post(`/api/class/${classId}/announcement`, newNotice.value, auth)
  newNotice.value = { title: '', content: '' }
  loadNotices()
}
async function postHw() {
  await axios.post(`/api/class/${classId}/assignment`, newHw.value, auth)
  newHw.value = { title: '', description: '' }
  loadHw()
}
async function postDiscussion() {
  await axios.post(`/api/class/${classId}/discussion`, newPost.value, auth)
  newPost.value = { title: '', content: '' }
  loadDiscuss()
}

async function genInvite() {
  const r = await axios.post(`/api/class/${classId}/invite`, {}, auth)
  inviteCode.value = r.data.code
  ElMessage.success('邀请码：' + r.data.code)
}

async function openHw(a) {
  currentHw.value = a
  const r = await axios.get(`/api/assignment/${a.id}`, auth)
  mySubmission.value = r.data.mySubmission?.content || ''
  myScore.value = r.data.mySubmission?.score ?? null
  myComment.value = r.data.mySubmission?.comment || ''
  hwDialog.value = true
}
async function submitHw() {
  await axios.post(`/api/assignment/${currentHw.value.id}/submit`, { content: mySubmission.value }, auth)
  ElMessage.success('提交成功')
  hwDialog.value = false
}

async function openDiscussion(d) {
  currentDiscussion.value = d
  const r = await axios.get(`/api/discussion/${d.id}`, auth)
  replies.value = r.data.replies || []
  discussDialog.value = true
}
async function doReply() {
  await axios.post(`/api/discussion/${currentDiscussion.value.id}/reply`, { content: replyText.value }, auth)
  replyText.value = ''
  openDiscussion(currentDiscussion.value)
}

onMounted(load)
</script>

<style scoped>
.class-detail { padding: 24px; }
.sub { color: #666; }
.invite-code { margin-left: 10px; color: #e6a23c; font-weight: bold; }
.toolbar { display: flex; gap: 10px; margin-bottom: 16px; align-items: center; }
.notice-item, .hw-item, .discuss-item { padding: 10px; border-bottom: 1px solid #eee; cursor: pointer; }
.reply { padding: 6px; background: #f5f5f5; margin: 4px 0; }
</style>
