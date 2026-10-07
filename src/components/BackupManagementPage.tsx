import type { ApplicationService, WorkspaceView } from "../application/contract";
import { DemoBackupPage, type DemoBackupPageProps } from "./DemoBackupPage";
import { NativeBackupManager } from "./NativeBackupManager";

/** App supplies its single application, frozen selected IDs and demo handlers. */
export function BackupManagementPage({ application, workspace, selectedIds, demo }: {
  application: ApplicationService; workspace: WorkspaceView; selectedIds: string[]; demo: DemoBackupPageProps;
}) {
  return application.mode === "native" ? <NativeBackupManager application={application} workspace={workspace} selectedIds={selectedIds} /> : <DemoBackupPage {...demo} />;
}
