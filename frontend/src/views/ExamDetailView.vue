<script setup>
import { ref, onMounted, computed, onUnmounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import axios from 'axios'

const route = useRoute()
const router = useRouter()
const examId = route.params.id
const token = localStorage.getItem('token')
const auth = { headers: { Authorization: 'Bearer ' + token } }

const detail = ref(null)
const answers = ref({})
const submitting = ref(false)

// 老师端：添加题目表单
const newQuestion = ref({ type: 'single', stem: '', optionsText: '', answer: '', score: 5 })
// 老师端：批阅表单
const gradeForm = ref({}) // { submissionId: {teacherScore, comment} }

const timer = ref(null)
const remainSeconds = ref(0)

const isTeacher = computed(() => detail.value?.myRole === 'teacher' || detail.value?.myRole === 'admin')

async function load() {
  try {
    const r = await axios.get(`/api/exam/${examId}`, auth)
    detail.value = r.data
    // 学生已交卷：回填答案
    if (r.data.mySubmission?.answers) {
      try { answers.value = JSON.parse(r.data.mySubmission.answers) } catch (e) { answers.value = {} }
    }
    // 老师端：初始化批阅表单
    if (r.data.submissions) {
      const gf = {}
      r.data.submissions.forEach(s => {
        gf[s.id] = { teacherScore: s.teacherScore ?? s.autoScore ?? 0, comment: s.comment || '' }
      })
      gradeForm.value = gf
    }
    startCountdown()
  } catch (e) {
    ElMessage.error(e.response?.data?.message || '加载考试失败')
  }
}

function startCountdown() {
  // 简单倒计时：从 endAt 倒推
  if (!detail.value?.exam?.endAt) return
  const end = new Date(detail.value.exam.endAt).getTime()
  const tick = () => {
    remainSeconds.value = Math.max(0, Math.floor((end - Date.now()) / 1000))
  }
  tick()
  timer.value = setInterval(tick, 1000)
}

onUnmounted(() => clearInterval(timer.value))

function fmtRemain() {
  const m = Math.floor(remainSeconds.value / 60)
  const s = remainSeconds.value % 60
  return `${m}:${String(s).padStart(2, '0')}`
}

async function submitExam() {
  try {
    await ElMessageBox.confirm('确定交卷吗？交卷后不能修改', '提示', { type: 'warning' })
  } catch { return }
  submitting.value = true
  try {
    const r = await axios.post(`/api/exam/${examId}/submit`, { answers: answers.value }, auth)
    if (r.data?.autoScore !== undefined) {
      ElMessage.success(`交卷成功，客观题得分：${r.data.autoScore}`)
    } else {
      ElMessage.success('交卷成功')
    }
    load()
  } catch (e) {
    ElMessage.error(e.response?.data?.message || '交卷失败')
  } finally {
    submitting.value = false
  }
}

async function addQuestion() {
  if (!newQuestion.value.stem.trim()) {
    ElMessage.warning('题干不能为空')
    return
  }
  try {
    let options = ''
    if (newQuestion.value.type === 'single') {
      const arr = newQuestion.value.optionsText.split(',').map(s => s.trim()).filter(Boolean)
      if (arr.length < 2) { ElMessage.warning('单选题至少两个选项'); return }
      options = JSON.stringify(arr)
    }
    await axios.post(`/api/exam/${examId}/questions`, {
      type: newQuestion.value.type,
      stem: newQuestion.value.stem,
      options,
      answer: newQuestion.value.answer,
      score: newQuestion.value.score,
    }, auth)
    ElMessage.success('题目已添加')
    newQuestion.value = { type: 'single', stem: '', optionsText: '', answer: '', score: 5 }
    load()
  } catch (e) {
    ElMessage.error(e.response?.data?.message || '添加失败')
  }
}

async function gradeSubmission(submissionId) {
  const g = gradeForm.value[submissionId]
  try {
    await axios.post(`/api/exam/submission/${submissionId}/grade`, {
      teacherScore: g.teacherScore,
      comment: g.comment,
    }, auth)
    ElMessage.success('批改成功')
    load()
  } catch (e) {
    ElMessage.error(e.response?.data?.message || '批改失败')
  }
}

onMounted(load)
</script>

<template>
  <div class="exam-detail" v-if="detail">
    <el-button @click="$router.push('/exams')" size="small">← 返回考试中心</el-button>
    <h1>{{ detail.exam.title }}</h1>
    <p class="sub">
      总分：{{ detail.exam.totalScore }} · 时长：{{ detail.exam.durationMinutes }}分钟 ·
      开始：{{ detail.exam.startAt?.replace('T', ' ').slice(0, 16) }} ·
      结束：{{ detail.exam.endAt?.replace('T', ' ').slice(0, 16) }}
      <el-tag v-if="!isTeacher" style="margin-left:12px">剩余时间：{{ fmtRemain() }}</el-tag>
    </p>

    <!-- 老师端：题目管理 -->
    <template v-if="isTeacher">
      <el-divider />
      <h3>题目管理（共 {{ detail.questions?.length || 0 }} 题）</h3>
      <div class="add-q-form">
        <el-select v-model="newQuestion.type" style="width:110px">
          <el-option label="单选题" value="single" />
          <el-option label="判断题" value="judge" />
          <el-option label="主观题" value="essay" />
        </el-select>
        <el-input v-model="newQuestion.stem" placeholder="题干" style="width:220px" />
        <el-input v-if="newQuestion.type === 'single'" v-model="newQuestion.optionsText" placeholder='选项逗号分隔' style="width:220px" />
        <el-input v-if="newQuestion.type !== 'essay'" v-model="newQuestion.answer" placeholder="答案" style="width:120px" />
        <el-input-number v-model="newQuestion.score" :min="1" :max="100" style="width:100px" />
        <el-button size="small" type="primary" @click="addQuestion">添加</el-button>
      </div>

      <h3 style="margin-top:20px">学生提交（{{ detail.submissions?.length || 0 }} 人）</h3>
      <el-table :data="detail.submissions" size="small" max-height="400">
        <el-table-column prop="username" label="学生" width="100" />
        <el-table-column prop="autoScore" label="客观题分" width="90" />
        <el-table-column label="主观题作答" min-width="200">
          <template #default="{ row }">
            <span v-if="row.answers">{{ row.answers }}</span>
            <span v-else>-</span>
          </template>
        </el-table-column>
        <el-table-column label="手打分" width="140">
          <template #default="{ row }">
            <el-input-number v-model="gradeForm[row.id].teacherScore" :min="0" :max="detail.exam.totalScore" size="small" style="width:110px" />
          </template>
        </el-table-column>
        <el-table-column label="评语" width="180">
          <template #default="{ row }">
            <el-input v-model="gradeForm[row.id].comment" size="small" />
          </template>
        </el-table-column>
        <el-table-column label="操作" width="80">
          <template #default="{ row }">
            <el-button size="small" type="primary" @click="gradeSubmission(row.id)">批改</el-button>
          </template>
        </el-table-column>
      </el-table>
    </template>

    <!-- 学生端：作答 -->
    <template v-else>
      <el-divider />
      <div v-if="detail.mySubmission" class="already-submitted">
        <el-alert type="success" :closable="false" :title="`你已交卷，客观题得分：${detail.mySubmission.autoScore ?? '待老师批主观题'}`" />
      </div>
      <div v-for="(q, i) in detail.questions" :key="q.id" class="q-block">
        <b>{{ i + 1 }}. {{ q.stem }}（{{ q.score }}分）</b>
        <el-radio-group v-if="q.type === 'single'" v-model="answers[q.id]" style="margin-top:6px">
          <el-radio v-for="(opt, oi) in (q.options ? JSON.parse(q.options) : [])" :key="oi" :value="'ABCD'[oi]">
            {{ 'ABCD'[oi] }}. {{ opt }}
          </el-radio>
        </el-radio-group>
        <el-radio-group v-else-if="q.type === 'judge'" v-model="answers[q.id]" style="margin-top:6px">
          <el-radio value="true">正确</el-radio>
          <el-radio value="false">错误</el-radio>
        </el-radio-group>
        <el-input v-else v-model="answers[q.id]" type="textarea" :rows="3" placeholder="请作答" style="margin-top:6px" />
      </div>
      <el-button v-if="!detail.mySubmission" type="primary" :loading="submitting" style="margin-top:16px" @click="submitExam">交卷</el-button>
    </template>
  </div>
</template>

<style scoped>
.exam-detail { padding: 24px; max-width: 900px; margin: 0 auto; }
.sub { color: #666; }
.add-q-form { display: flex; gap: 8px; align-items: center; flex-wrap: wrap; margin-bottom: 10px; }
.q-block { padding: 12px 0; border-bottom: 1px dashed #eee; }
.already-submitted { margin-bottom: 16px; }
</style>
