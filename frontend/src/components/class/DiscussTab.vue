<template>
  <div>
    <div v-if="isMember" class="toolbar">
      <el-input v-model="newPost.title" placeholder="帖子标题" style="width:200px" />
      <el-input v-model="newPost.content" placeholder="内容" style="width:300px" />
      <el-button type="primary" @click="postDiscussion">发帖</el-button>
    </div>

    <div class="discuss-list">
      <el-card
        v-for="d in discussions"
        :key="d.id"
        class="discuss-card"
        shadow="hover"
        @click="openDiscussion(d)"
      >
        <b class="discuss-title">{{ d.title }}</b>
        <p class="discuss-snippet">{{ d.content.substring(0, 60) }}</p>
        <small class="discuss-time">{{ d.lastReplyAt || d.createdAt }}</small>
      </el-card>
    </div>

    <!-- 讨论详情弹窗 -->
    <el-dialog v-model="discussDialog" :title="currentDiscussion?.title" width="600px">
      <p class="discuss-full">{{ currentDiscussion?.content }}</p>
      <el-divider />
      <div v-for="r in replies" :key="r.id" class="reply">
        <b>{{ r.username }}：</b>{{ r.content }}
      </div>
      <el-input v-model="replyText" placeholder="回复" style="margin-top:10px" />
      <el-button type="primary" style="margin-top:10px" @click="doReply">回复</el-button>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import axios from 'axios'

const props = defineProps({
  classId: { type: [String, Number], required: true },
  isMember: { type: Boolean, default: false },
})

const token = localStorage.getItem('token')
const auth = { headers: { Authorization: 'Bearer ' + token } }

const discussions = ref([])
const newPost = ref({ title: '', content: '' })

const discussDialog = ref(false)
const currentDiscussion = ref(null)
const replies = ref([])
const replyText = ref('')

async function loadDiscuss() {
  const r = await axios.get(`/api/class/${props.classId}/discussions`, auth)
  discussions.value = r.data || []
}

async function postDiscussion() {
  if (!newPost.value.title.trim() || !newPost.value.content.trim()) {
    ElMessage.warning('标题和内容都不能为空')
    return
  }
  try {
    await axios.post(`/api/class/${props.classId}/discussion`, newPost.value, auth)
    newPost.value = { title: '', content: '' }
    ElMessage.success('发帖成功')
    loadDiscuss()
  } catch (e) {
    ElMessage.error(e.response?.data?.message || '发帖失败')
  }
}

async function openDiscussion(d) {
  currentDiscussion.value = d
  const r = await axios.get(`/api/discussion/${d.id}`, auth)
  replies.value = r.data.replies || []
  discussDialog.value = true
}

async function doReply() {
  if (!replyText.value.trim()) return
  try {
    await axios.post(`/api/discussion/${currentDiscussion.value.id}/reply`, { content: replyText.value }, auth)
    replyText.value = ''
    ElMessage.success('回复成功')
    openDiscussion(currentDiscussion.value)
  } catch (e) {
    ElMessage.error(e.response?.data?.message || '回复失败')
  }
}

onMounted(loadDiscuss)
</script>

<style scoped>
.toolbar { display: flex; gap: 10px; margin-bottom: 16px; align-items: center; }
.discuss-list { display: flex; flex-direction: column; gap: 12px; }
.discuss-card { cursor: pointer; }
.discuss-card :deep(.el-card__body) { padding: 14px 16px; }
.discuss-title { font-size: 15px; color: #1f2937; }
.discuss-snippet {
  color: #6b7280; font-size: 13px; margin: 8px 0 0;
  overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
}
.discuss-time { color: #9ca3af; font-size: 12px; }
.discuss-full { white-space: pre-wrap; line-height: 1.7; }
.reply { padding: 6px; background: #f5f5f5; margin: 4px 0; border-radius: 4px; }
</style>
