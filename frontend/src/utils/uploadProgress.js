// 上传进度的统一算法：资源面板、课程详情、课程列表快捷上传、创建页共用这一套规则。
//
// 为什么要单独成文件：同一个文件在三个页面上传，进度数字必须一致，
// 否则用户第一次看到"这个页面 60%、那个页面 80%"就不会再信进度条了。
//
// 关键约定：**字节发完 ≠ 上传完成**。请求还在等服务端写对象存储、建库记录、
// 排队转码，所以传输进度封顶 99%，拿到响应才置 100。
// 中间那段显示"服务端处理中"，而不是让进度条死死贴在 100% 看着像卡死。

// 传输阶段的进度上限（未收到服务端 ack 之前不许超过它）
export const TRANSFER_CEIL = 99
// 收到响应之后的完成值
export const DONE_PERCENT = 100

/**
 * 已传字节 → 百分比。total 未知（浏览器没给 lengthComputable）时返回 0，
 * 让调用方退回"上传中"文案而不是显示一根假进度条。
 */
export function transferPercent(loaded, total) {
  const t = Number(total)
  if (!Number.isFinite(t) || t <= 0) return 0
  const p = Math.round(((Number(loaded) || 0) / t) * 100)
  if (p <= 0) return 0
  return Math.min(TRANSFER_CEIL, p)
}

/**
 * 把 axios 的进度事件拆成 {loaded,total,computable}。
 * computable=false 时调用方应只显示阶段文案、不显示百分比。
 */
export function readProgressEvent(e) {
  const total = Number(e?.total) || 0
  const loaded = Number(e?.loaded) || 0
  return { loaded, total, computable: total > 0 }
}

// 阶段标识 → 给人看的中文。resuming/uploading/merging 由 videoUpload.js 报出，
// transferring/processing/queued 由表单上传与服务端回包报出。
export const PHASE_LABELS = {
  resuming: '检查断点续传',
  uploading: '分片直传中',
  merging: '服务端合并分片中',
  transferring: '上传中',
  processing: '服务端处理中',
  queued: '已提交，服务器排队转码中',
  done: '上传完成',
}

export function phaseLabel(phase) {
  return PHASE_LABELS[phase] || PHASE_LABELS.transferring
}

export function formatBytes(n) {
  const bytes = Number(n)
  // null/undefined 是"不知道多大"，和"0 字节的空文件"不是一回事
  if (n == null || !Number.isFinite(bytes) || bytes < 0) return '-'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let i = 0
  let v = bytes
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i += 1
  }
  return `${v.toFixed(i === 0 ? 0 : 1)}${units[i]}`
}

/**
 * 进度条下面那一行小字：阶段 · 文件名 · 已传/总量。
 * 缺哪块就不拼哪块，避免出现 "上传中 ·  · 0B/0B" 这种噪音。
 */
export function progressText({ phase, file, loaded, total } = {}) {
  const parts = []
  if (phase) parts.push(phaseLabel(phase))
  if (file) parts.push(String(file))
  const t = Number(total) || 0
  if (t > 0) parts.push(`${formatBytes(Number(loaded) || 0)} / ${formatBytes(t)}`)
  return parts.join(' · ')
}

/**
 * 生成一个 axios onUploadProgress 回调：把进度事件转成统一的
 * {percent, phase, loaded, total, file} 汇报给上层的 onProgress。
 * 字节发满时自动切到"服务端处理中"——那正是进度条最容易看起来卡死的一刻。
 */
export function makeUploadProgressReporter(fileLabel, onProgress) {
  return (e) => {
    const { loaded, total, computable } = readProgressEvent(e)
    const sentAll = computable && loaded >= total
    onProgress?.({
      percent: transferPercent(loaded, total),
      phase: sentAll ? 'processing' : 'transferring',
      loaded,
      total: computable ? total : 0,
      file: fileLabel,
    })
  }
}

/** 收到响应：100% + 结束阶段。视频转码是异步的，用 queued 说清楚后话。 */
export function reportDone(onProgress, { file, phase = 'done', total } = {}) {
  onProgress?.({ percent: DONE_PERCENT, phase, loaded: total || 0, total: total || 0, file })
}

/** 多个文件一次提交时没有单个文件名，用"N 个视频"当标签。 */
export function batchLabel(count, kind = '文件') {
  const n = Number(count) || 0
  return n > 1 ? `${n} 个${kind}` : ''
}

// ===== 组件侧的小状态盒 =====
// 三个上传页面都要做同一件事：把 onProgress 汇报的对象并进一个响应式结构，
// 上传开始前清零。写成纯函数是为了能在 Node 里直接单测（组件本身不测）。

export function newProgressState() {
  return { percent: 0, phase: '', file: '', loaded: 0, total: 0 }
}

/** 收拢一次汇报；字段缺失就不覆盖旧值（阶段文案不会被后来的空值抹掉）。 */
export function applyProgressEvent(state, e) {
  if (!state || !e) return state
  if (typeof e.percent === 'number') state.percent = e.percent
  if (e.phase) state.phase = e.phase
  if (e.file) state.file = e.file
  if (typeof e.loaded === 'number') state.loaded = e.loaded
  if (typeof e.total === 'number') state.total = e.total
  return state
}

export function resetProgressState(state) {
  if (!state) return state
  state.percent = 0
  state.phase = ''
  state.file = ''
  state.loaded = 0
  state.total = 0
  return state
}

/** 进度条下面那行字（阶段 · 文件 · 已传/总量）。 */
export function progressLine(state) {
  return progressText(state || {})
}
