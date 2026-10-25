<template>
  <div>
    <!-- 老师：发起签到事件 -->
    <div v-if="isTeacher" class="toolbar">
      <el-input v-model="newSignin.title" placeholder="签到标题" style="width:180px" />
      <el-input-number v-model="newSignin.duration" :min="1" :max="120" placeholder="分钟" style="width:120px" />
      <span style="color:#999;font-size:12px">分钟</span>
      <el-button type="primary" @click="startSignin">发起签到</el-button>
    </div>

    <!-- 当前进行中的签到事件 -->
    <el-card v-if="activeSignin" class="event-card" shadow="hover">
      <template #header>
        <div class="event-head">
          <b>{{ activeSignin.title || '课堂签到' }}</b>
          <el-tag type="danger" size="small">进行中</el-tag>
        </div>
      </template>
      <p class="event-time">
        开始：{{ fmtTime(activeSignin.startAt) }} · 截止：{{ fmtTime(activeSignin.expiresAt) }}
      </p>

      <!-- 学生端：输入签到码 -->
      <div v-if="!isTeacher" class="signin-student">
        <el-input v-model="signinCode" placeholder="输入老师给的6位签到码" style="width:220px" maxlength="6" />
        <el-button type="primary" style="margin-left:8px" @click="doSignin">签到</el-button>
      </div>

      <!-- 老师端：已签/未签名单 -->
      <div v-if="isTeacher" class="roster">
        <el-divider content-position="left">已签到（{{ signedList.length }}人）</el-divider>
        <div v-for="r in signedList" :key="r.id" class="signin-row">
          <el-tag size="small" type="success">已签</el-tag>
          {{ r.username }} <small>{{ fmtTime(r.signedAt) }}</small>
        </div>
        <el-divider content-position="left">未签到（{{ unsignedList.length }}人）</el-divider>
        <div v-for="u in unsignedList" :key="u.userId" class="signin-row unsigned">
          <el-tag size="small" type="info">未签</el-tag>
          {{ u.username }}
        </div>
      </div>
    </el-card>

    <el-empty v-else description="当前没有进行中的签到" />

    <!-- 历史签到事件（老师端） -->
    <template v-if="isTeacher && historyList.length">
      <el-divider content-position="left">历史签到</el-divider>
      <el-card v-for="h in historyList" :key="h.id" class="history-card" shadow="never">
        <b>{{ h.title || '课堂签到' }}</b>
        <p class="event-time">
          {{ fmtTime(h.startAt) }} ~ {{ fmtTime(h.expiresAt) }}
          <el-tag v-if="isExpired(h.expiresAt)" size="small" type="info" style="margin-left:8px">已结束</el-tag>
          <el-tag v-else size="small" type="warning">进行中</el-tag>
        </p>
      </el-card>
    </template>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import axios from 'axios'

const props = defineProps({
  classId: { type: [String, Number], required: true },
  isTeacher: { type: Boolean, default: false },
})

const token = localStorage.getItem('token')
const auth = { headers: { Authorization: 'Bearer ' + token } }

const activeSignin = ref(null)
const signinCode = ref('')
const signedList = ref([])
const unsignedList = ref([])
const historyList = ref([])
const newSignin = ref({ title: '', duration: 5 })

function fmtTime(t) {
  if (!t) return ''
  return String(t).replace('T', ' ').slice(0, 16)
}
function isExpired(t) {
  return new Date(t) < new Date()
}

async function loadSignin() {
  try {
    const r = await axios.get(`/api/class/${props.classId}/signin`, auth)
    activeSignin.value = r.data || null
    if (activeSignin.value && props.isTeacher) {
      try {
        const rec = await axios.get(`/api/class/${props.classId}/signin/records`, auth)
        signedList.value = rec.data?.signed || []
        unsignedList.value = rec.data?.unsigned || []
      } catch (e) {
        signedList.value = []
        unsignedList.value = []
      }
    }
  } catch (e) {
    activeSignin.value = null
  }
  // 老师端加载历史
  if (props.isTeacher) {
    try {
      const h = await axios.get(`/api/class/${props.classId}/signin/history`, auth)
      historyList.value = h.data || []
    } catch (e) {
      historyList.value = []
    }
  }
}

async function startSignin() {
  try {
    await axios.post(`/api/class/${props.classId}/signin/start`, {
      title: newSignin.value.title || '课堂签到',
      duration: newSignin.value.duration || 5,
    }, auth)
    ElMessage.success('签到已发起')
    newSignin.value = { title: '', duration: 5 }
    loadSignin()
  } catch (e) {
    ElMessage.error(e.response?.data?.message || '发起签到失败')
  }
}

async function doSignin() {
  if (!signinCode.value.trim()) return
  try {
    await axios.post(`/api/class/${props.classId}/signin/do`, { code: signinCode.value.trim() }, auth)
    ElMessage.success('签到成功')
    signinCode.value = ''
    loadSignin()
  } catch (e) {
    ElMessage.error(e.response?.data?.message || '签到失败')
  }
}

onMounted(loadSignin)
defineExpose({ reload: loadSignin })
</script>

<style scoped>
.toolbar { display: flex; gap: 10px; margin-bottom: 16px; align-items: center; }
.event-card { margin-bottom: 12px; }
.event-head { display: flex; justify-content: space-between; align-items: center; }
.event-time { color: #666; font-size: 13px; margin: 4px 0 0; }
.signin-student { margin-top: 12px; }
.roster { margin-top: 8px; }
.signin-row { padding: 4px 0; border-bottom: 1px dashed #eee; font-size: 14px; }
.signin-row.unsigned { color: #999; }
.history-card { margin-bottom: 8px; }
</style>
