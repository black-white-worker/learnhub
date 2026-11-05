<script setup>
import { ref, watch, computed, onMounted, onUnmounted } from 'vue'
import { useRouter, useRoute, RouterView } from 'vue-router'
import { ElMessage } from 'element-plus'
import { Reading, Notebook, EditPen, Collection, ArrowDown, User, Bell } from '@element-plus/icons-vue'
import { logout as apiLogout, fetchInfo, fetchNotifications } from './api/index.js'

const router = useRouter()
const route = useRoute()

// 当前登录用户（登录成功后由 LoginView 写入 localStorage）
const username = ref(localStorage.getItem('username') || '')
const userRole = ref(localStorage.getItem('role') || '')
// 初始账号 / 管理员重置密码后必须先改密，顶栏常驻提示
const mustChangePassword = ref(false)

// 路由切换时同步登录状态：
watch(
  () => route.fullPath,
  () => {
    username.value = localStorage.getItem('username') || ''
    userRole.value = localStorage.getItem('role') || ''
  }
)

// 只有老师/管理员能创建课程（问题2）
const canCreateCourse = computed(() => userRole.value === 'teacher' || userRole.value === 'admin')
// 只有管理员能看到管理后台入口
const isAdmin = computed(() => userRole.value === 'admin')

// 顶部导航菜单
const menus = computed(() => {
  const base = [
    { label: '首页', icon: Reading, path: '/' },
    {
      label: '课程中心',
      icon: Notebook,
      children: [
        ...(canCreateCourse.value ? [{ label: '创建课程', path: '/course/create' }] : []),
        { label: '课程列表', path: '/courses' },
      ],
    },
    { label: '班级中心', icon: User, path: '/classes' },
    { label: '考试中心', icon: EditPen, path: '/exams' },
    ...(isAdmin.value
      ? [{
          label: '管理后台',
          icon: Collection,
          children: [
            { label: '用户管理', path: '/admin/users' },
            { label: '课程管理', path: '/admin/courses' },
          ],
        }]
      : []),
  ]
  return base
})
const activeMenu = ref('/')

// 课程中心下拉菜单是否高亮：路由命中创建课程/课程列表任一个就高亮
const isCourseMenuActive = computed(() =>
  ['/course/create', '/courses'].includes(route.path)
)

function onMenuClick(menu) {
  if (menu.path) {
    activeMenu.value = menu.path
    router.push(menu.path)
  } else {
    ElMessage.info('「' + menu.label + '」建设中，敬请期待')
  }
}

// 课程中心下拉菜单选中：跳转到对应路由并高亮
function onMenuCommand(path) {
  activeMenu.value = path
  router.push(path)
}

function goLogin() {
  router.push('/login')
}

function onUserCommand(cmd) {
  if (cmd === 'profile') {
    router.push('/profile')
  } else if (cmd === 'logout') {
    logout()
  }
}

// 退出登录：先让后端删掉 Redis 会话，再清本地。
// 只清 localStorage 的话 token 在服务端还能用满 24 小时（账号被盗时尤其危险）。
async function logout() {
  try {
    await apiLogout()
  } catch {
    // 网络失败也要允许本地退出，服务端会话靠 TTL 兜底
  }
  localStorage.removeItem('token')
  localStorage.removeItem('username')
  localStorage.removeItem('role')
  username.value = ''
  userRole.value = ''
  mustChangePassword.value = false
  ElMessage.success('已退出登录')
  router.push('/')
}

// 从服务端刷新会话：角色 / 是否必须改密以数据库为准，
// 避免管理员改完角色后前端还按旧 localStorage 渲染入口。
async function refreshSession() {
  if (!localStorage.getItem('token')) return
  try {
    const res = await fetchInfo()
    const info = res.data || {}
    username.value = info.username || ''
    userRole.value = info.role || 'student'
    mustChangePassword.value = Boolean(info.mustChangePassword)
    localStorage.setItem('username', username.value)
    localStorage.setItem('role', userRole.value)
  } catch {
    // 401 已由拦截器统一跳登录，其余错误保持本地状态
  }
}

// ===== 未读消息数（顶栏铃铛红点）=====
const unreadCount = ref(0)
async function loadUnread() {
  if (!username.value) { unreadCount.value = 0; return }
  try {
    const r = await fetchNotifications()
    unreadCount.value = r.data?.unread || 0
  } catch {
    // 静默失败，不打扰用户
  }
}

// ===== 实时通知（WebSocket）=====
// 用长连接替代 60s 轮询：服务端有新通知时主动推过来，未读数秒级更新。
// 连接管理抽到 utils/notificationSocket.js（可在 node 下单测）：
// token 走 query（浏览器握手不能带自定义请求头），协议随页面自动切换（ws/wss）。
import { createNotificationSocket } from './utils/notificationSocket.js'

const notificationSocket = createNotificationSocket({
  getToken: () => localStorage.getItem('token'),
  onUnread: () => { unreadCount.value += 1; loadUnread() },
  onAnnouncement: () => { unreadCount.value += 1; loadUnread() },
})

onMounted(() => {
  refreshSession()
  loadUnread()
  window.addEventListener('dsh:session-updated', refreshSession)
  notificationSocket.connect() // 实时推送替代轮询
})
onUnmounted(() => {
  notificationSocket.disconnect()
  window.removeEventListener('dsh:session-updated', refreshSession)
})
// 登录态变化（登录 / 退出）时重连，确保用的是最新 token
watch(username, () => { loadUnread(); notificationSocket.disconnect(); notificationSocket.connect() })
</script>

<template>
  <el-container class="layout">
    <!-- 顶部导航（吸顶） -->
    <el-header class="header" height="64px">
      <div class="header-inner">
        <div class="logo" @click="router.push('/')">
          <span class="logo-icon">📚</span>
          <span class="logo-text">学习平台</span>
        </div>
        <nav class="menu">
          <template v-for="m in menus" :key="m.label">
            <!-- 带 children 的菜单渲染为下拉（课程中心） -->
            <el-dropdown v-if="m.children" trigger="click" @command="onMenuCommand">
              <div class="menu-item" :class="{ active: isCourseMenuActive }">
                <el-icon><component :is="m.icon" /></el-icon>
                <span>{{ m.label }}</span>
                <el-icon class="menu-arrow"><ArrowDown /></el-icon>
              </div>
              <template #dropdown>
                <el-dropdown-menu>
                  <el-dropdown-item
                    v-for="c in m.children"
                    :key="c.path"
                    :command="c.path"
                  >
                    {{ c.label }}
                  </el-dropdown-item>
                </el-dropdown-menu>
              </template>
            </el-dropdown>
            <!-- 普通菜单项 -->
            <div
              v-else
              class="menu-item"
              :class="{ active: m.path === activeMenu }"
              @click="onMenuClick(m)"
            >
              <el-icon><component :is="m.icon" /></el-icon>
              <span>{{ m.label }}</span>
            </div>
          </template>
        </nav>
        <div class="user-area">
          <template v-if="username">
            <el-badge :value="unreadCount" :hidden="unreadCount === 0" :max="99" class="bell-badge">
              <div class="bell" @click="router.push('/profile')">
                <el-icon :size="20"><Bell /></el-icon>
              </div>
            </el-badge>
            <el-dropdown trigger="click" @command="onUserCommand">
              <div class="user-trigger">
                <el-avatar :size="32" class="avatar">{{ username.charAt(0).toUpperCase() }}</el-avatar>
                <span class="username">{{ username }}</span>
              </div>
              <template #dropdown>
                <el-dropdown-menu>
                  <el-dropdown-item command="profile">个人主页</el-dropdown-item>
                  <el-dropdown-item command="logout" divided>退出登录</el-dropdown-item>
                </el-dropdown-menu>
              </template>
            </el-dropdown>
          </template>
          <template v-else>
            <el-button type="primary" @click="goLogin">登录</el-button>
          </template>
        </div>
      </div>
    </el-header>

    <!-- 初始密码 / 管理员重置密码后的强制改密提示 -->
    <div v-if="mustChangePassword" class="pwd-alert">
      <span>当前账号仍在使用初始密码或管理员重置的临时密码，请尽快修改。</span>
      <el-button link type="primary" @click="router.push('/profile')">去修改</el-button>
    </div>

    <!-- 内容区 -->
    <el-main class="main">
      <RouterView />
    </el-main>

    <!-- 页脚 -->
    <el-footer class="footer">
      学习平台 · 基于 Go + Vue3 构建 · 教学演示项目
    </el-footer>
  </el-container>
</template>

<style scoped>
.layout {
  min-height: 100vh;
}

/* ===== 顶部导航 ===== */
.header {
  position: sticky;
  top: 0;
  z-index: 100;
  background: linear-gradient(90deg, #1e3a8a 0%, #2563eb 100%);
  padding: 0;
  box-shadow: 0 2px 8px rgba(30, 58, 138, 0.25);
}

.header-inner {
  max-width: 1280px;
  height: 100%;
  margin: 0 auto;
  display: flex;
  align-items: center;
  gap: 32px;
  padding: 0 24px;
}

.logo {
  display: flex;
  align-items: center;
  gap: 8px;
  cursor: pointer;
  user-select: none;
}

.logo-icon {
  font-size: 24px;
}

.logo-text {
  font-size: 18px;
  font-weight: 700;
  color: #fff;
}

.menu {
  display: flex;
  gap: 4px;
  flex: 1;
}

.menu-item {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 8px 16px;
  border-radius: 8px;
  cursor: pointer;
  color: rgba(255, 255, 255, 0.85);
  font-size: 14px;
  transition: all 0.2s;
}

.menu-item:hover {
  background: rgba(255, 255, 255, 0.15);
  color: #fff;
}

.menu-arrow {
  font-size: 12px;
  margin-left: 2px;
}

.menu-item.active {
  color: #fff;
  background: rgba(255, 255, 255, 0.2);
  font-weight: 600;
}

/* ===== 用户区 ===== */
.user-area {
  display: flex;
  align-items: center;
  gap: 14px;
}

.bell {
  color: #fff;
  cursor: pointer;
  padding: 6px;
  border-radius: 8px;
  display: flex;
  align-items: center;
}
.bell:hover { background: rgba(255,255,255,0.15); }

.user-trigger {
  display: flex;
  align-items: center;
  gap: 8px;
  cursor: pointer;
  padding: 4px 10px;
  border-radius: 8px;
}
.user-trigger:hover {
  background: rgba(255, 255, 255, 0.15);
}

.avatar {
  background: rgba(255, 255, 255, 0.25);
  color: #fff;
  font-weight: 600;
}

.username {
  font-size: 14px;
  color: #fff;
}

/* ===== 内容区 / 页脚 ===== */
.main {
  padding: 0;
}

.pwd-alert {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 8px 16px;
  font-size: 13px;
  color: #92400e;
  background: #fffbeb;
  border-bottom: 1px solid #fde68a;
}

.footer {
  text-align: center;
  color: #9ca3af;
  font-size: 13px;
  background: #fff;
  border-top: 1px solid #e5e7eb;
}
</style>
