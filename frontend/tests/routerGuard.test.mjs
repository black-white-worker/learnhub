// 路由守卫单测：锁住"未登录不能进班级/考试页"这条最容易被改坏的安全边界。

import { test } from 'node:test'
import assert from 'node:assert/strict'

import { resolveNavigation } from '../src/router/guard.js'

const route = (path, requiresAuth = false) => ({
  path,
  fullPath: path,
  meta: requiresAuth ? { requiresAuth } : {},
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
