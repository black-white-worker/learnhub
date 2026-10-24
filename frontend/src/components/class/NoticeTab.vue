<template>
  <div>
    <div v-if="isTeacher" class="toolbar">
      <el-input v-model="newNotice.title" placeholder="公告标题" style="width:200px" />
      <el-input v-model="newNotice.content" type="textarea" placeholder="公告内容" style="width:400px" />
      <el-button type="primary" @click="postNotice">发布</el-button>
    </div>
    <div v-for="n in notices" :key="n.id" class="notice-item" @click="openNotice(n)">
      <el-badge is-dot :hidden="n.readByMe" class="notice-dot">
        <el-tag v-if="n.isTop" type="warning" size="small">置顶</el-tag>
      </el-badge>
      <b :class="{ 'notice-unread': !n.readByMe }">{{ n.title }}</b>
      <span v-if="isTeacher" class="read-stat">已读 {{ n.readCount }}</span>
      <p>{{ n.content }}</p>
      <small>{{ n.createdAt }}</small>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import axios from 'axios'

const props = defineProps({
  classId: { type: [String, Number], required: true },
  isTeacher: { type: Boolean, default: false },
})

const token = localStorage.getItem('token')
const auth = { headers: { Authorization: 'Bearer ' + token } }

const notices = ref([])
const newNotice = ref({ title: '', content: '' })

async function loadNotices() {
  const r = await axios.get(`/api/class/${props.classId}/announcements`, auth)
  notices.value = r.data || []
}

async function postNotice() {
  await axios.post(`/api/class/${props.classId}/announcement`, newNotice.value, auth)
  newNotice.value = { title: '', content: '' }
  loadNotices()
}

// 打开公告：未读则调接口标记已读，本地立刻把红点去掉
async function openNotice(n) {
  if (n.readByMe) return
  try {
    await axios.post(`/api/class/announcement/${n.id}/read`, {}, auth)
    n.readByMe = true
    n.readCount = (n.readCount || 0) + 1
  } catch (e) {
    // 标记已读失败不影响浏览
  }
}

onMounted(loadNotices)
defineExpose({ reload: loadNotices })
</script>

<style scoped>
.toolbar { display: flex; gap: 10px; margin-bottom: 16px; align-items: center; }
.notice-item { padding: 10px; border-bottom: 1px solid #eee; cursor: pointer; }
.notice-unread { color: #1f2937; }
.read-stat { margin-left: 8px; color: #909399; font-size: 12px; }
.notice-dot { margin-right: 4px; }
</style>
