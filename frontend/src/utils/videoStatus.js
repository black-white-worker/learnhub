// 视频后处理（faststart）状态在前端的展示规则。
//
// 为什么要单独成文件：这段判断同时被课程详情、观看页和单测需要，
// 而且它有一个很容易写错的分支 —— 老数据 / 老后端根本不返回 processStatus，
// 这时候只能按 faststarted 推断，绝不能显示成"转码失败"把老师吓去重点重试。
//
// 状态取值与后端 course_videos.process_status 一致：
//   pending      排队中（还没轮到）
//   processing   转码中（worker 已认领）
//   done         已生成 moov 前置版本，可以边下边播、自由拖进度条
//   failed       自动重试 3 次仍失败，可以手工重试

export const POLL_INTERVAL_MS = 5000

// 未处理的原视频照样能播（只是拖动进度条要等整片下载完），
// 所以 pending/processing 一律不给红色，也不挡住"播放"按钮。
const LABELS = {
  pending: { text: '排队转码中', type: 'info', hint: '视频已可播放；转码完成后拖动进度条才会即时响应' },
  processing: { text: '正在转码', type: 'info', hint: '服务器正在生成可快速拖动的版本，通常几分钟内完成' },
  done: { text: '已优化', type: 'success', hint: '已生成 faststart 版本，可边下边播、自由拖动进度条' },
  failed: { text: '转码失败', type: 'danger', hint: '' },
}

/**
 * 归一化后端返回的处理状态。
 * @param {{processStatus?: string, faststarted?: boolean}} video
 * @returns {'pending'|'processing'|'done'|'failed'}
 */
export function normalizeProcessStatus(video) {
  const raw = String(video?.processStatus || '').trim().toLowerCase()
  if (raw === 'done' || raw === 'failed' || raw === 'processing' || raw === 'pending') return raw
  // 后端没给状态（老数据或老版本）：按是否有 faststart 版本推断
  return video?.faststarted ? 'done' : 'pending'
}

export function processStatusTag(video) {
  const status = normalizeProcessStatus(video)
  const label = LABELS[status] || LABELS.pending
  return {
    status,
    text: label.text,
    type: label.type,
    // 失败时把后端原因带上，老师一眼知道是格式问题还是服务器问题
    hint: status === 'failed' ? video?.processError || label.hint || '转码失败，可点右侧"重试"重新排队' : label.hint,
  }
}

// 只有失败状态才允许人工重新排队：pending/processing 本来就在队列里，
// done 再转一遍是白烧服务器 CPU。
export function canRetryProcess(video) {
  return normalizeProcessStatus(video) === 'failed'
}

// 还有没有"在跑"的任务 —— 决定要不要继续轮询。
export function hasRunningProcess(videos) {
  return (videos || []).some((v) => {
    const s = normalizeProcessStatus(v)
    return s === 'pending' || s === 'processing'
  })
}
