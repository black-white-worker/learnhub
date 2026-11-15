#!/usr/bin/env node
// 汇总 raw/ 下所有压测 JSON，输出 markdown 表格。
const fs = require('node:fs');
const path = require('node:path');

const rawDir = process.argv[2] || '/tmp/lpperf-20261109/raw';
const files = fs.readdirSync(rawDir).filter((f) => f.endsWith('.json'));

const rows = [];
for (const f of files) {
  const data = JSON.parse(fs.readFileSync(path.join(rawDir, f), 'utf8'));
  for (const s of data.steps || []) {
    rows.push({
      label: data.label || f.replace('.json', ''),
      mode: data.mode,
      conc: s.concurrency,
      rps: s.requestsPerSecond,
      p50: s.latencyMs.p50,
      p95: s.latencyMs.p95,
      p99: s.latencyMs.p99,
      ok: s.successRatePercent,
      through: s.throughputKBps,
      codes: JSON.stringify(s.statusCodes),
      free: s.memAvailableMbEnd,
      aborted: s.aborted || ''
    });
  }
}
// 按 label、conc 排序
rows.sort((a, b) => (a.label === b.label ? a.conc - b.conc : a.label.localeCompare(b.label)));

console.log('| 接口 | 并发 | QPS | p50 | p95 | p99 | 成功率 | 吞吐KB/s | 内存MB | 状态 |');
console.log('|------|------|-----|-----|-----|-----|--------|----------|--------|------|');
for (const r of rows) {
  console.log(`| ${r.label} | ${r.conc} | ${r.rps} | ${r.p50}ms | ${r.p95}ms | ${r.p99}ms | ${r.ok}% | ${r.through} | ${r.free} | ${r.aborted || 'OK'} |`);
}
