// 大文件视频分片直传（>250MB）+ 断点续传。
//
// 为什么不能继续用整文件表单上传：
//   1. 一次 POST 几百 MB，浏览器/网关抖动一下就从头再来，用户等到天荒地老；
//   2. axios 的 onUploadProgress 只有"已发字节"，发出去不等于落到 MinIO
//      （中间还有浏览器缓冲、Nginx 缓冲），进度条到 99% 然后失败是最常见的坑；
//   3. 后端中转大文件会把 API 容器的内存/临时盘吃满。
//
// 方案：前端把文件切成 5MB 的分片，逐片 PUT 到后端签发的预签名 URL（直连 MinIO），
// 全部落地后再调 complete 让后端合并。后端不碰字节流。
//
// 断点续传靠两样东西：
//   - localStorage 里记 uploadId（刷新页面还在）；
//   - 服务端 /multipart/status 回报"桶里真实存在哪些分片、各多少字节"。
// 只信服务端那一份：本地记账可能在"传完但没记上"的那一刻丢掉，
// 而分片大小对不上说明这片是半截的，必须重传。
//
// 纯函数 + 依赖注入（api / storage / put），所以在 Node 里就能跑单测。

import { DONE_PERCENT, formatBytes, transferPercent } from './uploadProgress.js'

// MinIO ComposeObject 要求除最后一片外每片 ≥ 5MB，所以 5MB 是下限也是默认值
export const CHUNK_SIZE = 5 * 1024 * 1024

// 250MB 以下走老的表单上传：一次请求就够，没必要多花十几倍的请求数
export const MULTIPART_THRESHOLD = 250 * 1024 * 1024

// 单个文件硬上限 800MB，与后端 Upload.MaxVideoBytes 的默认值一致。
// 在这里挡一道是为了"选完立刻报错"：不挡的话要等分片 init 回包（或被服务端拒）
// 才知道传不了，用户白等一轮。后端仍是权威校验，改上限要两边一起改。
export const MAX_FILE_BYTES = 800 * 1024 * 1024

// 一批签 40 片：预签名 URL 有效期 15 分钟，签太早会在传到时过期
export const SIGN_WINDOW = 40

// 同时传 4 片：再多会挤满家用上行带宽，反而拖慢整体
export const PART_CONCURRENCY = 4

// 单片最多重试 3 次（每次都可能带一次重签）
export const PART_RETRIES = 3

const RESUME_PREFIX = 'lp-video-upload'

export function shouldUseMultipart(file, threshold = MULTIPART_THRESHOLD) {
  return Number(file?.size || 0) > threshold
}

// 总分片数（按字节切，最后一片可能不满）
export function chunkPlan(fileSize, chunkSize = CHUNK_SIZE) {
  const size = Number(fileSize) || 0
  const step = Number(chunkSize) || CHUNK_SIZE
  return { chunkSize: step, total: Math.max(1, Math.ceil(size / step)) }
}

// 第 n 片（从 1 开始）对应的字节区间
export function partRange(n, fileSize, chunkSize = CHUNK_SIZE) {
  const step = Number(chunkSize) || CHUNK_SIZE
  const start = (n - 1) * step
  const end = Math.max(start, Math.min(Number(fileSize) || 0, start + step))
  return { start, end, size: end - start }
}

// 服务端回报的分片 → 真正传完的片号集合。
// 判定标准是"片号存在 且 字节数正好等于该片应有大小"：
// 大小不符 = 半截分片（PUT 中途断线），留着它会合出一个坏视频。
export function completedParts(parts, fileSize, chunkSize = CHUNK_SIZE) {
  const { total } = chunkPlan(fileSize, chunkSize)
  const done = new Set()
  for (const p of parts || []) {
    const n = Number(p?.partNumber)
    if (!Number.isInteger(n) || n < 1 || n > total) continue
    if (Number(p?.size) === partRange(n, fileSize, chunkSize).size) done.add(n)
  }
  return done
}

// 还没传完的片号（升序）
export function missingParts(parts, fileSize, chunkSize = CHUNK_SIZE) {
  const done = completedParts(parts, fileSize, chunkSize)
  const { total } = chunkPlan(fileSize, chunkSize)
  const out = []
  for (let n = 1; n <= total; n += 1) {
    if (!done.has(n)) out.push(n)
  }
  return out
}

// 续传记账 key：课程 + 文件名 + 大小 + 修改时间。
// 换了别的文件（哪怕同名）也不会误接上次的分片。
export function resumeKey(courseId, file) {
  return [
    RESUME_PREFIX,
    courseId,
    encodeURIComponent(file?.name || ''),
    file?.size || 0,
    file?.lastModified || 0,
  ].join(':')
}

// localStorage 在无痕模式/配额满时会让整个上传直接抛错，
// 这里统一吞掉：丢了续传记账只是要重传，不该挡住上传本身。
export function saveResume(storage, key, session) {
  try {
    storage?.setItem(key, JSON.stringify(session))
  } catch {
    /* 存不下就当没有续传 */
  }
}

export function loadResume(storage, key) {
  try {
    const raw = storage?.getItem(key)
    return raw ? JSON.parse(raw) : null
  } catch {
    return null
  }
}

export function clearResume(storage, key) {
  try {
    storage?.removeItem(key)
  } catch {
    /* 忽略 */
  }
}

export class UploadCancelled extends Error {
  constructor(message = '上传已取消') {
    super(message)
    this.name = 'UploadCancelled'
  }
}

// 数组切片（不用 slice 链式写法，方便按窗口取）
function chunkArray(arr, size) {
  const out = []
  for (let i = 0; i < arr.length; i += size) out.push(arr.slice(i, i + size))
  return out
}

// 解包 rawClient 的 {data} 信封：api 层用的是不解包的实例
function unwrap(res) {
  return res?.data ?? res
}

// 有界并发跑一批任务：全部跑完才返回，任一任务失败不影响其他任务
// （失败重试在任务内部自己做，这里的并发只是"同时最多几个"）
async function runBounded(tasks, limit) {
  const queue = tasks.slice()
  const runners = []
  const n = Math.max(1, Math.min(Number(limit) || 1, queue.length))
  for (let i = 0; i < n; i += 1) {
    runners.push(
      (async () => {
        while (queue.length) {
          const task = queue.shift()
          await task()
        }
      })(),
    )
  }
  await Promise.all(runners)
}

/**
 * 分片直传一个视频文件。
 *
 * @param {File} file                要上传的视频
 * @param {object} opts
 * @param {number} opts.courseId
 * @param {object} opts.api          api/index.js 里的 multipart 相关方法
 * @param {function} opts.onProgress ({percent,loaded,total,phase}) => void  // loaded 用进度条通用字段名
 * @param {AbortSignal} opts.signal  取消上传（会顺手 abort 服务端会话并清记账）
 * @param {Storage} opts.storage     默认 globalThis.localStorage
 * @param {function} opts.put        单片 PUT 实现，默认 globalThis.fetch（测试可注入）
 * @returns {Promise<object>}        complete 接口返回的 {courseId, video}
 */
export async function uploadByMultipart(file, opts) {
  const {
    courseId,
    api,
    onProgress,
    signal,
    storage = globalThis.localStorage,
    put = defaultPut,
    concurrency = PART_CONCURRENCY,
    windowSize = SIGN_WINDOW,
    retries = PART_RETRIES,
  } = opts || {}

  if (!file || !file.size) throw new Error('空文件，无法上传')
  if (file.size > MAX_FILE_BYTES) {
    throw new Error(
      `${file.name} 有 ${formatBytes(file.size)}，超过单个文件 ${MAX_FILE_BYTES >> 20}MB 上限`,
    )
  }
  if (!api) throw new Error('缺少 api，无法分片上传')

  const key = resumeKey(courseId, file)
  // 取消只处理一次：多个并发分片任务会同时发现 signal 被置位，
  // 各调一次 abort 会让服务端收到好几个重复请求（还会互相覆盖错误）。
  let cancelHandled = false
  const abortIfCancelled = async (uploadId) => {
    if (!signal?.aborted) return
    if (!cancelHandled) {
      cancelHandled = true
      // 用户取消：让服务端删掉已传分片（顺带归还当天额度），本地记账一起清掉
      if (uploadId) await api.abortMultipartUpload({ uploadId }).catch(() => {})
      clearResume(storage, key)
    }
    throw new UploadCancelled()
  }

  // ===== 1. 续传：先看服务端还记得哪些片 =====
  let session = loadResume(storage, key)
  let done = new Set()
  if (session?.uploadId) {
    const status = await api
      .fetchMultipartUploadStatus({ uploadId: session.uploadId })
      .then(unwrap)
      .catch(() => null)
    const usable =
      status &&
      status.resumable &&
      Number(status.fileSize) === file.size &&
      status.fileName === file.name
    if (usable) {
      // 分片大小以服务端为准：换窗口/换浏览器后本地记的 chunkSize 可能是旧的，
      // 边界一变，已有的分片就对不上新切片了。
      session.chunkSize = Number(status.chunkSize) || session.chunkSize || CHUNK_SIZE
      done = completedParts(status.parts, file.size, session.chunkSize)
    } else {
      clearResume(storage, key)
      session = null
    }
  }

  // ===== 2. 没有可用会话就新建一个 =====
  if (!session) {
    await abortIfCancelled(null)
    const init = unwrap(await api.initMultipartUpload({ courseId, fileName: file.name, fileSize: file.size }))
    if (!init?.uploadId) throw new Error('服务端没有返回 uploadId')
    session = {
      uploadId: init.uploadId,
      chunkSize: Number(init.chunkSize) || CHUNK_SIZE,
      objectName: init.objectName,
    }
    saveResume(storage, key, session)
    done = new Set()
  }

  const { chunkSize, total } = chunkPlan(file.size, session.chunkSize)
  const report = (phase, uploadedSet) => {
    let uploaded = 0
    for (const n of uploadedSet) uploaded += partRange(n, file.size, chunkSize).size
    // 分片全落地只是字节到了桶里，服务端 ComposeObject 还没回话，
    // 所以除 done 外一律钉在 99%：让进度条贴在 100% 不动是最容易被当成卡死的形状。
    const percent =
      phase === 'done' ? DONE_PERCENT : transferPercent(uploaded, file.size)
    onProgress?.({
      percent,
      // 字段名跟 uploadProgress.js 的状态盒对齐（loaded/total），
      // 不然分片上传的字节数进不了进度条，会一直显示 0B
      loaded: uploaded,
      total: file.size,
      phase,
      partCount: total,
    })
  }
  report('resuming', done)

  // ===== 3. 缺哪些片，按窗口批量签 URL 后并发直传 =====
  const missing = []
  for (let n = 1; n <= total; n += 1) {
    if (!done.has(n)) missing.push(n)
  }

  for (const group of chunkArray(missing, windowSize)) {
    await abortIfCancelled(session.uploadId)
    let urls = await signParts(api, session.uploadId, group, retries)

    const tasks = group.map((n) => async () => {
      const { start, end } = partRange(n, file.size, chunkSize)
      const blob = file.slice(start, end)
      let lastErr
      for (let attempt = 0; attempt < retries; attempt += 1) {
        if (signal?.aborted) break
        try {
          await put(urls[n], blob, { signal })
          // 成功一片就记账：中途刷新页面时这一片不用重传
          done.add(n)
          saveResume(storage, key, { ...session, doneParts: Array.from(done) })
          report('uploading', done)
          return
        } catch (err) {
          lastErr = err
          if (signal?.aborted) break
          // 403/408/5xx 多半是 URL 过期或网络抖动，重签这一片再试
          urls = { ...urls, ...(await resignOne(api, session.uploadId, n, retries)) }
          await delay(300 * (attempt + 1))
        }
      }
      if (signal?.aborted) {
        await abortIfCancelled(session.uploadId)
      }
      throw new Error(`第 ${n} 片上传失败：${lastErr?.message || '未知原因'}`)
    })

    await runBounded(tasks, concurrency)
    await abortIfCancelled(session.uploadId)
  }

  // ===== 4. 让后端合并 =====
  report('merging', done)
  const parts = []
  for (let n = 1; n <= total; n += 1) {
    parts.push({ partNumber: n, size: partRange(n, file.size, chunkSize).size })
  }
  let res
  try {
    res = unwrap(await api.completeMultipartUpload({ uploadId: session.uploadId, parts }))
  } catch (err) {
    // 合并失败保留记账：修好之后重传同一片或调 complete 都能继续
    throw new Error(err?.response?.data?.msg || err?.message || '分片合并失败，请重试')
  }
  clearResume(storage, key)
  report('done', done)
  return res
}

async function signParts(api, uploadId, numbers, retries) {
  let lastErr
  for (let attempt = 0; attempt < Math.max(1, retries); attempt += 1) {
    try {
      const resp = unwrap(await api.signMultipartParts({ uploadId, partNumbers: numbers }))
      const urls = {}
      for (const p of resp?.parts || []) urls[Number(p.partNumber)] = p.url
      const missingUrl = numbers.filter((n) => !urls[n])
      if (missingUrl.length) throw new Error(`服务端没有签出第 ${missingUrl.join(',')} 片`)
      return urls
    } catch (err) {
      lastErr = err
      await delay(300 * (attempt + 1))
    }
  }
  throw new Error(`获取直传地址失败：${lastErr?.message || '未知原因'}`)
}

async function resignOne(api, uploadId, n, retries) {
  try {
    return await signParts(api, uploadId, [n], retries)
  } catch {
    return {}
  }
}

function delay(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms))
}

// 直传一片：PUT 到预签名 URL。
// 不手动设 Content-Type —— 预签名没把 content-type 算进签名，
// 让浏览器按 Blob 类型带上去，MinIO 只认签名不认类型。
async function defaultPut(url, blob, { signal } = {}) {
  const res = await fetch(url, { method: 'PUT', body: blob, signal })
  if (!res.ok) {
    const err = new Error(`直传失败（HTTP ${res.status}）`)
    err.status = res.status
    throw err
  }
  return res
}

export default uploadByMultipart
