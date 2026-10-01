import type { ProxyCheckReport } from "./contract";

// Library checks and per-session preflights share the same stage vocabulary.
const labels: Record<string, string> = { queued: "等待网络调度", validation: "配置核对", credentials: "读取受保护认证", connection: "连接代理", "local-channel": "本机通道", "local-authorization": "前检调用身份", "upstream-connection": "上游连接", "proxy-tls": "代理TLS", authentication: "隧道与认证", tunnel: "建立隧道", "upstream-authentication": "上游认证/隧道", "socks-negotiation": "SOCKS5方法协商", "target-resolution": "目标解析策略", "domain-target": "远端域名连接", "ipv4-target": "IPv4目标连接", "ipv6-target": "IPv6目标连接", "target-tls": "目标TLS", target: "目标访问", exit: "实际出口", "upstream-result": "上游结果", result: "前检结果", "storage-pending": "观测结果待保存", completed: "已完成", failed: "检查失败", cancelled: "已取消", "application-interrupted": "上次检查被中断" };

export function proxyStageLabel(stage: string) { return labels[stage] ?? stage; }
export function proxyResolutionLabel(policy: ProxyCheckReport["resolutionPolicy"]) { return policy === "remote-target-dns" ? "目标域名交给SOCKS5上游解析；IP按IPv4/IPv6字节发送" : "未记录目标解析策略"; }
