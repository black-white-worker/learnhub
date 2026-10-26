<template>
  <div>
    <div v-if="isTeacher" class="toolbar">
      <el-input v-model="newNotice.title" placeholder="公告标题" style="width:200px" />
      <el-input v-model="newNotice.content" type="textarea" placeholder="公告内容" style="width:400px" />
      <el-button type="primary" @click="postNotice">发布</el-button>
    </div>

    <!-- 公告卡片列表（问题8：卡片明显，不再全白融入背景） -->
    <div class="notice-list">
      <el-card
        v-for="n in notices"
        :key="n.id"
        class="notice-card"
        shadow="hover"
        @click="openNotice(n)"
      >
        <div class="notice-title-row">
          <el-badge is-dot :hidden="n.readByMe">
            <el-tag v-if="n.isTop" type="warning" size="small">置顶</el-tag>
          </el-badge>
          <b :class="{ 'notice-unread': !n.readByMe }">{{ n.title }}</b>
        </div>
        <p class="notice-snippet">{{ n.content }}</p>
        <div class="notice-footer">
          <small>{{ n.createdAt }}</small>
          <span class="notice-footer-right">
            <span v-if="isTeacher" class="read-stat">已读 {{ n.readCount }}</span>
            <el-button
              v-if="isTeacher"
              type="danger"
              size="small"
              link
              @click.stop="deleteNotice(n)"
            >删除</el-button>
          </span>
        </div>
      </el-card>
    </div>

    <!-- 公告详情弹窗 -->
    <el-dialog v-model="detailVisible" :title="current?.title" width="600px">
      <p class="notice-full-content">{{ current?.content }}</p>
      <template #footer>
        <el-button @click="detailVisible = false">关闭</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import axios from 'axios'

const props = defineProps({
  classId: { type: [String, Number], required: true },
  isTeacher: { type: Boolean, default: false },
})

const token = localStorage.getItem('token')
const auth = { headers: { Authorization: 'Bearer ' + token } }

const notices = ref([])
const newNotice = ref({ title: '', content: '' })
const detailVisible = ref(false)
const current = ref(null)

async function loadNotices() {
  const r = await axios.get(`/api/class/${props.classId}/announcements`, auth)
  notices.value = r.data || []
}

async function postNotice() {
  if (!newNotice.value.title.trim() || !newNotice.value.content.trim()) {
    ElMessage.warning('标题和内容都不能为空')
    return
  }
  try {
    await axios.post(`/api/class/${props.classId}/announcement`, newNotice.value, auth)
    newNotice.value = { title: '', content: '' }
    ElMessage.success('发布成功')
    loadNotices()
  } catch (e) {
    ElMessage.error(e.response?.data?.message || '发布失败')
  }
}

async function openNotice(n) {
  current.value = n
  detailVisible.value = true
  if (n.readByMe) return
  try {
    await axios.post(`/api/class/announcement/${n.id}/read`, {}, auth)
    n.readByMe = true
    n.readCount = (n.readCount || 0) + 1
  } catch (e) {}
}

async function deleteNotice(n) {
  try {
    await ElMessageBox.confirm(`确定删除公告《${n.title}》吗？`, '删除公告', {
      type: 'warning', confirmButtonText: '删除', cancelButtonText: '取消',
    })
  } catch { return }
  try {
    await axios.delete(`/api/class/announcement/${n.id}`, auth)
    ElMessage.success('公告已删除')
    loadNotices()
  } catch (e) {
    ElMessage.error(e.response?.data?.message || '删除失败')
  }
}

onMounted(loadNotices)
defineExpose({ reload: loadNotices })
</script>

<style scoped>
.toolbar { display: flex; gap: 10px; margin-bottom: 16px; align-items: center; }
.notice-list { display: flex; flex-direction: column; gap: 12px; }
.notice-card { cursor: pointer; }
.notice-card :deep(.el-card__body) { padding: 14px 16px; }
.notice-title-row { display: flex; align-items: center; gap: 8px; font-size: 15px; }
.notice-unread { color: #1f2937; font-weight: 700; }
.notice-snippet {
  color: #6b7280; font-size: 13px; margin: 8px 0 0;
  overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
}
.notice-footer { display: flex; justify-content: space-between; margin-top: 8px; align-items: center; }
.notice-footer-right { display: flex; align-items: center; gap: 8px; }
.read-stat { color: #909399; font-size: 12px; }
.notice-full-content { white-space: pre-wrap; line-height: 1.7; color: #374151; }
</style>
