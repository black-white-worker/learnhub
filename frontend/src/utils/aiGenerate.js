// AI 出题结果 → 平台题目结构的纯转换逻辑（抽出来是为了脱离浏览器单测）。
//
// 必须在这里转换的原因是：ai-service 出的题和平台入库的题字段形状不一样。
//   - 选项：AI 给数组，平台存 options 字段的 JSON 字符串；
//   - 判断题：AI 给布尔 true/false，平台按字符串 "true"/"false" 判分
//     （见 assignment_utils_test.go 的 {Type:"judge", Answer:"true"}），
//     直接把布尔 POST 过去连 Go 的 string 字段都反序列化不了；
//   - 多选题：AI 可能给 "ABD"，也可能给 ["A","B","D"]，统一成逗号分隔。
//
// 题型只放平台作业能渲染的 4 种：AssignmentTab 里没有 fill 分支，
// AI 生成的填空题加进去只会显示成空白题干，所以前端不暴露这个选项。

export const GEN_TYPES = [
  { value: 'single', label: '单选题' },
  { value: 'multiple', label: '多选题' },
  { value: 'judge', label: '判断题' },
  { value: 'essay', label: '主观题' },
]

export const DIFFICULTIES = ['简单', '中等', '困难']

// 题目来源：默认「按知识点出的简体中文专业题」（自动检索中文权威资料作为依据）；
// bank 是上一版接入的英文通识题库，只适合课堂热身，不按知识点出题。
export const GEN_SOURCES = [
  { value: 'ai', label: '中文专业题·按知识点' },
  { value: 'bank', label: '通识热身·英文题库（汉化）' },
]

// 学科语境：让术语、符号、难度贴合学科；空值由服务端按知识点自行判断。
export const GEN_SUBJECTS = [
  { value: '', label: '自动识别学科' },
  { value: '物理学', label: '物理学' },
  { value: '化学', label: '化学' },
  { value: '生物学', label: '生物学' },
  { value: '数学', label: '数学' },
  { value: '统计学', label: '统计学' },
  { value: '计算机科学', label: '计算机科学' },
  { value: '医学', label: '医学' },
  { value: '经济学', label: '经济学' },
  { value: '历史学', label: '历史学' },
  { value: '地理学', label: '地理学' },
  { value: '文学', label: '文学 / 语言学' },
  { value: '法学', label: '法学' },
  { value: '教育学', label: '教育学' },
  { value: '心理学', label: '心理学' },
]

// 每次生成都换一个新种子：服务端据此采样命题蓝图、洗牌选项、抖动采样参数。
// 抽出来是为了能在 node --test 里验证「两次请求的载荷确实不同」。
export function newGenerationSeed() {
  const c = globalThis.crypto
  if (c?.randomUUID) return c.randomUUID()
  if (c?.getRandomValues) {
    const bytes = new Uint8Array(8)
    c.getRandomValues(bytes)
    return Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('')
  }
  return `${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`
}

// 出题作业号：用于在服务端「断点续传」。刷新后前端用同一个 job_id 重新订阅 SSE，
// 服务端作业与连接解耦，不会因刷新而中断或重跑。优先用 crypto.randomUUID。
export function newJobId() {
  const c = globalThis.crypto
  if (c?.randomUUID) return c.randomUUID()
  if (c?.getRandomValues) {
    const b = new Uint8Array(16)
    c.getRandomValues(b)
    return Array.from(b, (x) => x.toString(16).padStart(2, '0')).join('')
  }
  return `job-${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`
}

// 进度快照持久化：用 localStorage（比 sessionStorage 更稳，关标签页 / 刷新 / 重开浏览器都不丢）。
// 刷新或关页后据此恢复，避免「一刷新进度就没了」。单用户多标签页共用同一份，无副作用。
const GEN_JOB_KEY = 'ai-generate-job'

export function saveGenerateJob(snapshot) {
  try {
    localStorage.setItem(GEN_JOB_KEY, JSON.stringify(snapshot))
  } catch {
    // localStorage 不可用时静默降级（不会丢功能，只是不跨刷新保留）
  }
}
export function loadGenerateJob() {
  try {
    return JSON.parse(localStorage.getItem(GEN_JOB_KEY) || 'null')
  } catch {
    return null
  }
}
export function clearGenerateJob() {
  try {
    localStorage.removeItem(GEN_JOB_KEY)
  } catch {
    // 忽略
  }
}

// ===== 出题记录（历史）=====
// 每次出题从「点下生成」那一刻就落一条记录（status=running），随进度更新，完成转 done、
// 失败转 interrupted；存 localStorage：刷新 / 关页 / 重开浏览器都不丢 —— 修复「只进行到
// 前两步的任务一刷新就没了」。老师可点进去回看题目、也可单独删除。
// 需要跨设备 / 多浏览器共享时再改服务端持久化。
const GEN_RECORDS_KEY = 'ai-generate-records'
const GEN_RECORDS_MAX = 50

export function loadRecords() {
  try {
    const arr = JSON.parse(localStorage.getItem(GEN_RECORDS_KEY) || '[]')
    return Array.isArray(arr) ? arr : []
  } catch {
    return []
  }
}
export function saveRecord(record) {
  const list = loadRecords().filter((r) => r.id !== record.id)
  list.unshift(record)
  const trimmed = list.slice(0, GEN_RECORDS_MAX)
  try {
    localStorage.setItem(GEN_RECORDS_KEY, JSON.stringify(trimmed))
  } catch {
    // 忽略
  }
  return trimmed
}
export function deleteRecord(id) {
  const list = loadRecords().filter((r) => r.id !== id)
  try {
    localStorage.setItem(GEN_RECORDS_KEY, JSON.stringify(list))
  } catch {
    // 忽略
  }
  return list
}
export function clearRecords() {
  try {
    localStorage.removeItem(GEN_RECORDS_KEY)
  } catch {
    // 忽略
  }
}

// 批量写回记录列表（用于刷新时把残留的「进行中」任务降级为「未完成」）。
export function saveRecords(list) {
  const trimmed = (Array.isArray(list) ? list : []).slice(0, GEN_RECORDS_MAX)
  try {
    localStorage.setItem(GEN_RECORDS_KEY, JSON.stringify(trimmed))
  } catch {
    // 忽略
  }
  return trimmed
}

// 由一次出题拼出一条记录对象（id 复用 job_id，便于去重与续传）。
// status: 'running'（进行中）/ 'done'（已完成）/ 'interrupted'（中断/失败）。
// 即使只跑到前两步就刷新，'running' 记录也已落盘，刷新后仍在出题记录里可见、可续传，不会丢。
export function buildRecord(payload = {}, meta = {}, questions = [], elapsed = 0, status = 'done', progress = null) {
  return {
    id: payload.job_id || newJobId(),
    createdAt: Date.now(),
    status,
    progress: progress || null,
    // 原始请求参数整包留存：「未完成」记录可凭它一键重试（按原参数重新出题）。
    payload: payload && typeof payload === 'object' ? { ...payload } : {},
    knowledge: payload.knowledge || '',
    qtype: meta.qtype || payload.qtype || 'single',
    count: payload.count || 0,
    source: payload.source || 'ai',
    subject: payload.subject || '',
    difficulty: payload.difficulty || '',
    score: payload.score || 0,
    meta: meta || {},
    questions: Array.isArray(questions) ? questions : [],
    elapsed,
  }
}

// 记录卡片上的一句话摘要（知识点 + 题数 + 时间；进行中/中断态单独标注）。
export function recordSummary(r = {}) {
  const status = r.status || 'done'
  const n = Array.isArray(r.questions) ? r.questions.length : 0
  const d = new Date(r.createdAt || Date.now())
  const pad = (x) => String(x).padStart(2, '0')
  const t = `${d.getMonth() + 1}月${d.getDate()}日 ${pad(d.getHours())}:${pad(d.getMinutes())}`
  if (status === 'running') return `${r.knowledge || '（未填知识点）'} · 生成中…`
  if (status === 'interrupted') return `${r.knowledge || '（未填知识点）'} · 未完成 · ${n} 题 · ${t}`
  return `${r.knowledge || '（未填知识点）'} · ${n} 题 · ${t}`
}

// 正在查看的历史记录 id 持久化，刷新后能自动重新打开同一条记录。
const GEN_VIEWING_KEY = 'ai-generate-viewing'
export function saveViewing(id) {
  try {
    if (id) localStorage.setItem(GEN_VIEWING_KEY, id)
    else localStorage.removeItem(GEN_VIEWING_KEY)
  } catch {
    // 忽略
  }
}
export function loadViewing() {
  try {
    return localStorage.getItem(GEN_VIEWING_KEY) || null
  } catch {
    return null
  }
}

// 组装一次出题请求。previousStems 是最近几批题干，服务端会把它们列为「禁止重复」，
// 这是「换一批不能还是同一批」最直接的保证（连模型真的偷懒返回旧题也会被判掉）。
export function buildGeneratePayload(form = {}, { seed, previousStems = [] } = {}) {
  const source = form.source === 'bank' ? 'bank' : 'ai'
  const stems = [...new Set(
    (Array.isArray(previousStems) ? previousStems : [])
      .map((s) => String(s ?? '').trim())
      .filter(Boolean)
  )].slice(0, 20)
  const count = Math.min(10, Math.max(1, Number(form.count) || 1))
  const score = Math.max(1, Number(form.score) || 5)
  return {
    knowledge: String(form.knowledge ?? '').trim().slice(0, 200),
    difficulty: form.difficulty || '中等',
    qtype: form.qtype || 'single',
    count,
    score,
    source,
    // 学科 + 中文资料增强：服务端按知识点检索中文权威条目作为命题依据，
    // 失败自动回退纯模型中文出题；关掉 grounding 可跳过外网检索。
    subject: String(form.subject ?? '').trim().slice(0, 24),
    grounding: form.grounding !== false,
    seed: seed || newGenerationSeed(),
    exclude_stems: stems,
  }
}

// 服务端新增的题目元数据（解析 / 考查点），兼容模型偶发的中文键名。
export function questionExplanation(q) {
  const v = q?.explanation ?? q?.解析 ?? q?.analysis ?? ''
  return String(v ?? '').trim()
}
export function questionFocus(q) {
  const v = q?.focus ?? q?.考查点 ?? q?.考点 ?? ''
  return String(v ?? '').trim()
}
// 命题依据：模型按中文资料命制时写出的「依据[1]：……」，用来核对专业性与出处。
export function questionBasis(q) {
  const v = q?.basis ?? q?.依据 ?? ''
  return String(v ?? '').trim()
}

const PROVIDER_LABELS = {
  opentdb: 'Open Trivia DB',
  'the-trivia-api': 'The Trivia API',
}

// 把 meta 压成一行人话，让老师看得见「这批题为什么和上一批不一样」。
export function generationMetaText(meta = {}) {
  if (!meta || typeof meta !== 'object') return ''
  const parts = []
  if (meta.seed) parts.push(`批次 ${String(meta.seed).slice(0, 8)}`)
  if (Array.isArray(meta.strategy) && meta.strategy.length) {
    const items = [...new Set(meta.strategy)].slice(0, 3).join(' / ')
    parts.push(meta.source === 'bank' ? `分类 ${items}` : `角度 ${items}`)
  }
  if (meta.source === 'bank' && meta.bank_provider) {
    parts.push(`来源 ${PROVIDER_LABELS[meta.bank_provider] || meta.bank_provider}${meta.localized ? '·已汉化' : ''}`)
  } else if (meta.source === 'ai' && String(meta.bank_status || '').startsWith('fallback')) {
    parts.push('题库不可用·已回退 AI 出题')
  }
  if (meta.source !== 'bank' && meta.grounding !== false) {
    const n = Array.isArray(meta.materials) ? meta.materials.length : 0
    if (meta.material_status === 'ok') {
      parts.push(`中文资料 ${n} 条${meta.subject ? `·${meta.subject}` : ''}`)
    } else if (meta.material_status === 'unavailable') {
      parts.push('资料源不可用·已用模型专业出题')
    } else if (meta.material_status === 'no_match') {
      parts.push('未检索到资料·已用模型专业出题')
    } else if (meta.material_status === 'disabled') {
      parts.push('未引用资料')
    }
  }
  if (meta.deduped) parts.push(`排重 ${meta.deduped} 题`)
  if (meta.discarded_count) parts.push(`自动重出 ${meta.discarded_count} 题`)
  if (meta.attempts > 1) parts.push(`补齐 ${meta.attempts} 轮`)
  return parts.join(' · ')
}

// 保留最近若干批题干，跨多次「换一批」持续生效（服务端自己也有历史缓存）。
export function collectStems(questions = [], previous = [], limit = 20) {
  const all = [
    ...(Array.isArray(questions) ? questions : []).map((q) => String(q?.stem ?? '').trim()),
    ...(Array.isArray(previous) ? previous : []).map((s) => String(s ?? '').trim()),
  ].filter(Boolean)
  return [...new Set(all)].slice(0, limit)
}

// 服务端校验没过（options 数量、答案越界等）的题不让勾选，避免污染作业。
export function isQuestionUsable(q) {
  return Boolean(q) && !q.invalid && String(q.stem || '').trim() !== ''
}

// 判断题答案归一：布尔 / "对" / "正确" / "1" 都算 true。
export function normalizeJudgeAnswer(ans) {
  if (typeof ans === 'boolean') return ans ? 'true' : 'false'
  const s = String(ans ?? '').trim().toLowerCase()
  if (['true', '1', '对', '正确', '是', 'yes', 'y', 't'].includes(s)) return 'true'
  if (['false', '0', '错', '错误', '否', 'no', 'n', 'f'].includes(s)) return 'false'
  return s
}

export function normalizeMultiAnswer(ans) {
  if (Array.isArray(ans)) {
    return ans
      .map((a) => String(a).trim().toUpperCase())
      .filter(Boolean)
      .sort()
      .join(',')
  }
  return String(ans ?? '')
    .split(/[,，\s]*/)
    .map((a) => a.trim().toUpperCase())
    .filter(Boolean)
    .sort()
    .join(',')
}

// toAddQuestionPayload 把一道 AI 题目转成 POST /api/assignment/:aid/question 的请求体。
export function toAddQuestionPayload(q, qtype) {
  const type = qtype || 'single'
  const score = Number(q?.score) > 0 ? Number(q.score) : 5
  const payload = {
    type,
    stem: String(q?.stem || '').trim(),
    options: '',
    answer: '',
    score,
  }
  const options = Array.isArray(q?.options) ? q.options.map((o) => String(o)) : []
  if (type === 'single' || type === 'multiple') {
    payload.options = JSON.stringify(options)
  }
  if (type === 'judge') {
    payload.answer = normalizeJudgeAnswer(q?.answer)
  } else if (type === 'multiple') {
    payload.answer = normalizeMultiAnswer(q?.answer)
  } else {
    payload.answer = Array.isArray(q?.answer) ? q.answer.join(' ') : String(q?.answer ?? '').trim()
  }
  return payload
}

// payloadProblem 返回这道题不能入库的原因（null 表示可以）。
// 服务端只标了结构不合格，没管"参考答案为空"——那种题批不了也没意义。
export function payloadProblem(payload) {
  if (!payload.stem) return '题干为空'
  if (payload.type === 'judge' && payload.answer !== 'true' && payload.answer !== 'false') {
    return '判断题答案必须是 对/错'
  }
  if ((payload.type === 'single' || payload.type === 'multiple') && !payload.options) {
    return '缺少选项'
  }
  if (payload.type === 'single' && !/^[A-D]$/.test(payload.answer)) {
    return '单选答案要写成 A-D 之一'
  }
  if (payload.type === 'multiple' && payload.answer.length < 2) {
    return '多选答案至少两个字母'
  }
  if (payload.type === 'essay' && !payload.answer) {
    return '主观题没有参考答案，AI 批改时无法对照'
  }
  return null
}

// 把模型/题库可能给出的各种「题目形状」归一成前端稳定结构。
// 模型偶尔会用中文键名、把 options 写成 JSON 字符串或对象、把答案写成
// "C. 地址寄存器（AR）" 这种连写；不归一时卡片就会出现「题干空白、只剩答案」。
export function normalizeOptions(raw) {
  if (raw == null || raw === '') return []
  if (Array.isArray(raw)) {
    return raw
      .map((o) => {
        if (o == null) return ''
        if (typeof o === 'object') {
          const v = o.text ?? o.label ?? o.value ?? o.content ?? o.option ?? Object.values(o)[0]
          return String(v ?? '').trim()
        }
        return String(o).trim()
      })
      // 选项自带的 "A. " 前缀要去掉，否则渲染出来会变成 "A. A. xxx"
      .map((s) => s.replace(/^\s*[A-Da-d]\s*[.、。:：)）]\s*/, ''))
      .filter(Boolean)
  }
  if (typeof raw === 'string') {
    const s = raw.trim()
    if (s.startsWith('[') || s.startsWith('{')) {
      try {
        const parsed = JSON.parse(s)
        if (Array.isArray(parsed)) return normalizeOptions(parsed)
        if (parsed && typeof parsed === 'object') {
          return normalizeOptions(Object.keys(parsed).sort().map((k) => parsed[k]))
        }
      } catch (_) {
        // 不是合法 JSON，按下面的分隔文本继续处理
      }
    }
    return normalizeOptions(s.split(/[,，;；\n]+/))
  }
  if (typeof raw === 'object') {
    return normalizeOptions(Object.keys(raw).sort().map((k) => raw[k]))
  }
  return []
}

export function normalizeQuestionShape(raw, qtype = 'single') {
  const q = raw && typeof raw === 'object' ? { ...raw } : {}
  const pick = (...keys) => {
    for (const k of keys) {
      const v = q[k]
      if (v !== undefined && v !== null && !(typeof v === 'string' && !v.trim())) return v
    }
    return undefined
  }
  const stem = String(pick('stem', '题干', '题目', '问题', 'question', 'title', 'content') ?? '').trim()
  const options = normalizeOptions(pick('options', '选项', 'choices', 'opts'))
  let answer = pick('answer', '答案', 'correctAnswer', 'correct_answer', 'key')
  if (typeof answer === 'boolean') answer = answer ? 'true' : 'false'
  else if (Array.isArray(answer)) answer = answer.map((a) => String(a).trim().toUpperCase()).join(',')
  else if (
    typeof answer === 'string' &&
    options.length &&
    !['judge', 'essay', 'fill'].includes(qtype)
  ) {
    // "C. 地址寄存器（AR）" → "C"（有选项时才收敛；没选项就保留原文，
    // 否则页面上连答案文字都看不见了）
    const m = answer.trim().match(/^\s*([A-Da-d](?:\s*[,，]\s*[A-Da-d])*)\s*[.、。:：]?\s*.+$/)
    if (m) answer = m[1].replace(/[\s,，]/g, '').toUpperCase()
  }
  const n = Number(pick('score', '分数', '分值'))
  const score = Number.isFinite(n) && n > 0 ? n : 0
  return { stem, options, answer: answer == null ? '' : String(answer).trim(), score }
}

export function normalizeGeneratedQuestions(list, qtype = 'single') {
  if (!Array.isArray(list)) return []
  return list.map((raw) => ({
    ...(raw && typeof raw === 'object' ? raw : {}),
    ...normalizeQuestionShape(raw, qtype),
  }))
}

// 展示用：把答案渲染成人话（判断题显示 对/错，选项显示 A/B/C/D 前缀）。
export function answerLabel(q, qtype) {
  if (qtype === 'judge') {
    const v = normalizeJudgeAnswer(q?.answer)
    return v === 'true' ? '对' : v === 'false' ? '错' : String(q?.answer ?? '')
  }
  if (Array.isArray(q?.options) && q.options.length) {
    const ans = qtype === 'multiple' ? normalizeMultiAnswer(q.answer) : String(q?.answer ?? '')
    // 平台多选存的是 "A,C"，AI 可能给 "AC"，统一按非字母切出选项字母
    const letters = ans.toUpperCase().split(/[^A-Z]/).filter(Boolean)
    const texts = q.options.map((o, i) => `${String.fromCharCode(65 + i)}. ${o}`)
    const picked = texts.filter((_, i) => letters.includes(String.fromCharCode(65 + i)))
    return picked.length ? picked.join('　') : `答案：${ans}`
  }
  return String(q?.answer ?? '')
}

export function optionLines(q) {
  if (!Array.isArray(q?.options)) return []
  return q.options.map((o, i) => ({ letter: String.fromCharCode(65 + i), text: String(o) }))
}

// ===== 出题阶段进度 =====
// 服务端按真实阶段推 SSE 事件（准备蓝图 → 检索资料 → 模型命制 → 质量校验 → 完成），
// 这里只做纯函数归约：不用计时器假装进度，UI 也能脱离浏览器单测。
export const GENERATE_STAGES = [
  { key: 'prepare', label: '准备命题蓝图', hint: '分配随机考查角度与答案位置' },
  { key: 'materials', label: '检索中文权威资料', hint: '中文维基百科 / 维基教科书' },
  { key: 'compose', label: '命制专业题', hint: '题干、选项、解析与依据' },
  { key: 'verify', label: '质量校验与排重', hint: '中文专业度、结构、相似题' },
]

export function newGenerateProgress() {
  return {
    stages: GENERATE_STAGES.map((s) => ({ ...s, status: 'pending', detail: '' })),
    active: '',
    done: false,
    summary: '',
    lastElapsed: 0,
  }
}

export function reduceGenerateProgress(state = newGenerateProgress(), evt = {}) {
  if (!evt || typeof evt !== 'object' || !evt.stage) return state
  const elapsed = Math.max(state.lastElapsed || 0, Number(evt.elapsed_ms) || 0)

  if (evt.stage === 'done') {
    return {
      ...state,
      stages: state.stages.map((s) => ({
        ...s,
        status: s.status === 'skipped' ? 'skipped' : 'done',
      })),
      active: '',
      done: true,
      summary: String(evt.detail || '出题完成'),
      lastElapsed: elapsed,
    }
  }

  // 以后服务端加新阶段，前端不能因此卡死或报错
  if (!state.stages.some((s) => s.key === evt.stage)) {
    return { ...state, lastElapsed: elapsed }
  }
  const status = evt.status === 'skipped' ? 'skipped'
    : evt.status === 'done' ? 'done'
      : evt.status === 'error' ? 'error' : 'running'

  return {
    ...state,
    stages: state.stages.map((s) => {
      if (s.key === evt.stage) {
        return {
          ...s,
          label: evt.label || s.label,
          status,
          detail: String(evt.detail || s.detail || s.hint),
        }
      }
      // 进入新阶段时，把上一个还在 running 的阶段收尾
      if (status === 'running' && s.status === 'running') return { ...s, status: 'done' }
      return s
    }),
    active: status === 'running' ? evt.stage : (state.active === evt.stage ? '' : state.active),
    lastElapsed: elapsed,
  }
}

// 进度条只按「已完成的阶段数」推进，进行中的阶段算半格，不会假装精确到百分比。
export function progressPercent(state = newGenerateProgress()) {
  if (state.done) return 100
  const stages = state.stages || []
  const total = stages.length || 1
  const finished = stages.filter((s) => s.status === 'done' || s.status === 'skipped').length
  const running = stages.some((s) => s.status === 'running')
  return Math.min(95, Math.round(((finished + (running ? 0.5 : 0)) / total) * 100))
}

export function generationProgressSummary(elapsed = 0, meta = {}) {
  const parts = [`耗时 ${elapsed} 秒`]
  if (Array.isArray(meta.materials) && meta.materials.length) {
    parts.push(`引用 ${meta.materials.length} 段资料`)
  }
  if (meta.attempts) parts.push(`${meta.attempts} 轮命制`)
  if (meta.discarded_count) parts.push(`重出 ${meta.discarded_count} 题`)
  if (meta.deduped) parts.push(`排重 ${meta.deduped} 题`)
  return parts.join(' · ')
}
