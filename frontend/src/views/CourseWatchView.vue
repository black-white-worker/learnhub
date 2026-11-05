<script setup>
import { ref, onMounted, onBeforeUnmount } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import axios from 'axios'
import {
  getVideoPlay,
  saveVideoProgress,
  fetchVideoProgress,
} from '../api/index.js'

const route = useRoute()
const router = useRouter()

const courseId = Number(route.params.id)
const videoId = Number(route.params.videoId)

const videoRef = ref(null)
const video = ref(null)
const loading = ref(true)
const saveTimer = ref(null)

// 笔记
const notes = ref([])
const noteText = ref('')
const auth = { headers: { Authorization: 'Bearer ' + (localStorage.getItem('token') || '') } }

async function loadNotes() {
  try {
    const r = await axios.get(`/api/course/note/${courseId}/${videoId}`, auth)
    notes.value = r.data || []
  } catch (e) {
    notes.value = []
  }
}

async function addNote() {
  if (!noteText.value.trim()) return
  const v = videoRef.value
  const ts = v ? Math.floor(v.currentTime) : 0
  try {
    await axios.post('/api/course/note', {
      courseId, videoId, timestampSeconds: ts, content: noteText.value.trim(),
    }, auth)
    ElMessage.success('笔记已保存')
    noteText.value = ''
    loadNotes()
  } catch (e) {
    ElMessage.error('保存失败')
  }
}

function seekTo(ts) {
  const v = videoRef.value
  if (!v) return
  v.currentTime = ts
  v.play()
}

function fmtTs(s) {
  const m = Math.floor(s / 60), sec = s % 60
  return `${m}:${sec.toString().padStart(2, '0')}`
}

async function load() {
  loading.value = true
  try {
    const res = await getVideoPlay(courseId, videoId)
    video.value = res.data.video

    // 拉上次播放位置，准备续播
    try {
      const prog = await fetchVideoProgress(courseId, videoId)
      video.value.lastPosition = prog.data.position || 0
    } catch {
      video.value.lastPosition = 0
    }
  } catch (e) {
    ElMessage.error('视频加载失败：' + (e.response?.data?.msg || e.message))
  } finally {
    loading.value = false
  }
}

// 元信息加载完：如果有上次进度，跳到该位置续播
function onLoadedMetadata() {
  const v = videoRef.value
  if (!v) return
  if (video.value && video.value.lastPosition > 5) {
    v.currentTime = video.value.lastPosition
  }
  // 续播的跳转不算"看过"，同步基准位置，避免第一帧被计成观看时长
  lastPosition = v.currentTime
}

// ===== 学习进度（按用户落库，对标 Open edX progress）=====
//
// 关键点：只上报"真实播放过的秒数增量"，拖动进度条不计入。
// 服务端用累计观看时长判断是否看完（>= 时长 90%），而不是"播到第几秒"，
// 否则拖到结尾会被算成看完了。
const watchedDelta = ref(0)      // 上次上报以来真实播放的秒数
let lastTickAt = 0               // 上一次 timeupdate 的时间戳（判断是否连续播放）
const REPORT_INTERVAL = 8000     // 每 8 秒上报一次

function onPlayStart() {
  lastTickAt = Date.now()
}

function onTimeUpdate() {
  const v = videoRef.value
  if (!v || !v.duration || v.paused) return

  // 1. 累计真实观看时长：只有两次 timeupdate 间隔接近正常播放节奏才算
  //    （拖动进度条会触发一次 timeupdate，但时间戳间隔会很短、位置跳变很大）
  const now = Date.now()
  const wall = (now - lastTickAt) / 1000
  lastTickAt = now
  const rate = v.playbackRate || 1
  if (wall > 0 && wall < 2) {
    const expected = wall * rate
    const moved = v.currentTime - lastPosition
    // 位置变化和真实时间大致吻合（允许 1 秒误差），才认为是"真的在看"
    if (Math.abs(moved - expected) < 1.5) {
      watchedDelta.value += Math.max(0, Math.min(moved, expected))
    }
  }
  lastPosition = v.currentTime

  // 2. 节流上报
  if (saveTimer.value) return
  saveTimer.value = setTimeout(async () => {
    saveTimer.value = null
    await reportProgress()
  }, REPORT_INTERVAL)
}

let lastPosition = 0

async function reportProgress() {
  const v = videoRef.value
  if (!v || !v.duration) return
  const delta = Math.round(watchedDelta.value)
  watchedDelta.value -= delta
  try {
    await saveVideoProgress(courseId, videoId, {
      position: Math.floor(v.currentTime),
      watchedDelta: delta,
      duration: Math.round(v.duration),
    })
  } catch {
    // 上报失败静默忽略，不影响观看；把增量加回去等下次一起报
    watchedDelta.value += delta
  }
}

// 离开页面前再报一次（用 sendBeacon 保证页面关闭也能送达）
function flushProgress() {
  const v = videoRef.value
  if (!v || !v.duration) return
  const token = localStorage.getItem('token')
  const body = JSON.stringify({
    position: Math.floor(v.currentTime),
    watchedDelta: Math.round(watchedDelta.value),
    duration: Math.round(v.duration),
  })
  watchedDelta.value = 0
  try {
    const url = `/api/course/video/progress/${courseId}/${videoId}`
    if (navigator.sendBeacon) {
      // sendBeacon 不能自定义 header，用 POST body 里的 token 走不通，
      // 因此退回同步 fetch（keepalive 允许页面卸载后继续发送）
      fetch(url, {
        method: 'POST',
        keepalive: true,
        headers: { 'Content-Type': 'application/json', Authorization: 'Bearer ' + token },
        body,
      })
    }
  } catch {
    // ignore
  }
}

onMounted(() => { load(); loadNotes() })
onBeforeUnmount(flushProgress)
</script>

<template>
  <div class="watch-page" v-loading="loading">
    <div class="bar">
      <el-button link @click="router.back()">← 返回课程</el-button>
      <span class="title">{{ video ? video.name : '视频播放' }}</span>
      <el-tag v-if="video && video.faststarted" type="success" size="small">可拖动播放</el-tag>
    </div>

    <div class="watch-layout">
      <div>
        <video
          v-if="video"
          ref="videoRef"
          :src="video.url"
          controls
          preload="metadata"
          class="player"
          @loadedmetadata="onLoadedMetadata"
          @play="onPlayStart"
          @timeupdate="onTimeUpdate"
        ></video>
      </div>
      <aside class="notes-panel">
        <h3>我的笔记</h3>
        <div class="note-input">
          <el-input v-model="noteText" type="textarea" :rows="3" placeholder="在当前时间点记笔记" />
          <el-button type="primary" size="small" style="margin-top:6px" @click="addNote">保存笔记</el-button>
        </div>
        <div v-for="n in notes" :key="n.id" class="note-item" @click="seekTo(n.timestampSeconds)">
          <span class="note-ts">{{ fmtTs(n.timestampSeconds) }}</span>
          <span class="note-content">{{ n.content }}</span>
        </div>
        <el-empty v-if="!notes.length" description="暂无笔记" :image-size="60" />
      </aside>
    </div>
  </div>
</template>

<style scoped>
.watch-page {
  max-width: 960px;
  margin: 0 auto;
  padding: 20px 16px 48px;
  min-height: 70vh;
}

.bar {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 14px;
}

.title {
  font-size: 16px;
  font-weight: 600;
  color: #1f2937;
  flex: 1;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.player {
  width: 100%;
  aspect-ratio: 16 / 9;
  background: #000;
  border-radius: 12px;
  display: block;
}

.watch-layout {
  display: grid;
  grid-template-columns: 1fr 280px;
  gap: 16px;
}

.notes-panel {
  border-left: 1px solid #eee;
  padding-left: 16px;
}

.notes-panel h3 { margin: 0 0 10px; font-size: 14px; }
.note-item { padding: 6px 4px; border-bottom: 1px dashed #eee; cursor: pointer; font-size: 13px; }
.note-ts { color: #e6a23c; font-weight: 600; margin-right: 6px; }
.note-content { color: #333; }
</style>
