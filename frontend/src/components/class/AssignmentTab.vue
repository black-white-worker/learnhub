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

    <!-- 作业列表（问题8：卡片式） -->
    <div class="hw-list">
      <el-card
        v-for="a in assignments"
        :key="a.id"
        class="hw-card"
        shadow="hover"
        @click="openHw(a)"
      >
        <b class="hw-card-title">{{ a.title }}</b>
        <p class="hw-card-meta">截止：{{ formatDeadline(a.deadline) }} · 总分：{{ a.totalScore }}</p>
      </el-card>
    </div>

    <!-- 作业详情弹窗 -->
    <el-dialog v-model="hwDialog" :title="currentHw?.title" width="760px" top="6vh">
      <p>{{ currentHw?.description }}</p>

      <!-- 题目渲染：single 单选 / multiple 多选 / judge 判断 / essay 主观题 textarea -->
      <div v-if="hwQuestions.length" class="hw-questions">
        <div v-for="(q, i) in hwQuestions" :key="q.id" class="hw-q">
          <b>{{ i + 1 }}. {{ q.stem }}（{{ q.score }}分）</b>
          <el-radio-group v-if="q.type === 'single'" v-model="myAnswers[q.id]" :disabled="isTeacher" style="margin-top:6px">
            <el-radio v-for="(opt, oi) in (q.options ? JSON.parse(q.options) : [])" :key="oi" :value="'ABCD'[oi]">
              {{ 'ABCD'[oi] }}. {{ opt }}
            </el-radio>
          </el-radio-group>
          <el-checkbox-group v-else-if="q.type === 'multiple'" :model-value="parseMultiAns(myAnswers[q.id])" @change="val => myAnswers[q.id] = [...val].sort().join(',')" :disabled="isTeacher" style="margin-top:6px">
            <el-checkbox v-for="(opt, oi) in (q.options ? JSON.parse(q.options) : [])" :key="oi" :value="'ABCD'[oi]">
              {{ 'ABCD'[oi] }}. {{ opt }}
            </el-checkbox>
          </el-checkbox-group>
          <el-radio-group v-else-if="q.type === 'judge'" v-model="myAnswers[q.id]" :disabled="isTeacher" style="margin-top:6px">
            <el-radio value="true">对</el-radio>
            <el-radio value="false">错</el-radio>
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
            <el-option label="多选题" value="multiple" />
            <el-option label="判断题" value="judge" />
            <el-option label="主观题" value="essay" />
          </el-select>
          <el-input v-model="newQuestion.stem" placeholder="题干" style="width:220px" />
          <el-input v-if="newQuestion.type==='single'||newQuestion.type==='multiple'" v-model="newQuestion.optionsText" placeholder='选项用逗号分隔' style="width:220px" />
          <el-radio-group v-if="newQuestion.type==='judge'" v-model="newQuestion.answer">
            <el-radio value="true">对</el-radio>
            <el-radio value="false">错</el-radio>
          </el-radio-group>
          <el-input v-else-if="newQuestion.type!=='essay'" v-model="newQuestion.answer" :placeholder="newQuestion.type==='multiple'?'答案字母逗号分隔':'答案(A/B/C/D)'" style="width:160px" />
          <el-input-number v-model="newQuestion.score" :min="1" :max="100" style="width:100px" />
          <el-button size="small" type="primary" @click="addQuestion">添加题目</el-button>
        </div>

        <el-divider />
        <div style="display:flex;align-items:center;justify-content:space-between;gap:12px">
          <h4>学生提交（{{ submissions.length }} 人）</h4>
          <!-- 逐份批阅（SpeedGrader）：一次取回整份队列，本地上下翻，不必逐份开关弹窗 -->
          <el-button
            v-if="currentHw"
            size="small"
            type="success"
            @click="$router.push(`/assignment/${currentHw.id}/grade`)"
          >
            逐份批阅
          </el-button>
        </div>
        <el-table :data="submissions" size="small" max-height="280">
          <el-table-column prop="username" label="学生" width="120" />
          <el-table-column prop="autoScore" label="客观题自动分" width="120" />
          <el-table-column label="总分" width="100">
            <template #default="{ row }">
              {{ displayAssignmentScore(row) }}
            </template>
          </el-table-column>
          <el-table-column label="状态" width="100">
            <template #default="{ row }">
              <el-tag v-if="row.teacherScore !== null && row.teacherScore !== undefined" type="success" size="small">已批改</el-tag>
              <el-tag v-else type="warning" size="small">待批改</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="操作" width="120">
            <template #default="{ row }">
              <el-button size="small" type="primary" @click="openSubmission(row)">查看答卷</el-button>
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

    <!-- 老师端：单个学生答卷弹窗 -->
    <el-dialog v-model="submissionDialog" :title="`答卷：${currentSubmission?.username || ''}`" width="820px" top="5vh">
      <div v-if="currentSubmission" class="paper">
        <div class="paper-head">
          <span>客观题自动分：<b>{{ currentSubmission.autoScore || 0 }}</b></span>
          <span style="margin-left:20px">总分：<b>{{ displayAssignmentScore(currentSubmission) }}</b></span>
          <a v-if="currentSubmission.fileUrl" :href="currentSubmission.fileUrl" target="_blank" style="margin-left:20px">查看附件</a>
        </div>

        <el-divider />

        <div v-for="(q, i) in hwQuestions" :key="q.id" class="paper-q">
          <div class="paper-q-stem">
            <b>{{ i + 1 }}. {{ q.stem }}</b>
            <span class="paper-q-score">（{{ q.score }}分）</span>
            <el-tag v-if="q.type === 'single'" size="small" type="info" style="margin-left:8px">单选</el-tag>
            <el-tag v-else-if="q.type === 'multiple'" size="small" type="info" style="margin-left:8px">多选</el-tag>
            <el-tag v-else-if="q.type === 'judge'" size="small" type="info" style="margin-left:8px">判断</el-tag>
            <el-tag v-else-if="q.type === 'fileupload'" size="small" type="info" style="margin-left:8px">附件</el-tag>
            <el-tag v-else size="small" type="warning" style="margin-left:8px">主观</el-tag>
          </div>

          <div class="paper-q-answer">
            <template v-if="q.type === 'single' || q.type === 'multiple'">
              学生答案：<b>{{ paperAnswer(q.id) || '未答' }}</b>
              <span style="color:#999;margin-left:12px">正确答案：{{ q.answer }}</span>
              <el-tag v-if="objectiveVerdict(paperAnswer(q.id), q.answer, q.type) === 'correct'" type="success" size="small" style="margin-left:8px">✓ 答对</el-tag>
              <el-tag v-else-if="objectiveVerdict(paperAnswer(q.id), q.answer, q.type) === 'wrong'" type="danger" size="small" style="margin-left:8px">✗ 答错</el-tag>
            </template>
            <template v-else-if="q.type === 'judge'">
              学生答案：<b>{{ paperAnswer(q.id) === 'true' ? '正确' : paperAnswer(q.id) === 'false' ? '错误' : '未答' }}</b>
              <span style="color:#999;margin-left:12px">正确答案：{{ q.answer === 'true' ? '正确' : '错误' }}</span>
              <el-tag v-if="objectiveVerdict(paperAnswer(q.id), q.answer, q.type) === 'correct'" type="success" size="small" style="margin-left:8px">✓ 答对</el-tag>
              <el-tag v-else-if="objectiveVerdict(paperAnswer(q.id), q.answer, q.type) === 'wrong'" type="danger" size="small" style="margin-left:8px">✗ 答错</el-tag>
            </template>
            <template v-else-if="q.type === 'fileupload'">
              <div style="margin-top:6px">学生作答：</div>
              <div class="paper-essay-box">{{ paperAnswer(q.id) || '（未作答）' }}</div>
            </template>
            <template v-else>
              <div style="margin-top:6px">学生作答：</div>
              <div class="paper-essay-box">{{ paperAnswer(q.id) || '（未作答）' }}</div>
            </template>
          </div>
        </div>

        <el-divider />

        <div v-if="currentSubmission.content" style="margin-bottom:12px">
          <b>额外文字作答：</b>
          <div class="paper-essay-box">{{ currentSubmission.content }}</div>
        </div>

        <!-- 主观题 AI 初评：与逐份批阅（SpeedGrader）同一个面板、同一套接口、同一份建议数据 -->
        <div v-if="currentSubmission" class="ai-block">
          <AiGradePanel
            :submission-id="currentSubmission.id"
            :questions="hwQuestions"
            :initial-grade="currentAiGrade()"
            :max-score="maxScoreOfHw()"
            :objective-score="currentSubmission.autoScore || 0"
            :stored-scores="currentSubmission._storedScores"
            :manual-types="['essay', 'fileupload']"
            :ai-enabled="queueMetaOf(currentHw && currentHw.id).aiEnabled"
            @adopt="onAdoptAi"
            @change="onAiScoresChange"
          />
        </div>
      </div>

      <template #footer>
        <div style="display:flex;gap:12px;align-items:center;flex-wrap:wrap">
          <span style="white-space:nowrap">总分：</span>
          <el-input-number v-model="currentSubmission._score" :min="0" :max="maxScoreOfHw()" size="small" style="width:110px" />
          <span style="color:#999;font-size:12px;white-space:nowrap">/ {{ maxScoreOfHw() }}（含客观题自动分 {{ currentSubmission.autoScore ?? 0 }}）</span>
          <el-input v-model="currentSubmission._comment" placeholder="给学生的评语" style="flex:1;min-width:160px" />
        </div>
        <div style="display:flex;justify-content:flex-end;gap:10px;margin-top:10px">
          <el-button @click="submissionDialog = false">关闭</el-button>
          <el-button type="primary" @click="saveGrade">保存批改</el-button>
        </div>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import axios from 'axios'
import { displayAssignmentScore } from '../../utils/assignmentScore.js'
import { mergeComment, objectiveVerdict } from '../../utils/aiGrade'
import AiGradePanel from './AiGradePanel.vue'

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

// 老师端：答卷弹窗
const submissionDialog = ref(false)
const currentSubmission = ref(null)

// 答卷弹窗与逐份批阅（SpeedGrader）共用同一份数据口径：
// 打开弹窗时拉一次 grade-queue，拿 aiEnabled / aiGrades 摘要 / maxScore（按作业缓存）。
// 这样「查看答卷」里的 AI 初评、满分上限与逐份批阅页完全一致。
const gradeQueueMeta = ref({}) // hwId → { aiEnabled, aiGrades, maxScore }

async function loadQueueMeta(hwId) {
  if (gradeQueueMeta.value[hwId]) return gradeQueueMeta.value[hwId]
  try {
    const r = await axios.get(`/api/assignment/${hwId}/grade-queue`, auth)
    const meta = {
      aiEnabled: Boolean(r.data.aiEnabled),
      aiGrades: r.data.aiGrades || {},
      maxScore: r.data.maxScore || currentHw.value?.totalScore || 100,
    }
    gradeQueueMeta.value = { ...gradeQueueMeta.value, [hwId]: meta }
    return meta
  } catch (e) {
    // 拉不到队列（权限 / 网络波动）：AI 面板置灰、满分退回作业总分，不打断批改
    const meta = { aiEnabled: false, aiGrades: {}, maxScore: currentHw.value?.totalScore || 100 }
    gradeQueueMeta.value = { ...gradeQueueMeta.value, [hwId]: meta }
    return meta
  }
}

function queueMetaOf(hwId) {
  return gradeQueueMeta.value[hwId] || { aiEnabled: false, aiGrades: {}, maxScore: currentHw.value?.totalScore || 100 }
}

// 当前查看提交的 AI 初评摘要（与 SpeedGrader 的 aiGradeOf 同构）
function currentAiGrade() {
  if (!currentSubmission.value || !currentHw.value) return null
  const grades = queueMetaOf(currentHw.value.id).aiGrades || {}
  // Go 的 map[int64] 序列化后 key 是字符串
  return grades[currentSubmission.value.id] || grades[String(currentSubmission.value.id)] || null
}

function maxScoreOfHw() {
  return queueMetaOf(currentHw.value?.id).maxScore
}

// 「采纳」口径三处统一（答卷弹窗 / SpeedGrader / 考试批改）：
// 面板按「客观题自动分 + 各主观题最终分」算好总分，这里只落进表单；
// 逐题分一起存，重新打开这份答卷时才能回填老师改过的每道题。
function applyAiScores({ score, scores }) {
  if (!currentSubmission.value) return
  if (score !== null && score !== undefined) {
    const max = maxScoreOfHw()
    currentSubmission.value._score = Math.min(Math.max(Math.round(Number(score) || 0), 0), max)
  }
  if (scores) currentSubmission.value._questionScores = { ...scores }
}

// 老师改任一题的最终分，总分输入框实时跟着重算
function onAiScoresChange(payload) {
  applyAiScores(payload)
}

function onAdoptAi({ score, scores, comment }) {
  applyAiScores({ score, scores })
  if (!currentSubmission.value || !comment) return
  const c = currentSubmission.value
  c._comment = mergeComment(c._comment, comment, c._aiComment)
  c._aiComment = comment
}

// 作业附件上传
const uploading = ref(false)
const fileList = ref([])
const myFileUrl = ref('')
const uploadAction = '/api/assignment/upload'
const uploadHeaders = { Authorization: 'Bearer ' + token }

// formatDeadline 兜底渲染截止时间
function formatDeadline(dl) {
  if (!dl) return '无截止时间'
  if (typeof dl === 'string') return dl || '无截止时间'
  if (typeof dl === 'object' && dl.Valid && dl.Time) return dl.Time
  return '无截止时间'
}

// 多选题答案 "A,C" -> ["A","C"]
function parseMultiAns(val) {
  if (!val) return []
  return String(val).split(',').map(s => s.trim()).filter(Boolean)
}

async function loadHw() {
  try {
    const r = await axios.get(`/api/class/${props.classId}/assignments`, auth)
    // 兼容分页返回 {list,total,...} 和旧版直接返回数组
    assignments.value = r.data?.list || (Array.isArray(r.data) ? r.data : [])
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
      _comment: s.comment || '',
    }))
  } catch (e) {
    submissions.value = []
  }
}

// 老师：打开某个学生的答卷。
// 评分模型与逐份批阅（SpeedGrader）完全一致：一个「总分」输入框（0~满分）
// + 评语 + AI 初评面板；最终分 = 客观题自动分 + 主观题分，由老师整体落笔。
function openSubmission(row) {
  currentSubmission.value = {
    ...row,
    // 初始总分：已批过用已批分数，未批用客观题自动分（与 SpeedGrader 同口径）
    _score: row.score ?? row.autoScore ?? 0,
    _comment: row.comment || '',
    // 上次保存的逐题最终评分（打开弹窗时的快照，供 AI 面板回填）。
    // 与 _questionScores 分开：后者是本次编辑的值，回传给面板会导致"输入一下就被重置"。
    _storedScores: row.questionScores || {},
    _questionScores: row.questionScores || {},
    _aiComment: '',
  }
  submissionDialog.value = true
  loadQueueMeta(currentHw.value.id)
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
    if (newQuestion.value.type === 'single' || newQuestion.value.type === 'multiple') {
      const arr = newQuestion.value.optionsText.split(',').map(s => s.trim()).filter(Boolean)
      if (arr.length < 2) {
        ElMessage.warning('至少填两个选项，用英文逗号分隔')
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

// 从答卷里取某道题的学生答案
function paperAnswer(qid) {
  if (!currentSubmission.value?.answers) return ''
  let ans = {}
  try { ans = JSON.parse(currentSubmission.value.answers) } catch (e) { return '' }
  return ans[qid] ?? ans[String(qid)] ?? ''
}

// 老师：保存批改（评分模型与逐份批阅一致：直接下发整份最终分）
async function saveGrade() {
  const sub = currentSubmission.value
  if (!sub) return
  const score = Number(sub._score)
  const maxScore = maxScoreOfHw()
  if (Number.isNaN(score) || score < 0 || score > maxScore) {
    ElMessage.warning(`分数需在 0 ~ ${maxScore} 之间`)
    return
  }
  try {
    await axios.post(`/api/assignment/submission/${sub.id}/grade`, {
      score,
      comment: sub._comment,
      questionScores: sub._questionScores || {},
    }, auth)
    ElMessage.success(`批改已保存：${sub.username} ${score} 分`)
    submissionDialog.value = false
    loadSubmissions(currentHw.value.id)
  } catch (e) {
    ElMessage.error(e.response?.data?.message || '保存失败')
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
.hw-list { display: flex; flex-direction: column; gap: 12px; }
.hw-card { cursor: pointer; }
.hw-card :deep(.el-card__body) { padding: 14px 16px; }
.hw-card-title { font-size: 15px; color: #1f2937; }
.hw-card-meta { color: #6b7280; font-size: 13px; margin: 6px 0 0; }
.hw-item { padding: 10px; border-bottom: 1px solid #eee; cursor: pointer; }
.hw-q { padding: 10px 0; border-bottom: 1px dashed #eee; }
.hw-questions { margin: 10px 0; }
.add-q-form { display: flex; gap: 8px; align-items: center; flex-wrap: wrap; margin-bottom: 8px; }
.upload-tip { color: #909399; font-size: 12px; margin-top: 4px; }
.essay-answer { font-size: 13px; line-height: 1.6; }
.essay-answer b { color: #303133; }
.paper { padding: 0 4px; }
.paper-head { font-size: 14px; }
.paper-q { padding: 12px 0; border-bottom: 1px dashed #eee; }
.paper-q-stem { font-size: 15px; margin-bottom: 8px; }
.paper-q-score { color: #909399; font-size: 13px; margin-left: 4px; }
.paper-q-answer { font-size: 14px; padding-left: 4px; }
.paper-essay-box { background: #f5f7fa; padding: 10px; border-radius: 4px; margin-top: 4px; white-space: pre-wrap; }
.ai-block { margin-top: 4px; }
</style>
