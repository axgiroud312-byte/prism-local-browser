import { useLayoutEffect, useRef, useState, type ChangeEvent, type Ref } from "react";
import { parseCookies, type Environment } from "../domain";
import { EnvironmentConfirmation, EnvironmentWindowFrame } from "./EnvironmentDialogParts";
import "./native-cookie.css";

/** App owns parsing/save/lifecycle; local reads never outlive this demo target or write through native. */
export function DemoCookieImportWindow({ environment, text, result, error, busy, onText, onParse, onSave, onClose, dialogRef }: {
  environment: Environment; text: string; result: ReturnType<typeof parseCookies> | null; error: string; busy?: boolean;
  onText: (text: string) => void; onParse: () => void; onSave: () => void; onClose: () => void; dialogRef?: Ref<HTMLDivElement>;
}) {
  const fileInput = useRef<HTMLInputElement>(null);
  const fileReadGeneration = useRef(0);
  const [fileReadError, setFileReadError] = useState("");
  useLayoutEffect(() => {
    fileReadGeneration.current += 1;
    setFileReadError("");
    // Invalidate on target change and unmount, including App-owned Escape/close.
    return () => { fileReadGeneration.current += 1; };
  }, [environment.id]);
  function invalidateFileRead() {
    setFileReadError("");
    return ++fileReadGeneration.current;
  }
  function changeText(value: string) {
    invalidateFileRead();
    onText(value);
  }
  function close() {
    invalidateFileRead();
    onClose();
  }
  async function readFile(event: ChangeEvent<HTMLInputElement>) {
    const input = event.currentTarget;
    const file = input.files?.[0];
    input.value = ""; // Reset the captured element before awaiting, so the same file can be chosen again.
    if (!file || busy) return;
    const generation = invalidateFileRead();
    try {
      const value = await file.text();
      if (generation === fileReadGeneration.current) onText(value);
    } catch {
      if (generation === fileReadGeneration.current) setFileReadError("读取文件失败，请重新选择文件或手动粘贴 Cookie 内容。原草稿已保留。");
    }
  }
  return <EnvironmentWindowFrame title="导入 Cookie" titleId="modal-title" dialogRef={dialogRef} onClose={close} closeLabel="关闭对话框" width={1050} height={result ? 609.72 : 551.72} className="env34-cookie-window" footer={<><button type="button" className="button" disabled={busy} onClick={close}>取消</button><button type="button" className="button primary" disabled={busy || !result?.cookies.length || !!result.errors.length} onClick={onSave}>导入到原型记录</button></>}>
    <p><strong>目标：{environment.name}</strong> · ID：{environment.id} · 示例数据导入</p>
    <div className="env34-cookie-input-grid"><div><label>Cookie 内容（JSON / Netscape）<textarea aria-label="Cookie 内容" autoComplete="off" spellCheck={false} rows={7} value={text} disabled={busy} placeholder={'[{"name":"sample","value":"SYNTHETIC","domain":"example.invalid","path":"/"}]'} onChange={event => changeText(event.currentTarget.value)} /></label><input ref={fileInput} type="file" accept=".json,.txt" hidden aria-label="读取示例Cookie文件" disabled={busy} onChange={readFile} /><div className="env34-inline"><button type="button" className="button" disabled={busy} onClick={() => fileInput.current?.click()}>选择文件</button><button type="button" className="text-button" disabled={busy} onClick={() => changeText(JSON.stringify([{ name: "sample", value: "SYNTHETIC", domain: "example.invalid", path: "/" }, { name: "empty", value: "", domain: "example.invalid", path: "/" }], null, 2))}>填入示例</button><button type="button" className="button" disabled={busy} onClick={onParse}>校验并预览</button></div></div><aside><h3>导入说明 · 演示模式</h3><p>接受完整 JSON 数组 / Netscape 文本，保留合法空值、session 和原始到期时间，不续期。</p><p>仅合并写入原型记录，不写真实浏览器。示例值会进入当前浏览器演示存储及演示快照，请勿输入真实账号 Cookie。</p><p>预览隐藏值；导入 Cookie 不等于恢复全部登录数据。不支持 ZIP 批量上传。</p></aside></div>
    {result ? <><p>校验结果 · {result.cookies.length} 条有效 / {result.errors.length} 条错误</p><div className="env34-table native-cookie-table"><table><thead><tr><th>序号</th><th>名称 / 域名</th><th>路径</th><th>语义 / 状态</th></tr></thead><tbody>{result.cookies.map((cookie, index) => <tr key={index}><td>{index + 1}</td><td>{cookie.name}<br />{cookie.domain}</td><td>{cookie.path}</td><td>{cookie.value === "" ? "空值保留" : "值已隐藏"}{Number(cookie.expires ?? cookie.expirationDate) > 0 && Number(cookie.expires ?? cookie.expirationDate) < Date.now() / 1000 ? " · 已过期，仅保留解析记录" : ""}</td></tr>)}{result.errors.map((message, index) => <tr key={`error-${index}`}><td>!</td><td colSpan={3} className="env34-error">{message}</td></tr>)}</tbody></table></div></> : <div className="env34-table"><table><thead><tr><th>序号</th><th>名称 / 域名</th><th>路径</th><th>语义 / 状态</th></tr></thead><tbody><tr><td colSpan={4}>尚未解析，没有写入。</td></tr></tbody></table></div>}
    {error && <p className="env34-error" role="alert">{error}</p>}
    {fileReadError && <p className="env34-error" role="alert">{fileReadError}</p>}
  </EnvironmentWindowFrame>;
}

export function DemoEnvironmentRemoveWindow({ count, error, busy, onClose, onRemove, dialogRef }: { count: number; error: string; busy?: boolean; onClose: () => void; onRemove: () => void; dialogRef?: Ref<HTMLDivElement> }) {
  return <EnvironmentConfirmation title="移除浏览器环境" titleId="modal-title" dialogRef={dialogRef} onCancel={onClose} onConfirm={onRemove} confirmLabel="确认移除" busy={busy} danger>
    <p>确认移除明确选中的 {count} 个环境？正在运行的环境必须先关闭。</p><p>仅移除示例记录及示例Cookie，不操作真实文件，demo 没有回收区。建议先保存原型快照。</p>{error && <p className="env34-error" role="alert">{error}</p>}
  </EnvironmentConfirmation>;
}
