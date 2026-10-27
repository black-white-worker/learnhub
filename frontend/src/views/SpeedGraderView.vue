<script setup>
// 逐份批阅（对标 Canvas SpeedGrader）。
//
// 改造前批作业的过程是：打开列表 → 点一份 → 弹窗填分数 → 保存 → 弹窗关闭 →
// 重新点开列表 → 找下一份。批 40 份要点 40 多次列表、触发 40 多次整页刷新。
//
// 这里的做法：一次取回整个队列（未批优先），在本地做"上一份/下一份"，
// 保存只发一次请求，保存后自动跳到下一份未批的。
// 快捷键：Ctrl+Enter 保存并下一份；J / → 下一份；K / ← 上一份。
import { computed, onMounted, onUnmounted, ref, nextTick } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import axios from 'axios'
import { toastError } from '../api/client'

const route = useRoute()
const router = useRouter()
const assignmentId = route.params.id

const loading = ref(true)
const saving = ref(false)
const assignment = ref(null)
const questions = ref([])
const items = ref([])
const maxScore = ref(100)
const currentIndex = ref(0)

// 当前这份的编辑态（分数/评语），切换时从条目里初始化
const scoreInput = ref(null)
const commentInput = ref('')
const commentRef = ref(null)

const COMMENT_TEMPLATES = [
  '思路正确，注意书写规范。',
  '完成了基本要求，细节还需打磨。',
  '答案不完整，建议对照课件再看一遍。',
  '很好！可以尝试更简洁的解法。',
]

const current = computed(() => items.value[currentIndex.value] || null)
const pendingCount = computed(() => items.value.filter((i) => i.score === null || i.score === undefined).length)
const gradedCount = computed(() => items.value.length - pendingCount.value)
const questionMap = computed(() => {
  const map = {}
  for (const q of questions.value) map[q.id] = q
  return map
})

// 把学生作答 JSON 展开成可读列表
const answerList = computed(() => {
  if (!current.value?.answers) return []
  let obj = {}
  try {
    obj = JSON.parse(current.value.answers)
  } catch {
    return []
  }
  return Object.entries(obj).map(([qid, val]) => {
    const q = questionMap.value[qid]
    return {
      qid,
      stem: q?.stem || `题目 #${qid}`,
      answer: Array.isArray(val) ? val.join(' / ') : String(val),
      score: q?.score,
      type: q?.type,
    }
  })
})

function applyCurrentToForm() {
  const item = current.value
  scoreInput.value = item?.score ?? item?.autoScore ?? null
  commentInput.value = item?.comment || ''
}

async function load() {
  loading.value = true
  try {
    const r = await axios.get(`/api/assignment/${assignmentId}/grade-queue`)
    assignment.value = r.data.assignment
    questions.value = r.data.questions || []
    items.value = r.data.items || []
    maxScore.value = r.data.maxScore || 100
    // 打开就定位到第一份未批的
    const firstPending = items.value.findIndex((i) => i.score === null || i.score === undefined)
    currentIndex.value = firstPending >= 0 ? firstPending : 0
    applyCurrentToForm()
  } catch (e) {
    toastError(e, '加载批阅队列失败')
  } finally {
    loading.value = false
  }
}

function goTo(index) {
  if (index < 0 || index >= items.value.length) return
  currentIndex.value = index
  applyCurrentToForm()
  nextTick(() => commentRef.value?.blur())
}

function prev() {
  goTo(currentIndex.value - 1)
}

function next() {
  goTo(currentIndex.value + 1)
}

// 跳到下一份未批的（没有就停住）
function nextPending() {
  for (let i = currentIndex.value + 1; i < items.value.length; i++) {
    const it = items.value[i]
    if (it.score === null || it.score === undefined) {
      goTo(i)
      return
    }
  }
  ElMessage.success('没有待批的作业了')
}

async function save(advance = true) {
  const item = current.value
  if (!item || saving.value) return
  const score = Number(scoreInput.value)
  if (Number.isNaN(score) || score < 0 || score > maxScore.value) {
    ElMessage.warning(`分数需在 0 ~ ${maxScore.value} 之间`)
    return
  }
  saving.value = true
  try {
    await axios.post(`/api/assignment/submission/${item.id}/grade`, {
      score,
      comment: commentInput.value,
    })
    // 本地更新，不整页刷新
    item.score = score
    item.comment = commentInput.value
    ElMessage.success(`已保存：${item.username} ${score} 分`)
    if (advance) nextPending()
  } catch (e) {
    toastError(e, '保存失败')
  } finally {
    saving.value = false
  }
}

function appendTemplate(text) {
  commentInput.value = commentInput.value ? commentInput.value + '\n' + text : text
}

function onKeydown(e) {
  const tag = (e.target?.tagName || '').toLowerCase()
  const inField = tag === 'input' || tag === 'textarea'
  if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') {
    e.preventDefault()
    save(true)
    return
  }
  if (inField) return
  if (e.key === 'j' || e.key === 'ArrowRight') {
    e.preventDefault()
    next()
  } else if (e.key === 'k' || e.key === 'ArrowLeft') {
    e.preventDefault()
    prev()
  }
}

function fmtTime(t) {
  if (!t) return ''
  return new Date(t).toLocaleString('zh-CN', { hour12: false })
}

onMounted(() => {
  load()
  window.addEventListener('keydown', onKeydown)
})
onUnmounted(() => window.removeEventListener('keydown', onKeydown))
</script>

<template>
  <div class="speedgrader" v-loading="loading">
    <div class="header">
      <el-button size="small" @click="router.back()">← 返回</el-button>
      <div class="title">
        {{ assignment?.title || '逐份批阅' }}
        <span class="meta">已批 {{ gradedCount }} / {{ items.length }}，待批 {{ pendingCount }}</span>
      </div>
      <div class="nav">
        <el-button size="small" :disabled="currentIndex === 0" @click="prev">上一份 (K)</el-button>
        <el-button size="small" :disabled="currentIndex >= items.length - 1" @click="next">下一份 (J)</el-button>
        <el-button size="small" type="primary" :loading="saving" @click="save(true)">保存并下一份</el-button>
      </div>
    </div>

    <div class="body" v-if="items.length">
      <!-- 左：队列 -->
      <aside class="queue">
        <div class="queue-title">学生（未批优先）</div>
        <div
          v-for="(it, idx) in items"
          :key="it.id"
          class="queue-item"
          :class="{ active: idx === currentIndex, done: it.score !== null && it.score !== undefined }"
          @click="goTo(idx)"
        >
          <span class="name">{{ it.username }}</span>
          <span class="badge" v-if="it.score !== null && it.score !== undefined">{{ it.score }} 分</span>
          <span class="badge pending" v-else>待批</span>
        </div>
      </aside>

      <!-- 中：学生答卷 -->
      <main class="paper" v-if="current">
        <div class="paper-head">
          <div>
            <strong>{{ current.username }}</strong>
            <span class="submitted">提交于 {{ fmtTime(current.submittedAt) }}</span>
          </div>
          <el-tag v-if="current.autoScore !== null" size="small" type="info">
            客观题自动得分 {{ current.autoScore }}
          </el-tag>
        </div>

        <div class="block" v-if="current.content">
          <div class="block-title">文字作答</div>
          <div class="content">{{ current.content }}</div>
        </div>

        <div class="block" v-if="answerList.length">
          <div class="block-title">题目作答</div>
          <div class="qa" v-for="a in answerList" :key="a.qid">
            <div class="stem">{{ a.stem }}</div>
            <div class="ans">学生答案：{{ a.answer || '（未作答）' }}</div>
          </div>
        </div>

        <div class="block" v-if="current.fileUrl">
          <div class="block-title">附件</div>
          <a :href="current.fileUrl" target="_blank" rel="noopener">打开学生上传的文件</a>
        </div>

        <div class="block" v-if="!current.content && !answerList.length && !current.fileUrl">
          <el-empty description="这份提交没有内容" :image-size="80" />
        </div>
      </main>

      <!-- 右：评分 -->
      <aside class="grade-panel" v-if="current">
        <div class="panel-title">评分</div>
        <el-input-number v-model="scoreInput" :min="0" :max="maxScore" :step="1" style="width: 100%" />
        <div class="hint">满分 {{ maxScore }} 分 · Ctrl+Enter 保存</div>

        <div class="panel-title" style="margin-top: 14px">评语</div>
        <el-input
          ref="commentRef"
          v-model="commentInput"
          type="textarea"
          :rows="5"
          placeholder="给学生的话（可选）"
        />
        <div class="templates">
          <el-tag
            v-for="t in COMMENT_TEMPLATES"
            :key="t"
            size="small"
            class="template-tag"
            @click="appendTemplate(t)"
          >
            {{ t }}
          </el-tag>
        </div>

        <el-button type="primary" style="width: 100%; margin-top: 14px" :loading="saving" @click="save(false)">
          保存（留在本份）
        </el-button>
      </aside>
    </div>

    <el-empty v-else-if="!loading" description="还没有学生提交" />
  </div>
</template>

<style scoped>
.speedgrader {
  max-width: 1400px;
  margin: 0 auto;
  padding: 16px;
}
.header {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 14px;
  flex-wrap: wrap;
}
.title {
  font-size: 18px;
  font-weight: 700;
  flex: 1;
}
.meta {
  font-size: 13px;
  color: #64748b;
  font-weight: 400;
  margin-left: 8px;
}
.body {
  display: grid;
  grid-template-columns: 220px minmax(0, 1fr) 300px;
  gap: 14px;
  align-items: start;
}
.queue,
.paper,
.grade-panel {
  background: #fff;
  border: 1px solid #e5e7eb;
  border-radius: 10px;
  padding: 12px;
}
.queue {
  max-height: 70vh;
  overflow: auto;
}
.queue-title,
.panel-title {
  font-size: 13px;
  color: #64748b;
  margin-bottom: 8px;
}
.queue-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 10px;
  border-radius: 6px;
  cursor: pointer;
  font-size: 14px;
}
.queue-item:hover {
  background: #f1f5f9;
}
.queue-item.active {
  background: #eff6ff;
  color: #1d4ed8;
  font-weight: 600;
}
.queue-item.done .name {
  color: #94a3b8;
}
.badge {
  font-size: 12px;
  color: #16a34a;
}
.badge.pending {
  color: #f59e0b;
}
.paper {
  min-height: 50vh;
}
.paper-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  border-bottom: 1px solid #f1f5f9;
  padding-bottom: 10px;
  margin-bottom: 12px;
}
.submitted {
  font-size: 12px;
  color: #94a3b8;
  margin-left: 8px;
}
.block {
  margin-bottom: 16px;
}
.block-title {
  font-size: 13px;
  color: #64748b;
  margin-bottom: 6px;
}
.content {
  white-space: pre-wrap;
  line-height: 1.7;
  color: #1e293b;
}
.qa {
  padding: 8px 0;
  border-bottom: 1px dashed #e2e8f0;
}
.stem {
  color: #334155;
  margin-bottom: 4px;
}
.ans {
  color: #0f172a;
  font-weight: 500;
}
.hint {
  font-size: 12px;
  color: #94a3b8;
  margin-top: 6px;
}
.templates {
  margin-top: 8px;
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}
.template-tag {
  cursor: pointer;
}
@media (max-width: 1100px) {
  .body {
    grid-template-columns: 1fr;
  }
  .queue {
    max-height: 200px;
  }
}
</style>
