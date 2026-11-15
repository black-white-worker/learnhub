// AI 答疑 SSE 解析单测。
// 锁的是最容易坏的一件事：块切一半就 JSON.parse，答案会整段丢。
import { test } from 'node:test'
import assert from 'node:assert/strict'

import {
  newChatState,
  parseSseBlock,
  readAiErrorBody,
  reduceChatEvent,
  takeSseBlocks,
} from '../src/utils/aiSse.js'

test('takeSseBlocks 只切出以空行结尾的完整块', () => {
  const { blocks, rest } = takeSseBlocks('data: {"delta":"你"}\n\ndata: {"del')
  assert.equal(blocks.length, 1)
  assert.equal(rest, 'data: {"del')
})

test('takeSseBlocks 兼容 \r\n 行尾', () => {
  const { blocks, rest } = takeSseBlocks('data: a\r\n\r\ndata: b\r\n')
  assert.deepEqual(blocks, ['data: a'])
  assert.equal(rest, 'data: b\r\n'.replace('\r\n', '\n'))
})

test('takeSseBlocks 空缓冲不崩', () => {
  assert.deepEqual(takeSseBlocks(''), { blocks: [], rest: '' })
  assert.deepEqual(takeSseBlocks(null), { blocks: [], rest: '' })
})

test('parseSseBlock 解析 event + data，注释行忽略', () => {
  const evt = parseSseBlock('event: refs\ndata: ["第一章 概率","第二章 随机变量"]\n: ping')
  assert.equal(evt.event, 'refs')
  assert.deepEqual(evt.json, ['第一章 概率', '第二章 随机变量'])
})

test('parseSseBlock 多条 data 行按规范用换行连接', () => {
  const evt = parseSseBlock('data: {"a":1}\ndata: tail')
  assert.equal(evt.data, '{"a":1}\ntail')
  assert.equal(evt.json, null) // 拼起来已经不是合法 JSON
})

test('reduceChatEvent 逐 token 累加 delta', () => {
  const s = newChatState()
  reduceChatEvent(s, parseSseBlock('data: {"delta":"Hello "}'))
  reduceChatEvent(s, parseSseBlock('data: {"delta":"world"}'))
  assert.equal(s.answer, 'Hello world')
  assert.equal(s.done, false)
})

test('reduceChatEvent refs 去重且保序', () => {
  const s = newChatState()
  reduceChatEvent(s, parseSseBlock('event: refs\ndata: ["B","A","B"]'))
  assert.deepEqual(s.refs, ['B', 'A'])
})

test('reduceChatEvent done 时以更完整的答案为准', () => {
  const s = newChatState()
  reduceChatEvent(s, parseSseBlock('data: {"delta":"半句"}'))
  reduceChatEvent(s, parseSseBlock('data: {"done":true,"answer":"完整的回答"}'))
  assert.equal(s.answer, '完整的回答')
  assert.equal(s.done, true)
})

test('done 的 answer 比已累加的短（异常数据）不会倒退', () => {
  const s = newChatState()
  reduceChatEvent(s, parseSseBlock('data: {"delta":"很长很长的一段回答"}'))
  reduceChatEvent(s, parseSseBlock('data: {"done":true,"answer":"短"}'))
  assert.equal(s.answer, '很长很长的一段回答')
})

test('reduceChatEvent 流内 error 记下来但不覆盖已有答案', () => {
  const s = newChatState()
  reduceChatEvent(s, parseSseBlock('data: {"delta":"前面正常"}'))
  reduceChatEvent(s, parseSseBlock('data: {"error":"模型超时"}'))
  assert.equal(s.error, '模型超时')
  assert.equal(s.answer, '前面正常')
})

test('reduceChatEvent 忽略未知事件', () => {
  const s = newChatState()
  reduceChatEvent(s, parseSseBlock('event: heartbeat\ndata: {}'))
  reduceChatEvent(s, null)
  assert.equal(s.answer, '')
})

test('readAiErrorBody 优先 FastAPI 的 detail', () => {
  assert.equal(readAiErrorBody('{"detail":"今日 AI 调用已达上限"}', 429), '今日 AI 调用已达上限')
})

test('readAiErrorBody 兼容 Go 信封的 message', () => {
  assert.equal(readAiErrorBody('{"code":503,"message":"AI 服务暂时不可用"}', 503), 'AI 服务暂时不可用')
})

test('readAiErrorBody 没正文时按状态码给兜底文案', () => {
  assert.equal(readAiErrorBody('', 429), '今天的 AI 调用次数已用完，明天再来')
  assert.equal(readAiErrorBody('', 403), '这个 AI 功能只有老师或管理员能用')
  assert.equal(readAiErrorBody('', 418), '请求失败（418）')
})

test('readAiErrorBody 纯文本过长会截断，不把整个 HTML 甩到界面上', () => {
  const long = 'x'.repeat(500)
  const msg = readAiErrorBody(long, 500)
  assert.equal(msg.length, 201) // 200 字符 + 省略号
  assert.ok(msg.endsWith('…'))
})
