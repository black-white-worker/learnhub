#!/usr/bin/env node
// 学习平台 loopback 压测驱动器（无第三方依赖），只在服务器本机 127.0.0.1:8888 跑。
// 参考 videonest 2026-11-07 harness，适配 go-zero 学习平台路由与鉴权。
// 护栏：每 5 秒采样 MemAvailable 与邻居金丝雀（lp:80 / hp:8081 / vn:8082），越线立刻停止发压。
const http = require('node:http');
const fs = require('node:fs');
const { performance } = require('node:perf_hooks');

const argv = {};
for (const a of process.argv.slice(2)) {
  const m = /^--([^=]+)(?:=(.*))?$/.exec(a);
  if (m) argv[m[1]] = m[2] === undefined ? true : m[2];
}
const base = new URL(argv.base || 'http://127.0.0.1:8888');
const mode = argv.mode || 'simple';
const secs = Number.parseInt(argv.secs || '30', 10);
const ladder = String(argv.ladder || '8').split(',').map((x) => Number.parseInt(x, 10));
const reqTimeout = Number.parseInt(argv['req-timeout'] || '15000', 10);
const minFreeMb = Number.parseInt(argv['min-free-mb'] || '50', 10);
const outDir = argv.out || '.';
const label = argv.label || mode;
const tokensFile = argv.tokens || '/tmp/lpperf-20261109/tokens.json';
const canaryList = String(argv.canary || 'http://127.0.0.1:80/,http://127.0.0.1:8081/,http://127.0.0.1:8082/').split(',');
const probeMarker = argv['probe-marker'] || 'LP20261109';
const stepGap = Number.parseInt(argv['step-gap'] || '15', 10);
const stopFile = argv['stop-file'] || null;
// 压测用的业务主键（课程 id=5 是生产库唯一一条课程）
const courseId = Number.parseInt(argv['course-id'] || '5', 10);
const videoId = Number.parseInt(argv['video-id'] || '1', 10);

function memAvailableMb() {
  try {
    for (const line of fs.readFileSync('/proc/meminfo', 'utf8').split('\n')) {
      const m = /^MemAvailable:\s+(\d+)\s+kB/.exec(line);
      if (m) return Math.round(Number(m[1]) / 1024);
    }
  } catch { /* 读不到就当未知 */ }
  return null;
}

function request(agent, opts, bodyText) {
  return new Promise((resolve) => {
    const started = performance.now();
    const body = bodyText == null ? null : Buffer.from(bodyText);
    const headers = { Accept: 'application/json', Connection: 'keep-alive', ...(opts.headers || {}) };
    if (body) {
      headers['Content-Type'] = 'application/json';
      headers['Content-Length'] = body.length;
    }
    const req = http.request({
      protocol: base.protocol,
      hostname: base.hostname,
      port: base.port || 80,
      method: opts.method || 'GET',
      agent,
      timeout: reqTimeout,
      path: opts.path,
      headers
    }, (res) => {
      const chunks = [];
      res.on('data', (c) => chunks.push(c));
      res.on('end', () => {
        const text = Buffer.concat(chunks).toString('utf8');
        let bytes = Buffer.concat(chunks).length;
        resolve({ status: res.statusCode || 0, latency: performance.now() - started, bytes, text });
      });
    });
    req.on('timeout', () => req.destroy(new Error('timeout')));
    req.on('error', (err) => resolve({ status: 0, latency: performance.now() - started, bytes: 0, error: err.code || err.name }));
    if (body) req.write(body);
    req.end();
  });
}

function head(urlText) {
  const u = new URL(urlText);
  return new Promise((resolve) => {
    const req = http.request({ protocol: u.protocol, hostname: u.hostname, port: u.port || 80, path: u.pathname, method: 'GET', timeout: 8000, agent: false }, (res) => {
      res.resume();
      res.on('end', () => resolve(res.statusCode || 0));
    });
    req.on('timeout', () => req.destroy(new Error('timeout')));
    req.on('error', () => resolve(0));
    req.end();
  });
}

const newStats = () => ({ completed: 0, succeeded: 0, failed: 0, bytes: 0, statusCodes: {}, errors: {}, latencies: [] });

function tally(stats, r) {
  stats.completed += 1;
  stats.bytes += r.bytes || 0;
  stats.latencies.push(r.latency);
  stats.statusCodes[r.status] = (stats.statusCodes[r.status] || 0) + 1;
  if (r.error) stats.errors[r.error] = (stats.errors[r.error] || 0) + 1;
  // 学习平台业务约定：HTTP 200 即成功，错误走 4xx/5xx + 业务 message
  if (r.status >= 200 && r.status < 300) stats.succeeded += 1; else stats.failed += 1;
}

function summarize(stats, elapsed) {
  stats.latencies.sort((a, b) => a - b);
  const pct = (p) => Number((stats.latencies[Math.min(stats.latencies.length - 1, Math.ceil(stats.latencies.length * p) - 1)] || 0).toFixed(2));
  return {
    requests: stats.completed,
    successful: stats.succeeded,
    failed: stats.failed,
    successRatePercent: Number((stats.succeeded / Math.max(stats.completed, 1) * 100).toFixed(3)),
    requestsPerSecond: Number((stats.completed / elapsed).toFixed(2)),
    throughputKBps: Number((stats.bytes / 1024 / elapsed).toFixed(2)),
    latencyMs: { p50: pct(0.5), p95: pct(0.95), p99: pct(0.99), max: pct(1) },
    statusCodes: stats.statusCodes,
    errors: stats.errors
  };
}

function buildScenario(conc) {
  const needTokens = ['auth-get', 'note-add', 'progress'].includes(mode) || argv.auth;
  let tokens = [];
  if (needTokens && fs.existsSync(tokensFile)) {
    try { tokens = JSON.parse(fs.readFileSync(tokensFile, 'utf8')); } catch { tokens = []; }
  }

  if (mode === 'simple') {
    // 通用只读：--path /api/xxx?a=1 --method GET
    const path = argv.path || '/api/course/list?page=1&pageSize=20';
    const method = String(argv.method || 'GET').toUpperCase();
    const bodyText = argv.body || null;
    const headers = {};
    if (argv.auth && tokens.length) headers.Authorization = `Bearer ${tokens[0].token}`;
    return async (agent) => request(agent, { path, method, headers }, bodyText);
  }

  if (mode === 'login') {
    // 登录接口：bcrypt CPU 密集，用探针账号密码反复登录
    const users = tokens.length ? tokens : [{ username: 'lpperf_1', password: 'PerfProbe2026!' }];
    return async (agent, w) => {
      const u = users[w % users.length];
      const body = JSON.stringify({ username: u.username, password: u.password });
      return request(agent, { path: '/api/user/login', method: 'POST' }, body);
    };
  }

  if (mode === 'register') {
    // 注册接口：限流 5次/分钟/IP，只能小并发。每次用不同用户名避免冲突。
    let counter = 0;
    return async (agent) => {
      counter += 1;
      const uname = `${probeMarker}_${process.pid}_${Date.now()}_${counter}`;
      const body = JSON.stringify({ username: uname, password: 'PerfProbe2026!' });
      return request(agent, { path: '/api/user/register', method: 'POST' }, body);
    };
  }

  if (mode === 'note-add') {
    // 写笔记：POST /api/course/note，body {courseId, videoId, timestampSeconds, content}
    let counter = 0;
    return async (agent, w) => {
      const t = tokens[w % tokens.length];
      counter += 1;
      const body = JSON.stringify({
        courseId,
        videoId,
        timestampSeconds: (counter % 60) * 10,
        content: `${probeMarker} 压测笔记 ${w}_${counter}`
      });
      return request(agent, { path: '/api/course/note', method: 'POST', headers: { Authorization: `Bearer ${t.token}` } }, body);
    };
  }

  if (mode === 'progress') {
    // 保存播放进度：POST /api/course/video/progress/:courseId/:videoId
    let counter = 0;
    return async (agent, w) => {
      const t = tokens[w % tokens.length];
      counter += 1;
      const body = JSON.stringify({ progress: (counter % 100) * 10, duration: 600 });
      return request(agent, { path: `/api/course/video/progress/${courseId}/${videoId}`, method: 'POST', headers: { Authorization: `Bearer ${t.token}` } }, body);
    };
  }

  throw new Error(`未知 mode: ${mode}`);
}

async function runStep(conc) {
  const scenario = buildScenario(conc);
  const agent = new http.Agent({ keepAlive: true, maxSockets: conc, maxFreeSockets: conc });
  const stats = newStats();
  const startedAt = performance.now();
  const stopAt = startedAt + secs * 1000;
  let aborted = null;
  let canaryBad = 0;
  const samples = [];

  const sampler = async () => {
    while (!aborted && performance.now() < stopAt) {
      await new Promise((r) => setTimeout(r, 5000));
      const free = memAvailableMb();
      const codes = await Promise.all(canaryList.map(head));
      canaryBad = codes.some((c) => c !== 200) ? canaryBad + 1 : 0;
      samples.push({ atSec: Math.round((performance.now() - startedAt) / 1000), memAvailableMb: free, canary: codes });
      if (free !== null && free < minFreeMb) aborted = `护栏：MemAvailable ${free}M < ${minFreeMb}M`;
      if (canaryBad >= 2) aborted = `护栏：邻居金丝雀连续 2 次非 200（${codes.join('/')}）`;
      if (stopFile && fs.existsSync(stopFile)) {
        aborted = `护栏：宿主守卫触发 ${fs.readFileSync(stopFile, 'utf8').trim()}`;
      }
    }
  };
  const mkWorker = (w) => async () => {
    let i = 0;
    while (!aborted && performance.now() < stopAt) { i += 1; tally(stats, await scenario(agent, w, i)); }
  };

  await Promise.all([sampler(), ...Array.from({ length: conc }, (_, w) => mkWorker(w)())]);
  const elapsed = (performance.now() - startedAt) / 1000;
  agent.destroy();
  return { concurrency: conc, startedAtWall: new Date(Date.now() - elapsed * 1000).toISOString(), elapsedSeconds: Number(elapsed.toFixed(2)), aborted, memAvailableMbEnd: memAvailableMb(), samples, ...summarize(stats, elapsed) };
}

(async () => {
  fs.mkdirSync(outDir, { recursive: true });
  const outPath = `${outDir}/${label}.json`;
  const runStartedAt = new Date().toISOString();
  const steps = [];
  for (const conc of ladder) {
    if (stopFile && fs.existsSync(stopFile)) {
      console.log(`跳过 ${label} conc=${conc}：宿主守卫已触发 ${fs.readFileSync(stopFile, 'utf8').trim()}`);
      break;
    }
    steps.push(await runStep(conc));
    fs.writeFileSync(outPath, JSON.stringify({ mode, label, args: argv, startedAt: runStartedAt, steps }, null, 2));
    const s = steps.at(-1);
    if (s.aborted) break;
    // 失败率 >5% 或出现 5xx，不再加档
    const failureRate = s.failed / Math.max(s.requests, 1);
    const has5xx = Object.keys(s.statusCodes).some((k) => k.startsWith('5'));
    if (failureRate > 0.05 || has5xx) break;
    await new Promise((r) => setTimeout(r, stepGap * 1000));
  }
  fs.writeFileSync(outPath, JSON.stringify({ mode, label, args: argv, startedAt: runStartedAt, finishedAt: new Date().toISOString(), steps }, null, 2));
  console.log(JSON.stringify({ label, mode, steps: steps.map((x) => ({ conc: x.concurrency, rps: x.requestsPerSecond, p50: x.latencyMs.p50, p95: x.latencyMs.p95, p99: x.latencyMs.p99, ok: x.successRatePercent, throughputKBps: x.throughputKBps, codes: x.statusCodes, freeMb: x.memAvailableMbEnd, aborted: x.aborted })) }));
})();
