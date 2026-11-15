<script setup>
// SpeedGrader / 答卷弹窗 / 考试批改共用的「主观题 AI 初评」面板。
//
// 核心口径（老师反复提过的那一条）：AI 分只是每道主观题的**默认值**，
// 老师可以逐题改；最终总分 = 客观题自动分 + 各主观题最终分之和。
// 面板只负责算和填，落笔仍然是老师的动作 —— 保存要按弹窗里的保存按钮，
// 后端建议表 assignment_ai_grades 与成绩册始终分开。
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { toastError } from '../../api/client'
import { getAiGrade, triggerAiGrade } from '../../api/ai'
import {
  AI_GRADE_POLL_MS,
  buildComment,
  countEssayQuestions,
  defaultFinalScores,
  gradeTag,
  manualRows,
  pollTimedOut,
  shouldPoll,
  totalWithObjective,
  triggerDisabledReason,
} from '../../utils/aiGrade'

const props = defineProps({
  submissionId: { type: [Number, String], default: 0 },
  questions: { type: Array, default: () => [] },
  // 队列接口一次性带回的 AI 摘要（submissionId → 摘要），避免逐份再问一次
  initialGrade: { type: Object, default: null },
  maxScore: { type: Number, default: 100 },
  aiEnabled: { type: Boolean, default: false },
  // 接口路径前缀：作业 '/assignment/submission'，考试 '/exam/submission'
  baseUrl: { type: String, default: '/assignment/submission' },
  // 客观题自动分：最终总分的另一半，由调用方按自己的口径算好传进来
  objectiveScore: { type: Number, default: 0 },
  // 上次保存的逐题最终评分（后端 questionScores 对象）；有值时优先于 AI 分回填
  storedScores: { type: Object, default: null },
  // 哪些题型算「老师手批」：作业只 essay（填空自动判分）；
  // 考试的填空/附件也不自动判，由调用方传 ['essay','fill','fileupload']
  manualTypes: { type: Array, default: () => ['essay'] },
})
const emit = defineEmits(['adopt', 'change'])

const grade = ref(null)
const busy = ref(false)
const timer = ref(null)
const startedAt = ref(0)
// 逐题最终评分：qid → 分数。老师可改，采纳时随总分一起下发。
const finalScores = ref({})

const essayCount = computed(() => props.initialGrade?.essayCount ?? countEssayQuestions(props.questions))
const view = computed(() => grade.value || props.initialGrade || null)
const tag = computed(() => gradeTag(view.value))
const rows = computed(() => manualRows(view.value?.items, props.questions, props.manualTypes))
const disabledReason = computed(() => triggerDisabledReason(props.aiEnabled, essayCount.value))
const hasStored = computed(() => Object.keys(props.storedScores || {}).length > 0)
// 没有逐题可改的题（纯客观卷）时整块评分区不出现，避免"0 分"误导。
// 重跑 AI（processing）期间先只显示进度，避免旧逐题分和"排队中"同时出现。
const showRows = computed(
  () =>
    rows.value.length > 0 &&
    view.value?.status !== 'processing' &&
    (view.value?.status === 'done' || hasStored.value)
)
const sums = computed(() => totalWithObjective(props.objectiveScore, rows.value, finalScores.value, props.maxScore))
// 逐题分加起来超过卷面总分：总分已被夹到满分，界面上说明一句，
// 免得老师看到「客观 95 + 主观 18 = 100」以为算错了。
const overMax = computed(() => sums.value.objective + sums.value.subjective > sums.value.total)
// AI 只批 essay：考试的填空/附件、解析失败的题会没有 AI 分，必须提示老师手填，
// 否则默认 0 分会跟着"采纳"静默把总分压低。
const missingAi = computed(() => rows.value.filter((r) => r.aiScore === null).length)
const canAdopt = computed(() => showRows.value)

// 逐题回填优先级：已保存的逐题分 > AI 初评分 > 0。
// 只在"换了一份提交 / AI 结果状态变了 / 后端重新下发了已存分数"时重置，
// 老师在输入框里改字不会触发（否则每敲一下就被 AI 分覆盖）。
//
// seededJSON 记录"这次回填后的样子"：change 事件只在老师真的改动时才发。
// 用比对内容而不是 nextTick 置标志位，避免和 Vue 的 flush 时序耦合；
// 否则一打开已批过的答卷，总分就会被"客观题 + AI 分"覆盖掉。
let seededJSON = ''

function seedFinals() {
  const next = defaultFinalScores(rows.value, props.storedScores)
  finalScores.value = next
  seededJSON = JSON.stringify(next)
}

function setGrade(g) {
  grade.value = g ? { ...g } : null
}

function stopPolling() {
  if (timer.value) {
    clearTimeout(timer.value)
    timer.value = null
  }
}

// 轮询用 setTimeout 串起来而不是 setInterval：
// 一次请求没回来就不会发下一次，慢网络下不会把自己打爆。
function schedulePoll() {
  stopPolling()
  if (!shouldPoll(view.value) || !props.submissionId) return
  if (!startedAt.value) startedAt.value = Date.now()
  if (pollTimedOut(Date.now() - startedAt.value)) {
    ElMessage.warning('AI 批改还没回来，可能排在队列里，稍后刷新这份再看')
    return
  }
  timer.value = setTimeout(async () => {
    await refresh(true)
    schedulePoll()
  }, AI_GRADE_POLL_MS)
}

async function refresh(silent = false) {
  if (!props.submissionId) return
  if (!silent) busy.value = true
  try {
    setGrade(await getAiGrade(props.submissionId, { base: props.baseUrl }))
  } catch (e) {
    if (!silent) toastError(e, '读取 AI 初评失败')
  } finally {
    if (!silent) busy.value = false
  }
}

async function trigger() {
  if (disabledReason.value) {
    ElMessage.warning(disabledReason.value)
    return
  }
  busy.value = true
  startedAt.value = Date.now()
  try {
    const resp = await triggerAiGrade(props.submissionId, { base: props.baseUrl })
    setGrade(resp?.grade || null)
    ElMessage.success(resp?.message || '已提交 AI 初评')
    schedulePoll()
  } catch (e) {
    toastError(e, '发起 AI 初评失败')
  } finally {
    busy.value = false
  }
}

// 采纳：把「客观题 + 逐题最终分」的合计算成总分，连同逐题分一起交给调用方。
// 调用方负责填进总分行 / 评语框，保存仍然要老师自己按。
function adopt() {
  if (!canAdopt.value) return
  const payload = {
    score: sums.value.total,
    subjectiveTotal: sums.value.subjective,
    // 拷贝一份：面板里的输入框还会继续改，父组件拿到的必须是当时的快照
    scores: { ...finalScores.value },
    comment: buildComment(view.value, rows.value, finalScores.value),
  }
  emit('adopt', payload)
  ElMessage.success(`已按逐题分数填入总分 ${payload.score} 分，确认后保存`)
}

// 老师改动任一题的最终分：立刻把「客观题 + 新的主观题合计」同步给调用方。
// 这样总分、逐题分、保存内容始终是同一套数，不会出现"改了题但总分没变"。
watch(
  finalScores,
  () => {
    const json = JSON.stringify(finalScores.value)
    if (json === seededJSON) return // 回填本身不算老师的改动
    seededJSON = json
    if (!showRows.value) return
    emit('change', { score: sums.value.total, scores: { ...finalScores.value } })
  },
  { deep: true }
)

// 换一份提交：丢掉上一份的结果和定时器，重新按已存分数 / AI 分回填
watch(
  () => props.submissionId,
  () => {
    stopPolling()
    setGrade(null)
    startedAt.value = 0
    seedFinals()
    if (props.initialGrade?.status === 'processing') schedulePoll()
  },
  { immediate: true }
)

// AI 结果从 processing 变 done、或后端重新下发了已存逐题分时再回填一次。
// 只盯这两个信号：刷新同一条 done 结果不会把老师改过的分数冲掉。
watch([() => view.value?.status, () => props.storedScores], seedFinals)

onBeforeUnmount(stopPolling)
</script>

<template>
  <div class="ai-panel">
    <div class="head">
      <span class="title">主观题 AI 初评</span>
      <el-tag :type="tag.type" size="small">{{ tag.text }}</el-tag>
    </div>

    <div class="hint" v-if="essayCount">本份含 {{ essayCount }} 道主观题</div>

    <el-button
      size="small"
      type="primary"
      plain
      :loading="busy"
      :disabled="Boolean(disabledReason)"
      @click="trigger"
    >
      {{ view?.status === 'processing' ? '重新发起' : '发起 AI 初评' }}
    </el-button>
    <el-button v-if="view" size="small" text @click="refresh()">刷新</el-button>

    <div class="reason" v-if="disabledReason">{{ disabledReason }}</div>

    <div class="body" v-if="view?.status === 'processing'">
      <span class="waiting">排队中或批改中，出结果会自动刷新…</span>
    </div>

    <div class="manual" v-else-if="view?.status === 'manual'">
      {{ view.error || 'AI 没能给出完整分数，请人工批改。' }}
    </div>

    <div class="failed" v-else-if="view?.status === 'failed'">
      {{ view.error || '批改失败，可以重新发起一次。' }}
    </div>

    <!--
      逐题评分区：AI 分只作为默认值显示在题目右上角，
      右边是可改的「最终评分」；底部汇总客观题 + 主观题 = 总分。
    -->
    <template v-if="showRows">
      <div class="ai-total" v-if="view?.status === 'done'">
        AI 初评合计 <strong>{{ view.score }}</strong> / {{ view.maxScore || maxScore }}
      </div>
      <div class="missing" v-if="missingAi">
        有 {{ missingAi }} 道题 AI 未给分（填空/附件或解析失败），请手动填写最终评分。
      </div>

      <div class="items">
        <div class="item" v-for="r in rows" :key="r.questionId">
          <div class="item-head">
            <span>{{ r.label }}</span>
            <span class="item-score" :class="{ empty: r.aiScore === null }">
              AI 初评 {{ r.scoreText }}
            </span>
          </div>
          <div class="item-stem" v-if="r.stem">{{ r.stem }}</div>

          <div class="final-row">
            <span class="final-label">最终评分</span>
            <el-input-number
              v-model="finalScores[r.questionId]"
              :min="0"
              :max="r.maxScore ?? undefined"
              :step="1"
              :precision="0"
              size="small"
              controls-position="right"
              class="final-input"
            />
            <span class="final-max">/ {{ r.maxScore ?? '?' }} 分</span>
          </div>

          <div class="item-comment" v-if="r.comment">{{ r.comment }}</div>
          <ul v-if="r.errorPoints.length">
            <li v-for="p in r.errorPoints" :key="p">{{ p }}</li>
          </ul>
          <div class="sug" v-if="r.suggestion">建议：{{ r.suggestion }}</div>
        </div>
      </div>

      <div class="sum">
        <div class="sum-text">
          客观题 <strong>{{ sums.objective }}</strong>
          <span class="op">+</span>
          主观题 <strong>{{ sums.subjective }}</strong>
          <span class="op">=</span>
          <strong class="sum-total">{{ sums.total }}</strong>
          <span class="of">/ {{ maxScore }}</span>
        </div>
        <el-button size="small" type="primary" :disabled="!canAdopt" @click="adopt">
          采纳为分数（{{ sums.total }} 分）
        </el-button>
      </div>
      <div class="sum-hint" v-if="overMax">
        逐题分合计超过卷面总分 {{ maxScore }}，总分已按满分计。
      </div>
      <div class="sum-hint">逐题分改动会实时更新总分；「采纳」还会把 AI 逐题评语写进评语框。保存仍需手动点击。</div>
    </template>
  </div>
</template>

<style scoped>
.ai-panel {
  border-top: 1px solid #f1f5f9;
  margin-top: 14px;
  padding-top: 12px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.head {
  display: flex;
  align-items: center;
  gap: 8px;
}
.title {
  font-size: 13px;
  color: #64748b;
}
.hint,
.reason {
  font-size: 12px;
  color: #94a3b8;
}
.waiting {
  font-size: 13px;
  color: #b45309;
}
.ai-total {
  font-size: 13px;
  color: #64748b;
}
.ai-total strong {
  font-size: 16px;
  color: #0f766e;
}
.missing {
  font-size: 12px;
  color: #b45309;
  background: #fffbeb;
  border-radius: 6px;
  padding: 6px 8px;
}
.items {
  display: flex;
  flex-direction: column;
  gap: 8px;
  max-height: 300px;
  overflow: auto;
}
.item {
  border: 1px solid #eef2f7;
  border-radius: 8px;
  padding: 8px;
  font-size: 13px;
}
.item-head {
  display: flex;
  justify-content: space-between;
  color: #475569;
}
.item-score {
  color: #16a34a;
}
.item-score.empty {
  color: #94a3b8;
}
.item-stem {
  color: #94a3b8;
  margin: 4px 0;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
.final-row {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 6px 0;
}
.final-label {
  color: #334155;
}
.final-input {
  width: 96px;
}
.final-max {
  color: #94a3b8;
}
.item-comment {
  color: #1e293b;
  white-space: pre-wrap;
}
.item ul {
  margin: 4px 0;
  padding-left: 16px;
  color: #b91c1c;
}
.sug {
  color: #0f766e;
}
.sum {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  border-top: 1px dashed #e2e8f0;
  padding-top: 10px;
  flex-wrap: wrap;
}
.sum-text {
  font-size: 13px;
  color: #475569;
}
.sum-text strong {
  color: #0f172a;
  font-size: 15px;
}
.sum-text .op {
  margin: 0 4px;
  color: #94a3b8;
}
.sum-text .sum-total {
  color: #16a34a;
  font-size: 20px;
}
.of {
  color: #94a3b8;
  font-size: 13px;
}
.sum-hint {
  font-size: 12px;
  color: #94a3b8;
}
.manual {
  font-size: 13px;
  color: #b45309;
}
.failed {
  font-size: 13px;
  color: #dc2626;
}
</style>
