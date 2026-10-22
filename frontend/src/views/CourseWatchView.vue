<script setup>
import { ref, onMounted, onBeforeUnmount } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
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
}

// 播放位置变化：节流上报（每 5 秒一次，避免频繁请求）
function onTimeUpdate() {
  const v = videoRef.value
  if (!v || !v.duration) return
  if (saveTimer.value) return
  saveTimer.value = setTimeout(async () => {
    saveTimer.value = null
    try {
      await saveVideoProgress(courseId, videoId, Math.floor(v.currentTime))
    } catch {
      // 上报失败静默忽略，不影响观看
    }
  }, 5000)
}

// 离开页面前再报一次当前位置
async function flushProgress() {
  const v = videoRef.value
  if (!v) return
  try {
    await saveVideoProgress(courseId, videoId, Math.floor(v.currentTime))
  } catch {
    // ignore
  }
}

onMounted(load)
onBeforeUnmount(flushProgress)
</script>

<template>
  <div class="watch-page" v-loading="loading">
    <div class="bar">
      <el-button link @click="router.back()">← 返回课程</el-button>
      <span class="title">{{ video ? video.name : '视频播放' }}</span>
      <el-tag v-if="video && video.faststarted" type="success" size="small">可拖动播放</el-tag>
    </div>

    <video
      v-if="video"
      ref="videoRef"
      :src="video.url"
      controls
      preload="metadata"
      class="player"
      @loadedmetadata="onLoadedMetadata"
      @timeupdate="onTimeUpdate"
    ></video>
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
</style>
