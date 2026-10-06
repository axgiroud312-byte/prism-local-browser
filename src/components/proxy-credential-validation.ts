import type { ProxyConfiguration } from "../application/contract";

/** Validate decoded replacement input only; keep/clear and HTTP Basic are separate. */
export function socks5CredentialByteError(type: ProxyConfiguration["type"], action: "keep" | "replace" | "clear", username: string, password: string): string | undefined {
  if (type !== "socks5" || action !== "replace") return;
  const encoder = new TextEncoder();
  if ([username, password].some(value => {
    const bytes = encoder.encode(value).length;
    return bytes < 1 || bytes > 255;
  })) return "SOCKS5 认证的用户名和密码各需 1–255 个 UTF-8 字节；请修正凭据，或明确清除认证。";
}
