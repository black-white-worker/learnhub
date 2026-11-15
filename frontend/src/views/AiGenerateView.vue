<script setup>
// AI 出题：老师给知识点 → AI 生成 → 老师审核勾选 → 加进某个作业。
//
// 三件事必须在客户端把关：
//   1. AI 说自己出了 5 道，服务端结构校验可能判其中几道不合格（invalid），
//      不合格的题不给勾选，否则作业里会出现没有答案的题；
//   2. 判断题 AI 给布尔、平台存字符串，选择题 AI 给数组、平台存 JSON ——
//      转换逻辑全在 utils/aiGenerate.js，这里只调用；
//   3. 入库走的是已有的 POST /api/assignment/:aid/question，一题一请求，
//      中途失败要告诉老师"已经进了几题"，不能只报"添加失败"。
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import axios from 'axios'
import { toastError } from '../api/client'
import { aiGenerate, loadAiStatus, streamAiGenerate } from '../api/ai'
import {
  DIFFICULTIES,
  GEN_SOURCES,
  GEN_SUBJECTS,
  GEN_TYPES,
  answerLabel,
  buildGeneratePayload,
  clearGenerateJob,
  collectStems,
  generationMetaText,
  generationProgressSummary,
  isQuestionUsable,
  loadGenerateJob,
  loadRecords,
  loadViewing,
  newGenerateProgress,
  newGenerationSeed,
  newJobId,
  normalizeGeneratedQuestions,
  optionLines,
  payloadProblem,
  progressPercent,
  questionBasis,
  questionExplanation,
  questionFocus,
  recordSummary,
  reduceGenerateProgress,
  saveGenerateJob,
  saveRecord,
  saveRecords,
  saveViewing,
  buildRecord,
  clearRecords,
  deleteRecord,
  toAddQuestionPayload,
} from '../utils/aiGenerate'

const router = useRouter()
const token = localStorage.getItem('token')
const auth = { headers: { Authorization: 'Bearer ' + token } }

const form = ref({
  knowledge: '', difficulty: '中等', qtype: 'single', count: 3, score: 5,
  source: 'ai', subject: '', grounding: true,
})
const loading = ref(false)
const adding = ref(false)
// 中文专业出题要检索资料 + 调大模型，实测 25~90 秒。
// 计时只用于「已等待多久」的文案；阶段状态全部来自服务端真实事件，不用定时器假装进度。
const STREAM_TIMEOUT = 150000
const elapsed = ref(0)
const progress = ref(newGenerateProgress())
const progressSummary = ref('')
const showStages = ref(false)
const streamFallback = ref(false)
const progressFailed = ref(false)
const resuming = ref(false) // 刷新后正在断点续传恢复进度

// ===== 出题记录（历史，localStorage 持久化）=====
// 每次成功出题沉淀一条，可点进去回看 / 删除；刷新、关页、重开浏览器都不丢。
const records = ref(loadRecords())
const viewingRecordId = ref(null) // 当前正在查看的历史记录 id（null=看最新一批）
const recordsOpen = ref(true) // 出题记录面板默认展开（常驻，与出题相互独立）
const currentViewing = computed(() => records.value.find((r) => r.id === viewingRecordId.value) || {})

// 题目列表展示：默认显示「当前这一批」出题结果；点开某条历史记录时显示该记录的题目。
// 两者用 viewingRecordId 区分、互不覆盖，所以「查看记录」不会打断 / 清除正在进行的出题。
const displayQuestions = computed(() =>
  viewingRecordId.value
    ? normalizeGeneratedQuestions(currentViewing.value.questions || [], currentViewing.value.qtype || form.value.qtype)
    : questions.value
)
const displayQtype = computed(() =>
  viewingRecordId.value
    ? (currentViewing.value.qtype || form.value.qtype)
    : resolvedQtype.value
)
let waitTimer = null
let abortTimer = null
let streamController = null
function startWaiting(from = 0) {
  elapsed.value = from
  clearInterval(waitTimer)
  waitTimer = setInterval(() => { elapsed.value += 1 }, 1000)
}
function stopWaiting() {
  clearInterval(waitTimer)
  waitTimer = null
}
function stopStream() {
  clearTimeout(abortTimer)
  abortTimer = null
  streamController = null
}
onUnmounted(() => {
  streamController?.abort()
  stopWaiting()
  stopStream()
})
const questions = ref([])
const selected = ref([])
const status = ref({ enabled: false, reason: '', features: [] })
// 服务端返回的题型（题库模式会自动把多选归一成单选），入库/渲染都用它，
// 不能继续拿表单里的 qtype 猜。
const resolvedQtype = ref('single')
const meta = ref({})
const recentStems = ref([])
const metaText = computed(() => generationMetaText(meta.value))
const bankMode = computed(() => form.value.source === 'bank')
// 本批命制依据（中文权威条目），只在按知识点出题时出现
const materialSources = computed(() =>
  Array.isArray(meta.value.materials) ? meta.value.materials.filter((m) => m && m.title) : []
)

// 切来源 / 题型后旧题不再匹配新题型，清空结果，避免拿旧题入库
function resetResults() {
  questions.value = []
  selected.value = []
  expanded.value = new Set()
  meta.value = {}
  recentStems.value = []
}
function onSourceChange() {
  resolvedQtype.value = form.value.qtype
  resetResults()
}
function onQtypeChange() {
  resolvedQtype.value = form.value.qtype
  resetResults()
}
// 卡片展开状态：默认折叠显示题干摘要+答案，点卡片展开看完整题干和 A/B/C/D 选项。
// 用 Set 存索引，题目数量变化（重新生成/加入作业）时统一重置。
const expanded = ref(new Set())

function toggleExpand(i) {
  const next = new Set(expanded.value)
  next.has(i) ? next.delete(i) : next.add(i)
  expanded.value = next
}
const allExpanded = computed(
  () => displayQuestions.value.length > 0 && expanded.value.size >= displayQuestions.value.length
)
function toggleAll() {
  expanded.value = allExpanded.value
    ? new Set()
    : new Set(questions.value.map((_, i) => i))
}
function stemPreview(q, i) {
  const s = String(q?.stem || '').trim()
  if (!s) return `第 ${i + 1} 题 · 未获取到题干，点开查看，建议重新生成`
  return s.length > 48 ? `${s.slice(0, 48)}…` : s
}

// 作业归属：班级 → 作业（只用现有的两个接口，不新建"题目草稿库"）
const classes = ref([])
const assignments = ref([])
// 初值 null：给 0 的话 el-select 会把它当选中项，框里显示一个光秃秃的 0
const classId = ref(null)
const assignmentId = ref(null)

const usableIndexes = computed(() =>
  displayQuestions.value.map((q, i) => (isQuestionUsable(q) ? i : -1)).filter((i) => i >= 0)
)
const invalidCount = computed(() => displayQuestions.value.length - usableIndexes.value.length)
const chosen = computed(() =>
  selected.value.map((i) => displayQuestions.value[i]).filter(Boolean)
)
// 勾了题但答案结构不合格的，禁止提交（payloadProblem 比服务端的校验更严一点）
const blockedReason = computed(() => {
  if (!chosen.value.length) return '还没勾选题目'
  if (!assignmentId.value) return '请选择要加入哪个作业'
  for (let i = 0; i < selected.value.length; i++) {
    const idx = selected.value[i]
    const p = toAddQuestionPayload(displayQuestions.value[idx], displayQtype.value)
    const problem = payloadProblem(p)
    if (problem) return `第 ${idx + 1} 题：${problem}`
  }
  return ''
})
// 查看历史记录时不允许「加入作业」（那批题是回看，且避免误改当前批次的已加入标记）
const canAdd = computed(() => !blockedReason.value && !adding.value && !viewingRecordId.value && !loading.value)

async function loadClasses() {
  try {
    const r = await axios.get('/api/class/my', auth)
    classes.value = r.data || []
  } catch (e) {
    toastError(e, '班级加载失败')
  }
}

async function loadAssignments() {
  assignmentId.value = null
  assignments.value = []
  if (!classId.value) return
  try {
    const r = await axios.get(`/api/class/${classId.value}/assignments`, auth)
    assignments.value = r.data?.list || (Array.isArray(r.data) ? r.data : [])
  } catch (e) {
    assignments.value = []
    toastError(e, '作业列表加载失败')
  }
}

// ===== 刷新保留进度：job_id + sessionStorage 断点续传 =====
// 每次出题生成客户端 job_id，连进度快照写进 sessionStorage。页面刷新后
// resumeIfNeeded() 检测未过期作业：已完成的直接恢复题目；进行中的用同一 job_id
// 重新订阅服务端 SSE（服务端作业与连接解耦，刷新不会中断或重跑）。
const JOB_SNAPSHOT_TTL = 180000 // 快照 3 分钟有效，超期不恢复

function persistProgress(payload) {
  if (!payload?.job_id) return
  saveGenerateJob({
    jobId: payload.job_id,
    payload,
    progress: progress.value,
    meta: meta.value,
    questions: questions.value,
    elapsed: elapsed.value,
    done: false,
    t: Date.now(),
  })
}

function persistDone(payload) {
  if (!payload?.job_id) return
  saveGenerateJob({
    jobId: payload.job_id,
    payload,
    progress: progress.value,
    meta: meta.value,
    questions: questions.value,
    elapsed: elapsed.value,
    done: true,
    t: Date.now(),
  })
}

async function generate() {
  if (loading.value) return // 生成要几十秒，防重复点击把额度打两次
  // 免费题库模式不依赖知识点（通识随机题）；AI 模式仍要求写具体一点
  if (!bankMode.value && form.value.knowledge.trim().length < 2) {
    ElMessage.warning('知识点写得具体一点，比如「假设检验的两类错误」')
    return
  }
  const payload = buildGeneratePayload(form.value, { previousStems: recentStems.value })
  payload.job_id = newJobId()
  clearGenerateJob() // 新的一批，覆盖旧快照
  // 开始新的一批生成：退出历史记录查看态，回到「最新一批」
  viewingRecordId.value = null
  saveViewing(null)
  saveGenerateJob({
    jobId: payload.job_id,
    payload,
    progress: newGenerateProgress(),
    meta: {},
    questions: [],
    elapsed: 0,
    done: false,
    t: Date.now(),
  })
  // 点下「生成」就立即落一条 running 态的任务记录：即使只跑到前两步就刷新 / 断网 / 失败，
  // 刷新后这条记录仍在出题记录列表里可见、可续传，不再凭空消失。
  records.value = saveRecord(buildRecord(payload, {}, [], 0, 'running', newGenerateProgress()))
  await runGeneration(payload)
}

async function runGeneration(payload, { isResume = false, initialElapsed = 0 } = {}) {
  if (loading.value && !isResume) return // 生成要几十秒，防重复点击把额度打两次
  loading.value = true
  startWaiting(initialElapsed)
  if (!isResume) {
    progress.value = newGenerateProgress()
    progressSummary.value = ''
    showStages.value = false
    streamFallback.value = false
    progressFailed.value = false
    questions.value = []
    meta.value = {} // 新一批开始，清掉上一批的元信息，避免进行中的任务记录串批
    selected.value = []
    expanded.value = new Set()
  }
  const controller = new AbortController()
  streamController = controller
  let receivedAny = false
  let timedOut = false
  abortTimer = setTimeout(() => {
    timedOut = true
    controller.abort()
  }, STREAM_TIMEOUT)

  try {
    let data
    // 断点续传重订阅：流偶发被代理 / 网络掐断时重试一次（连的是同一个 job，不重跑模型）；
    // 普通首发出题则不加重试，避免重复烧额度。
    const streamAttempts = isResume ? 2 : 1
    for (let s = 0; s < streamAttempts; s++) {
      try {
        data = await streamAiGenerate({
          data: payload,
          signal: controller.signal,
          onProgress: (evt) => {
            receivedAny = true
            progress.value = reduceGenerateProgress(progress.value, evt)
            persistProgress(payload) // 每次进度都落盘，刷新可断点续传
            // 同步更新 running 态任务记录：刷新后能回看「进行到第几步」而不是记录消失
            records.value = saveRecord(
              buildRecord(payload, meta.value, questions.value, elapsed.value, 'running', progress.value)
            )
          },
        })
        break
      } catch (se) {
        if (s < streamAttempts - 1 && !timedOut && se?.name !== 'AbortError') {
          await new Promise((r) => setTimeout(r, 700))
          continue
        }
        // 只在「流根本没建立」时回退到老 JSON 接口。已经收到阶段事件说明服务端
        // 正在出题，重跑一次会烧两遍额度、还要多等几十秒。断点续传重订阅时不回退，
        // 否则可能和服务端已在跑的作业重复烧额度。
        const canFallback = !isResume && !receivedAny && !timedOut && se?.name !== 'AbortError'
        if (!canFallback) throw se
        streamFallback.value = true
        progress.value = reduceGenerateProgress(progress.value, {
          stage: 'compose', status: 'running', detail: '进度流不可用，已改用标准出题接口',
        })
        data = await aiGenerate(payload)
        progress.value = reduceGenerateProgress(progress.value, {
          stage: 'done', status: 'done', detail: '出题完成',
        })
        break
      }
    }
    resolvedQtype.value = data?.qtype || form.value.qtype
    meta.value = data?.meta || {}
    progressSummary.value = generationProgressSummary(elapsed.value, meta.value)

    // 模型没按题目格式返回（截断 / 纯文本）：不把整段错误 JSON 摊给老师看，
    // 直接提示「重新生成」，不沉淀记录、也不当作可恢复作业。
    if (data?.raw) {
      questions.value = []
      selected.value = []
      progressFailed.value = true
      progressSummary.value = '模型未返回有效题目，请点「换一批」或「生成」重新生成'
      ElMessage.warning('模型这次没有返回有效题目，请点「换一批」重新生成')
      // 本次没拿到题：任务记录如实标为「未完成」，保留在记录里而不是消失
      records.value = saveRecord(
        buildRecord(payload, meta.value, [], elapsed.value, 'interrupted', progress.value)
      )
      clearGenerateJob()
      return
    }
    // 先归一化：兼容模型偶发的中文键名 / JSON 字符串选项 / "C. xxx" 连写答案，
    // 否则卡片会出现「题干空白、只剩参考答案」。
    questions.value = normalizeGeneratedQuestions(data?.questions, resolvedQtype.value)
    recentStems.value = collectStems(questions.value, recentStems.value)
    // 默认全选合格的题
    selected.value = usableIndexes.value.slice()
    if (meta.value.hint) {
      ElMessage.warning(meta.value.hint)
    } else if (String(meta.value.bank_status || '').startsWith('fallback')) {
      ElMessage.warning('免费题库暂时不可用，已自动回退到中文专业题出题')
    } else if (!bankMode.value && meta.value.material_status === 'unavailable') {
      ElMessage.info('中文资料源暂时不可达，已用模型的学科知识出专业题')
    } else if (!bankMode.value && meta.value.material_status === 'no_match') {
      ElMessage.info('没检索到该知识点的中文条目，已用模型知识出题；把知识点写得更标准可提升依据质量')
    } else if (bankMode.value && meta.value.localized === false) {
      ElMessage.info('题库题目暂时没能汉化，已展示英文原文')
    }
    if (data?.invalid_count) {
      ElMessage.warning(`有 ${data.invalid_count} 道题未通过质量校验，已置灰不可入库；建议点「换一批」`)
    }
    if (!questions.value.length && !meta.value.hint) {
      ElMessage.info('没有生成题目，换个知识点或难度再试一次')
    }
    persistDone(payload)
    // 出题完成后若正在查看历史记录，回到「最新一批」让老师看到刚生成的题
    if (viewingRecordId.value) {
      viewingRecordId.value = null
      saveViewing(null)
      selected.value = []
      expanded.value = new Set()
    }
    // 沉淀为一条出题记录（即使部分废题被排除，也保留实际得到的题，方便回看）
    const rec = buildRecord(payload, meta.value, questions.value, elapsed.value, 'done', progress.value)
    records.value = saveRecord(rec)
    recordsOpen.value = true // 出题完成后自动展开记录面板，让刚生成的题可见可回看
  } catch (e) {
    // 失败时保留阶段面板：老师能看到卡在「检索资料」还是「模型命制」，
    // 而不是只收到一句笼统的「出题失败」。
    progressFailed.value = true
    progressSummary.value = isResume
      ? '出题中途断开，可刷新页面或点「生成 / 换一批」重试'
      : ('已等待 ' + elapsed.value + ' 秒，出题未完成')
    // 出题失败/中断：任务记录标为「未完成」，刷新后仍留在出题记录里，不会丢
    records.value = saveRecord(
      buildRecord(payload, meta.value, questions.value, elapsed.value, 'interrupted', progress.value)
    )
    // 首发出题失败：清掉快照避免刷新反复重试；续传场景下保留快照，便于再次重连。
    if (!isResume) clearGenerateJob()
    // 真正超过前端上限时给可操作的建议，而不是笼统的「出题失败」
    if (timedOut || e?.name === 'AbortError' || e?.code === 'ECONNABORTED'
        || /timeout/i.test(String(e?.message || ''))) {
      ElMessage.error('本次出题耗时过长，请减少题量或关闭「引用中文资料」后重试')
    } else {
      toastError(e, '出题失败')
    }
  } finally {
    stopStream()
    stopWaiting()
    loading.value = false
    resuming.value = false
  }
}

// 刷新后恢复：直接看 sessionStorage 里有没有没过期的出题作业。
function resumeIfNeeded() {
  const snap = loadGenerateJob()
  if (!snap || !snap.jobId) return
  if (Date.now() - (snap.t || 0) > JOB_SNAPSHOT_TTL) {
    clearGenerateJob()
    return
  }
  // 已完成：直接恢复题目，无需再连服务端（刷新后题目也不丢）
  if (snap.done && Array.isArray(snap.questions) && snap.questions.length) {
    loading.value = false
    progress.value = snap.progress || newGenerateProgress()
    progressSummary.value = generationProgressSummary(snap.elapsed || 0, snap.meta || {})
    meta.value = snap.meta || {}
    resolvedQtype.value = meta.value.qtype || form.value.qtype
    questions.value = normalizeGeneratedQuestions(snap.questions, resolvedQtype.value)
    selected.value = usableIndexes.value.slice()
    showStages.value = false
    return
  }
  // 进行中：恢复进度面板，用同一 job_id 重新订阅服务端流（断点续传）
  if (snap.payload && snap.payload.job_id) {
    loading.value = true
    resuming.value = true
    progress.value = snap.progress || newGenerateProgress()
    progressSummary.value = ''
    showStages.value = true
    elapsed.value = snap.elapsed || 0
    runGeneration(snap.payload, { isResume: true, initialElapsed: snap.elapsed || 0 })
  }
}

// ===== 出题记录：点进去回看 / 删除 / 清空 =====
// 打开一条历史记录：只切换「题目列表」的展示源，绝不打断 / 清除正在进行的出题作业。
// 进行中的出题（loading=true）会原样继续，进度面板依旧可见；查看完点「返回」即回到最新一批。
function openRecord(r) {
  viewingRecordId.value = r.id
  saveViewing(r.id)
  selected.value = []
  expanded.value = new Set()
}

// 退出历史查看，回到「最新一批 / 空」状态。
function returnToLatest() {
  viewingRecordId.value = null
  saveViewing(null)
  selected.value = []
  expanded.value = new Set()
}

function removeRecord(id) {
  ElMessageBox.confirm('确定删除这条出题记录？删除后无法恢复。', '删除记录', {
    confirmButtonText: '删除',
    cancelButtonText: '取消',
    type: 'warning',
  }).then(() => {
    records.value = deleteRecord(id)
    if (viewingRecordId.value === id) returnToLatest()
    ElMessage.success('已删除该记录')
  }).catch(() => {})
}

// 重试一条「未完成」的出题记录：按原参数（知识点 / 题型 / 数量 / 难度…）重新发起，
// 换新 seed 与新 job_id，避免服务端蓝图和上一批完全一样。旧的未完成记录随之移除，
// 由新一轮生成落一条新的 running 记录——重试成功与否，列表里始终只有这条任务的最新状态。
async function retryRecord(r) {
  if (loading.value) return // 有任务正在进行时不可重试，防止两批同时烧额度
  if (!r || r.status === 'running') return
  const base = (r.payload && typeof r.payload === 'object' && 'knowledge' in r.payload)
    ? { ...r.payload } // 新记录：整包留存了原始参数，按原样重发
    : { // 兼容老记录（未存 payload）：用记录上的字段兜底重建
      knowledge: r.knowledge || '',
      difficulty: r.difficulty || '中等',
      qtype: r.qtype || 'single',
      count: r.count || 1,
      score: r.score || 5,
      source: r.source === 'bank' ? 'bank' : 'ai',
      subject: r.subject || '',
      grounding: true,
      exclude_stems: [],
    }
  base.exclude_stems = Array.isArray(base.exclude_stems) ? base.exclude_stems.slice(0, 20) : []
  base.seed = newGenerationSeed() // 新蓝图 / 新采样，重试不是原样复读上一批
  base.job_id = newJobId()
  clearGenerateJob()
  viewingRecordId.value = null
  saveViewing(null)
  saveGenerateJob({
    jobId: base.job_id,
    payload: base,
    progress: newGenerateProgress(),
    meta: {},
    questions: [],
    elapsed: 0,
    done: false,
    t: Date.now(),
  })
  records.value = saveRecords(loadRecords().filter((x) => x.id !== r.id))
  records.value = saveRecord(buildRecord(base, {}, [], 0, 'running', newGenerateProgress()))
  ElMessage.info('已按原参数重新发起出题')
  await runGeneration(base)
}

function clearAllRecords() {
  if (!records.value.length) return
  ElMessageBox.confirm(`确定清空全部 ${records.value.length} 条出题记录？`, '清空记录', {
    confirmButtonText: '清空',
    cancelButtonText: '取消',
    type: 'warning',
  }).then(() => {
    clearRecords()
    records.value = []
    if (viewingRecordId.value) returnToLatest()
    ElMessage.success('已清空出题记录')
  }).catch(() => {})
}

// 记录卡片副标题：来源 / 学科 / 难度 / 资料段数等。
function recordMetaLine(r) {
  const m = r.meta || {}
  const parts = []
  if (r.source === 'bank') parts.push('通识热身')
  else {
    parts.push('中文专业题')
    if (r.subject) parts.push(r.subject)
  }
  if (r.difficulty) parts.push(r.difficulty)
  if (m.material_status === 'ok' && Array.isArray(m.materials)) parts.push(`资料 ${m.materials.length} 段`)
  if (m.attempts > 1) parts.push(`${m.attempts} 轮命制`)
  return parts.join(' · ')
}

async function addToAssignment() {
  if (!canAdd.value) {
    ElMessage.warning(blockedReason.value)
    return
  }
  adding.value = true
  let ok = 0
  let failed = null
  for (const idx of selected.value) {
    const payload = toAddQuestionPayload(questions.value[idx], resolvedQtype.value)
    try {
      await axios.post(`/api/assignment/${assignmentId.value}/question`, payload, auth)
      ok++
      // 已经入库的题从列表里摘掉，重复点按钮不会加两遍
      questions.value[idx].added = true
    } catch (e) {
      failed = e?.response?.data?.message || e?.message || '添加失败'
      break
    }
  }
  adding.value = false
  if (failed) {
    ElMessage.error(`已加入 ${ok} 题，第 ${ok + 1} 题失败：${failed}`)
  } else {
    ElMessage.success(`已加入 ${ok} 题`)
  }
  questions.value = questions.value.filter((q) => !q.added)
  selected.value = usableIndexes.value.slice()
  expanded.value = new Set() // 列表下标变了，展开状态不能沿用
  // 题目被加入作业后同步快照，刷新仍能看到剩余结果而不是回到出题前
  const snap = loadGenerateJob()
  if (snap && snap.jobId) saveGenerateJob({ ...snap, questions: questions.value })
}

function goGrade() {
  if (!assignmentId.value) return
  router.push(`/assignment/${assignmentId.value}/grade`)
}

onMounted(async () => {
  status.value = await loadAiStatus()
  loadClasses()
  const snap = loadGenerateJob()
  // 优先恢复进行中的出题（断点续传）；否则若刷新前正在看某条历史记录，重新打开它
  if (snap && snap.jobId && Date.now() - (snap.t || 0) <= JOB_SNAPSHOT_TTL) {
    resumeIfNeeded()
    return
  }
  clearGenerateJob()
  // 没有可续传的快照：把残留的「进行中」任务记录降级为「未完成」，
  // 避免一条早已不跑的任务在记录里永远显示「生成中…」。
  if (records.value.some((r) => r.status === 'running')) {
    records.value = saveRecords(records.value.map((r) => (
      r.status === 'running' ? { ...r, status: 'interrupted' } : r
    )))
  }
  const vId = loadViewing()
  if (vId) {
    const r = loadRecords().find((x) => x.id === vId)
    if (r) openRecord(r)
  }
})
</script>

<template>
  <div class="ai-generate">
    <el-alert
      v-if="!status.enabled"
      type="warning"
      :closable="false"
      :title="status.reason || 'AI 功能未启用'"
      description="AI 出题需要后端配置好 AiService 才能生成。"
      style="margin-bottom: 14px"
    />

    <el-alert
      v-if="!bankMode"
      type="success"
      :closable="false"
      title="按知识点生成简体中文专业题"
      style="margin-bottom: 12px"
    >
      先按知识点检索免费的中文权威条目作为事实依据，再由模型命制带解析、带依据编号的专业课题目；
      英文题、繁体题、空壳常识题会被判废重出。资料不可达时自动改用模型的学科知识出题，不影响生成。
      <div class="bank-links">
        资料源：
        <a href="https://zh.wikipedia.org" target="_blank" rel="noopener noreferrer">中文维基百科</a>
        ·
        <a href="https://zh.wikibooks.org" target="_blank" rel="noopener noreferrer">中文维基教科书</a>
        （免费、无需密钥，CC BY-SA 4.0；仅作命题依据，不直接抄题）
      </div>
    </el-alert>

    <el-alert
      v-if="bankMode"
      type="warning"
      :closable="false"
      title="通识热身 · 英文题库（不按知识点出题）"
      style="margin-bottom: 12px"
    >
      这是英文通识随机题经 AI 汉化后的结果，适合课堂热身，<b>不能替代按知识点出的中文专业题</b>；
      仅支持单选/判断，多选题自动按单选处理。需要专业考核请切回「中文专业题·按知识点」。
      <div class="bank-links">
        题库：
        <a href="https://opentdb.com" target="_blank" rel="noopener noreferrer">Open Trivia DB</a>
        ·
        <a href="https://the-trivia-api.com" target="_blank" rel="noopener noreferrer">The Trivia API</a>
        （均免费、无需密钥，CC BY-SA 4.0）
      </div>
    </el-alert>

    <div class="form">
      <el-input
        v-model="form.knowledge"
        type="textarea"
        :rows="2"
        :placeholder="bankMode ? '题库模式可不填知识点；填写也只作为备注' : '知识点写得具体些，例如「爱因斯坦光电方程与截止频率」'"
        style="max-width: 520px"
      />
      <div class="row">
        <el-select v-model="form.source" style="width: 210px" @change="onSourceChange">
          <el-option v-for="s in GEN_SOURCES" :key="s.value" :label="s.label" :value="s.value" />
        </el-select>
        <el-select
          v-if="!bankMode"
          v-model="form.subject"
          style="width: 145px"
          placeholder="学科（可选）"
          @change="resetResults"
        >
          <el-option v-for="s in GEN_SUBJECTS" :key="s.value" :label="s.label" :value="s.value" />
        </el-select>
        <span v-if="!bankMode" class="label">引用中文资料</span>
        <el-switch v-if="!bankMode" v-model="form.grounding" @change="resetResults" />
        <el-select v-model="form.qtype" style="width: 130px" @change="onQtypeChange">
          <el-option v-for="t in GEN_TYPES" :key="t.value" :label="t.label" :value="t.value" />
        </el-select>
        <el-select v-model="form.difficulty" style="width: 110px">
          <el-option v-for="d in DIFFICULTIES" :key="d" :label="d" :value="d" />
        </el-select>
        <el-input-number v-model="form.count" :min="1" :max="10" />
        <span class="label">题</span>
        <el-input-number v-model="form.score" :min="1" :max="100" />
        <span class="label">分/题</span>
        <el-button type="primary" :loading="loading" @click="generate">生成</el-button>
        <el-button :loading="loading" :disabled="loading" @click="generate">
          换一批
        </el-button>
        <span class="label">每次换批都会换随机命题角度并排除最近出过的题</span>
      </div>
      <!-- 出题阶段进度：四个阶段都由服务端真实事件驱动，不用定时器伪造百分比 -->
      <div
        v-if="loading || progressSummary"
        class="progress-panel"
        :class="{ 'is-done': !loading && !progressFailed, 'is-failed': !loading && progressFailed }"
      >
        <div class="progress-head">
          <span class="progress-title">
            {{ resuming
              ? '正在恢复出题进度…'
              : (loading
                ? (bankMode ? '正在抽取通识题并汉化' : '正在按知识点命制中文专业题')
                : (progressFailed ? '本批出题未完成' : '本批出题完成')) }}
          </span>
          <span class="progress-time">
            {{ loading ? '已等待 ' + elapsed + ' 秒' : progressSummary }}
          </span>
        </div>
        <div v-if="loading || progressFailed" class="bar">
          <div class="bar-fill" :style="{ width: progressPercent(progress) + '%' }"></div>
        </div>
        <div v-if="loading || showStages || progressFailed" class="steps">
          <div v-for="(s, i) in progress.stages" :key="s.key" class="step" :class="s.status">
            <span class="step-dot">
              {{ s.status === 'done' ? '✓' : s.status === 'skipped' ? '–' : s.status === 'error' ? '!' : i + 1 }}
            </span>
            <div class="step-body">
              <div class="step-label">{{ s.label }}</div>
              <div class="step-detail">{{ s.detail || s.hint }}</div>
            </div>
            <span v-if="s.status === 'running'" class="step-state">进行中</span>
            <span v-else-if="s.status === 'skipped'" class="step-state">跳过</span>
          </div>
        </div>
        <div v-if="loading" class="progress-foot">
          <span v-if="resuming">检测到页面刷新，正在从断点恢复进度，不会重新出题。</span>
          <span v-else>阶段由服务端实时推送，不是模拟动画；模型生成通常占 20~60 秒，请勿关闭页面。</span>
        </div>
        <div v-else class="progress-foot">
          <span v-if="progressFailed">可重试；若多次超时，建议减少题量或关闭「引用中文资料」。</span>
          <span v-else-if="streamFallback">进度流不可用，已自动用标准接口完成出题。</span>
          <el-button size="small" text @click="showStages = !showStages">
            {{ showStages ? '收起阶段明细' : '查看阶段明细' }}
          </el-button>
        </div>
      </div>
    </div>

    <!-- 出题记录：点「生成」即落一条记录（进行中/未完成/已完成），可点进去回看、可删除；刷新/关页都不丢 -->
    <div class="records-card">
      <div class="records-head" @click="recordsOpen = !recordsOpen">
        <span class="records-title">📚 出题记录（{{ records.length }}）</span>
        <span class="records-action">
          <el-button v-if="records.length" size="small" text type="danger" @click.stop="clearAllRecords">清空</el-button>
          <span class="chev" :class="{ up: recordsOpen }">▾</span>
        </span>
      </div>
      <div v-if="recordsOpen" class="records-body">
        <div v-if="!records.length" class="records-empty">还没有出题记录，生成题目后会自动沉淀在这里，随时回看或删除。</div>
        <div
          v-for="r in records"
          :key="r.id"
          class="rec"
          :class="{ active: viewingRecordId === r.id }"
          @click="openRecord(r)"
        >
          <div class="rec-main">
            <div class="rec-summary">
              <span
                v-if="r.status === 'running' || r.status === 'interrupted'"
                class="rec-tag"
                :class="r.status"
              >{{ r.status === 'running' ? '生成中' : '未完成' }}</span>
              {{ recordSummary(r) }}
            </div>
            <div class="rec-meta">{{ recordMetaLine(r) }}</div>
          </div>
          <span class="rec-actions">
            <el-button
              v-if="r.status === 'interrupted'"
              size="small"
              text
              type="primary"
              :disabled="loading"
              @click.stop="retryRecord(r)"
            >重试</el-button>
            <el-button size="small" text type="danger" @click.stop="removeRecord(r.id)">删除</el-button>
          </span>
        </div>
      </div>
    </div>

    <div class="target">
      <el-select v-model="classId" placeholder="选班级" style="width: 180px" @change="loadAssignments">
        <el-option v-for="c in classes" :key="c.id" :label="c.name" :value="c.id" />
      </el-select>
      <el-select v-model="assignmentId" placeholder="选作业" style="width: 220px" :disabled="!assignments.length">
        <el-option v-for="a in assignments" :key="a.id" :label="a.title" :value="a.id" />
      </el-select>
      <span class="hint">勾选题目后一键加入所选作业</span>
      <el-button v-if="assignmentId" size="small" text @click="goGrade">去批改</el-button>
    </div>

    <div class="list" v-if="displayQuestions.length">
      <div v-if="viewingRecordId" class="viewing-banner">
        <span>正在查看历史出题记录 · {{ recordSummary(currentViewing) }}</span>
        <el-button size="small" text @click="returnToLatest">返回 / 查看最新</el-button>
      </div>
      <div class="list-bar">
        <span class="meta">共 {{ displayQuestions.length }} 题 · 点击卡片查看完整题干与选项</span>
        <el-button size="small" text @click="toggleAll">
          {{ allExpanded ? '全部收起' : '全部展开' }}
        </el-button>
      </div>
      <div v-if="!viewingRecordId && metaText" class="meta-line">本批随机 · {{ metaText }}</div>
      <div v-if="!viewingRecordId && materialSources.length" class="sources">
        <span class="src-label">本批依据</span>
        <a
          v-for="(m, i) in materialSources"
          :key="i"
          :href="m.url"
          target="_blank"
          rel="noopener noreferrer"
          @click.stop
        >[{{ i + 1 }}] {{ m.title }}<span v-if="m.provider"> · {{ m.provider }}</span></a>
        <span class="label">CC BY-SA 4.0</span>
      </div>
      <el-checkbox-group v-model="selected">
        <div
          v-for="(q, i) in displayQuestions"
          :key="i"
          class="q"
          :class="{ invalid: !isQuestionUsable(q), open: expanded.has(i) }"
          @click="toggleExpand(i)"
        >
          <!-- 头部有复选框，阻止冒泡，避免勾选时误触发展开/收起 -->
          <div class="q-head" @click.stop>
            <el-checkbox :value="i" :disabled="!isQuestionUsable(q)">第 {{ i + 1 }} 题</el-checkbox>
            <el-tag v-if="q.invalid" size="small" type="danger">{{ q.invalid_reason || '结构不合格' }}</el-tag>
            <span
              class="toggle"
              role="button"
              tabindex="0"
              :aria-expanded="expanded.has(i)"
              @click="toggleExpand(i)"
              @keydown.enter="toggleExpand(i)"
              @keydown.space.prevent="toggleExpand(i)"
            >
              {{ expanded.has(i) ? '收起' : '展开题目' }}
              <span class="chev" :class="{ up: expanded.has(i) }">▾</span>
            </span>
          </div>

          <!-- 折叠态：题干摘要 + 答案 -->
          <div v-if="!expanded.has(i)" class="stem preview" :class="{ 'is-missing': !q.stem }">
            {{ stemPreview(q, i) }}
          </div>
          <!-- 展开态：完整题干 + 全部选项；缺数据也给明确提示，不再留一片空白 -->
          <template v-else>
            <div v-if="q.stem" class="stem full">{{ q.stem }}</div>
            <div v-else class="stem is-missing">（模型这次没返回题干，建议重新生成）</div>
            <div class="opts" v-if="optionLines(q).length">
              <div v-for="o in optionLines(q)" :key="o.letter" class="opt">{{ o.letter }}. {{ o.text }}</div>
            </div>
            <div v-else-if="displayQtype === 'single' || displayQtype === 'multiple'" class="opts is-missing">
              （选项缺失，建议重新生成）
            </div>
            <div v-if="questionExplanation(q)" class="explanation">
              <span class="exp-label">解析</span><span>{{ questionExplanation(q) }}</span>
            </div>
            <div v-if="questionBasis(q)" class="basis">
              <span class="basis-label">命题依据</span>{{ questionBasis(q) }}
            </div>
            <div class="tags" v-if="questionFocus(q) || q.quality_warning || q.source || q.quality_score">
              <el-tag v-if="questionFocus(q)" size="small" effect="plain">{{ questionFocus(q) }}</el-tag>
              <el-tag v-if="q.quality_score" size="small" type="success" effect="plain">
                专业度 {{ q.quality_score }}
              </el-tag>
              <el-tag v-if="q.quality_warning" size="small" type="warning" effect="plain">
                {{ q.quality_warning }}
              </el-tag>
              <el-tag v-if="q.source" size="small" type="info" effect="plain">公开题库</el-tag>
            </div>
          </template>

          <div class="ans">
            <span class="ans-label">参考答案</span>
            <span>{{ answerLabel(q, displayQtype) || '（空）' }}</span>
            <span class="score">{{ q.score || form.score }} 分</span>
          </div>
        </div>
      </el-checkbox-group>

      <div class="footer">
        <span class="meta" v-if="invalidCount">{{ invalidCount }} 道结构不合格，已排除</span>
        <span class="meta" v-else-if="blockedReason && chosen.length">{{ blockedReason }}</span>
        <el-button
          type="primary"
          :loading="adding"
          :disabled="!canAdd"
          @click="addToAssignment"
        >
          加入作业（{{ chosen.length }} 题）
        </el-button>
      </div>
    </div>

    <el-empty v-else-if="!loading" description="先生成题目，再勾选加入作业" :image-size="80" />
  </div>
</template>

<style scoped>
.ai-generate {
  max-width: 900px;
  margin: 0 auto;
  padding: 16px;
}
.row {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 10px;
  flex-wrap: wrap;
}
.label {
  font-size: 13px;
  color: #64748b;
}
.target {
  display: flex;
  align-items: center;
  gap: 10px;
  margin: 14px 0;
  flex-wrap: wrap;
}
.hint {
  font-size: 12px;
  color: #94a3b8;
  flex: 1;
}
.list-bar {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 8px;
}
.q {
  /* 关键：Element Plus 的 .el-checkbox-group 会设 font-size: 0 来消除
     inline-block 间隙。卡片内所有元素都会继承 0，导致只写了 line-height:1.7
     的题干/选项被算成 0 高度 —— DOM 里有文字，页面上却看不见，只剩显式设了
     字号的 .ans 答案行。这里必须显式把字号找回来。 */
  font-size: 14px;
  line-height: 1.6;
  background: #fff;
  border: 1px solid #e5e7eb;
  border-radius: 10px;
  padding: 12px 14px;
  margin-bottom: 10px;
  cursor: pointer;
  transition: border-color 0.15s, box-shadow 0.15s;
}
.q:hover {
  border-color: #93c5fd;
}
.q.open {
  border-color: #3b82f6;
  box-shadow: 0 0 0 2px rgba(59, 130, 246, 0.08);
}
.q.invalid {
  background: #fff5f5;
  border-color: #fecaca;
  opacity: 0.75;
}
.q-head {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 6px;
}
.toggle {
  margin-left: auto;
  font-size: 12px;
  color: #2563eb;
  padding: 2px 6px;
  border-radius: 6px;
  cursor: pointer;
  user-select: none;
}
.toggle:hover {
  background: #eff6ff;
}
.toggle:focus-visible {
  outline: 2px solid #3b82f6;
  outline-offset: 2px;
}
.chev {
  display: inline-block;
  transition: transform 0.15s;
}
.chev.up {
  transform: rotate(180deg);
}
.stem {
  font-size: 14px;
  color: #1e293b;
  line-height: 1.7;
}
.stem.preview {
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  color: #334155;
}
.stem.full {
  white-space: pre-wrap;
  word-break: break-word;
}
.stem.is-missing,
.opts.is-missing {
  color: #b45309;
  background: #fffbeb;
  border-radius: 6px;
  padding: 4px 8px;
  font-size: 13px;
}
.opts {
  margin: 8px 0;
}
.opts.is-missing {
  margin: 8px 0;
}
.opt {
  font-size: 14px;
  color: #475569;
  line-height: 1.7;
}
.ans {
  display: flex;
  align-items: baseline;
  gap: 8px;
  font-size: 13px;
  color: #0f172a;
  border-top: 1px dashed #e2e8f0;
  padding-top: 8px;
}
.ans-label {
  color: #64748b;
}
.score {
  margin-left: auto;
  color: #64748b;
}
.footer {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-top: 12px;
}
.meta {
  font-size: 13px;
  color: #94a3b8;
  flex: 1;
}
.meta-line {
  font-size: 12px;
  color: #64748b;
  background: #f8fafc;
  border-radius: 6px;
  padding: 6px 10px;
  margin-bottom: 10px;
  line-height: 1.5;
}
.explanation {
  font-size: 13px;
  color: #334155;
  line-height: 1.7;
  background: #f8fafc;
  border-left: 3px solid #93c5fd;
  border-radius: 4px;
  padding: 6px 10px;
  margin: 8px 0;
}
.exp-label {
  color: #2563eb;
  font-weight: 600;
  margin-right: 8px;
}
.tags {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
  margin: 6px 0;
  font-size: 12px;
}
.sources {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
  font-size: 12px;
  color: #64748b;
  background: #f8fafc;
  border-radius: 6px;
  padding: 6px 10px;
  margin-bottom: 10px;
  line-height: 1.6;
}
.src-label,
.basis-label {
  color: #2563eb;
  font-weight: 600;
  flex-shrink: 0;
}
.sources a {
  color: #2563eb;
  text-decoration: none;
}
.sources a:hover {
  text-decoration: underline;
}
.basis {
  font-size: 12px;
  color: #475569;
  line-height: 1.6;
  margin: 6px 0;
}
.basis-label {
  margin-right: 8px;
}
.bank-links {
  font-size: 12px;
  color: #64748b;
  margin-top: 4px;
}
.bank-links a {
  color: #2563eb;
  text-decoration: none;
}
.bank-links a:hover {
  text-decoration: underline;
}
.progress-panel {
  margin-top: 12px;
  border: 1px solid #bfdbfe;
  background: #f8fbff;
  border-radius: 8px;
  padding: 10px 14px;
}
.progress-panel.is-done {
  border-color: #bbf7d0;
  background: #f6fef9;
}
.progress-panel.is-failed {
  border-color: #fecaca;
  background: #fff8f8;
}
.progress-panel.is-failed .progress-title {
  color: #b91c1c;
}
.progress-head {
  display: flex;
  align-items: baseline;
  gap: 12px;
  font-size: 13px;
}
.progress-title {
  color: #1d4ed8;
  font-weight: 600;
}
.is-done .progress-title {
  color: #15803d;
}
.progress-time {
  margin-left: auto;
  color: #64748b;
  font-variant-numeric: tabular-nums;
}
.bar {
  height: 6px;
  background: #e2e8f0;
  border-radius: 999px;
  overflow: hidden;
  margin: 10px 0 12px;
}
.bar-fill {
  height: 100%;
  background: linear-gradient(90deg, #3b82f6, #6366f1);
  border-radius: 999px;
  transition: width 0.45s ease;
}
.steps {
  display: grid;
  gap: 8px;
}
.step {
  display: flex;
  align-items: center;
  gap: 10px;
  font-size: 13px;
  color: #94a3b8;
}
.step-dot {
  flex: 0 0 20px;
  width: 20px;
  height: 20px;
  border-radius: 50%;
  display: grid;
  place-items: center;
  font-size: 11px;
  background: #e2e8f0;
  color: #64748b;
}
.step.done { color: #334155; }
.step.done .step-dot { background: #16a34a; color: #fff; }
.step.running { color: #1d4ed8; }
.step.running .step-dot {
  background: #3b82f6;
  color: #fff;
  animation: step-pulse 1.4s ease-in-out infinite;
}
.step.skipped { opacity: 0.7; }
.step.error { color: #b91c1c; }
.step.error .step-dot { background: #dc2626; color: #fff; }
@keyframes step-pulse {
  0%, 100% { box-shadow: 0 0 0 0 rgba(59, 130, 246, 0.35); }
  50% { box-shadow: 0 0 0 5px rgba(59, 130, 246, 0); }
}
.step-body {
  flex: 1;
  min-width: 0;
}
.step-label { font-weight: 600; }
.step-detail {
  font-size: 12px;
  color: #94a3b8;
  margin-top: 2px;
  line-height: 1.5;
}
.step.running .step-detail { color: #3b82f6; }
.step-state {
  font-size: 12px;
  flex-shrink: 0;
}
.progress-foot {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-top: 8px;
  font-size: 12px;
  color: #94a3b8;
}
/* 出题记录面板 */
.records-card {
  margin: 14px 0;
  border: 1px solid #e5e7eb;
  border-radius: 10px;
  background: #fff;
  overflow: hidden;
}
.records-head {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 14px;
  cursor: pointer;
  user-select: none;
}
.records-head:hover {
  background: #f8fafc;
}
.records-title {
  font-weight: 600;
  color: #1e293b;
  font-size: 14px;
}
.records-action {
  margin-left: auto;
  display: flex;
  align-items: center;
  gap: 8px;
}
.records-body {
  border-top: 1px solid #f1f5f9;
  padding: 6px 10px 10px;
}
.records-empty {
  padding: 14px;
  text-align: center;
  color: #94a3b8;
  font-size: 13px;
}
.rec {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 10px;
  border-radius: 8px;
  cursor: pointer;
  transition: background 0.12s, border-color 0.12s;
  border: 1px solid transparent;
}
.rec:hover {
  background: #f1f5ff;
}
.rec.active {
  background: #eff6ff;
  border-color: #93c5fd;
}
.rec-main {
  flex: 1;
  min-width: 0;
}
.rec-summary {
  font-size: 13px;
  color: #0f172a;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.rec-tag {
  display: inline-block;
  margin-right: 6px;
  padding: 0 6px;
  border-radius: 8px;
  font-size: 11px;
  line-height: 18px;
  vertical-align: 1px;
}
.rec-tag.running {
  background: #ecfdf5;
  border: 1px solid #6ee7b7;
  color: #047857;
}
.rec-tag.interrupted {
  background: #fef2f2;
  border: 1px solid #fca5a5;
  color: #b91c1c;
}
.rec-actions {
  flex-shrink: 0;
  white-space: nowrap;
}
.rec-meta {
  font-size: 12px;
  color: #64748b;
  margin-top: 2px;
}
/* 查看历史记录时的提示条 */
.viewing-banner {
  display: flex;
  align-items: center;
  gap: 10px;
  background: #eff6ff;
  border: 1px solid #bfdbfe;
  color: #1d4ed8;
  border-radius: 8px;
  padding: 8px 12px;
  margin-bottom: 10px;
  font-size: 13px;
}
.viewing-banner > span {
  flex: 1;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
</style>
