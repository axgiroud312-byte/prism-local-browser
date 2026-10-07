import type { DeviceProfile, EnvironmentConfiguration, FingerprintPreview } from "./contract.ts";

export function fingerprintMatchesConfiguration(profile: DeviceProfile, c: EnvironmentConfiguration) {
  return profile.seed === c.seed && profile.kernelId === c.coreId && profile.templateVersion === c.fingerprintVersion &&
    profile.language === c.language && profile.timezone === c.timezone && profile.cpu === c.cpu && profile.width === c.width && profile.height === c.height;
}

export function applyFingerprint<T extends EnvironmentConfiguration>(configuration: T, profile: DeviceProfile): T {
  return { ...configuration, seed: profile.seed, coreId: profile.kernelId, fingerprintVersion: profile.templateVersion,
    language: profile.language, timezone: profile.timezone, cpu: profile.cpu, width: profile.width, height: profile.height };
}

// An explicitly labelled demo checksum, not native SHA-256/evidence. Exact
// service-side field comparison also protects preview commits; no native use.
export function demoProfile(c: EnvironmentConfiguration, revision: number): DeviceProfile {
  const languages: Record<string, string[]> = { "en-US": ["en-US", "en"], "en-GB": ["en-GB", "en"], "de-DE": ["de-DE", "de", "en"], "ja-JP": ["ja-JP", "ja", "en"], "en-SG": ["en-SG", "en"], "zh-CN": ["zh-CN", "zh", "en"] };
  const profile: DeviceProfile = {
    schemaVersion: 1, configRevision: revision, seed: c.seed,
    templateId: "windows-desktop-v1", templateVersion: c.fingerprintVersion, generatorVersion: "demo-windows-v1",
    platform: "windows", platformVersion: "15.0.0", brand: "Chrome", brandVersion: "",
    kernelId: c.coreId, coreActualVersion: "", coreExecutableSha256: "", adapterVersion: "demo-only", capabilityVersion: "demo-capabilities-v1",
    language: c.language, acceptLanguages: [...(languages[c.language] ?? [c.language])], uiLanguage: "system", timezone: c.timezone,
    regionPreset: "demo-preference", cpu: c.cpu, width: c.width, height: c.height, parameters: [], configHash: "",
  };
  let hash = 2166136261;
  for (const character of JSON.stringify(profile)) hash = Math.imul(hash ^ character.charCodeAt(0), 16777619);
  return { ...profile, configHash: `demo-${(hash >>> 0).toString(16).padStart(8, "0")}` };
}

export function fingerprintChanges(before: DeviceProfile | undefined, after: DeviceProfile): FingerprintPreview["changes"] {
  const fields = ["seed", "kernelId", "language", "timezone", "cpu", "width", "height"] as const;
  return fields.filter(field => before?.[field] !== after[field]).map(field => ({ field, before: before ? String(before[field]) : "未保存", after: String(after[field]) }));
}

export function demoFingerprint(profile: DeviceProfile, before?: DeviceProfile, action = "edit", restoredFrom?: number): FingerprintPreview {
  return { mode: "demo", previewProfile: profile, action, restoredFrom, changes: fingerprintChanges(before, profile),
    capabilityReport: { kernelId: profile.kernelId, evidenceStatus: "demo-only", observedFingerprint: null, canLaunchNative: false,
      capabilities: [
        { field: "identity/seed/cpu/acceptLanguages/timezone", status: "configurable", source: "demo-only", note: "只演示档案输入与修订，不是内核能力实测。" },
        { field: "gpu/memory/font/canvas/audio", status: "unverified", source: "demo-only", note: "没有真实内核读值，不显示假GPU/字体或已验证噪声。" },
        { field: "screen", status: "system", source: "application-policy", note: "窗口偏好不等于屏幕指纹；真实屏幕值未读取。" },
        { field: "uiLanguage", status: "unverified", source: "not-probed", note: "网站语言不代表菜单语言已验收。" },
      ] } };
}
