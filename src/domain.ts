export type Status = "ready" | "starting" | "running" | "stopping" | "error";
export type ProxyStatus = "unchecked" | "connected" | "failed";
export interface Cookie {
  name: string;
  value: string;
  domain: string;
  path: string;
  secure?: boolean;
  httpOnly?: boolean;
  sameSite?: string;
  expires?: number;
  expirationDate?: number;
  partitionKey?: unknown;
  [key: string]: unknown;
}
export interface Environment {
  id: string;
  code: string;
  name: string;
  group: string;
  note: string;
  proxyId: string;
  coreId: string;
  seed: string;
  language: string;
  timezone: string;
  cpu: string;
  width: number;
  height: number;
  urls: string;
  restoreTabs: boolean;
  status: Status;
  error?: string;
  cookies: Cookie[];
  createdAt: string;
  lastOpened?: string;
  fingerprintVersion: string;
}
export interface ProxyNode {
  id: string;
  name: string;
  type: "http" | "https" | "socks5";
  host: string;
  port: number;
  username: string;
  password: string;
  country: string;
  status: ProxyStatus;
  latency?: number;
  simulateFailure?: boolean;
}
export interface Kernel {
  id: string;
  version: string;
  available: boolean;
  source: string;
  note: string;
}
export interface Activity {
  id: string;
  time: string;
  action: string;
  target: string;
  result: "success" | "error" | "info";
  detail: string;
}
export interface Snapshot {
  format: "prism-prototype";
  schemaVersion: 1;
  createdAt: string;
  environments: Environment[];
  proxies: ProxyNode[];
  kernels: Kernel[];
}
export interface Backup {
  id: string;
  name: string;
  createdAt: string;
  snapshot: Snapshot;
}
export interface State {
  schemaVersion: 1;
  environments: Environment[];
  proxies: ProxyNode[];
  kernels: Kernel[];
  backups: Backup[];
  activities: Activity[];
}
export const STORAGE_KEY = "prism-local-browser-v1";
export const uid = (prefix: string) => `${prefix}-${crypto.randomUUID()}`;
export const now = () => new Date().toISOString();
export function uniqueSeed(existing: string[]) {
  const used = new Set(existing);
  let candidate: string;
  do {
    candidate = String(
      (crypto.getRandomValues(new Uint32Array(1))[0] % 2147483646) + 1,
    );
  } while (used.has(candidate));
  return candidate;
}
export const regions: Record<
  string,
  { language: string; timezone: string; label: string }
> = {
  US: { language: "en-US", timezone: "America/New_York", label: "美国" },
  GB: { language: "en-GB", timezone: "Europe/London", label: "英国" },
  DE: { language: "de-DE", timezone: "Europe/Berlin", label: "德国" },
  JP: { language: "ja-JP", timezone: "Asia/Tokyo", label: "日本" },
  SG: { language: "en-SG", timezone: "Asia/Singapore", label: "新加坡" },
  CN: { language: "zh-CN", timezone: "Asia/Shanghai", label: "中国" },
};
export function seedState(): State {
  const proxies: ProxyNode[] = [
    {
      id: "px-us",
      name: "美国 · 纽约 01",
      type: "socks5",
      host: "192.0.2.18",
      port: 1080,
      username: "demo",
      password: "",
      country: "US",
      status: "connected",
      latency: 126,
    },
    {
      id: "px-gb",
      name: "英国 · 伦敦 01",
      type: "http",
      host: "198.51.100.24",
      port: 8080,
      username: "",
      password: "",
      country: "GB",
      status: "connected",
      latency: 168,
    },
    {
      id: "px-de",
      name: "德国 · 法兰克福 01",
      type: "socks5",
      host: "203.0.113.36",
      port: 1080,
      username: "",
      password: "",
      country: "DE",
      status: "failed",
      simulateFailure: true,
    },
    {
      id: "px-jp",
      name: "日本 · 东京 01",
      type: "https",
      host: "192.0.2.48",
      port: 8443,
      username: "",
      password: "",
      country: "JP",
      status: "connected",
      latency: 82,
    },
    {
      id: "px-sg",
      name: "新加坡 · 节点 01",
      type: "socks5",
      host: "198.51.100.56",
      port: 1080,
      username: "",
      password: "",
      country: "SG",
      status: "unchecked",
    },
  ];
  const names = [
    "北美主店",
    "英国精品店",
    "欧洲家居店",
    "日本生活馆",
    "北美选品环境",
    "东南亚新店",
    "英国运营环境",
    "产品测试环境",
  ];
  const assignments = [
    "px-us",
    "px-gb",
    "px-de",
    "px-jp",
    "px-us",
    "px-sg",
    "px-gb",
    "",
  ];
  const groups = [
    "日常运营",
    "日常运营",
    "日常运营",
    "日常运营",
    "选品调研",
    "新店筹备",
    "日常运营",
    "测试环境",
  ];
  const createdAt = now();
  return {
    schemaVersion: 1,
    environments: names.map((name, i) => {
      const proxy = proxies.find((p) => p.id === assignments[i]);
      const region = regions[proxy?.country || "CN"];
      return {
        id: `env-${i + 1}`,
        code: String(i + 1).padStart(3, "0"),
        name,
        group: groups[i],
        note: i === 2 ? "代理连接异常，请先检查节点" : "",
        proxyId: assignments[i],
        coreId: "core-148",
        seed: String(162740021 + i * 18487),
        language: region.language,
        timezone: region.timezone,
        cpu: "auto",
        width: 1280,
        height: 800,
        urls: "",
        restoreTabs: true,
        status: i < 2 ? "running" : i === 2 ? "error" : "ready",
        error: i === 2 ? "代理连接失败，已阻止启动。" : undefined,
        cookies: [],
        createdAt,
        fingerprintVersion: "windows-desktop-v1",
      };
    }),
    proxies,
    kernels: [
      {
        id: "core-148",
        version: "148.0.7778.215",
        available: true,
        source: "fingerprint-chromium",
        note: "公开补丁可审计 · 原型演示基线",
      },
      {
        id: "core-150",
        version: "150",
        available: false,
        source: "fingerprint-chromium",
        note: "上游候选版本 · 需验证实际构建与能力",
      },
    ],
    backups: [],
    activities: [
      {
        id: "event-welcome",
        time: createdAt,
        action: "载入示例工作区",
        target: "本机工作区",
        result: "info",
        detail: "当前是交互原型，所有运行与检测状态均为模拟。",
      },
    ],
  };
}
export function validateEnvironment(
  e: Environment,
  state: Pick<State, "kernels" | "proxies">,
) {
  if (!e.name.trim()) return "请输入环境名称。";
  if (!state.kernels.some((k) => k.id === e.coreId))
    return "请选择已登记的内核。";
  if (e.proxyId && !state.proxies.some((p) => p.id === e.proxyId))
    return "绑定的代理不存在，请重新选择。";
  if (!/^\d+$/.test(e.seed) || +e.seed < 1 || +e.seed > 2147483647)
    return "指纹种子应为 1 至 2147483647 的整数。";
  if (
    ![e.width, e.height].every(
      (n) => Number.isInteger(n) && n >= 400 && n <= 7680,
    )
  )
    return "窗口宽高应为 400 至 7680 的整数。";
  try {
    new Intl.DateTimeFormat("en", { timeZone: e.timezone });
  } catch {
    return "时区无效，请选择有效时区。";
  }
  for (const url of e.urls
    .split(/\n/)
    .map((s) => s.trim())
    .filter(Boolean)) {
    try {
      if (!["http:", "https:"].includes(new URL(url).protocol))
        return "启动网址仅支持 http 或 https。";
    } catch {
      return "启动网址格式无效，每行填写一个完整网址。";
    }
  }
  return null;
}
export function launchError(
  env: Environment,
  state: Pick<State, "kernels" | "proxies">,
) {
  if (!state.kernels.find((k) => k.id === env.coreId)?.available)
    return "绑定内核尚不可用，请到内核管理检查；不会改用其他内核。";
  if (env.proxyId) {
    const p = state.proxies.find((p) => p.id === env.proxyId);
    if (!p || p.status !== "connected")
      return "代理未通过检查，已阻止启动；不会切换为直连。";
  }
  return null;
}
export interface ProxyParseRow {
  line: number;
  raw: string;
  node?: ProxyNode;
  error?: string;
}
export function parseProxyText(text: string): ProxyParseRow[] {
  return text
    .split(/\r?\n/)
    .map((raw, index) => ({ raw: raw.trim(), line: index + 1 }))
    .filter((r) => r.raw && !r.raw.startsWith("#"))
    .map((row) => {
      try {
        const legacy =
          !row.raw.includes("://") && !row.raw.startsWith("[")
            ? row.raw.match(/^([^:]+):(\d+):([^:]*):(.*)$/)
            : null;
        const normalized = legacy
          ? `http://${encodeURIComponent(legacy[3])}:${encodeURIComponent(legacy[4])}@${legacy[1]}:${legacy[2]}`
          : row.raw.includes("://")
            ? row.raw
            : `http://${row.raw}`;
        const u = new URL(normalized);
        const type = u.protocol.slice(0, -1);
        if (!["http", "https", "socks5"].includes(type))
          throw new Error("仅支持 HTTP、HTTPS、SOCKS5");
        if (
          !u.hostname ||
          (u.pathname && u.pathname !== "/") ||
          u.search ||
          u.hash
        )
          throw new Error("请使用协议://用户名:密码@地址:端口");
        const port = u.port
          ? +u.port
          : type === "https"
            ? 443
            : type === "http"
              ? 80
              : 1080;
        if (!Number.isInteger(port) || port < 1 || port > 65535)
          throw new Error("端口必须在 1 至 65535 之间");
        return {
          ...row,
          node: {
            id: uid("px"),
            name: `导入代理 ${row.line}`,
            type: type as ProxyNode["type"],
            host: u.hostname,
            port,
            username: decodeURIComponent(u.username),
            password: decodeURIComponent(u.password),
            country: "US",
            status: "unchecked" as const,
          },
        };
      } catch (error) {
        return {
          ...row,
          error: error instanceof Error ? error.message : "格式错误",
        };
      }
    });
}
export function parseCookies(text: string): {
  cookies: Cookie[];
  errors: string[];
} {
  const errors: string[] = [];
  const cookies: Cookie[] = [];
  let source: unknown;
  if (text.trimStart().startsWith("[") || text.trimStart().startsWith("{")) {
    try {
      source = JSON.parse(text);
    } catch {
      return { cookies, errors: ["JSON 格式错误，请粘贴 Cookie 数组。"] };
    }
  } else {
    const rows: Record<string, unknown>[] = [];
    text.split(/\r?\n/).forEach((line, index) => {
      if (
        !line.trim() ||
        (line.startsWith("#") && !line.startsWith("#HttpOnly_"))
      )
        return;
      const parts = line.replace(/^#HttpOnly_/, "").split("\t");
      if (
        parts.length !== 7 ||
        !["TRUE", "FALSE"].includes(parts[1]) ||
        !["TRUE", "FALSE"].includes(parts[3]) ||
        !/^\d+$/.test(parts[4])
      ) {
        errors.push(
          `第 ${index + 1} 行：Netscape 格式需要 7 个以制表符分隔的字段。`,
        );
        return;
      }
      rows.push({
        domain: parts[0],
        path: parts[2],
        secure: parts[3] === "TRUE",
        httpOnly: line.startsWith("#HttpOnly_"),
        ...(Number(parts[4]) > 0 ? { expires: Number(parts[4]) } : {}),
        name: parts[5],
        value: parts[6],
      });
    });
    source = rows;
    if (!rows.length && !errors.length)
      errors.push("请粘贴 JSON Cookie 数组或 Netscape 文本。");
  }
  if (!Array.isArray(source))
    return {
      cookies,
      errors: [
        '应为 Cookie 对象数组，例如 [{"name":"session","value":"demo","domain":"example.com"}]。',
      ],
    };
  for (const [i, item] of source.entries()) {
    if (
      !item ||
      typeof item !== "object" ||
      typeof item.name !== "string" ||
      !item.name ||
      typeof item.value !== "string" ||
      typeof item.domain !== "string" ||
      !item.domain ||
      /[\s/:]/.test(item.domain)
    ) {
      errors.push(
        `第 ${i + 1} 条：name、value、domain 格式不正确（value 可以为空）。`,
      );
      continue;
    }
    if (
      item.path !== undefined &&
      (typeof item.path !== "string" || !item.path.startsWith("/"))
    ) {
      errors.push(`第 ${i + 1} 条：path 必须以 / 开头。`);
      continue;
    }
    const expires = item.expires ?? item.expirationDate;
    if (
      expires !== undefined &&
      (typeof expires !== "number" || !Number.isFinite(expires))
    ) {
      errors.push(`第 ${i + 1} 条：过期时间必须是秒时间戳。`);
      continue;
    }
    cookies.push({ ...item, path: item.path || "/" });
  }
  return { cookies, errors };
}
export function mergeCookies(existing: Cookie[], incoming: Cookie[]) {
  const key = (c: Cookie) =>
    JSON.stringify([c.domain, c.path, c.name, c.partitionKey ?? null]);
  const map = new Map(existing.map((c) => [key(c), c]));
  incoming.forEach((c) => map.set(key(c), c));
  return [...map.values()];
}
export function createSnapshot(state: State): Snapshot {
  return {
    format: "prism-prototype",
    schemaVersion: 1,
    createdAt: now(),
    environments: structuredClone(state.environments).map((e) => ({
      ...e,
      status: "ready",
      error: undefined,
    })),
    proxies: state.proxies.map((p) => ({
      ...p,
      password: "",
      status: "unchecked",
      latency: undefined,
    })),
    kernels: structuredClone(state.kernels),
  };
}
export function parseSnapshot(text: string): Snapshot {
  const s = JSON.parse(text);
  if (
    !s ||
    s.format !== "prism-prototype" ||
    s.schemaVersion !== 1 ||
    !Array.isArray(s.environments) ||
    !Array.isArray(s.proxies) ||
    !Array.isArray(s.kernels)
  )
    throw new Error("文件不是受支持的棱镜原型快照。");
  for (const k of s.kernels) {
    if (
      !k ||
      typeof k.id !== "string" ||
      !k.id ||
      typeof k.version !== "string" ||
      !k.version ||
      typeof k.available !== "boolean" ||
      typeof k.source !== "string" ||
      typeof k.note !== "string"
    )
      throw new Error("快照内核数据不完整。");
  }
  for (const p of s.proxies) {
    if (
      !p ||
      typeof p.id !== "string" ||
      typeof p.name !== "string" ||
      typeof p.host !== "string" ||
      typeof p.username !== "string" ||
      typeof p.country !== "string" ||
      !["http", "https", "socks5"].includes(p.type) ||
      !Number.isInteger(p.port) ||
      p.port < 1 ||
      p.port > 65535
    )
      throw new Error("快照代理数据不完整。");
  }
  for (const e of s.environments) {
    if (
      !e ||
      typeof e.id !== "string" ||
      typeof e.name !== "string" ||
      typeof e.group !== "string" ||
      typeof e.note !== "string" ||
      typeof e.code !== "string" ||
      typeof e.seed !== "string" ||
      typeof e.timezone !== "string" ||
      typeof e.language !== "string" ||
      typeof e.urls !== "string" ||
      !Array.isArray(e.cookies) ||
      validateEnvironment(e, s) ||
      parseCookies(JSON.stringify(e.cookies)).errors.length
    )
      throw new Error("快照环境数据不完整或引用无效。");
  }
  for (const e of s.environments) {
    if (
      !e.id ||
      !e.code ||
      typeof e.proxyId !== "string" ||
      typeof e.coreId !== "string" ||
      !["ready", "starting", "running", "stopping", "error"].includes(
        e.status,
      ) ||
      typeof e.cpu !== "string" ||
      !["auto", "4", "8", "12", "16"].includes(e.cpu) ||
      typeof e.restoreTabs !== "boolean" ||
      typeof e.fingerprintVersion !== "string" ||
      typeof e.createdAt !== "string" ||
      !Number.isFinite(Date.parse(e.createdAt)) ||
      (e.lastOpened !== undefined &&
        (typeof e.lastOpened !== "string" ||
          !Number.isFinite(Date.parse(e.lastOpened))))
    )
      throw new Error("快照环境字段或时间无效。");
    e.cookies = parseCookies(JSON.stringify(e.cookies)).cookies;
  }
  for (const p of s.proxies)
    if (
      !p.id ||
      !p.host ||
      typeof p.password !== "string" ||
      !regions[p.country] ||
      !["unchecked", "connected", "failed"].includes(p.status)
    )
      throw new Error("快照代理字段无效。");
  for (const list of [s.environments, s.proxies, s.kernels])
    if (new Set(list.map((i: { id: string }) => i.id)).size !== list.length)
      throw new Error("快照包含重复 ID。");
  return s as Snapshot;
}
export function restoreSnapshot(state: State, snapshot: Snapshot): State {
  if (
    state.environments.some((e) =>
      ["running", "starting", "stopping"].includes(e.status),
    )
  )
    throw new Error("请先停止所有环境，再恢复快照。");
  return {
    ...state,
    environments: snapshot.environments.map((e) => ({
      ...e,
      status: "ready",
      error: undefined,
    })),
    proxies: snapshot.proxies.map((p) => ({
      ...p,
      password: "",
      status: "unchecked",
      latency: undefined,
    })),
    kernels: structuredClone(snapshot.kernels),
  };
}
