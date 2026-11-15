#!/usr/bin/env node
// 探针账号建立：注册 4 个可识别用户 + 登录取 token，写入 tokens.json。
// 注册接口限流 5 次/分钟/IP，所以本脚本一次只跑 4 个账号，重复执行会自动跳过已存在的账号。
const fs = require('node:fs');
const { performance } = require('node:perf_hooks');

const out = process.argv[2] || '/tmp/lpperf-20261109/tokens.json';
const prefix = process.argv[3] || 'lpperf';
const password = 'PerfProbe2026!';
const base = 'http://127.0.0.1:8888';

async function post(path, body, headers = {}) {
  const started = performance.now();
  const res = await fetch(base + path, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', Accept: 'application/json', ...headers },
    body: JSON.stringify(body)
  });
  const text = await res.text();
  let json = null;
  try { json = JSON.parse(text); } catch { /* 非 JSON 保留原文 */ }
  return { status: res.status, json, text, ms: Number((performance.now() - started).toFixed(1)) };
}

(async () => {
  const tokens = [];
  const log = [];
  for (let i = 1; i <= 4; i += 1) {
    const username = `${prefix}_${i}`;
    let reg = await post('/api/user/register', { username, password });
    if (reg.status !== 200) {
      // 已存在或撞限流：记录下来，仍尝试登录
      log.push({ username, step: 'register', status: reg.status, body: reg.text.slice(0, 200) });
    } else {
      log.push({ username, step: 'register', status: reg.status, ms: reg.ms });
    }
    const login = await post('/api/user/login', { username, password });
    // 学习平台登录成功返回裸 JSON {token, username, role, mustChangePassword}
    const token = login.json?.token;
    if (!token) {
      log.push({ username, step: 'login', status: login.status, body: login.text.slice(0, 200) });
      continue;
    }
    log.push({ username, step: 'login', status: login.status, bcryptLatencyMs: login.ms, role: login.json.role });
    tokens.push({ username, role: login.json.role, token });
  }
  fs.writeFileSync(out, JSON.stringify(tokens, null, 2));
  console.log(JSON.stringify({ created: tokens.length, log }, null, 2));
  if (!tokens.length) process.exitCode = 1;
})();
