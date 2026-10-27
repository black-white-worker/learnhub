<script setup>
// 考试详情页：老师端用 QuestionEditor 出题器（三栏），学生端用问卷式大卡片+答题卡。
import { ref, onMounted, computed, onUnmounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import QuestionEditor from '../components/QuestionEditor.vue'
import {
  getExamDetail,
  startExam,
  saveExamDraft,
  submitExam as apiSubmitExam,
  gradeExamSubmission,
  reportExamViolation,
} from '../api/index.js'

const route = useRoute()
const router = useRouter()
const examId = route.params.id

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

// 是否已交卷（学生端）：以服务端状态为准。
// status: started=考试中 / submitted=已交卷 / expired=超时被收卷
const hasSubmitted = computed(() => {
  const s = detail.value?.mySubmission?.status
  return s === 'submitted' || s === 'expired'
})

// 服务端给出的"还能不能交"：时限内 + 未交卷 + 不是老师。
// 改造前没排除老师：老师打开一场已到期的考试时，倒计时归零会触发自动交卷，
// 后端返回 403「老师无需参加考试」，于是弹出一个很生硬的提示。
// 老师本来就不该有"交卷"这个动作，这里直接从源头掐掉。
const canSubmit = computed(() =>
  !isTeacher.value && detail.value?.canSubmit !== false && !hasSubmitted.value
)

// 是否展示答案（服务端在整场结束后才下发标准答案）
const answersVisible = computed(() => detail.value?.answersVisible === true)

// 违纪（切屏）计数：服务端累加，前端只负责上报与提示
const violations = ref(0)

const studentQuestions = computed(() => {
  if (!detail.value?.questions) return []
  return detail.value.questions.map(q => {
    let options = []
    try { options = q.options ? JSON.parse(q.options) : [] } catch (e) {}
    return { ...q, options }
  })
})

// ===== 答案解析辅助函数 =====
// 把 "A,C" / "A" / '["A","C"]' 统一拆成大写字母数组
function splitLetters(s) {
  if (!s) return []
  let t = String(s).trim().replace(/^\[|\]$/g, '')
  return t.split(',').map(x => x.trim().replace(/^["']|["']$/g, '').toUpperCase()).filter(Boolean)
}

// 学生在某题上的答案数组（单选 ["A"]、多选 ["A","C"]、判断 ["true"]）
function studentPickArr(answersMap, qid) {
  const v = answersMap?.[qid]
  if (v === undefined || v === null || v === '') return []
  if (Array.isArray(v)) return v.map(x => String(x).toUpperCase())
  return splitLetters(v)
}

// 某选项字母是否被学生选了
function isPicked(pickArr, letter) {
  return pickArr.map(x => String(x).toUpperCase()).includes(String(letter).toUpperCase())
}

// 某选项字母是否在标准答案里（单选/多选）
function isCorrectAnswer(q, letter) {
  if (!q?.answer) return false
  return splitLetters(q.answer).includes(String(letter).toUpperCase())
}

// 该题实际自动得分（从后端 perQuestion 拿，保留 1 位小数）
function questionScore(qid) {
  const v = detail.value?.perQuestion?.[qid]
  if (v === undefined || v === null) return null
  return Math.round(v * 10) / 10
}

// 老师批改时：某题学生答案数组
function submissionPickArr(q) {
  return studentPickArr(submissionAnswers.value, q.id)
}

// ===== 老师端：某份提交在某一题上的得分 =====
//
// 关键：这里必须用"这份答卷自己的得分"，不能用 detail.perQuestion。
// perQuestion 是学生端自己的分数明细，老师端 Detail 不会返回它 ——
// 之前弹窗读它，导致每道学生明明选对的题都显示"得分 0"。
// 现在后端按 submissionId 返回 submissionScores，这里按 id 取。
function submissionQuestionScore(sub, qid) {
  if (!sub) return null
  const map = detail.value?.submissionScores?.[sub.id]
  if (!map) return null
  const v = map[qid]
  return v === undefined || v === null ? null : v
}

// 得分标签的文案：客观题给"得分 X / Y"，没答案记录时说"未答"而不是"0 分"
function scoreText(score, full) {
  if (score === null || score === undefined) return '未答 0'
  return `得分 ${score} / ${full}`
}

// 得分标签的颜色：满分绿、部分分黄、0 分红
function scoreTagType(score, full) {
  if (score === null || score === undefined) return 'danger'
  if (score >= full) return 'success'
  if (score > 0) return 'warning'
  return 'danger'
}

// 列表里的"客观题分"：优先用服务端重算的明细求和，
// 保证和弹窗里逐题显示的分数一致（历史答卷的 auto_score 是旧规则算的，可能对不上）。
function objectiveScore(sub) {
  if (!sub) return null
  const map = detail.value?.submissionScores?.[sub.id]
  if (!map) return sub.autoScore ?? null
  let sum = 0
  for (const k of Object.keys(map)) sum += Number(map[k]) || 0
  return Math.round(sum * 10) / 10
}

// 判断题选项字母映射
function judgeOptionValue(letter) {
  return letter === '对' ? 'true' : 'false'
}

async function load() {
  try {
    const r = await getExamDetail(examId)
    detail.value = r.data
    if (r.data.mySubmission?.answers) {
      try { answers.value = JSON.parse(r.data.mySubmission.answers) } catch (e) { answers.value = {} }
    }
    // 服务端记录的违纪次数（老师端也能看到）
    violations.value = r.data.mySubmission?.violations || 0
    if (r.data.submissions) {
      const gf = {}
      r.data.submissions.forEach(s => {
        gf[s.id] = { teacherScore: s.teacherScore ?? s.autoScore ?? 0, comment: s.comment || '' }
      })
      gradeForm.value = gf
    }
    // 重新加载时先停掉旧计时器，避免切题/刷新后叠加多个 tick
    if (timer.value) {
      clearInterval(timer.value)
      timer.value = null
    }
    await ensureStarted()
    startCountdown()
    startDraftAutosave()
  } catch (e) {
    ElMessage.error(e.response?.data?.message || '加载考试失败')
  }
}

// ===== 开始作答 =====
//
// 服务端的"个人时长"从 started_at 起算。历史前端没有"开始考试"入口，
// 学生直接进入答题区 —— 那样 started_at 永远为空，时长限制形同虚设。
// 这里在进入页面时补一次开考（接口本身幂等：重复调用不会重置时间），
// 之后拿服务端返回的 deadlineAt 作为倒计时依据。
let autoStartTried = false

async function ensureStarted() {
  if (autoStartTried) return
  autoStartTried = true
  if (isTeacher.value || hasSubmitted.value) return
  if (detail.value?.mySubmission || !canSubmit.value) return
  try {
    const r = await startExam(examId)
    if (r.data?.deadlineAt) {
      detail.value.deadlineAt = r.data.deadlineAt
    }
  } catch (e) {
    // 未到开考时间 / 已结束：保持页面现状，不打断浏览
  }
}

// ===== 作答草稿自动保存 =====
//
// 为什么需要：学生答案只存在内存里，断网/崩溃/误关页面时服务端什么都没有，
// 而超时收卷是按服务端记录判分的 —— 旧实现里这就是 0 分。
// 这里每 15 秒把"有变化"的答案存一次；内容没变不发请求。
const DRAFT_INTERVAL_MS = 15000
let draftTimer = null
let lastDraftJson = ''
const draftSavedAt = ref('')

function startDraftAutosave() {
  if (draftTimer) {
    clearInterval(draftTimer)
    draftTimer = null
  }
  // 只对"还在作答的学生"启用：老师端、已交卷、已超时都不需要
  if (isTeacher.value || hasSubmitted.value || !canSubmit.value) return
  lastDraftJson = JSON.stringify(answers.value || {})
  draftTimer = setInterval(saveDraftNow, DRAFT_INTERVAL_MS)
}

async function saveDraftNow() {
  if (isTeacher.value || hasSubmitted.value || !canSubmit.value) return
  const payload = JSON.stringify(answers.value || {})
  if (payload === lastDraftJson) return
  try {
    await saveExamDraft(examId, answers.value)
    lastDraftJson = payload
    draftSavedAt.value = new Date().toLocaleTimeString()
  } catch (e) {
    // 409 = 已经交卷/收卷；停止自动保存，避免每 15 秒刷一次错误
    if (e.response?.status === 409) {
      if (draftTimer) clearInterval(draftTimer)
      draftTimer = null
    }
    // 其余错误（网络抖动）静默重试，不打扰答题
  }
}

function startCountdown() {
  // 老师端没有"作答/交卷"概念，不启动倒计时（也就不会触发自动交卷）
  if (isTeacher.value) return
  // 用服务端下发的个人截止时间（= 开考时间 + 时长 与整场结束的较早者）。
  // 改造前用 exam.endAt，迟到进场的人会被多算时间；本地改系统时间也能骗过前端。
  const deadline = detail.value?.deadlineAt || detail.value?.exam?.endAt
  if (!deadline) return
  const end = new Date(deadline).getTime()
  const tick = () => {
    remainSeconds.value = Math.max(0, Math.floor((end - Date.now()) / 1000))
    // 到点自动交卷：服务端也有后台任务兜底，这里是让在线学生有明确落点
    if (remainSeconds.value === 0 && canSubmit.value && !submitting.value) {
      clearInterval(timer.value)
      autoSubmit()
    }
  }
  tick()
  timer.value = setInterval(tick, 1000)
}

// 倒计时归零自动交卷（不弹确认框，避免学生点"取消"继续答）
async function autoSubmit() {
  // 老师端不参与考试：这里再兜一层，防止计时器被误启动后仍然发出交卷请求
  if (isTeacher.value || hasSubmitted.value || submitting.value) return
  submitting.value = true
  try {
    await apiSubmitExam(examId, answers.value)
    ElMessage.warning('考试时间到，已自动交卷')
    await load()
  } catch (e) {
    ElMessage.error(e.response?.data?.message || '自动交卷失败，请联系老师')
  } finally {
    submitting.value = false
  }
}

// 切屏/离开页面检测（tab_limit 的落地方式）：
// 只在"考试中"统计，服务端累加，前端只做提示。
function onVisibilityChange() {
  if (document.visibilityState !== 'hidden') return
  if (isTeacher.value || hasSubmitted.value || !canSubmit.value) return
  reportExamViolation(examId)
    .then((r) => {
      violations.value = r.data?.violations || violations.value + 1
      const limit = detail.value?.exam?.tabLimit || 0
      if (limit > 0 && violations.value >= limit) {
        ElMessage.error(`已切换窗口 ${violations.value} 次，超过限制（${limit} 次），老师会收到提醒`)
      } else {
        ElMessage.warning(`检测到离开考试页面（第 ${violations.value} 次），请勿切屏`)
      }
    })
    .catch(() => {})
}
onUnmounted(() => {
  clearInterval(timer.value)
  if (draftTimer) clearInterval(draftTimer)
})

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
  if (!canSubmit.value) {
    ElMessage.warning(hasSubmitted.value ? '你已经交卷了，不能重复提交' : '已超过考试时限，无法再提交')
    return
  }
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
    const r = await apiSubmitExam(examId, answers.value)
    ElMessage.success(r.data.autoScore !== undefined ? `交卷成功，客观题得分：${r.data.autoScore}` : '交卷成功')
    await load()
  } catch (e) {
    // 409：已经交过卷（并发/另开标签页）；后端是唯一权威
    if (e.response?.status === 409) {
      ElMessage.warning('你已经交卷了，不能重复提交')
      await load()
    } else {
      ElMessage.error(e.response?.data?.message || '交卷失败')
    }
  } finally {
    submitting.value = false
  }
}

async function gradeSubmission(submissionId) {
  const g = gradeForm.value[submissionId]
  try {
    await gradeExamSubmission(submissionId, {
      teacherScore: g.teacherScore, comment: g.comment,
    })
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

onMounted(() => {
  load()
  document.addEventListener('visibilitychange', onVisibilityChange)
})
onUnmounted(() => {
  document.removeEventListener('visibilitychange', onVisibilityChange)
  clearInterval(timer.value)
})
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
          <template #default="{ row }">{{ objectiveScore(row) ?? '未判' }}</template>
        </el-table-column>
        <el-table-column prop="teacherScore" label="老师评分" width="100">
          <template #default="{ row }">{{ row.teacherScore ?? '-' }}</template>
        </el-table-column>
        <el-table-column label="切屏" width="80">
          <template #default="{ row }">
            <el-tag v-if="(row.violations || 0) > 0" size="small" type="danger">{{ row.violations }} 次</el-tag>
            <span v-else style="color:#c0c4cc">0</span>
          </template>
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
      <el-dialog v-model="showSubmission" :title="`${activeSubmission?.username} 的答卷`" width="720px" top="5vh">
        <div v-if="activeSubmission" style="max-height:65vh;overflow-y:auto">
          <div v-for="(q, i) in detail.questions" :key="q.id" style="padding:12px;border-bottom:1px solid #eee">
            <div style="font-weight:600;margin-bottom:8px">
              {{ i+1 }}. {{ q.stem }}
              <span style="color:#999;font-weight:400">({{ q.score }}分)</span>
              <el-tag v-if="['single','multiple','judge'].includes(q.type)" size="small" style="margin-left:8px"
                      :type="scoreTagType(submissionQuestionScore(activeSubmission, q.id), q.score)">
                {{ scoreText(submissionQuestionScore(activeSubmission, q.id), q.score) }}
              </el-tag>
              <el-tag v-else size="small" type="info" style="margin-left:8px">主观题 · 待老师批</el-tag>
            </div>
            <div style="margin-left:16px">
              <!-- 单选/多选：列出所有选项，正确答案绿勾，学生选了蓝勾，错选红叉 -->
              <div v-if="q.type === 'single' || q.type === 'multiple'">
                <div v-for="(opt, oi) in (typeof q.options === 'string' ? JSON.parse(q.options) : q.options)" :key="oi"
                     :style="{
                       margin:'4px 0',
                       padding:'4px 8px',
                       borderRadius:'4px',
                       background: isPicked(submissionPickArr(q), 'ABCD'[oi]) ? '#ecf5ff' : 'transparent',
                     }">
                  <span :style="{color: isCorrectAnswer(q, 'ABCD'[oi]) ? '#67c23a' : '#333', fontWeight: isCorrectAnswer(q, 'ABCD'[oi]) ? 600 : 400}">
                    {{ 'ABCD'[oi] }}. {{ opt }}
                  </span>
                  <span v-if="isCorrectAnswer(q, 'ABCD'[oi])" style="color:#67c23a;margin-left:8px">✓ 正确答案</span>
                  <span v-if="isPicked(submissionPickArr(q), 'ABCD'[oi]) && !isCorrectAnswer(q, 'ABCD'[oi])" style="color:#f56c6c;margin-left:8px">✗ 学生错选</span>
                  <span v-if="isPicked(submissionPickArr(q), 'ABCD'[oi]) && isCorrectAnswer(q, 'ABCD'[oi])" style="color:#409eff;margin-left:8px">· 学生选对</span>
                </div>
              </div>
              <!-- 判断 -->
              <div v-else-if="q.type === 'judge'">
                <div v-for="j in [
                  {label:'对', val:'true'},
                  {label:'错', val:'false'}
                ]" :key="j.val"
                     :style="{
                       margin:'4px 0',
                       padding:'4px 8px',
                       borderRadius:'4px',
                       background: isPicked(submissionPickArr(q), j.val) ? '#ecf5ff' : 'transparent',
                       color: isCorrectAnswer(q, j.val) ? '#67c23a' : '#333',
                       fontWeight: isCorrectAnswer(q, j.val) ? 600 : 400,
                     }">
                  {{ j.label === '对' ? '✓ 对' : '✗ 错' }}
                  <span v-if="isCorrectAnswer(q, j.val)" style="color:#67c23a;margin-left:8px">✓ 正确答案</span>
                  <span v-if="isPicked(submissionPickArr(q), j.val) && !isCorrectAnswer(q, j.val)" style="color:#f56c6c;margin-left:8px">✗ 学生选错</span>
                  <span v-if="isPicked(submissionPickArr(q), j.val) && isCorrectAnswer(q, j.val)" style="color:#409eff;margin-left:8px">· 学生选对</span>
                </div>
              </div>
              <!-- 主观题/填空 -->
              <div v-else>
                <div style="font-size:13px;color:#999;margin-bottom:4px">学生答案：</div>
                <div style="background:#f5f7fa;padding:8px;border-radius:4px;white-space:pre-wrap">{{ submissionAnswers[q.id] || '(未答)' }}</div>
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
        <el-tag type="danger" v-if="!hasSubmitted">剩余时间：{{ fmtRemain() }}</el-tag>
        <span v-if="!hasSubmitted && draftSavedAt" class="draft-tip">草稿已保存 {{ draftSavedAt }}</span>
        <el-button size="small" @click="showAnswerSheet = true">答题卡</el-button>
      </div>

      <!-- 已交卷：答题回顾（类似老师查看答卷） -->
      <div v-if="hasSubmitted" class="review-area">
        <el-alert type="success" :closable="false" style="margin-bottom:16px"
                  :title="`你已交卷，客观题得分：${detail.mySubmission.autoScore ?? '待老师批主观题'}${detail.mySubmission.teacherScore ? '，老师评分：' + detail.mySubmission.teacherScore : ''}`" />

        <div v-for="(q, i) in studentQuestions" :key="q.id" class="review-card">
          <div class="review-head">
            <span class="q-no">{{ i + 1 }}.</span>
            <span class="q-stem-inline">{{ q.stem }}</span>
            <el-tag v-if="answersVisible && ['single','multiple','judge'].includes(q.type)" size="small"
                    :type="scoreTagType(questionScore(q.id), q.score)">
              {{ questionScore(q.id) ?? 0 }} / {{ q.score }} 分
            </el-tag>
            <!-- 考试没结束时不下发答案，也不能显示"0 分"误导学生 -->
            <el-tag v-else-if="!answersVisible" size="small" type="info">解析将在考试结束后公布</el-tag>
          </div>

          <div class="review-body">
            <!-- 单选/多选 -->
            <div v-if="q.type === 'single' || q.type === 'multiple'">
              <div v-for="(opt, oi) in q.options" :key="oi"
                   :style="{
                     padding:'6px 10px',
                     margin:'4px 0',
                     borderRadius:'4px',
                     background: isPicked(studentPickArr(answers, q.id), 'ABCD'[oi]) ? '#ecf5ff' : 'transparent',
                   }">
                <span :style="{color: isCorrectAnswer(q, 'ABCD'[oi]) ? '#67c23a' : '#333', fontWeight: isCorrectAnswer(q, 'ABCD'[oi]) ? 600 : 400}">
                  {{ 'ABCD'[oi] }}. {{ opt }}
                </span>
                <span v-if="isCorrectAnswer(q, 'ABCD'[oi])" style="color:#67c23a;margin-left:8px">✓ 正确答案</span>
                <span v-if="isPicked(studentPickArr(answers, q.id), 'ABCD'[oi]) && !isCorrectAnswer(q, 'ABCD'[oi])" style="color:#f56c6c;margin-left:8px">✗ 你的错选</span>
                <span v-if="isPicked(studentPickArr(answers, q.id), 'ABCD'[oi]) && isCorrectAnswer(q, 'ABCD'[oi])" style="color:#409eff;margin-left:8px">· 你的选择</span>
              </div>
            </div>
            <!-- 判断 -->
            <div v-else-if="q.type === 'judge'">
              <div v-for="j in [
                {label:'对', val:'true'},
                {label:'错', val:'false'}
              ]" :key="j.val"
                   :style="{
                     padding:'6px 10px',
                     margin:'4px 0',
                     borderRadius:'4px',
                     background: isPicked(studentPickArr(answers, q.id), j.val) ? '#ecf5ff' : 'transparent',
                     color: isCorrectAnswer(q, j.val) ? '#67c23a' : '#333',
                     fontWeight: isCorrectAnswer(q, j.val) ? 600 : 400,
                   }">
                {{ j.label === '对' ? '✓ 对' : '✗ 错' }}
                <span v-if="isCorrectAnswer(q, j.val)" style="color:#67c23a;margin-left:8px">✓ 正确答案</span>
                <span v-if="isPicked(studentPickArr(answers, q.id), j.val) && !isCorrectAnswer(q, j.val)" style="color:#f56c6c;margin-left:8px">✗ 你的错选</span>
                <span v-if="isPicked(studentPickArr(answers, q.id), j.val) && isCorrectAnswer(q, j.val)" style="color:#409eff;margin-left:8px">· 你的选择</span>
              </div>
            </div>
            <!-- 填空/简答 -->
            <div v-else>
              <div style="font-size:13px;color:#999;margin-bottom:4px">你的答案：</div>
              <div style="background:#f5f7fa;padding:8px;border-radius:4px;white-space:pre-wrap">{{ answers[q.id] || '(未答)' }}</div>
            </div>
          </div>
        </div>
      </div>

      <!-- 未交卷且还能交：答题区 -->
      <div v-else-if="canSubmit" class="student-area">
        <!-- 违纪提示：服务端累计次数（tab_limit 配置时才提示阈值） -->
        <el-alert v-if="violations > 0" type="warning" :closable="false" style="margin-bottom:12px"
                  :title="`已记录 ${violations} 次离开考试页面${detail.exam.tabLimit ? '（上限 ' + detail.exam.tabLimit + ' 次）' : ''}`" />
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

      <!-- 未交卷但已过时限：不再显示答题区，避免"答完了却交不上" -->
      <div v-else class="review-area">
        <el-alert type="warning" :closable="false"
                  title="考试已结束，无法继续作答或提交。如需补交，请联系老师开启补交时间。" />
      </div>

      <!-- 答题卡弹窗 -->
      <el-dialog v-model="showAnswerSheet" title="答题卡" width="360px">
        <div class="sheet-grid">
          <div
            v-for="(q, i) in studentQuestions"
            :key="q.id"
            class="sheet-cell"
            :class="sheetCellClass(q, i)"
            @click="onSheetClick(i)"
          >
            {{ i + 1 }}
          </div>
        </div>
        <div style="margin-top:12px;color:#666;font-size:13px">
          <template v-if="hasSubmitted">
            客观题得分 {{ detail.mySubmission.autoScore ?? 0 }} 分
            <div style="margin-top:6px;font-size:12px">
              <span style="color:#67c23a">■</span> 全对
              <span style="color:#e6a23c;margin-left:8px">■</span> 部分得分
              <span style="color:#f56c6c;margin-left:8px">■</span> 错误
              <span style="color:#909399;margin-left:8px">■</span> 主观题
            </div>
          </template>
          <template v-else>
            已答 {{ studentQuestions.length - unansweredCount }} / 共 {{ studentQuestions.length }} 题
            <span v-if="unansweredCount" style="color:#f56c6c">，剩 {{ unansweredCount }} 题未答</span>
          </template>
        </div>
      </el-dialog>
    </template>
  </div>
</template>

<style scoped>
.exam-detail { padding: 0; }
.teacher-header { display: flex; align-items: center; gap: 12px; padding: 12px 16px; background: #fff; border-bottom: 1px solid #eee; }
.student-topbar { display: flex; align-items: center; gap: 12px; padding: 14px 24px; background: #fff; border-bottom: 1px solid #eee; position: sticky; top: 0; z-index: 10; }
.draft-tip { font-size: 12px; color: #9ca3af; }
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
