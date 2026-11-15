// 路由守卫单测：锁住"未登录不能进班级/考试页"这条最容易被改坏的安全边界。

import { test } from 'node:test'
import assert from 'node:assert/strict'

import { resolveNavigation } from '../src/router/guard.js'

const route = (path, requiresAuth = false, meta = {}) => ({
  path,
  fullPath: path,
  meta: { ...(requiresAuth ? { requiresAuth } : {}), ...meta },
})

test('未登录访问受保护页面 → 跳登录并带 redirect', () => {
  const result = resolveNavigation(route('/class/1', true), null)
  assert.notEqual(result, true)
  assert.equal(result.path, '/login')
  assert.equal(result.query.redirect, '/class/1')
})

test('未登录访问公开页面 → 放行', () => {
  assert.equal(resolveNavigation(route('/courses'), null), true)
  assert.equal(resolveNavigation(route('/'), ''), true)
})

test('已登录访问受保护页面 → 放行', () => {
  assert.equal(resolveNavigation(route('/class/1', true), 'token-abc'), true)
})

test('已登录还去登录页 → 回首页', () => {
  const result = resolveNavigation(route('/login'), 'token-abc')
  assert.notEqual(result, true)
  assert.equal(result.path, '/')
})

test('未登录去登录页 → 放行（否则死循环）', () => {
  assert.equal(resolveNavigation(route('/login'), null), true)
})

test('没有 meta 的路由不会崩', () => {
  assert.equal(resolveNavigation({ path: '/x', fullPath: '/x' }, null), true)
})

// ===== AI 出题等受限角色页面（meta.roles）=====
const teacherOnly = { roles: ['teacher', 'admin'] }

test('学生进 AI 出题 → 回首页（服务端还会再拦一次）', () => {
  const result = resolveNavigation(route('/ai/generate', true, teacherOnly), 'tok', 'student')
  assert.notEqual(result, true)
  assert.equal(result.path, '/')
})

test('老师和管理员进 AI 出题 → 放行', () => {
  assert.equal(resolveNavigation(route('/ai/generate', true, teacherOnly), 'tok', 'teacher'), true)
  assert.equal(resolveNavigation(route('/ai/generate', true, teacherOnly), 'tok', 'admin'), true)
})

test('角色还没同步到本地时按未授权处理，宁可不给进', () => {
  assert.equal(resolveNavigation(route('/ai/generate', true, teacherOnly), 'tok', null).path, '/')
})

test('没登录优先跳登录，不会先撞角色判断', () => {
  const result = resolveNavigation(route('/ai/generate', true, teacherOnly), null, null)
  assert.equal(result.path, '/login')
  assert.equal(result.query.redirect, '/ai/generate')
})

test('不限制角色的页面照旧放行', () => {
  assert.equal(resolveNavigation(route('/ai/chat', true), 'tok', 'student'), true)
})
