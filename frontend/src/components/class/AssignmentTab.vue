<template>
  <div>
    <!-- 老师：布置作业 -->
    <div v-if="isTeacher" class="toolbar">
      <el-input v-model="newHw.title" placeholder="作业标题" style="width:180px" />
      <el-input v-model="newHw.description" placeholder="作业描述" style="width:240px" />
      <el-date-picker
        v-model="newHw.deadline"
        type="datetime"
        placeholder="截止时间（可选）"
        value-format="YYYY-MM-DDTHH:mm"
        style="width:200px"
      />
      <el-input-number v-model="newHw.totalScore" :min="1" :max="100" placeholder="总分" style="width:110px" />
      <el-button type="primary" @click="postHw">布置</el-button>
    </div>

    <!-- 作业列表 -->
    <div v-for="a in assignments" :key="a.id" class="hw-item" @click="openHw(a)">
      <b>{{ a.title }}</b>
      <p>截止：{{ formatDeadline(a.deadline) }} · 总分：{{ a.totalScore }}</p>
    </div>

    <!-- 作业详情弹窗 -->
    <el-dialog v-model="hwDialog" :title="currentHw?.title" width="760px" top="6vh">
      <p>{{ currentHw?.description }}</p>

      <!-- 题目渲染：single 单选 / judge 判断 / essay 主观题 textarea -->
      <div v-if="hwQuestions.length" class="hw-questions">
        <div v-for="(q, i) in hwQuestions" :key="q.id" class="hw-q">
          <b>{{ i + 1 }}. {{ q.stem }}（{{ q.score }}分）</b>
          <el-radio-group v-if="q.type === 'single'" v-model="myAnswers[q.id]" :disabled="isTeacher" style="margin-top:6px">
            <el-radio v-for="(opt, oi) in (q.options ? JSON.parse(q.options) : [])" :key="oi" :value="'ABCD'[oi]">
              {{ 'ABCD'[oi] }}. {{ opt }}
            </el-radio>
          </el-radio-group>
          <el-radio-group v-else-if="q.type === 'judge'" v-model="myAnswers[q.id]" :disabled="isTeacher" style="margin-top:6px">
            <el-radio value="true">正确</el-radio>
            <el-radio value="false">错误</el-radio>
          </el-radio-group>
          <el-input v-else v-model="myAnswers[q.id]" type="textarea" :rows="2" :disabled="isTeacher" placeholder="请输入你的作答" style="margin-top:6px" />
        </div>
      </div>

      <el-divider />

      <!-- 老师端：题目管理 + 提交批改 -->
      <template v-if="isTeacher">
        <el-alert type="info" :closable="false" title="老师视角：你可以添加客观题、查看学生提交并打分" style="margin-bottom:12px" />

        <h4>题目管理（当前 {{ hwQuestions.length }} 题）</h4>
        <div class="add-q-form">
          <el-select v-model="newQuestion.type" style="width:110px">
            <el-option label="单选题" value="single" />
            <el-option label="判断题" value="judge" />
            <el-option label="主观题" value="essay" />
          </el-select>
          <el-input v-model="newQuestion.stem" placeholder="题干" style="width:220px" />
          <el-input v-if="newQuestion.type === 'single'" v-model="newQuestion.optionsText" placeholder='选项用逗号分隔，如: A选项,B选项' style="width:220px" />
          <el-input v-if="newQuestion.type !== 'essay'" v-model="newQuestion.answer" placeholder="答案(A/B/C/D 或 true/false)" style="width:160px" />
          <el-input-number v-model="newQuestion.score" :min="1" :max="100" style="width:100px" />
          <el-button size="small" type="primary" @click="addQuestion">添加题目</el-button>
        </div>

        <el-divider />
        <h4>学生提交（{{ submissions.length }} 人）</h4>
        <el-table :data="submissions" size="small" max-height="280">
          <el-table-column prop="username" label="学生" width="100" />
          <el-table-column prop="content" label="文字作答" show-overflow-tooltip />
          <el-table-column label="附件" width="100">
            <template #default="{ row }">
              <a v-if="row.fileUrl" :href="row.fileUrl" target="_blank">查看附件</a>
              <span v-else>-</span>
            </template>
          </el-table-column>
          <el-table-column prop="autoScore" label="客观题分" width="90" />
          <el-table-column label="手打分" width="120">
            <template #default="{ row }">
              <el-input-number v-model="row._grade" :min="0" :max="currentHw?.totalScore || 100" size="small" style="width:90px" />
            </template>
          </el-table-column>
          <el-table-column label="评语" width="160">
            <template #default="{ row }">
              <el-input v-model="row._comment" size="small" placeholder="评语" />
            </template>
          </el-table-column>
          <el-table-column label="操作" width="90">
            <template #default="{ row }">
              <el-button size="small" type="primary" @click="gradeSubmission(row)">打分</el-button>
            </template>
          </el-table-column>
        </el-table>
      </template>

      <!-- 学生端：提交区 -->
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

      <div v-if="myScore !== null && myScore !== undefined" style="margin-top:10px">
        得分：{{ myScore }} 评语：{{ myComment || '无' }}
        <div v-if="myFileUrl" style="margin-top:6px">
          已交附件：<a :href="myFileUrl" target="_blank">查看附件</a>
        </div>
      </div>
    </el-dialog>
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

const assignments = ref([])
// 布置作业：title/description/deadline/totalScore
const newHw = ref({ title: '', description: '', deadline: '', totalScore: 100 })

const hwDialog = ref(false)
const currentHw = ref(null)
const mySubmission = ref('')
const myScore = ref(null)
const myComment = ref('')
const hwQuestions = ref([])
const myAnswers = ref({})

// 老师端：添加题目表单
const newQuestion = ref({ type: 'single', stem: '', optionsText: '', answer: '', score: 5 })
// 老师端：学生提交列表
const submissions = ref([])

// 作业附件上传
const uploading = ref(false)
const fileList = ref([])
const myFileUrl = ref('')
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
  try {
    const r = await axios.get(`/api/class/${props.classId}/assignments`, auth)
    assignments.value = r.data || []
  } catch (e) {
    assignments.value = []
  }
}

async function postHw() {
  if (!newHw.value.title.trim()) {
    ElMessage.warning('作业标题不能为空')
    return
  }
  try {
    await axios.post(`/api/class/${props.classId}/assignment`, {
      title: newHw.value.title,
      description: newHw.value.description,
      deadline: newHw.value.deadline || '',
      totalScore: newHw.value.totalScore || 100,
    }, auth)
    newHw.value = { title: '', description: '', deadline: '', totalScore: 100 }
    ElMessage.success('作业已布置')
    loadHw()
  } catch (e) {
    ElMessage.error(e.response?.data?.message || '布置失败')
  }
}

async function openHw(a) {
  currentHw.value = a
  try {
    const r = await axios.get(`/api/assignment/${a.id}`, auth)
    mySubmission.value = r.data.mySubmission?.content || ''
    // 后端 SubmissionRow.MarshalJSON 现在直接输出数字或 null
    myScore.value = r.data.mySubmission?.score ?? r.data.mySubmission?.autoScore ?? null
    myComment.value = r.data.mySubmission?.comment || ''
    myFileUrl.value = r.data.mySubmission?.fileUrl || ''
    fileList.value = myFileUrl.value
      ? [{ name: '已交附件', url: myFileUrl.value }]
      : []
    hwQuestions.value = r.data.questions || []
    myAnswers.value = {}
    if (r.data.mySubmission?.answers) {
      try { myAnswers.value = JSON.parse(r.data.mySubmission.answers) } catch (e) { myAnswers.value = {} }
    }
  } catch (e) {
    ElMessage.error(e.response?.data?.message || '加载作业详情失败')
  }
  hwDialog.value = true

  // 老师端：拉学生提交列表
  if (props.isTeacher) {
    loadSubmissions(a.id)
  }
}

// 老师：拉学生提交列表
async function loadSubmissions(aid) {
  try {
    const r = await axios.get(`/api/assignment/${aid}/submissions`, auth)
    submissions.value = (r.data || []).map(s => ({
      ...s,
      // 打分表单初始值：已打分用已有分数，否则空
      _grade: s.score ?? s.autoScore ?? null,
      _comment: s.comment || '',
    }))
  } catch (e) {
    submissions.value = []
  }
}

// 老师：添加一道客观题
async function addQuestion() {
  if (!newQuestion.value.stem.trim()) {
    ElMessage.warning('题干不能为空')
    return
  }
  if (newQuestion.value.type !== 'essay' && !newQuestion.value.answer.trim()) {
    ElMessage.warning('选择题/判断题答案不能为空')
    return
  }
  try {
    let options = ''
    if (newQuestion.value.type === 'single') {
      const arr = newQuestion.value.optionsText.split(',').map(s => s.trim()).filter(Boolean)
      if (arr.length < 2) {
        ElMessage.warning('单选题至少填两个选项，用英文逗号分隔')
        return
      }
      options = JSON.stringify(arr)
    }
    await axios.post(`/api/assignment/${currentHw.value.id}/questions`, {
      type: newQuestion.value.type,
      stem: newQuestion.value.stem,
      options,
      answer: newQuestion.value.answer.trim(),
      score: newQuestion.value.score || 5,
    }, auth)
    ElMessage.success('题目已添加')
    newQuestion.value = { type: 'single', stem: '', optionsText: '', answer: '', score: 5 }
    // 刷新题目列表
    const r = await axios.get(`/api/assignment/${currentHw.value.id}`, auth)
    hwQuestions.value = r.data.questions || []
  } catch (e) {
    ElMessage.error(e.response?.data?.message || '添加题目失败')
  }
}

// 老师：给某个学生提交打分
async function gradeSubmission(row) {
  try {
    await axios.post(`/api/assignment/submission/${row.id}/grade`, {
      score: row._grade,
      comment: row._comment,
    }, auth)
    ElMessage.success('打分成功')
    loadSubmissions(currentHw.value.id)
  } catch (e) {
    ElMessage.error(e.response?.data?.message || '打分失败')
  }
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
.toolbar { display: flex; gap: 10px; margin-bottom: 16px; align-items: center; flex-wrap: wrap; }
.hw-item { padding: 10px; border-bottom: 1px solid #eee; cursor: pointer; }
.hw-q { padding: 10px 0; border-bottom: 1px dashed #eee; }
.hw-questions { margin: 10px 0; }
.add-q-form { display: flex; gap: 8px; align-items: center; flex-wrap: wrap; margin-bottom: 8px; }
.upload-tip { color: #909399; font-size: 12px; margin-top: 4px; }
</style>
