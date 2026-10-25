<template>
  <div>
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
const signinRecords = ref([])

async function loadSignin() {
  try {
    const r = await axios.get(`/api/class/${props.classId}/signin`, auth)
    activeSignin.value = r.data || null
    // 签名单接口要求老师权限，学生端不调，避免 403 报错
    if (activeSignin.value && props.isTeacher) {
      try {
        const rec = await axios.get(`/api/class/${props.classId}/signin/records`, auth)
        signinRecords.value = rec.data || []
      } catch (e) {
        signinRecords.value = []
      }
    } else {
      signinRecords.value = []
    }
  } catch (e) {
    activeSignin.value = null
    signinRecords.value = []
  }
}

async function startSignin() {
  try {
    await axios.post(`/api/class/${props.classId}/signin/start`, {}, auth)
    ElMessage.success('签到已发起')
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
</script>

<style scoped>
.toolbar { display: flex; gap: 10px; margin-bottom: 16px; align-items: center; }
.signin-row { padding: 4px 0; border-bottom: 1px dashed #eee; }
</style>
