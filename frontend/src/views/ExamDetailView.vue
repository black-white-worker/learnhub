<script setup>
// 考试详情页：老师端用 QuestionEditor 出题器（三栏），学生端用问卷式大卡片+答题卡。
import { ref, onMounted, computed, onUnmounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import axios from 'axios'
import QuestionEditor from '../components/QuestionEditor.vue'

const route = useRoute()
const router = useRouter()
const examId = route.params.id
const token = localStorage.getItem('token')
const auth = { headers: { Authorization: 'Bearer ' + token } }

const detail = ref(null)
const answers = ref({})
const submitting = ref(false)
const currentQ = ref(0) // 学生端当前题号
const showAnswerSheet = ref(false)

// 老师端批阅表单
const gradeForm = ref({})
const showSubmission = ref(false)
const activeSubmission = ref(null)

const timer = ref(null)
const remainSeconds = ref(0)

const isTeacher = computed(() => detail.value?.myRole === 'teacher' || detail.value?.myRole === 'admin')

const studentQuestions = computed(() => {
  if (!detail.value?.questions) return []
  return detail.value.questions.map(q => {
    let options = []
    try { options = q.options ? JSON.parse(q.options) : [] } catch (e) {}
    return { ...q, options }
  })
})

async function load() {
  try {
    const r = await axios.get(`/api/exam/${examId}`, auth)
    detail.value = r.data
    if (r.data.mySubmission?.answers) {
      try { answers.value = JSON.parse(r.data.mySubmission.answers) } catch (e) { answers.value = {} }
    }
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

// 学生端：判断某题是否已答
function isAnswered(q) {
  const v = answers.value[q.id]
  if (v === undefined || v === null || v === '') return false
  if (Array.isArray(v)) return v.length > 0
  return true
}

const unansweredCount = computed(() => {
  return studentQuestions.value.filter(q => !isAnswered(q)).length
})

async function submitExam() {
  if (unansweredCount.value > 0) {
    try {
      await ElMessageBox.confirm(`还有 ${unansweredCount.value} 题未作答，确定提交吗？`, '提示', { type: 'warning' })
    } catch { return }
  } else {
    try {
      await ElMessageBox.confirm('确定交卷吗？交卷后不能修改', '提示', { type: 'warning' })
    } catch { return }
  }
  submitting.value = true
  try {
    const r = await axios.post(`/api/exam/${examId}/submit`, { answers: answers.value }, auth)
    ElMessage.success(r.data.autoScore !== undefined ? `交卷成功，客观题得分：${r.data.autoScore}` : '交卷成功')
    load()
  } catch (e) {
    ElMessage.error(e.response?.data?.message || '交卷失败')
  } finally {
    submitting.value = false
  }
}

async function gradeSubmission(submissionId) {
  const g = gradeForm.value[submissionId]
  try {
    await axios.post(`/api/exam/submission/${submissionId}/grade`, {
      teacherScore: g.teacherScore, comment: g.comment,
    }, auth)
    ElMessage.success('批改成功')
    load()
    showSubmission.value = false
  } catch (e) {
    ElMessage.error(e.response?.data?.message || '批改失败')
  }
}

// 打开学生答卷详情
function openSubmission(row) {
  activeSubmission.value = row
  showSubmission.value = true
}

// 解析学生答案为 {qid: value}
const submissionAnswers = computed(() => {
  if (!activeSubmission.value?.answers) return {}
  try { return JSON.parse(activeSubmission.value.answers) } catch (e) { return {} }
})

onMounted(load)
</script>

<template>
  <div class="exam-detail" v-if="detail">
    <!-- 老师端：三栏出题器 -->
    <template v-if="isTeacher">
      <div class="teacher-header">
        <el-button @click="$router.back()" size="small">← 返回</el-button>
        <h2 style="margin:0">{{ detail.exam.title }}</h2>
      </div>
      <QuestionEditor
        module="exam"
        :resource-id="examId"
        :initial-questions="detail.questions"
        :initial-settings="{
          title: detail.exam.title,
          description: detail.exam.description,
          startAt: (detail.exam.startAt || '').replace(' ', 'T').slice(0, 16),
          endAt: (detail.exam.endAt || '').replace(' ', 'T').slice(0, 16),
          durationMinutes: detail.exam.durationMinutes,
          totalScore: detail.exam.totalScore,
          allowLate: detail.exam.allowLate,
          tabLimit: detail.exam.tabLimit,
        }"
        :available-types="['single','multiple','judge','fill','essay']"
      />

      <!-- 老师端：提交列表 -->
      <el-divider />
      <h3 style="margin-left:12px">学生提交（{{ detail.submissions?.length || 0 }} 人）</h3>
      <el-table :data="detail.submissions" size="small" style="margin:0 12px" max-height="400">
        <el-table-column label="学生" width="120">
          <template #default="{ row }">
            <el-link type="primary" @click="openSubmission(row)">{{ row.username }}</el-link>
          </template>
        </el-table-column>
        <el-table-column prop="autoScore" label="客观题分" width="100">
          <template #default="{ row }">{{ row.autoScore ?? '未判' }}</template>
        </el-table-column>
        <el-table-column prop="teacherScore" label="老师评分" width="100">
          <template #default="{ row }">{{ row.teacherScore ?? '-' }}</template>
        </el-table-column>
        <el-table-column prop="submittedAt" label="提交时间" min-width="180">
          <template #default="{ row }">{{ row.submittedAt ? new Date(row.submittedAt).toLocaleString() : '-' }}</template>
        </el-table-column>
        <el-table-column label="操作" width="120">
          <template #default="{ row }">
            <el-button size="small" type="primary" @click="openSubmission(row)">查看答卷</el-button>
          </template>
        </el-table-column>
      </el-table>

      <!-- 学生答卷详情弹窗 -->
      <el-dialog v-model="showSubmission" :title="`${activeSubmission?.username} 的答卷`" width="700px" top="5vh">
        <div v-if="activeSubmission" style="max-height:60vh;overflow-y:auto">
          <div v-for="(q, i) in detail.questions" :key="q.id" style="padding:12px;border-bottom:1px solid #eee">
            <div style="font-weight:600;margin-bottom:8px">{{ i+1 }}. {{ q.stem }} <span style="color:#999;font-weight:400">({{ q.score }}分)</span></div>
            <div style="margin-left:16px">
              <div v-if="q.type === 'single' || q.type === 'multiple'">
                <div v-for="(opt, oi) in (typeof q.options === 'string' ? JSON.parse(q.options) : q.options)" :key="oi"
                     :style="{color: submissionAnswers[q.id] === 'ABCD'[oi] ? '#409eff' : '#333', margin:'4px 0'}">
                  {{ 'ABCD'[oi] }}. {{ opt }}
                  <span v-if="submissionAnswers[q.id] === 'ABCD'[oi]" style="color:#67c23a;margin-left:8px">✓ 学生选了这个</span>
                </div>
              </div>
              <div v-else-if="q.type === 'judge'">
                <div style="color:{{submissionAnswers[q.id]==='true'?'#409eff':'#333'}}">✓ 对</div>
                <div style="color:{{submissionAnswers[q.id]==='false'?'#409eff':'#333'}}">✗ 错</div>
              </div>
              <div v-else>
                <div style="background:#f5f7fa;padding:8px;border-radius:4px;white-space:pre-wrap">学生答案：{{ submissionAnswers[q.id] || '(未答)' }}</div>
              </div>
            </div>
          </div>
        </div>
        <template #footer>
          <div style="display:flex;gap:12px;align-items:center">
            <span style="color:#666">老师评分：</span>
            <el-input-number v-model="gradeForm[activeSubmission.id].teacherScore" :min="0" :max="detail.exam.totalScore" size="small" />
            <el-input v-model="gradeForm[activeSubmission.id].comment" placeholder="评语" size="small" style="flex:1" />
            <el-button @click="showSubmission = false">关闭</el-button>
            <el-button type="primary" @click="gradeSubmission(activeSubmission.id)">保存批改</el-button>
          </div>
        </template>
      </el-dialog>
    </template>

    <!-- 学生端：问卷式大卡片 -->
    <template v-else>
      <div class="student-topbar">
        <el-button @click="$router.back()" size="small">← 返回</el-button>
        <h2 style="margin:0">{{ detail.exam.title }}</h2>
        <el-tag type="danger" v-if="!detail.mySubmission">剩余时间：{{ fmtRemain() }}</el-tag>
        <el-button size="small" @click="showAnswerSheet = true">答题卡</el-button>
      </div>

      <div v-if="detail.mySubmission" class="already-submitted">
        <el-alert type="success" :closable="false" :title="`你已交卷，客观题得分：${detail.mySubmission.autoScore ?? '待老师批主观题'}`" />
      </div>

      <div v-else class="student-area">
        <!-- 当前题 -->
        <div v-if="studentQuestions.length" class="q-card">
          <div class="q-head">
            <span class="q-no">{{ currentQ + 1 }} / {{ studentQuestions.length }}</span>
            <span v-if="studentQuestions[currentQ].required" class="q-star">*</span>
            <span class="q-score">{{ studentQuestions[currentQ].score }}分</span>
          </div>
          <div class="q-stem">{{ studentQuestions[currentQ].stem }}</div>

          <el-radio-group v-if="studentQuestions[currentQ].type === 'single'" v-model="answers[studentQuestions[currentQ].id]" class="q-options">
            <el-radio v-for="(opt, oi) in studentQuestions[currentQ].options" :key="oi" :value="'ABCD'[oi]" class="q-option-btn">
              <b>{{ 'ABCD'[oi] }}.</b> {{ opt }}
            </el-radio>
          </el-radio-group>

          <el-checkbox-group v-else-if="studentQuestions[currentQ].type === 'multiple'"
            :model-value="answers[studentQuestions[currentQ].id] ? answers[studentQuestions[currentQ].id].split(',').filter(Boolean) : []"
            @change="val => answers[studentQuestions[currentQ].id] = val.sort().join(',')"
            class="q-options">
            <el-checkbox v-for="(opt, oi) in studentQuestions[currentQ].options" :key="oi" :value="'ABCD'[oi]" class="q-option-btn">
              <b>{{ 'ABCD'[oi] }}.</b> {{ opt }}
            </el-checkbox>
          </el-checkbox-group>

          <el-radio-group v-else-if="studentQuestions[currentQ].type === 'judge'" v-model="answers[studentQuestions[currentQ].id]" class="q-options">
            <el-radio value="true" class="q-option-btn">✓ 对</el-radio>
            <el-radio value="false" class="q-option-btn">✗ 错</el-radio>
          </el-radio-group>

          <el-input v-else v-model="answers[studentQuestions[currentQ].id]" type="textarea" :rows="6"
            :placeholder="studentQuestions[currentQ].type === 'fill' ? '请填写答案' : '请输入你的作答...'" />
        </div>

        <!-- 底部导航 -->
        <div class="q-nav">
          <el-button :disabled="currentQ === 0" @click="currentQ--">上一题</el-button>
          <template v-if="currentQ < studentQuestions.length - 1">
            <el-button type="primary" @click="currentQ++">下一题</el-button>
          </template>
          <template v-else>
            <el-button type="success" :loading="submitting" @click="submitExam">提交试卷</el-button>
          </template>
        </div>
      </div>

      <!-- 答题卡弹窗 -->
      <el-dialog v-model="showAnswerSheet" title="答题卡" width="320px">
        <div class="sheet-grid">
          <div
            v-for="(q, i) in studentQuestions"
            :key="q.id"
            class="sheet-cell"
            :class="{ answered: isAnswered(q), current: i === currentQ }"
            @click="currentQ = i; showAnswerSheet = false"
          >
            {{ i + 1 }}
          </div>
        </div>
        <div style="margin-top:12px;color:#666;font-size:13px">
          已答 {{ studentQuestions.length - unansweredCount }} / 共 {{ studentQuestions.length }} 题
          <span v-if="unansweredCount" style="color:#f56c6c">，剩 {{ unansweredCount }} 题未答</span>
        </div>
      </el-dialog>
    </template>
  </div>
</template>

<style scoped>
.exam-detail { padding: 0; }
.teacher-header { display: flex; align-items: center; gap: 12px; padding: 12px 16px; background: #fff; border-bottom: 1px solid #eee; }
.student-topbar { display: flex; align-items: center; gap: 12px; padding: 14px 24px; background: #fff; border-bottom: 1px solid #eee; position: sticky; top: 0; z-index: 10; }
.student-area { max-width: 800px; margin: 24px auto; padding: 0 16px; }
.already-submitted { margin: 16px; }
.q-card { background: #fff; border-radius: 12px; padding: 28px; box-shadow: 0 2px 12px rgba(0,0,0,.06); min-height: 360px; }
.q-head { display: flex; align-items: center; gap: 10px; margin-bottom: 16px; }
.q-no { font-size: 18px; font-weight: 700; color: #409eff; }
.q-star { color: #f56c6c; font-size: 18px; }
.q-score { margin-left: auto; color: #999; font-size: 13px; }
.q-stem { font-size: 17px; line-height: 1.7; margin-bottom: 20px; }
.q-options { display: flex; flex-direction: column; gap: 10px; }
.q-option-btn { display: flex; align-items: center; padding: 12px 16px; border: 1px solid #e4e7ed; border-radius: 8px; margin-right: 0; white-space: normal; height: auto; }
.q-nav { display: flex; justify-content: space-between; margin-top: 20px; }
.sheet-grid { display: grid; grid-template-columns: repeat(5, 1fr); gap: 8px; }
.sheet-cell { aspect-ratio: 1; display: flex; align-items: center; justify-content: center; background: #f0f2f5; border-radius: 6px; cursor: pointer; font-size: 14px; }
.sheet-cell.answered { background: #67c23a; color: #fff; }
.sheet-cell.current { outline: 2px solid #409eff; }
</style>
