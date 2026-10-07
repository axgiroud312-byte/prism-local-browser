import { useState } from "react";
import type { WorkspaceView } from "../application/contract";
import { KernelManagementPage } from "./KernelManagementPage";
import { KernelDetailsWindow } from "./KernelDetailsWindow";

export function DemoKernelManager({ workspace }: { workspace: WorkspaceView }) {
  const [detailId, setDetailId] = useState("");
  const detail = workspace.state.kernels.find(record => record.id === detailId);
  return <><KernelManagementPage native={false} records={workspace.state.kernels.map(record => ({ ...record, architecture: "Windows x64（演示）", statusText: record.available ? "演示基线 · 未真实安装" : "待验证", usedCount: workspace.state.environments.filter(environment => environment.coreId === record.id).length }))} onInspect={setDetailId} />{detail && <KernelDetailsWindow demoRecord={detail} onClose={() => setDetailId("")} />}</>;
}
