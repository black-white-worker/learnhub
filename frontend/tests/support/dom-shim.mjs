// Node 测试环境的最小浏览器 shim。
//
// 背景：src/utils/courseCreateFlow.js 会读 localStorage.getItem('username')，
// 而 node --test 里没有 localStorage，导致 courseCreateFlow.test.mjs 在
// "最终提交时兜底上传失败" 这个用例上直接抛 'localStorage is not defined'。
// 这里补一个内存实现，测试用 import 顺序保证它先于业务模块执行。

if (typeof globalThis.localStorage === 'undefined') {
  const store = new Map()
  globalThis.localStorage = {
    getItem: (key) => (store.has(String(key)) ? store.get(String(key)) : null),
    setItem: (key, value) => store.set(String(key), String(value)),
    removeItem: (key) => store.delete(String(key)),
    clear: () => store.clear(),
    key: (index) => Array.from(store.keys())[index] ?? null,
    get length() {
      return store.size
    },
  }
}
