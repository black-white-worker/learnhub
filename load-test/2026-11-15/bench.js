#!/usr/bin/env node
/*
 * 通用读写压测器（同一份代码同时用于 VideoNest 与 LearningPlatform，保证口径一致）。
 * 设计参考项目内既有 harness（videonest docs/performance-results/2026-11-07-load-test/harness/perfload.js
 * 与 learningplatform load-test/2026-11-09/perfload.js）：keep-alive 连接池、按权重混合场景、
 * 闭式并发（每个 worker 串行发请求）、百分位统计。
 *
 * 用法：
 *   node bench.js --config=scenarios.json --out=raw --label=read --suite=read \
 *        --secs=20 --ladder=8,16,32,64 [--scenario=hot] [--warmup-secs=8] [--tokens=tokens.json]
 */
const fs = require('node:fs');
const path = require('node:path');
const http = require('node:http');
const { performance } = require('node:perf_hooks');

const argv = {};
for (const a of process.argv.slice(2)) {
  const m = /^--([^=]+)(?:=(.*))?$/.exec(a);
  if (m) argv[m[1]] = m[2] === undefined ? true : m[2];
}

if (!argv.config) {
  console.error('Usage: node bench.js --config=<scenarios.json> --out=<dir> --label=<name> ' +
    '--suite=read|mixed|write --secs=20 --ladder=8,16,32,64 [--scenario=name]');
  process.exit(2);
}

const cfgPath = path.resolve(argv.config);
const cfg = JSON.parse(fs.readFileSync(cfgPath, 'utf8'));
const target = new URL(cfg.target);
const outDir = path.resolve(argv.out || './raw');
const suite = argv.suite || 'read';
const secs = Number.parseInt(argv.secs || cfg.secs || '20', 10);
const ladder = String(argv.ladder || '8,16,32,64').split(',').map((x) => Number.parseInt(x, 10));
const warmupSecs = Number.parseInt(argv['warmup-secs'] || '8', 10);
const reqTimeout = Number.parseInt(argv['req-timeout'] || '15000', 10);
const label = argv.label || suite;

const tokensPath = argv.tokens || cfg.tokens;
let tokens = [];
if (tokensPath && fs.existsSync(tokensPath)) {
  try { tokens = JSON.parse(fs.readFileSync(tokensPath, 'utf8')); } catch { tokens = []; }
}

const allScenarios = cfg.scenarios || [];
let active;
if (argv.scenario) {
  active = allScenarios.filter((s) => s.name === argv.scenario);
} else if (suite === 'mixed') {
  active = allScenarios.filter((s) => s.suite === 'read' || s.suite === 'write');
} else {
  active = allScenarios.filter((s) => s.suite === suite);
}
if (!active.length) throw new Error(`没有匹配的场景: suite=${suite} scenario=${argv.scenario || '(any)'}`);

const weightTotal = active.reduce((sum, s) => sum + (s.weight || 1), 0);

function pickScenario() {
  let v = Math.random() * weightTotal;
  for (const s of active) {
    v -= (s.weight || 1);
    if (v < 0) return s;
  }
  return active[active.length - 1];
}

function fill(template, s, w, seq) {
  if (template == null) return template;
  return String(template).replace(/\{\{(.*?)\}\}/g, (_, key) => {
    if (key === 'seq') return String(seq);
    if (key === 'worker') return String(w);
    const params = s.params || {};
    if (Object.prototype.hasOwnProperty.call(params, key)) {
      const arr = params[key];
      return String(arr[seq % arr.length]);
    }
    return `{{${key}}}`;
  });
}

function buildRequest(s, w, seq) {
  const method = Array.isArray(s.methods) ? s.methods[seq % s.methods.length] : (s.method || 'GET').toUpperCase();
  const reqPath = fill(s.path, s, w, seq);
  const bodyText = s.body ? fill(s.body, s, w, seq) : null;
  const headers = { Accept: 'application/json', Connection: 'keep-alive' };
  if (bodyText != null) {
    headers['Content-Type'] = 'application/json';
  }
  if (s.auth) {
    if (!tokens.length) throw new Error('场景需要鉴权，但 tokens 为空');
    const t = tokens[(w + seq) % tokens.length];
    headers.Authorization = `Bearer ${t.token}`;
  }
  return { method, path: reqPath, headers, bodyText };
}

function request(agent, req) {
  return new Promise((resolve) => {
    const started = performance.now();
    const body = req.bodyText == null ? null : Buffer.from(req.bodyText);
    if (body) req.headers['Content-Length'] = body.length;
    const r = http.request({
      protocol: target.protocol,
      hostname: target.hostname,
      port: target.port || 80,
      method: req.method,
      path: req.path,
      agent,
      timeout: reqTimeout,
      headers: req.headers
    }, (res) => {
      let bytes = 0;
      res.on('data', (c) => { bytes += c.length; });
      res.on('end', () => resolve({ status: res.statusCode || 0, latency: performance.now() - started, bytes }));
    });
    r.on('timeout', () => r.destroy(new Error('timeout')));
    r.on('error', (err) => resolve({ status: 0, latency: performance.now() - started, bytes: 0, error: err.code || err.name }));
    if (body) r.write(body);
    r.end();
  });
}

const newStats = () => ({ completed: 0, succeeded: 0, failed: 0, bytes: 0, statusCodes: {}, errors: {}, latencies: [] });

function tally(stats, r) {
  stats.completed += 1;
  stats.bytes += r.bytes || 0;
  stats.latencies.push(r.latency);
  stats.statusCodes[r.status] = (stats.statusCodes[r.status] || 0) + 1;
  if (r.error) stats.errors[r.error] = (stats.errors[r.error] || 0) + 1;
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

async function runStep(conc, durationSecs, collect) {
  const agent = new http.Agent({ keepAlive: true, maxSockets: conc, maxFreeSockets: conc });
  const total = newStats();
  const per = Object.fromEntries(active.map((s) => [s.name, newStats()]));
  const startedAt = performance.now();
  const stopAt = startedAt + durationSecs * 1000;

  const worker = async (w) => {
    let seq = 0;
    while (performance.now() < stopAt) {
      seq += 1;
      const s = active.length === 1 ? active[0] : pickScenario();
      const r = await request(agent, buildRequest(s, w, seq));
      if (collect) { tally(total, r); tally(per[s.name], r); }
    }
  };
  await Promise.all(Array.from({ length: conc }, (_, w) => worker(w)));
  const elapsed = (performance.now() - startedAt) / 1000;
  agent.destroy();
  if (!collect) return null;
  return {
    concurrency: conc,
    elapsedSeconds: Number(elapsed.toFixed(2)),
    total: summarize(total, elapsed),
    scenarios: Object.fromEntries(Object.entries(per).map(([k, v]) => [k, summarize(v, elapsed)]))
  };
}

(async () => {
  fs.mkdirSync(outDir, { recursive: true });
  const outPath = path.join(outDir, `${label}.json`);
  const run = { target: target.toString(), label, suite, scenario: argv.scenario || null, secs, ladder, startedAt: new Date().toISOString(), steps: [], warmup: null };

  // 预热：固定 8 并发，只跑不采集，让 JVM/连接池/缓存进入稳态
  if (warmupSecs > 0) {
    run.warmup = { concurrency: Math.min(8, ladder[0]), seconds: warmupSecs };
    await runStep(run.warmup.concurrency, warmupSecs, false);
  }

  for (const conc of ladder) {
    const step = await runStep(conc, secs, true);
    run.steps.push(step);
    fs.writeFileSync(outPath, JSON.stringify(run, null, 2));
    console.log(`${label} conc=${conc} qps=${step.total.requestsPerSecond} p95=${step.total.latencyMs.p95}ms fail=${step.total.failed}`);
  }
  console.log(`done -> ${outPath}`);
})();
