<template>
  <div>
    <!-- 老师：发起签到事件 -->
    <div v-if="isTeacher" class="toolbar">
      <el-input v-model="newSignin.title" placeholder="签到标题" style="width:180px" />
      <el-input-number v-model="newSignin.duration" :min="1" :max="120" placeholder="分钟" style="width:120px" />
      <span style="color:#999;font-size:12px">分钟</span>
      <el-button type="primary" @click="startSignin">发起签到</el-button>
    </div>

    <!-- 所有进行中的签到（可多个） -->
    <template v-if="activeSignins.length">
      <el-card v-for="s in activeSignins" :key="s.id" class="event-card" shadow="hover" @click="openDetail(s)">
        <template #header>
          <div class="event-head">
            <b>{{ s.title || '课堂签到' }}</b>
            <el-tag type="danger" size="small">进行中</el-tag>
          </div>
        </template>
        <p class="event-time">
          开始：{{ fmtTime(s.startAt) }} · 截止：{{ fmtTime(s.expiresAt) }}
        </p>

        <!-- 老师端：显示签到码 + 数量概览 + 结束按钮 -->
        <div v-if="isTeacher" class="teacher-panel" @click.stop>
          <div class="code-row">
            <span class="code-label">签到码：</span>
            <span class="big-code">{{ s.code || '------' }}</span>
          </div>
          <span class="stat">已签 {{ counts[s.id]?.signed ?? '-' }} 人 · 未签 {{ counts[s.id]?.unsigned ?? '-' }} 人</span>
          <el-button type="danger" size="small" @click="endSignin(s)">结束签到</el-button>
        </div>

        <!-- 学生端：输入签到码 -->
        <div v-else class="signin-student" @click.stop>
          <el-input v-model="signinCode" placeholder="输入老师给的6位签到码" style="width:220px" maxlength="6" />
          <el-button type="primary" style="margin-left:8px" @click="doSignin(s)">签到</el-button>
        </div>
      </el-card>
    </template>
    <el-empty v-else description="当前没有进行中的签到" />

    <!-- 历史签到（已结束），点卡片看名单 -->
    <template v-if="filteredHistory.length">
      <el-divider content-position="left">历史签到</el-divider>
      <el-card
        v-for="h in filteredHistory"
        :key="h.id"
        class="history-card"
        shadow="hover"
        @click="openDetail(h)"
      >
        <div class="event-head">
          <b>{{ h.title || '课堂签到' }}</b>
          <el-tag size="small" type="info">已结束</el-tag>
        </div>
        <p class="event-time">
          {{ fmtTime(h.startAt) }} ~ {{ fmtTime(h.expiresAt) }}
          <span class="stat" style="margin-left:12px">已签 {{ counts[h.id]?.signed ?? '-' }} 人</span>
        </p>
      </el-card>
    </template>

    <!-- 签到详情弹窗：名单 -->
    <el-dialog v-model="detailVisible" :title="currentDetail?.title || '签到详情'" width="600px">
      <template v-if="currentDetail">
        <p>
          时间：{{ fmtTime(currentDetail.startAt) }} ~ {{ fmtTime(currentDetail.expiresAt) }}<br/>
          签到码：<b>{{ currentDetail.code }}</b>
        </p>
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
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, reactive, computed, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import axios from 'axios'

const props = defineProps({
  classId: { type: [String, Number], required: true },
  isTeacher: { type: Boolean, default: false },
})

const token = localStorage.getItem('token')
const auth = { headers: { Authorization: 'Bearer ' + token } }

const historyList = ref([])
const signinCode = ref('')
const newSignin = ref({ title: '', duration: 5 })

// 每个签到的已签/未签数量缓存 { [signinId]: {signed, unsigned} }
const counts = reactive({})

// 详情弹窗
const detailVisible = ref(false)
const currentDetail = ref(null)
const signedList = ref([])
const unsignedList = ref([])

function fmtTime(t) {
  if (!t) return ''
  return String(t).replace('T', ' ').slice(0, 16)
}
function isExpired(t) {
  return new Date(t) < new Date()
}

const activeSignins = computed(() => historyList.value.filter(s => !isExpired(s.expiresAt)))
const filteredHistory = computed(() => historyList.value.filter(s => isExpired(s.expiresAt)))

async function loadSignin() {
  try {
    const h = await axios.get(`/api/class/${props.classId}/signin/history`, auth)
    historyList.value = h.data || []
    // 老师端：批量拉每个签到的已签/未签数量
    if (props.isTeacher) {
      historyList.value.forEach(s => loadCounts(s.id))
    }
  } catch (e) {
    historyList.value = []
  }
}

// 只拉数量，不展开名单
async function loadCounts(signinId) {
  try {
    const r = await axios.get(`/api/class/${props.classId}/signin/records?signinId=${signinId}`, auth)
    counts[signinId] = {
      signed: r.data?.signed?.length || 0,
      unsigned: r.data?.unsigned?.length || 0,
    }
  } catch (e) {
    counts[signinId] = { signed: '-', unsigned: '-' }
  }
}

async function openDetail(s) {
  currentDetail.value = s
  detailVisible.value = true
  if (!props.isTeacher) return
  try {
    const r = await axios.get(`/api/class/${props.classId}/signin/records?signinId=${s.id}`, auth)
    signedList.value = r.data?.signed || []
    unsignedList.value = r.data?.unsigned || []
  } catch (e) {
    signedList.value = []
    unsignedList.value = []
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

async function doSignin(s) {
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

async function endSignin(s) {
  try {
    await ElMessageBox.confirm(`确定结束签到《${s.title || '课堂签到'}》吗？`, '结束签到', {
      type: 'warning', confirmButtonText: '结束', cancelButtonText: '取消',
    })
  } catch { return }
  try {
    await axios.post(`/api/class/${props.classId}/signin/end`, {}, auth)
    ElMessage.success('签到已结束')
    loadSignin()
  } catch (e) {
    ElMessage.error(e.response?.data?.message || '结束失败')
  }
}

onMounted(loadSignin)
defineExpose({ reload: loadSignin })
</script>

<style scoped>
.toolbar { display: flex; gap: 10px; margin-bottom: 16px; align-items: center; }
.event-card { margin-bottom: 12px; cursor: pointer; }
.event-head { display: flex; justify-content: space-between; align-items: center; }
.event-time { color: #666; font-size: 13px; margin: 4px 0 12px; }
.teacher-panel { display: flex; align-items: center; gap: 16px; margin-top: 8px; }
.code-row { display: flex; align-items: baseline; gap: 6px; }
.code-label { color: #666; font-size: 13px; }
.big-code { font-size: 28px; font-weight: 700; color: #e6a23c; letter-spacing: 4px; font-family: monospace; }
.stat { color: #666; font-size: 13px; }
.signin-student { margin-top: 4px; }
.history-card { margin-bottom: 8px; cursor: pointer; }
.signin-row { padding: 4px 0; border-bottom: 1px dashed #eee; font-size: 14px; }
.signin-row.unsigned { color: #999; }
</style>
