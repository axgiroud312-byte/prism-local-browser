import { seedState, type State } from "../../../src/domain.ts";

// Independently expressed in the Prism model; not an archived API response.
export function referenceWorkspace(): State {
  const base = seedState(), template = base.environments[7];
  return {
    ...base,
    environments: Array.from({ length: 12 }, (_, index) => ({
      ...template, id: `synthetic-reference-${index + 1}`, code: String(index + 1),
      name: ["工作环境 A", "工作环境 B", "测试环境 C"][index % 3] + (index >= 3 ? ` ${index + 1}` : ""),
      group: index % 2 ? "测试环境" : "工作环境", note: "合成参考数据",
      proxyId: index % 2 ? "synthetic-socks" : "", seed: String(180000001 + index),
      status: "ready", cookies: [], createdAt: "2026-10-06T09:00:00+08:00", lastOpened: undefined,
    })),
    proxies: [
      { id: "synthetic-socks", name: "合成 SOCKS5", type: "socks5", host: "192.0.2.11", port: 1080, country: "US", username: "", password: "", status: "connected" },
      { id: "synthetic-http", name: "合成 HTTP", type: "http", host: "198.51.100.21", port: 8080, country: "US", username: "", password: "", status: "unchecked" },
    ], backups: [], activities: [],
  };
}
