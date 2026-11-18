#!/usr/bin/env node
// 压测探针账号引导：注册 4 个可识别账号并登录拿 token。
// 复刻项目既有 harness 的 bootstrap 逻辑，只把端口/前缀参数化。
// 用法: node bootstrap.js <out-tokens.json> <prefix> <baseUrl> <registerPath> <loginPath>
const fs = require('node:fs');

const out = process.argv[2];
const prefix = process.argv[3] || 'bench';
const base = process.argv[4] || 'http://127.0.0.1:8888';
const registerPath = process.argv[5] || '/api/user/register';
const loginPath = process.argv[6] || '/api/user/login';
const password = 'Bench2026!';

async function post(p, body) {
  const res = await fetch(base + p, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
    body: JSON.stringify(body)
  });
  const text = await res.text();
  let json = null;
  try { json = JSON.parse(text); } catch { /* keep raw */ }
  return { status: res.status, json, text };
}

(async () => {
  const tokens = [];
  const log = [];
  for (let i = 1; i <= 4; i += 1) {
    const username = `${prefix}_${i}`;
    const reg = await post(registerPath, { username, password, nickname: `压测探针${i}` });
    log.push({ username, register: reg.status });
    const login = await post(loginPath, { username, password });
    if (!login.json) { log.push({ username, login: login.status, raw: login.text.slice(0, 160) }); continue; }
    // VideoNest: {code,data:{token}}；学习平台: 裸 {token}
    const token = login.json?.token || login.json?.data?.token;
    if (!token) { log.push({ username, login: login.status, raw: login.text.slice(0, 160) }); continue; }
    tokens.push({ username, token });
  }
  fs.writeFileSync(out, JSON.stringify(tokens, null, 2));
  console.log(JSON.stringify({ saved: tokens.length, out, log }, null, 2));
  if (!tokens.length) process.exitCode = 1;
})();
