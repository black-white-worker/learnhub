<script setup>
// AI 答疑（RAG 流式对话）。
//
// 三个坑这里都处理了：
//   1. 流式接口只能走 fetch，拿不到 axios 的 401/信封拦截器 —— 错误文案自己解析；
//   2. 「停止生成」用 AbortController，中止不算失败，不能弹红条；
//   3. 历史存在服务端（按用户 + 课程，只留最近 5 轮），换课程必须重新拉，
//      否则界面显示的是上一门课的回答。
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { toastError } from '../api/client'
import { aiSessionClear, aiSessionHistory, loadAiStatus, streamAiChat } from '../api/ai'
import { newChatState, reduceChatEvent } from '../utils/aiSse'
import { fetchCourseList } from '../api/index'

const courses = ref([])
// 初值必须是 null：el-select 拿到 0 会当成"有个选中项"，框里显示一个光秃秃的 0
const courseId = ref(null)
const messages = ref([])
const input = ref('')
const streaming = ref(false)
const status = ref({ enabled: false, reason: '', features: [] })

const listRef = ref(null)
let abortCtrl = null

const canSend = computed(() => Boolean(courseId.value) && input.value.trim() && !streaming.value)

async function scrollBottom() {
  await nextTick()
  const el = listRef.value
  if (el) el.scrollTop = el.scrollHeight
}

async function loadCourses() {
  try {
    const res = await fetchCourseList()
    courses.value = res.data?.courses || []
    if (!courseId.value && courses.value.length) courseId.value = courses.value[0].id
  } catch (e) {
    toastError(e, '课程列表加载失败')
  }
}

async function loadHistory() {
  if (!courseId.value) return
  try {
    const data = await aiSessionHistory(courseId.value)
    messages.value = (data?.messages || []).map((m) => ({
      role: m.role === 'assistant' ? 'assistant' : 'user',
      content: m.content || '',
      refs: [],
    }))
  } catch (e) {
    // 历史拉不到不影响新开对话，只是看不见前面的话；401 会自己跳登录，不用弹提示
    messages.value = []
    if (e?.response?.status !== 401) ElMessage.warning('对话历史加载失败：' + e.message)
  }
  scrollBottom()
}

watch(courseId, loadHistory)

async function send() {
  const query = input.value.trim()
  if (!query || !courseId.value || streaming.value) return
  messages.value.push({ role: 'user', content: query, refs: [] })
  input.value = ''
  const holder = { role: 'assistant', content: '', refs: [], pending: true }
  messages.value.push(holder)
  const state = newChatState()
  streaming.value = true
  abortCtrl = new AbortController()
  scrollBottom()
  try {
    await streamAiChat({
      query,
      courseId: courseId.value,
      signal: abortCtrl.signal,
      onEvent: (evt) => {
        reduceChatEvent(state, evt)
        holder.content = state.answer
        holder.refs = [...state.refs]
        if (state.error) holder.error = state.error
        scrollBottom()
      },
    })
    if (state.error) holder.error = state.error
    else if (!state.answer) holder.content = '（这轮没有回答内容，可能是资料里没检索到相关内容）'
  } catch (e) {
    if (e?.name === 'AbortError') {
      holder.stopped = true
    } else {
      holder.error = e?.message || '答疑请求失败'
      if (e?.status === 429) ElMessage.warning(holder.error)
    }
  } finally {
    holder.pending = false
    streaming.value = false
    abortCtrl = null
    scrollBottom()
  }
}

function stop() {
  abortCtrl?.abort()
}

async function clearHistory() {
  if (!courseId.value) return
  try {
    await ElMessageBox.confirm('清空这门课的历史后，AI 会忘记之前的提问。继续？', '清空对话', {
      type: 'warning',
      confirmButtonText: '清空',
      cancelButtonText: '取消',
    })
  } catch {
    return
  }
  try {
    await aiSessionClear(courseId.value)
    messages.value = []
    ElMessage.success('已清空')
  } catch (e) {
    toastError(e, '清空失败')
  }
}

function onKeydown(e) {
  // Enter 发送，Shift+Enter 换行（输入框是多行的）
  if (e.key === 'Enter' && !e.shiftKey && !e.isComposing) {
    e.preventDefault()
    send()
  }
}

onMounted(async () => {
  status.value = await loadAiStatus()
  loadCourses()
})
onUnmounted(() => abortCtrl?.abort())
</script>

<template>
  <div class="ai-chat">
    <div class="bar">
      <el-select v-model="courseId" placeholder="选择课程" style="width: 220px" filterable>
        <el-option v-for="c in courses" :key="c.id" :label="c.name" :value="c.id" />
      </el-select>
      <span class="hint">历史只保留最近 5 轮，换课程会自动切换上下文</span>
      <el-button size="small" text @click="clearHistory" :disabled="!messages.length">清空对话</el-button>
    </div>

    <el-alert
      v-if="!status.enabled"
      class="notice"
      type="warning"
      :closable="false"
      :title="status.reason || 'AI 功能未启用'"
      description="答疑需要后端配置好 AiService（URL + 内部令牌）才能用。"
    />
    <el-alert
      v-else
      class="notice"
      type="info"
      :closable="false"
      title="回答只依据已入库的课程资料；没有检索到内容时会直接说明，不会编造。"
    />

    <div class="messages" ref="listRef">
      <div v-for="(m, i) in messages" :key="i" class="msg" :class="m.role">
        <div class="bubble">
          <div class="content">{{ m.content }}<span v-if="m.pending" class="cursor">▍</span></div>
          <div class="refs" v-if="m.refs && m.refs.length">
            <el-tag v-for="r in m.refs" :key="r" size="small" type="info">{{ r }}</el-tag>
          </div>
          <div class="error" v-if="m.error">{{ m.error }}</div>
          <div class="stopped" v-if="m.stopped">已停止生成</div>
        </div>
      </div>
      <el-empty v-if="!messages.length" description="选一门课，问问它这一章讲了什么" :image-size="80" />
    </div>

    <div class="composer">
      <el-input
        v-model="input"
        type="textarea"
        :rows="3"
        resize="none"
        placeholder="问题，例如「第二章的假设检验和第一章有什么联系」（Enter 发送，Shift+Enter 换行）"
        @keydown="onKeydown"
      />
      <div class="actions">
        <el-button v-if="streaming" @click="stop">停止</el-button>
        <el-button type="primary" :disabled="!canSend" :loading="streaming" @click="send">发送</el-button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.ai-chat {
  max-width: 900px;
  margin: 0 auto;
  padding: 16px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.bar {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}
.hint {
  font-size: 12px;
  color: #94a3b8;
  flex: 1;
}
.messages {
  background: #fff;
  border: 1px solid #e5e7eb;
  border-radius: 10px;
  padding: 14px;
  height: 52vh;
  min-height: 260px;
  overflow: auto;
}
.msg {
  display: flex;
  margin-bottom: 12px;
}
.msg.user {
  justify-content: flex-end;
}
.bubble {
  max-width: 82%;
  padding: 10px 12px;
  border-radius: 10px;
  background: #f1f5f9;
}
.msg.user .bubble {
  background: #eff6ff;
}
.content {
  white-space: pre-wrap;
  line-height: 1.7;
  color: #1e293b;
  word-break: break-word;
}
.cursor {
  animation: blink 1s infinite;
  color: #94a3b8;
}
@keyframes blink {
  50% {
    opacity: 0;
  }
}
.refs {
  margin-top: 8px;
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}
.error {
  margin-top: 8px;
  color: #dc2626;
  font-size: 13px;
}
.stopped {
  margin-top: 6px;
  color: #94a3b8;
  font-size: 12px;
}
.composer {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}
</style>
