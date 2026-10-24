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
          <div v-if="isTeacher" class="toolbar">
            <el-input v-model="newNotice.title" placeholder="公告标题" style="width:200px" />
            <el-input v-model="newNotice.content" type="textarea" placeholder="公告内容" style="width:400px" />
            <el-button type="primary" @click="postNotice">发布</el-button>
          </div>
          <div v-for="n in notices" :key="n.id" class="notice-item" @click="openNotice(n)">
            <el-badge is-dot :hidden="n.readByMe" class="notice-dot">
              <el-tag v-if="n.isTop" type="warning" size="small">置顶</el-tag>
            </el-badge>
            <b :class="{ 'notice-unread': !n.readByMe }">{{ n.title }}</b>
            <span v-if="isTeacher" class="read-stat">已读 {{ n.readCount }}</span>
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

        <el-tab-pane label="签到" name="signin">
          <div v-if="isTeacher" class="toolbar">
            <el-button type="primary" @click="startSignin">发起签到（5分钟有效）</el-button>
            <span v-if="activeSignin" style="margin-left:12px">当前签到码：<b style="color:#e6a23c">{{ activeSignin.code }}</b></span>
          </div>
          <div v-if="activeSignin">
            <div v-if="!isTeacher" class="signin-student">
              <el-input v-model="signinCode" placeholder="输入老师给的6位签到码" style="width:220px" maxlength="6" />
              <el-button type="primary" style="margin-left:8px" @click="doSignin">签到</el-button>
            </div>
            <div v-if="isTeacher" style="margin-top:12px">
              <b>已签名单（{{ signinRecords.length }}人）：</b>
              <div v-for="r in signinRecords" :key="r.id" class="signin-row">
                {{ r.username }} <small>{{ r.signedAt }}</small>
              </div>
            </div>
          </div>
          <el-empty v-else description="当前没有进行中的签到" />
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

// 签到
const activeSignin = ref(null)
const signinCode = ref('')
const signinRecords = ref([])

const isTeacher = computed(() => detail.value?.myRole === 'teacher' || detail.value?.myRole === 'admin')
const isMember = computed(() => !!detail.value?.myRole)

async function load() {
  loading.value = true
  try {
    const r = await axios.get(`/api/class/${classId}`, auth)
    detail.value = r.data
    // 邀请码由后端随详情一并返回，进页面即展示
    inviteCode.value = r.data.inviteCode || ''
    // 查当前用户在班角色
    const me = localStorage.getItem('username')
    const myMember = r.data.members.find(m => m.username === me)
    detail.value.myRole = myMember ? myMember.role : ''
    loadNotices()
    loadHw()
    loadDiscuss()
    loadSignin()
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
async function loadSignin() {
  try {
    const r = await axios.get(`/api/class/${classId}/signin`, auth)
    activeSignin.value = r.data || null
    if (activeSignin.value) {
      const rec = await axios.get(`/api/class/${classId}/signin/records`, auth)
      signinRecords.value = rec.data || []
    } else {
      signinRecords.value = []
    }
  } catch (e) {
    activeSignin.value = null
  }
}

async function startSignin() {
  await axios.post(`/api/class/${classId}/signin/start`, {}, auth)
  ElMessage.success('签到已发起')
  loadSignin()
}
async function doSignin() {
  if (!signinCode.value.trim()) return
  try {
    await axios.post(`/api/class/${classId}/signin/do`, { code: signinCode.value.trim() }, auth)
    ElMessage.success('签到成功')
    signinCode.value = ''
    loadSignin()
  } catch (e) {
    ElMessage.error(e.response?.data?.message || '签到失败')
  }
}

async function postNotice() {
  await axios.post(`/api/class/${classId}/announcement`, newNotice.value, auth)
  newNotice.value = { title: '', content: '' }
  loadNotices()
}

// 打开公告：未读则调接口标记已读，本地立刻把红点去掉
async function openNotice(n) {
  if (n.readByMe) return
  try {
    await axios.post(`/api/class/announcement/${n.id}/read`, {}, auth)
    n.readByMe = true
    n.readCount = (n.readCount || 0) + 1
  } catch (e) {
    // 标记已读失败不影响浏览
  }
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
.notice-unread { color: #1f2937; }
.read-stat { margin-left: 8px; color: #909399; font-size: 12px; }
.notice-dot { margin-right: 4px; }
.reply { padding: 6px; background: #f5f5f5; margin: 4px 0; }
.signin-student { margin: 12px 0; }
.signin-row { padding: 4px 0; border-bottom: 1px dashed #eee; }
</style>
