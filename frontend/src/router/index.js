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
  ],
})

export default router
