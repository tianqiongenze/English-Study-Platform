// vitest 全局 setup:node 环境无 localStorage,auth.ts 模块加载时即读取。
// 在任何业务模块 import 之前注入 stub。
class LocalStorageStub implements Storage {
  private m = new Map<string, string>();
  get length() { return this.m.size; }
  clear() { this.m.clear(); }
  getItem(k: string) { return this.m.get(k) ?? null; }
  key(i: number) { return Array.from(this.m.keys())[i] ?? null; }
  removeItem(k: string) { this.m.delete(k); }
  setItem(k: string, v: string) { this.m.set(k, v); }
}

(globalThis as { localStorage: Storage }).localStorage = new LocalStorageStub();
