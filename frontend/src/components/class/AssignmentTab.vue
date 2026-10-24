<template>
  <div>
    <div v-if="isTeacher" class="toolbar">
      <el-input v-model="newHw.title" placeholder="作业标题" style="width:200px" />
      <el-input v-model="newHw.description" placeholder="作业描述" style="width:300px" />
      <el-button type="primary" @click="postHw">布置</el-button>
    </div>
    <div v-for="a in assignments" :key="a.id" class="hw-item" @click="openHw(a)">
      <b>{{ a.title }}</b>
      <p>截止：{{ formatDeadline(a.deadline) }} · 总分：{{ a.totalScore }}</p>
    </div>

    <!-- 作业详情弹窗 -->
    <el-dialog v-model="hwDialog" :title="currentHw?.title" width="640px">
      <p>{{ currentHw?.description }}</p>
      <div v-if="hwQuestions.length" class="hw-questions">
        <div v-for="(q, i) in hwQuestions" :key="q.id" class="hw-q">
          <b>{{ i + 1 }}. {{ q.stem }}（{{ q.score }}分）</b>
          <el-radio-group v-model="myAnswers[q.id]" style="margin-top:6px">
            <el-radio v-for="(opt, oi) in (q.options ? JSON.parse(q.options) : [])" :key="oi" :value="'ABCD'[oi]">
              {{ 'ABCD'[oi] }}. {{ opt }}
            </el-radio>
            <el-radio v-if="!q.options" value="true">正确</el-radio>
            <el-radio v-if="!q.options" value="false">错误</el-radio>
          </el-radio-group>
        </div>
      </div>
      <el-divider />

      <!-- 老师/创建者：无需提交 -->
      <div v-if="isTeacher" class="teacher-tip">
        <el-alert type="info" :closable="false" title="你是本课程老师/创建者，无需提交作业" />
      </div>

      <!-- 学生：提交区 -->
      <template v-else>
        <el-input v-model="mySubmission" type="textarea" :rows="4" placeholder="填写作业内容（与附件至少填一项）" />
        <div style="margin-top:10px">
          <el-upload
            :action="uploadAction"
            :headers="uploadHeaders"
            :before-upload="beforeUpload"
            :on-success="onUploadSuccess"
            :on-error="onUploadError"
            :show-file-list="true"
            :limit="1"
            :file-list="fileList"
            accept=".pdf,.doc,.docx,.ppt,.pptx,.xls,.xlsx,.zip,.rar,.7z,.jpg,.jpeg,.png,.txt,.md"
          >
            <el-button size="small">上传附件</el-button>
            <template #tip>
              <div class="upload-tip">支持文档/压缩包/图片，最大 50MB</div>
            </template>
          </el-upload>
        </div>
        <el-button type="primary" style="margin-top:10px" :loading="uploading" @click="submitHw">提交</el-button>
      </template>

      <div v-if="myScore !== null" style="margin-top:10px">
        得分：{{ myScore }} 评语：{{ myComment }}
        <div v-if="myFileUrl" style="margin-top:6px">
          已交附件：<a :href="myFileUrl" target="_blank">{{ myFileName || '查看附件' }}</a>
        </div>
      </div>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import axios from 'axios'

defineProps({
  classId: { type: [String, Number], required: true },
  isTeacher: { type: Boolean, default: false },
})

const token = localStorage.getItem('token')
const auth = { headers: { Authorization: 'Bearer ' + token } }

const assignments = ref([])
const newHw = ref({ title: '', description: '' })

const hwDialog = ref(false)
const currentHw = ref(null)
const mySubmission = ref('')
const myScore = ref(null)
const myComment = ref('')
const hwQuestions = ref([])
const myAnswers = ref({})

// 作业附件上传
const uploading = ref(false)
const fileList = ref([])
const myFileUrl = ref('')
const myFileName = ref('')
const uploadAction = '/api/assignment/upload'
const uploadHeaders = { Authorization: 'Bearer ' + token }

// formatDeadline 兜底渲染截止时间：
// 后端已统一输出 "YYYY-MM-DD HH:mm" 字符串；旧数据/异常情况下可能是对象或空，统一转成"无截止时间"。
function formatDeadline(dl) {
  if (!dl) return '无截止时间'
  if (typeof dl === 'string') return dl || '无截止时间'
  if (typeof dl === 'object' && dl.Valid && dl.Time) return dl.Time
  return '无截止时间'
}

async function loadHw() {
  const r = await axios.get(`/api/class/${props.classId}/assignments`, auth)
  assignments.value = r.data || []
}

async function postHw() {
  await axios.post(`/api/class/${props.classId}/assignment`, newHw.value, auth)
  newHw.value = { title: '', description: '' }
  loadHw()
}

async function openHw(a) {
  currentHw.value = a
  const r = await axios.get(`/api/assignment/${a.id}`, auth)
  mySubmission.value = r.data.mySubmission?.content || ''
  myScore.value = r.data.mySubmission?.score ?? r.data.mySubmission?.autoScore ?? null
  myComment.value = r.data.mySubmission?.comment || ''
  myFileUrl.value = r.data.mySubmission?.fileUrl || ''
  myFileName.value = r.data.mySubmission?.fileName || ''
  fileList.value = myFileUrl.value
    ? [{ name: myFileName.value || '已交附件', url: myFileUrl.value }]
    : []
  hwQuestions.value = r.data.questions || []
  myAnswers.value = {}
  if (r.data.mySubmission?.answers) {
    try { myAnswers.value = JSON.parse(r.data.mySubmission.answers) } catch (e) { myAnswers.value = {} }
  }
  hwDialog.value = true
}

function beforeUpload(file) {
  if (file.size > 50 * 1024 * 1024) {
    ElMessage.error('附件不能超过 50MB')
    return false
  }
  uploading.value = true
  return true
}
function onUploadSuccess(res) {
  uploading.value = false
  myFileUrl.value = res.url
  myFileName.value = res.name
  fileList.value = [{ name: res.name, url: res.url }]
  ElMessage.success('附件上传成功')
}
function onUploadError() {
  uploading.value = false
  ElMessage.error('附件上传失败')
}

async function submitHw() {
  const hasText = mySubmission.value.trim() !== ''
  const hasFile = !!myFileUrl.value
  const hasAnswer = Object.keys(myAnswers.value).length > 0
  if (!hasText && !hasFile && !hasAnswer) {
    ElMessage.warning('提交内容不能为空：请填写作业文本、上传附件或作答客观题')
    return
  }
  try {
    const r = await axios.post(`/api/assignment/${currentHw.value.id}/submit`, {
      content: mySubmission.value,
      fileUrl: myFileUrl.value,
      answers: myAnswers.value,
    }, auth)
    if (r.data?.autoScore !== undefined) {
      ElMessage.success(`提交成功，客观题自动得分：${r.data.autoScore}`)
    } else {
      ElMessage.success('提交成功')
    }
    hwDialog.value = false
    loadHw()
  } catch (e) {
    ElMessage.error(e.response?.data?.message || '提交失败')
  }
}

onMounted(loadHw)
defineExpose({ reload: loadHw })
</script>

<style scoped>
.toolbar { display: flex; gap: 10px; margin-bottom: 16px; align-items: center; }
.hw-item { padding: 10px; border-bottom: 1px solid #eee; cursor: pointer; }
.hw-q { padding: 10px 0; border-bottom: 1px dashed #eee; }
.hw-questions { margin: 10px 0; }
.teacher-tip { margin: 10px 0; }
.upload-tip { color: #909399; font-size: 12px; margin-top: 4px; }
</style>
