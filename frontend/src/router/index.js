import { createRouter, createWebHistory } from 'vue-router'
import HomeView from '../views/HomeView.vue'

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes: [
    {
      path: '/',
      name: 'home',
      component: HomeView,
    },
    {
      path: '/about',
      name: 'about',
      // route level code-splitting
      // this generates a separate chunk (About.[hash].js) for this route
      // which is lazy-loaded when the route is visited.
      component: () => import('../views/AboutView.vue'),
    },
    {
      path: '/login',
      name: 'login',
      component: () => import('../views/LoginView.vue'),
    },
    {
      path: '/register',
      name: 'register',
      component: () => import('../views/RegisterView.vue'),
    },
    {
      path: '/course/create',
      name: 'course-create',
      // 课程创建页（来自「课程创建」分支）
      component: () => import('../views/CourseCreateView.vue'),
    },
    {
      path: '/courses',
      name: 'courses',
      // 课程列表页（来自「课程列表」分支）：展示 GET /api/course/list 返回的课程
      component: () => import('../views/CourseListView.vue'),
    },
    {
      path: '/course/:id/resources',
      name: 'course-resources',
      // 课程资源页（来自「课程资源」分支）：PPT / Word / PDF 讲义的上传与查看
      component: () => import('../views/CourseResourcesView.vue'),
    },
    {
      path: '/course/:id',
      name: 'course-detail',
      // 课程详情页（来自「课程编辑」分支）：卡片点击进入，展示封面 / 基本信息 / 视频 / 资源
      component: () => import('../views/CourseDetailView.vue'),
    },
  ],
})

export default router
