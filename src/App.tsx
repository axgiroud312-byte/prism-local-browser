import {
  useEffect,
  useRef,
  useState,
  useSyncExternalStore,
  type ReactNode,
} from "react";
import {
  Bell,
  BookOpen,
  Box,
  CheckCircle2,
  ChevronLeft,
  ChevronRight,
  CircleHelp,
  Copy,
  EllipsisVertical,
  FileJson,
  Fingerprint,
  Folder,
  Globe2,
  HardDrive,
  History,
  Layers3,
  LayoutGrid,
  LoaderCircle,
  Monitor,
  Network,
  Settings2,
  Square,
  Trash2,
  TriangleAlert,
  X,
} from "lucide-react";
import {
  createSnapshot,
  launchError,
  mergeCookies,
  now,
  parseCookies,
  regions,
  restoreSnapshot,
  uid,
  type Environment,
  type Activity,
  type Snapshot,
  type State,
} from "./domain";
import {
  newerOperationEvent,
  operationIsTerminal,
  type ApplicationService,
  type OperationEvent,
  type EnvironmentPreview,
  type FingerprintPreview,
  type ProfileRevision,
  type Operation,
} from "./application/contract";
import { applyFingerprint, fingerprintMatchesConfiguration } from "./application/fingerprint-model";
import prdText from "../docs/PRD.md?raw";
import developmentText from "../docs/DEVELOPMENT.md?raw";
import kernelText from "../docs/KERNEL.md?raw";
import userGuideText from "../docs/USER_GUIDE.md?raw";
import { NativeKernelManager } from "./components/NativeKernelManager";
import { NativeMigrationManager } from "./components/NativeMigrationManager";
import { NativeProxyManager } from "./components/NativeProxyManager";
import { NativeCookieImport } from "./components/NativeCookieImport";
import { NativeBatchDialog, type NativeBatchDialogInput } from "./components/NativeBatchDialog";
import { readRuntimeStartPlan } from "./application/runtime-start-plan";
import { NativeRecycleManager } from "./components/NativeRecycleManager";
import { NativeDiagnostics } from "./components/NativeDiagnostics";
import { EnvironmentEditorWindow } from "./components/EnvironmentEditorWindow";
import { DemoCookieImportWindow, DemoEnvironmentRemoveWindow } from "./components/DemoEnvironmentWindows";
import { EnvironmentConfirmation } from "./components/EnvironmentDialogParts";
import { EnvironmentFilters } from "./components/EnvironmentFilters";
import { EnvironmentGroups } from "./components/EnvironmentGroups";
import { ReferencePopover } from "./components/ReferenceUi";
import { EnvironmentRuntimeDetails } from "./components/EnvironmentRuntimeDetails";
import { DemoProxyManager } from "./components/DemoProxyManager";
import { DemoKernelManager } from "./components/DemoKernelManager";
import { ProxyImportWindow } from "./components/ProxyImportWindow";
import { getProxyImportSession } from "./components/proxy-import-session";
import { BackupManagementPage } from "./components/BackupManagementPage";
import { DemoRestoreWindow } from "./components/DemoBackupPage";
import { ActivityPage } from "./components/ActivityPage";
import { HelpPage } from "./components/HelpPage";
import { lockBodyScroll, lockModalBackground, maintainModalFocus, modalLayer, ownsTopModal, restoreModalFocus, topModalElement } from "./components/modal-lifecycle";

type Route =
  "environments" | "groups" | "proxies" | "kernels" | "backups" | "activity" | "guide";
type Drawer = {
  kind: "create" | "edit";
  environment: Environment;
  previewId: string;
  requestId: string;
  creationOperationId?: string;
  creationUnconfirmed?: boolean;
  openAfterCreate?: boolean;
  expectedRevision?: number;
  fingerprint?: FingerprintPreview;
  history?: ProfileRevision[];
  userDataRef?: string;
};
type EnvironmentOutcome = {
  id: string;
  name: string;
  action: "打开" | "关闭" | "分组";
  state: "pending" | "accepted" | "success" | "error" | "skipped";
  message: string;
  operationId?: string;
  created?: boolean;
  group?: string;
};
type PendingEnvironmentConfirmation =
  | { kind: "dirty"; previewId: string; draft: string; profileHash: string; quantity: number }
  | { kind: "force"; environmentId: string; sessionId: string; name: string };
const fingerprintInputKey = (environment: Environment) => JSON.stringify([
  environment.coreId, environment.seed, environment.fingerprintVersion,
  environment.language, environment.timezone, environment.cpu,
  environment.width, environment.height,
]);
type Dialog =
  | { kind: "delete"; ids: string[] }
  | { kind: "cookies"; id: string }
  | { kind: "restore"; snapshot: Snapshot; name: string }
  | { kind: "group"; ids: string[] };
const routeInfo = {
  environments: {
    label: "浏览器环境",
    icon: LayoutGrid,
    description: "每一份环境，都有独立的工作空间。",
    req: "ENV-001 · ENV-002 · FP-001",
  },
  groups: { label: "分组管理", icon: Layers3, description: "修改现有环境的分组标签。", req: "ENV-001" },
  proxies: {
    label: "代理管理",
    icon: Network,
    description: "集中管理网络出口，为每个环境分配代理。",
    req: "PRX-001",
  },
  kernels: {
    label: "内核管理",
    icon: Box,
    description: "明确每个环境使用的版本，按计划验证与升级。",
    req: "CORE-001 · FP-002",
  },
  backups: {
    label: "备份与恢复",
    icon: HardDrive,
    description: "保存环境配置，让每一次调整都有据可回。",
    req: "BKP-001",
  },
  activity: {
    label: "操作记录",
    icon: History,
    description: "查看创建、启动、修改和恢复的完整操作结果。",
    req: "ENV-003 · DATA-001",
  },
  guide: {
    label: "产品与开发",
    icon: BookOpen,
    description: "从页面交互到实现约定，使用同一份需求依据。",
    req: "DOC-001",
  },
};
const statusLabels = {
  ready: "待启动",
  running: "运行中",
  starting: "启动中",
  stopping: "关闭中",
  error: "需处理",
};
const sleep = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms));
const time = (date?: string) =>
  date && Number.isFinite(Date.parse(date))
    ? new Intl.DateTimeFormat("zh-CN", {
        month: "2-digit",
        day: "2-digit",
        hour: "2-digit",
        minute: "2-digit",
      }).format(new Date(date))
    : "尚未打开";
const environmentTime = (date: string) => {
  if (!Number.isFinite(Date.parse(date))) return "—";
  const value = new Date(date);
  return `${new Intl.DateTimeFormat("zh-CN", { month: "long", day: "numeric" }).format(value)} ${new Intl.DateTimeFormat("zh-CN", { hour: "2-digit", minute: "2-digit", hourCycle: "h23" }).format(value)}`;
};
function Button({
  children,
  className = "",
  ...props
}: React.ButtonHTMLAttributes<HTMLButtonElement>) {
  return (
    <button className={`button ${className}`} {...props}>
      {children}
    </button>
  );
}
function Empty({
  title,
  text,
  action,
}: {
  title: string;
  text: string;
  action?: ReactNode;
}) {
  return (
    <div className="empty">
      <div className="empty-symbol">
        <Layers3 size={30} />
      </div>
      <h3>{title}</h3>
      <p>{text}</p>
      {action}
    </div>
  );
}
function Field({
  label,
  hint,
  children,
}: {
  label: string;
  hint?: string;
  children: ReactNode;
}) {
  return (
    <label className="field">
      <span>{label}</span>
      {children}
      {hint && <small>{hint}</small>}
    </label>
  );
}
function docHref(href?: string) {
  if (!href) return "#/guide";
  if (/^(https?:|#|\/#)/.test(href)) return href;
  if (/(?:PRD|DEVELOPMENT|KERNEL|USER_GUIDE)\.md/.test(href)) return "#/guide";
  if (href.includes("README.md"))
    return "https://github.com/axgiroud312-byte/prism-local-browser";
  return (
    "https://github.com/axgiroud312-byte/prism-local-browser/blob/main/" +
    (href.startsWith("../") ? href.slice(3) : "docs/" + href)
  );
}
function download(name: string, content: string, type = "application/json") {
  const url = URL.createObjectURL(new Blob([content], { type }));
  const a = document.createElement("a");
  a.href = url;
  a.download = name;
  a.click();
  setTimeout(() => URL.revokeObjectURL(url), 1500);
}

export default function App({ application }: { application: ApplicationService }) {
  const workspace = useSyncExternalStore(application.subscribe, application.getSnapshot);
  const state = workspace.state;
  const nativeMode = application.mode === "native";
  const environmentTotal = nativeMode ? workspace.environmentPage?.total ?? state.environments.length : state.environments.length;
  const storageIssue = workspace.issue?.message || "";
  const current = useRef(state);
  current.current = state;
  const [route, setRoute] = useState<Route>(() => {
    const name = location.hash.replace("#/", "") as Route;
    return name in routeInfo ? name : "environments";
  });
  const [search, setSearch] = useState("");
  const [group, setGroup] = useState("全部分组");
  const [status, setStatus] = useState("all");
  const [selected, setSelected] = useState<string[]>([]);
  const [page, setPage] = useState(1);
  const [drawer, setDrawer] = useState<Drawer | null>(null);
  const [drawerSuspended, setDrawerSuspended] = useState(false);
  const drawerVisible = !!drawer && !drawerSuspended;
  const drawerRef = useRef(drawer);
  drawerRef.current = drawer;
  const drawerRuntime = drawer?.kind === "edit" ? workspace.runtimeSessions?.[drawer.environment.id] : undefined;
  const drawerNetworkResources = drawer?.kind === "edit" ? workspace.networkResources?.[drawer.environment.id] : undefined;
  const profileBusy = nativeMode && (!!drawerNetworkResources || !!drawerRuntime && (["starting", "running", "stopping"].includes(drawerRuntime.state) || !!drawerRuntime.pid || drawerRuntime.resourcesPending || drawerRuntime.needsReconcile || drawerRuntime.persistencePending));
  const previewOpenSequence = useRef(0);
  const fingerprintBusy = useRef(false);
  const automaticPreviewAttempt = useRef("");
  const [previewError, setPreviewError] = useState("");
  const [dialog, setDialog] = useState<Dialog | null>(null);
  const legacyDialog = dialog?.kind !== "restore" ? dialog : null;
  const [environmentConfirmation, setEnvironmentConfirmation] = useState<PendingEnvironmentConfirmation | null>(null);
  const [confirmationLayer, setConfirmationLayer] = useState(120);
  const environmentConfirmationRef = useRef<PendingEnvironmentConfirmation | null>(null);
  const [nativeProxyImportOpen, setNativeProxyImportOpen] = useState(false);
  const [migrationOpen, setMigrationOpen] = useState(false);
  const proxyImportSession = getProxyImportSession(application);
  const [draftProxyImportOpen, setDraftProxyImportOpen] = useState(false);
  const [draftProxyImportBusy, setDraftProxyImportBusy] = useState(false);
  const [nativeCookieEnvironment, setNativeCookieEnvironment] = useState<Environment | null>(null);
  const [nativeBatchInput, setNativeBatchInput] = useState<NativeBatchDialogInput | null>(null);
  const [nativeBackupSelection, setNativeBackupSelection] = useState<string[]>([]);
  const [nativeRecycleSelection, setNativeRecycleSelection] = useState<string[] | null>(null);
  const [environmentQueryBusy, setEnvironmentQueryBusy] = useState(false);
  const [formError, setFormError] = useState("");
  const [generating, setGenerating] = useState(false);
  const [savePending, setSavePending] = useState(false);
  const [quantity, setQuantity] = useState(1);
  const quantityRef = useRef(quantity);
  quantityRef.current = quantity;
  const [toast, setToast] = useState<{ text: string; error?: boolean } | null>(
    null,
  );
  const toastTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const [menu, setMenu] = useState<string | null>(null);
  const [checking, setChecking] = useState<string[]>([]);
  const [cookieText, setCookieText] = useState("");
  const [cookieResult, setCookieResult] = useState<ReturnType<
    typeof parseCookies
  > | null>(null);
  const [groupName, setGroupName] = useState("");
  const [docTab, setDocTab] = useState<"user" | "prd" | "development" | "kernel">(nativeMode ? "user" : "prd");
  const [batch, setBatch] = useState<{
    label: string;
    done: number;
    total: number;
  } | null>(null);
  const batchCancelled = useRef(false);
  const batchBusy = useRef(false);
  const saving = useRef(false);
  const creationOperation = useRef<string | null>(null);
  const latestOperationEvent = useRef<OperationEvent | undefined>(undefined);
  const checkingBusy = useRef(false);
  const runtimeActions = useRef(new Set<string>());
  const [runtimeActionIds, setRuntimeActionIds] = useState<string[]>([]);
  const [outcomes, setOutcomes] = useState<EnvironmentOutcome[]>([]);
  const outcomesRef = useRef(outcomes);
  outcomesRef.current = outcomes;
  const [groupPending, setGroupPending] = useState(false);
  const initialDraft = useRef("");
  const initialFingerprintHash = useRef("");
  const overlayRef = useRef<HTMLDivElement>(null);
  const workspaceOverlayRef = useRef<HTMLDivElement>(null);
  const drawerOverlayRef = useRef<HTMLDivElement>(null);
  const confirmationOverlayRef = useRef<HTMLDivElement>(null);
  const confirmationReturnFocus = useRef<HTMLElement | null>(null);
  const drawerReturnFocus = useRef<HTMLElement | null>(null);
  const draftProxyOverlayRef = useRef<HTMLDivElement>(null);
  const proxyReturnFocus = useRef<HTMLElement | null>(null);
  const [menuAnchor, setMenuAnchor] = useState<HTMLElement | null>(null);
  const searchRef = useRef<HTMLInputElement>(null);
  const uiBlocked = Boolean(drawerVisible || dialog || environmentConfirmation || draftProxyImportOpen || nativeBatchInput || nativeCookieEnvironment || nativeRecycleSelection || storageIssue);
  const appInert = Boolean(drawerVisible || legacyDialog || environmentConfirmation || draftProxyImportOpen || nativeBatchInput || nativeCookieEnvironment || nativeRecycleSelection || storageIssue);
  useEffect(() => { if (uiBlocked) setMenu(null); }, [uiBlocked]);
  const notify = (text: string, error = false) => {
    setToast({ text, error });
    if (toastTimer.current) clearTimeout(toastTimer.current);
    toastTimer.current = setTimeout(() => setToast(null), 4200);
  };
  const updateDemo = (fn: (s: State) => State, activity?: Pick<Activity, "action" | "target" | "detail" | "result">) => {
    const result = application.compatibility?.update((s) => {
      const next = fn(s);
      return activity ? { ...next, activities: [{ ...activity, id: uid("log"), time: now() }, ...next.activities] } : next;
    }) ?? { ok: false as const, mode: application.mode, error: { code: "CAPABILITY_UNSUPPORTED", message: "此演示操作尚未接入桌面服务。", retryable: false } };
    if (!result.ok) notify(result.error.message, true);
    return result;
  };
  const update = (fn: (s: State) => State, activity?: Pick<Activity, "action" | "target" | "detail" | "result">) => updateDemo(fn, activity).ok;
  const log = (
    action: string,
    target: string,
    detail: string,
    result: "success" | "error" | "info" = "success",
  ) => ({ action, target, detail, result });
  useEffect(() => {
    const changed = (e: StorageEvent) => application.compatibility?.handleStorageChange(e);
    window.addEventListener("storage", changed);
    return () => window.removeEventListener("storage", changed);
  }, [application]);
  useEffect(() => application.subscribeEvents((next) => {
    if (next.mode !== application.mode || next.operationId !== creationOperation.current) return;
    const event = newerOperationEvent(latestOperationEvent.current, next);
    latestOperationEvent.current = event;
    if (event) setBatch({ label: "创建环境", done: event.operation.completedIds.length, total: event.operation.total });
  }), [application]);
  const nativeRuntimeActive = nativeMode && (Object.keys(workspace.networkResources ?? {}).length > 0 || !!workspace.migrationMaintenance || !!workspace.maintenance || !!workspace.recycleMaintenance || (workspace.restoreOperations ?? []).some(operation => !operationIsTerminal(operation)) || Object.values(workspace.runtimeSessions ?? {}).some(session => ["starting", "running", "stopping"].includes(session.state) || !!session.pid || session.resourcesPending || session.needsReconcile || session.persistencePending) || (workspace.batchOperations ?? []).some(operation => !operationIsTerminal(operation)));
  useEffect(() => {
    if (!nativeRuntimeActive || !application.refresh) return;
    let cancelled = false;
    let timer: ReturnType<typeof setTimeout>;
    const poll = async () => {
      if (cancelled) return;
      await application.refresh?.();
      if (!cancelled) timer = setTimeout(() => void poll(), 1000);
    };
    timer = setTimeout(() => void poll(), 1000);
    return () => { cancelled = true; clearTimeout(timer); };
  }, [application, nativeRuntimeActive]);
  useEffect(() => {
    if (!nativeMode || !application.queryEnvironments) return;
    let cancelled = false;
    setEnvironmentQueryBusy(true);
    const timer = setTimeout(() => {
      void application.queryEnvironments!({ page, pageSize: 10, search, group: group === "全部分组" ? "" : group, status }).then(result => {
        if (cancelled) return; setEnvironmentQueryBusy(false);
        if (!result.ok) notify(result.error.message, true);
      });
    }, 180);
    return () => { cancelled = true; clearTimeout(timer); };
  }, [application, nativeMode, page, search, group, status]);
  useEffect(() => {
    const change = () => {
      const r = location.hash.replace("#/", "") as Route;
      setRoute(r in routeInfo ? r : "environments");
      setSearch("");
      setSelected([]);
      setPage(1);
      setMenu(null);
    };
    window.addEventListener("hashchange", change);
    return () => window.removeEventListener("hashchange", change);
  }, []);
  function showEnvironmentConfirmation(pending: PendingEnvironmentConfirmation) {
    if (environmentConfirmationRef.current || application.getSnapshot().issue) return;
    const top = topModalElement();
    setConfirmationLayer(Math.min(158, Math.max(120, top ? modalLayer(top) + 2 : 120)));
    confirmationReturnFocus.current = document.activeElement as HTMLElement;
    environmentConfirmationRef.current = pending;
    setEnvironmentConfirmation(pending);
  }
  function cancelEnvironmentConfirmation() {
    environmentConfirmationRef.current = null;
    setEnvironmentConfirmation(null);
  }
  function discardDrawer(previewId: string) {
    const draft = drawerRef.current;
    if (!draft || draft.previewId !== previewId || saving.current || draft.creationOperationId || draft.creationUnconfirmed) return;
    previewOpenSequence.current++;
    drawerRef.current = null;
    void application.discardPreview(previewId);
    setDrawer(null);
    setDrawerSuspended(false);
    setPreviewError("");
  }
  const closeDrawer = () => {
    const draft = drawerRef.current;
    if (!draft || saving.current || draft.creationOperationId || draft.creationUnconfirmed || environmentConfirmationRef.current) return;
    const signature = JSON.stringify(draft.environment), profileHash = draft.fingerprint?.previewProfile.configHash ?? "";
    if (signature !== initialDraft.current || profileHash !== initialFingerprintHash.current || draft.kind === "create" && quantityRef.current !== 1) {
      showEnvironmentConfirmation({ kind: "dirty", previewId: draft.previewId, draft: signature, profileHash, quantity: quantityRef.current });
    } else discardDrawer(draft.previewId);
  };
  function confirmEnvironmentAction() {
    const pending = environmentConfirmationRef.current;
    if (!pending || application.getSnapshot().issue || topModalElement() !== confirmationOverlayRef.current) return;
    cancelEnvironmentConfirmation(); // Consume once before any asynchronous operation.
    if (pending.kind === "force") {
      void submitRuntimeSessionAction(pending.environmentId, pending.sessionId, "force");
      return;
    }
    const draft = drawerRef.current;
    if (!draft || draft.previewId !== pending.previewId || JSON.stringify(draft.environment) !== pending.draft || (draft.fingerprint?.previewProfile.configHash ?? "") !== pending.profileHash || quantityRef.current !== pending.quantity || saving.current || draft.creationOperationId || draft.creationUnconfirmed) {
      notify("草稿或保存状态已变化，未放弃任何预览；请检查当前内容后重新确认。", true);
      return;
    }
    discardDrawer(pending.previewId);
  }
  useEffect(() => {
    if (environmentConfirmation) return;
    const target = confirmationReturnFocus.current;
    confirmationReturnFocus.current = null;
    if (!target) return;
    const timer = setTimeout(() => {
      const fallback = drawerRef.current ? drawerOverlayRef.current : drawerReturnFocus.current;
      const eligible = target.isConnected && !target.closest("[inert]") && !target.matches(":disabled") && target.getClientRects().length > 0;
      restoreModalFocus(eligible ? target : fallback);
    }, 0);
    return () => clearTimeout(timer);
  }, [Boolean(environmentConfirmation)]);
  useEffect(() => {
    if (drawerVisible || drawer || environmentConfirmation) return;
    const target = drawerReturnFocus.current;
    if (!target) return;
    const timer = setTimeout(() => restoreModalFocus(target), 0);
    return () => clearTimeout(timer);
  }, [drawerVisible, Boolean(drawer), Boolean(environmentConfirmation)]);
  useEffect(() => {
    const key = (e: KeyboardEvent) => {
      if (e.defaultPrevented) return;
      if (storageIssue) {
        if (e.key === "Escape" || (e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "k") e.preventDefault();
        return;
      }
      const top = topModalElement();
      if (top && ![drawerOverlayRef.current, overlayRef.current, draftProxyOverlayRef.current, confirmationOverlayRef.current].some(ownsTopModal)) {
        if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "k") e.preventDefault();
        return;
      }
      if (environmentConfirmationRef.current) return; // Capture listener owns warning keys, not the lower form/manager.
      if (nativeBatchInput || nativeCookieEnvironment || nativeRecycleSelection) {
        if ((e.ctrlKey || e.metaKey) && e.key === "k") e.preventDefault();
        return;
      }
      if (draftProxyImportOpen || legacyDialog) {
        if ((e.ctrlKey || e.metaKey) && e.key === "k") e.preventDefault();
        if (e.key === "Escape" && !draftProxyImportBusy && !groupPending) {
          e.preventDefault();
          if (draftProxyImportOpen) closeDraftProxyImport();
          else setDialog(null);
        }
        return;
      }
      if ((e.ctrlKey || e.metaKey) && e.key === "k") {
        e.preventDefault();
        searchRef.current?.focus();
      }
      if (e.key === "Escape") {
        setMenu(null);
        if (drawerVisible) closeDrawer();
      }
    };
    window.addEventListener("keydown", key);
    return () => window.removeEventListener("keydown", key);
  }, [generating, drawer, drawerVisible, legacyDialog, environmentConfirmation, draftProxyImportOpen, draftProxyImportBusy, groupPending, nativeBatchInput, nativeCookieEnvironment, nativeRecycleSelection, storageIssue]);
  useEffect(() => {
    if (draftProxyImportOpen || !drawerVisible || !proxyReturnFocus.current) return;
    const timer = setTimeout(() => {
      restoreModalFocus(proxyReturnFocus.current);
    }, 40);
    return () => clearTimeout(timer);
  }, [draftProxyImportOpen, drawerVisible]);
  useEffect(() => {
    if (!formError) return;
    const overlay = legacyDialog ? overlayRef.current : drawerVisible ? drawerOverlayRef.current : null;
    overlay?.querySelector('[role="alert"]')?.scrollIntoView({ block: "nearest" });
  }, [formError, Boolean(legacyDialog), drawerVisible]);
  useEffect(() => {
    if (environmentConfirmation?.kind !== "force") return;
    const lowerLayers = [...document.querySelectorAll<HTMLElement>(
      '.app-shell > .overlay:not(.workspace-blocker), body > .pk35-overlay, body > .local-page-overlay',
    )].filter(element => modalLayer(element) < confirmationLayer);
    return lockModalBackground(lowerLayers, true);
  }, [environmentConfirmation?.kind, confirmationLayer]);
  useEffect(() => {
    if (!storageIssue) return;
    const lowerLayers = [...document.querySelectorAll<HTMLElement>(
      '.app-shell > .overlay:not(.workspace-blocker), body > .pk35-overlay, body > .local-page-overlay',
    )].filter(element => modalLayer(element) < 160);
    return lockModalBackground(lowerLayers, true);
  }, [Boolean(storageIssue)]);
  useEffect(() => {
    if (!drawerVisible && !legacyDialog && !environmentConfirmation && !draftProxyImportOpen && !storageIssue) return;
    const releaseScroll = lockBodyScroll();
    const previous = document.activeElement as HTMLElement;
    const topOverlay = () => storageIssue ? workspaceOverlayRef.current : environmentConfirmationRef.current ? confirmationOverlayRef.current : draftProxyImportOpen ? draftProxyOverlayRef.current : legacyDialog ? overlayRef.current : drawerOverlayRef.current;
    const focusable = (overlay: HTMLElement) => [...overlay.querySelectorAll<HTMLElement>(
      'button:not(:disabled), input:not(:disabled), select:not(:disabled), textarea:not(:disabled), a[href], summary, [tabindex="0"]',
    )].filter(element => element.getClientRects().length > 0 && !element.closest("[inert]"));
    const focusFirst = () => {
      const overlay = topOverlay();
      if (overlay && ownsTopModal(overlay)) (focusable(overlay)[0] ?? overlay).focus();
    };
    const timer = setTimeout(focusFirst, 30);
    const releaseFocus = maintainModalFocus(topOverlay);
    const trap = (e: KeyboardEvent) => {
      const overlay = topOverlay();
      if (e.defaultPrevented || !ownsTopModal(overlay) || !overlay) return;
      if (storageIssue && (e.key === "Escape" || (e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "k")) {
        e.preventDefault();
        e.stopImmediatePropagation();
        return;
      }
      if (environmentConfirmationRef.current && (e.key === "Escape" || (e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "k")) {
        e.preventDefault();
        e.stopImmediatePropagation();
        if (e.key === "Escape") cancelEnvironmentConfirmation();
        return;
      }
      if (e.key !== "Tab") return;
      if (storageIssue || environmentConfirmationRef.current) e.stopImmediatePropagation();
      const elements = focusable(overlay);
      const first = elements[0],
        last = elements.at(-1);
      if (!first) {
        e.preventDefault();
        topModalElement()?.focus();
      } else if (e.shiftKey && (document.activeElement === first || !overlay.contains(document.activeElement))) {
        e.preventDefault();
        last?.focus();
      } else if (!e.shiftKey && (document.activeElement === last || !overlay.contains(document.activeElement))) {
        e.preventDefault();
        first?.focus();
      }
    };
    const repairFocus = (event: FocusEvent) => {
      const overlay = topOverlay();
      if (overlay && ownsTopModal(overlay) && !overlay.contains(event.target as Node)) focusFirst();
    };
    document.addEventListener("keydown", trap, true);
    document.addEventListener("focusin", repairFocus);
    return () => {
      releaseScroll();
      releaseFocus();
      clearTimeout(timer);
      document.removeEventListener("keydown", trap, true);
      document.removeEventListener("focusin", repairFocus);
      restoreModalFocus(previous?.isConnected && !previous.closest("[inert]") ? previous : menuAnchor);
    };
  }, [drawerVisible, Boolean(legacyDialog), Boolean(environmentConfirmation), draftProxyImportOpen, Boolean(storageIssue)]);
  const navigate = (r: Route) => {
    location.hash = `/${r}`;
  };
  const groups = nativeMode && workspace.environmentPage ? workspace.environmentPage.groups : [
    ...new Set(state.environments.map((e) => e.group).filter(Boolean)),
  ];
  const visible = nativeMode ? state.environments : state.environments.filter(
    (e) =>
      (!search ||
        `${e.name} ${e.code} ${e.note}`
          .toLowerCase()
          .includes(search.toLowerCase())) &&
      (group === "全部分组" || e.group === group) &&
      (status === "all" || e.status === status),
  );
  const filteredTotal = nativeMode ? workspace.environmentPage?.filteredTotal ?? visible.length : visible.length;
  const pageCount = Math.max(1, Math.ceil(filteredTotal / 10));
  const pageItems = nativeMode ? visible : visible.slice(
    (Math.min(page, pageCount) - 1) * 10,
    Math.min(page, pageCount) * 10,
  );
  const running = nativeMode ? workspace.environmentPage?.runningCount ?? 0 : state.environments.filter(
    (e) => e.status === "running",
  ).length;
  const errors = nativeMode ? workspace.environmentPage?.errorCount ?? 0 : state.environments.filter((e) => e.status === "error").length;
  const usableKernels = state.kernels.filter(kernel => kernel.available && (!nativeMode || workspace.kernelRecords?.some(record => record.id === kernel.id && record.status === "verified")));
  const patchDraft = (value: Partial<Environment>) => {
    if (environmentConfirmationRef.current || saving.current || drawerRef.current?.creationOperationId || drawerRef.current?.creationUnconfirmed) return;
    if (profileBusy && Object.keys(value).some(key => !["name", "group", "note"].includes(key))) return;
    setFormError("");
    setDrawer((d) =>
      d ? { ...d, requestId: uid("request"), environment: { ...d.environment, ...value } } : d,
    );
  };
  async function openCreate(template?: Environment) {
    if (environmentConfirmationRef.current) return;
    if (drawerRef.current) { setDrawerSuspended(false); return; }
    if (saving.current || batchBusy.current) { notify("请等待当前操作完成，或取消余下任务。", true); return; }
    drawerReturnFocus.current = menuAnchor?.isConnected && menu ? menuAnchor : document.activeElement as HTMLElement;
    const sequence = ++previewOpenSequence.current;
    const result = await application.previewEnvironment({ kind: "create", sourceId: template?.id });
    if (sequence !== previewOpenSequence.current) { if (result.ok) void application.discardPreview(result.data.previewId); return; }
    if (!result.ok) { notify(result.error.message, true); return; }
    const preview = result.data;
    const environment = { ...preview.environment, coreId: usableKernels.some(kernel => kernel.id === preview.environment.coreId) ? preview.environment.coreId : usableKernels.find(kernel => kernel.id === workspace.defaultKernel?.kernelId)?.id ?? usableKernels[0]?.id ?? preview.environment.coreId };
    if (!template && group !== "全部分组") environment.group = group;
    initialDraft.current = JSON.stringify(environment);
    initialFingerprintHash.current = preview.fingerprint?.previewProfile.configHash ?? "";
    automaticPreviewAttempt.current = "";
    setPreviewError("");
    setDrawer({ kind: "create", ...preview, environment, requestId: uid("request") });
    setFormError("");
    setQuantity(1);
    setDrawerSuspended(false);
    setMenu(null);
  }
  async function openEdit(e: Environment) {
    if (environmentConfirmationRef.current) return;
    if (drawerRef.current) { setDrawerSuspended(false); return; }
    if (saving.current || batchBusy.current) { notify("请等待当前操作完成，或取消余下任务。", true); return; }
    drawerReturnFocus.current = menuAnchor?.isConnected ? menuAnchor : document.activeElement as HTMLElement;
    const sequence = ++previewOpenSequence.current;
    const result = await application.previewEnvironment({ kind: "edit", sourceId: e.id });
    if (sequence !== previewOpenSequence.current) { if (result.ok) void application.discardPreview(result.data.previewId); return; }
    if (!result.ok) { notify(result.error.message, true); return; }
    const preview = result.data;
    initialDraft.current = JSON.stringify(preview.environment);
    initialFingerprintHash.current = preview.fingerprint?.previewProfile.configHash ?? "";
    automaticPreviewAttempt.current = "";
    setPreviewError("");
    setDrawer({ kind: "edit", ...preview, requestId: uid("request") });
    setQuantity(1);
    setDrawerSuspended(false);
    setFormError("");
    setMenu(null);
    const history = await application.listFingerprintRevisions(e.id);
    setDrawer(current => current?.previewId === preview.previewId ? { ...current, history: history.ok ? history.data : undefined } : current);
    if (!history.ok && previewOpenSequence.current === sequence) setFormError(history.error.message);
  }
  function applyProfilePreview(previewId: string, preview: EnvironmentPreview) {
    setDrawer(current => {
      if (!current || current.previewId !== previewId || !preview.fingerprint) return current;
      return { ...current, requestId: uid("request"), fingerprint: preview.fingerprint, userDataRef: preview.userDataRef ?? current.userDataRef, environment: applyFingerprint(current.environment, preview.fingerprint.previewProfile) };
    });
  }
  async function generateProfile(regenerate = false, automatic = false) {
    const draft = drawerRef.current;
    if (environmentConfirmationRef.current || !draft || draft.creationOperationId || draft.creationUnconfirmed || fingerprintBusy.current || saving.current || profileBusy) return;
    const target = draft.previewId;
    const inputKey = fingerprintInputKey(draft.environment);
    fingerprintBusy.current = true;
    setGenerating(true);
    setPreviewError("");
    try {
      const result = await application.generateFingerprint({ previewId: target, kernelId: draft.environment.coreId, templateId: draft.environment.fingerprintVersion, overrides: draft.environment, regenerate });
      if (drawerRef.current?.previewId !== target || fingerprintInputKey(drawerRef.current.environment) !== inputKey) return;
      if (result.ok) {
        if (automatic && !initialFingerprintHash.current) initialFingerprintHash.current = result.data.fingerprint?.previewProfile.configHash ?? "";
        applyProfilePreview(target, result.data);
        if (regenerate) notify("已换一套指纹草稿；保存才生效，取消保留原身份。");
      } else setPreviewError(result.error.message);
    } catch {
      if (drawerRef.current?.previewId === target) setPreviewError("指纹预览暂时无法读取，草稿已保留，请重试。");
    } finally { fingerprintBusy.current = false; setGenerating(false); }
  }
  const draftFingerprintKey = drawer ? fingerprintInputKey(drawer.environment) : "";
  const selectedKernelRecord = workspace.kernelRecords?.find(record => record.id === drawer?.environment.coreId);
  const canGenerateProfile = !!drawer && usableKernels.some(kernel => kernel.id === drawer.environment.coreId);
  const profileIsFresh = !!drawer?.fingerprint && fingerprintMatchesConfiguration(drawer.fingerprint.previewProfile, drawer.environment);
  const pendingConfiguration = nativeMode && drawer?.environment.coreId === "kernel-pending";
  const canSaveProfile = drawer?.kind === "edit" ? pendingConfiguration || profileIsFresh : profileIsFresh && canGenerateProfile;
  useEffect(() => {
    if (environmentConfirmation || !drawer || drawer.creationOperationId || drawer.creationUnconfirmed || profileBusy || !canGenerateProfile || profileIsFresh || generating || savePending) return;
    const key = `${drawer.previewId}:${draftFingerprintKey}`;
    if (automaticPreviewAttempt.current === key) return;
    const timer = setTimeout(() => {
      if (environmentConfirmationRef.current || fingerprintBusy.current || saving.current) return;
      automaticPreviewAttempt.current = key;
      void generateProfile(false, true);
    }, 120);
    return () => clearTimeout(timer);
  }, [Boolean(environmentConfirmation), drawer?.previewId, drawer?.creationOperationId, drawer?.creationUnconfirmed, draftFingerprintKey, canGenerateProfile, profileIsFresh, profileBusy, generating, savePending]);
  async function previewProfileRestore(revision: number) {
    if (environmentConfirmationRef.current || !drawer || fingerprintBusy.current || saving.current || profileBusy) return;
    const target = drawer.previewId;
    fingerprintBusy.current = true; setGenerating(true); setFormError("");
    try {
      const result = await application.previewFingerprintRestore(target, revision);
      if (drawerRef.current?.previewId !== target) return;
      if (result.ok) { applyProfilePreview(target, result.data); notify("旧档案已加载为回滚预览；保存才生效，名称、代理和数据保持不变。"); }
      else setFormError(result.error.message);
    } finally { fingerprintBusy.current = false; setGenerating(false); }
  }
  async function saveEnvironment(openAfterCreate = false) {
    if (environmentConfirmationRef.current || batchBusy.current || saving.current || fingerprintBusy.current || !drawer) return;
    const target = drawer.previewId;
    saving.current = true;
    setSavePending(true);
    try {
      const request = { previewId: drawer.previewId, configuration: drawer.environment, requestId: drawer.requestId, profileHash: drawer.fingerprint?.previewProfile.configHash };
      if (drawer.kind === "edit") {
        const result = drawer.fingerprint && (!nativeMode || drawer.environment.coreId !== "kernel-pending")
          ? await application.commitFingerprintRevision({ ...request, expectedRevision: drawer.expectedRevision!, environmentId: drawer.environment.id, profileHash: drawer.fingerprint.previewProfile.configHash })
          : await application.updateEnvironment({ ...request, expectedRevision: drawer.expectedRevision! });
        if (drawerRef.current?.previewId !== target) return;
        if (!result.ok) { setFormError(result.error.message); return; }
        notify("环境配置已保存");
        setDrawer(null);
        return;
      }
      const recoveringCreation = !!drawer.creationOperationId || !!drawer.creationUnconfirmed;
      if (!recoveringCreation && (!Number.isSafeInteger(quantity) || quantity < 1)) { setFormError("数量必须是可精确表示的正整数；没有保存总数产品配额。"); return; }
      if (!recoveringCreation && nativeMode && quantity > 1) {
        const planned = await application.previewBatch?.({ kind: "create", create: { ...request, count: quantity } });
        if (drawerRef.current?.previewId !== target) return;
        if (!planned?.ok) { setFormError(planned && !planned.ok ? planned.error.message : "当前桌面服务未提供持久批次。"); return; }
        setNativeBatchInput({ kind: "create", initialPage: planned.data }); setDrawer(null); void application.discardPreview(target);
        return;
      }
      let operation: Operation | undefined;
      if (drawer.creationOperationId) {
        const existing = await application.getOperation(drawer.creationOperationId);
        if (!existing.ok) { setFormError(`创建结果尚未确认：${existing.error.message}。重试只核实原任务，不会重复创建。`); return; }
        operation = existing.data;
      } else {
        const accepted = await application.createBatch({ ...request, count: quantity });
        if (drawerRef.current?.previewId !== target) return;
        if (!accepted.ok) {
          if (nativeMode && ["NATIVE_UNAVAILABLE", "CAPABILITY_UNSUPPORTED"].includes(accepted.error.code)) setDrawer(draft => draft?.previewId === target ? { ...draft, creationUnconfirmed: true, openAfterCreate: openAfterCreate || draft.openAfterCreate } : draft);
          else setDrawer(draft => draft?.previewId === target ? { ...draft, creationUnconfirmed: false, openAfterCreate: undefined } : draft);
          setFormError(accepted.error.message); return;
        }
        operation = accepted.data.operation;
        setDrawer(draft => draft?.previewId === target ? { ...draft, creationOperationId: operation!.id, creationUnconfirmed: false, openAfterCreate: openAfterCreate || draft.openAfterCreate } : draft);
      }
      batchBusy.current = true;
      creationOperation.current = operation.id;
      latestOperationEvent.current = undefined;
      if (!nativeMode && quantity > 1) setDrawer(null);
      let inspected = { ok: true as const, mode: application.mode, data: operation } as Awaited<ReturnType<ApplicationService["getOperation"]>>;
      while (inspected.ok && !operationIsTerminal(inspected.data)) {
        setBatch({ label: "创建环境", done: inspected.data.completedIds.length, total: inspected.data.total });
        await sleep(30);
        inspected = await application.getOperation(creationOperation.current);
      }
      if (!inspected.ok) { setFormError(`创建结果尚未确认：${inspected.error.message}。请重试核实原任务，不会重复创建。`); return; }
      else {
        operation = inspected.data;
        if (!operation.completedIds.length) {
          setFormError(operation.error?.message || "创建未完成，草稿已保留，请修正后重试。");
          setDrawer(draft => draft?.previewId === target || !draft && quantity > 1 ? { ...(draft ?? drawer), creationOperationId: undefined, creationUnconfirmed: false, openAfterCreate: undefined, requestId: uid("request") } : draft);
          return;
        }
        setDrawer(null);
        void application.discardPreview(target);
        notify(`已创建 ${operation.completedIds.length} 个环境${operation.state !== "completed" ? "；其余未完成，请查看操作记录" : ""}`, operation.state === "failed");
      }
      setGroup("全部分组"); setStatus("all"); setSearch(""); setPage(1);
      // Creation is already committed. Opening/retrying uses only these IDs.
      batchBusy.current = false;
      setBatch(null);
      if (openAfterCreate || drawer.openAfterCreate) await launch(operation.completedIds, true);
    } finally {
      saving.current = false;
      setSavePending(false);
      if (creationOperation.current) {
        batchBusy.current = false;
        creationOperation.current = null;
        setBatch(null);
      }
    }
  }
  function beginRuntimeAction(id: string) {
    if (runtimeActions.current.has(id)) return false;
    runtimeActions.current.add(id);
    setRuntimeActionIds([...runtimeActions.current]);
    return true;
  }
  function endRuntimeAction(id: string) {
    runtimeActions.current.delete(id);
    setRuntimeActionIds([...runtimeActions.current]);
  }
  function setOutcome(outcome: EnvironmentOutcome) {
    setOutcomes(previous => {
      const found = previous.find(item => item.id === outcome.id);
      const next = { ...found, ...outcome, operationId: outcome.operationId, created: outcome.created ?? found?.created };
      return found ? previous.map(item => item.id === outcome.id ? next : item) : [...previous, next];
    });
  }
  function applyRuntimeOperation(id: string, operation: Operation) {
    setOutcomes(previous => previous.map(item => {
      if (item.id !== id || item.operationId !== operation.id) return item;
      const finished = operationIsTerminal(operation);
      return { ...item,
        state: !finished ? "accepted" : operation.state === "completed" ? "success" : "error",
        message: !finished ? operation.persistencePending ? "结果待保存，请稍候核实" : item.action === "打开" ? "正在打开，等待浏览器就绪" : "正在关闭，等待资源释放" : operation.state === "completed" ? item.action === "打开" ? "已打开" : "已关闭，浏览数据已保留" : operation.error?.message || "任务未完成，可修正后重试",
      };
    }));
  }
  const pendingRuntimeOutcomeKey = outcomes.filter(item => item.operationId && ["pending", "accepted"].includes(item.state)).map(item => `${item.id}:${item.operationId}`).join("|");
  useEffect(() => {
    if (!nativeMode || !pendingRuntimeOutcomeKey) return;
    let cancelled = false;
    let timer: ReturnType<typeof setTimeout>;
    const read = async () => {
      for (const item of outcomesRef.current.filter(item => item.operationId && ["pending", "accepted"].includes(item.state))) {
        const result = await application.getOperation(item.operationId!);
        if (cancelled) return;
        if (result.ok) applyRuntimeOperation(item.id, result.data);
        else setOutcomes(previous => previous.map(outcome => outcome.id === item.id && outcome.operationId === item.operationId && ["pending", "accepted"].includes(outcome.state) ? { ...outcome, message: `结果尚未确认：${result.error.message}；正在自动核实，不会重复提交。` } : outcome));
      }
      if (!cancelled) timer = setTimeout(() => void read(), 750);
    };
    void read();
    return () => { cancelled = true; clearTimeout(timer); };
  }, [application, nativeMode, pendingRuntimeOutcomeKey]);
  function failDemoWrite(targets: string[], index: number, action: "打开" | "关闭", message: string) {
    const id = targets[index];
    const saved = application.getSnapshot().state.environments.find(item => item.id === id);
    const detail = saved && ["starting", "stopping"].includes(saved.status) ? `${message} 已保存状态仍为${statusLabels[saved.status]}；恢复存储后重试${action}会继续保存模拟结果。` : message;
    const unexecuted = new Set(targets.slice(index + 1));
    setOutcomes(previous => previous.map(item => {
      if (item.action !== action || !["pending", "accepted"].includes(item.state)) return item;
      if (item.id === id) return { ...item, state: "error", message: detail };
      if (unexecuted.has(item.id)) return { ...item, state: "skipped", message: "前一项未能保存，本项尚未执行；恢复存储后可重新操作。" };
      return item;
    }));
  }
  async function launch(ids: string[], created = false) {
    if (nativeMode) {
      if (!application.startRuntime) { notify("当前桌面版本未接入真实启停。", true); return; }
      const targets = [...new Set(ids)].filter(id => !runtimeActions.current.has(id));
      if (!targets.length) return;
      targets.forEach(beginRuntimeAction);
      try {
        for (const id of targets) {
          const name = current.current.environments.find(environment => environment.id === id)?.name ?? "所选环境";
          setOutcome({ id, name, action: "打开", state: "pending", message: "正在读取保存的配置", created });
          // Resolve each exact ID, including off-page selections; one failure
          // must not prevent other selected environments from opening.
          const planned = await readRuntimeStartPlan(application, [id]);
          if (!planned.ok) { setOutcome({ id, name, action: "打开", state: "error", message: planned.error.message, created }); continue; }
          const item = planned.data[0];
          const result = await application.startRuntime({ environmentId: item.environmentId, requestId: uid("request"), networkPolicy: item.networkPolicy, expectedRevision: item.expectedRevision });
          if (!result.ok) { setOutcome({ id, name: item.name, action: "打开", state: "error", message: result.error.message, created }); continue; }
          setOutcome({ id, name: item.name, action: "打开", state: "accepted", operationId: result.data.operation.id, message: "打开任务已受理，等待浏览器就绪", created });
          applyRuntimeOperation(id, result.data.operation);
        }
      } finally { targets.forEach(endRuntimeAction); }
      setMenu(null);
      return;
    }
    if (batchBusy.current) {
      notify("请等待当前批次完成，或先取消剩余任务。", true);
      return;
    }
    batchBusy.current = true;
    batchCancelled.current = false;
    let done = 0;
    setMenu(null);
    const targets = [...new Set(ids)].filter(id => !runtimeActions.current.has(id));
    targets.forEach(id => setOutcome({ id, name: current.current.environments.find(environment => environment.id === id)?.name ?? "所选环境", action: "打开", state: "pending", message: "等待模拟打开", created }));
    try {
      for (const [index, id] of targets.entries()) {
        const snapshot = application.getSnapshot().state;
        const e = snapshot.environments.find((i) => i.id === id);
        if (batchCancelled.current) {
          setOutcomes(previous => previous.map(item => item.id === id && item.action === "打开" && item.state === "pending" ? { ...item, state: "skipped", message: "已取消，未继续打开" } : item));
          continue;
        }
        if (!e) { setOutcome({ id, name: "所选环境", action: "打开", state: "error", message: "环境已不存在，请刷新列表", created }); continue; }
        if (["running", "stopping"].includes(e.status) || runtimeActions.current.has(id)) {
          setOutcome({ id, name: e.name, action: "打开", state: "skipped", message: e.status === "running" ? "已运行，无需重复打开" : "请先完成关闭，再打开", created });
          continue;
        }
        setBatch({ label: "启动环境", done, total: targets.length });
        const failure = launchError(e, snapshot);
        if (failure) {
          const result = updateDemo((s) => ({
            ...s,
            environments: s.environments.map((i) =>
              i.id === id ? { ...i, status: "error", error: failure } : i,
            ),
          }), log("启动被阻止", e.name, failure, "error"));
          if (!result.ok) { failDemoWrite(targets, index, "打开", result.error.message); break; }
          setOutcome({ id, name: e.name, action: "打开", state: "error", message: failure, created });
          done++;
          continue;
        }
        // A failed completion write leaves the saved starting state intact.
        // Retrying finishes that write instead of treating it as active work.
        if (e.status !== "starting") {
          const result = updateDemo((s) => ({
            ...s,
            environments: s.environments.map((i) =>
              i.id === id ? { ...i, status: "starting", error: undefined } : i,
            ),
          }));
          if (!result.ok) { failDemoWrite(targets, index, "打开", result.error.message); break; }
          setOutcome({ id, name: e.name, action: "打开", state: "accepted", message: "正在模拟打开", created });
          await sleep(650);
        }
        if (application.getSnapshot().state.environments.find((i) => i.id === id)?.status === "starting") {
          const result = updateDemo((s) => ({
            ...s,
            environments: s.environments.map((i) =>
              i.id === id && i.status === "starting"
                ? { ...i, status: "running", lastOpened: now() }
                : i,
            ),
          }), log("模拟启动", e.name, "演示状态已变为运行中；未启动真实浏览器进程。"));
          if (!result.ok) { failDemoWrite(targets, index, "打开", result.error.message); break; }
        }
        const latest = application.getSnapshot().state.environments.find(item => item.id === id);
        setOutcomes(previous => previous.map(item => item.id === id && item.action === "打开" ? { ...item, state: latest?.status === "running" ? "success" : "skipped", message: latest?.status === "running" ? "已模拟打开，未启动真实浏览器" : "打开已取消" } : item));
        done++;
      }
    } finally { batchBusy.current = false; setBatch(null); }
  }
  async function stop(ids: string[]) {
    if (nativeMode) {
      if (!application.stopRuntime) { notify("当前桌面版本未接入真实停止。", true); return; }
      for (const id of [...new Set(ids)]) {
        const name = current.current.environments.find(environment => environment.id === id)?.name ?? outcomesRef.current.find(item => item.id === id)?.name ?? "所选环境";
        if (application.getSnapshot().runtimeSessions?.[id]?.needsReconcile) { setOutcome({ id, name, action: "关闭", state: "error", message: "会话待核对，请使用行内核对会话；不会按PID结束进程" }); continue; }
        if (!beginRuntimeAction(id)) continue;
        setOutcome({ id, name, action: "关闭", state: "pending", message: "正在提交关闭" });
        try {
          const result = await application.stopRuntime({ environmentId: id, requestId: uid("request") });
          if (!result.ok) setOutcome({ id, name, action: "关闭", state: "error", message: result.error.message });
          else {
            setOutcome({ id, name, action: "关闭", state: "accepted", operationId: result.data.operation.id, message: "关闭任务已受理，等待资源释放" });
            applyRuntimeOperation(id, result.data.operation);
          }
        } finally { endRuntimeAction(id); }
      }
      setMenu(null);
      return;
    }
    batchCancelled.current = true;
    const targets = [...new Set(ids)].filter(id => !runtimeActions.current.has(id));
    targets.forEach(id => setOutcome({ id, name: current.current.environments.find(environment => environment.id === id)?.name ?? "所选环境", action: "关闭", state: "pending", message: "等待模拟关闭" }));
    for (const [index, id] of targets.entries()) {
      const e = application.getSnapshot().state.environments.find((i) => i.id === id);
      if (!e) { setOutcome({ id, name: "所选环境", action: "关闭", state: "error", message: "环境已不存在，请刷新列表" }); continue; }
      if (!["running", "starting", "stopping"].includes(e.status)) { setOutcome({ id, name: e.name, action: "关闭", state: "skipped", message: "已停止，无需重复关闭" }); continue; }
      if (!beginRuntimeAction(id)) continue;
      setOutcome({ id, name: e.name, action: "关闭", state: "accepted", message: "正在模拟关闭" });
      try {
        if (e.status !== "stopping") {
          const result = updateDemo((s) => ({
            ...s,
            environments: s.environments.map((i) =>
              i.id === id ? { ...i, status: "stopping" } : i,
            ),
          }));
          if (!result.ok) { failDemoWrite(targets, index, "关闭", result.error.message); break; }
          await sleep(350);
        }
        const result = updateDemo((s) => ({
          ...s,
          environments: s.environments.map((i) =>
            i.id === id ? { ...i, status: "ready" } : i,
          ),
        }), log("模拟关闭", e.name, "固定指纹和示例 Cookie 已保留。"));
        if (!result.ok) { failDemoWrite(targets, index, "关闭", result.error.message); break; }
        setOutcome({ id, name: e.name, action: "关闭", state: "success", message: "已模拟关闭，指纹与示例数据已保留" });
      } finally { endRuntimeAction(id); }
    }
  }
  async function handleRuntimeSessionAction(id: string, expectedSessionId: string, action: "force" | "reconcile") {
    if (environmentConfirmationRef.current) return;
    if (action === "force") {
      if (!eligibleRuntimeSession(id, expectedSessionId, action) || runtimeActions.current.has(id)) return;
      showEnvironmentConfirmation({ kind: "force", environmentId: id, sessionId: expectedSessionId, name: application.getSnapshot().state.environments.find(environment => environment.id === id)?.name ?? "所选环境" });
      return;
    }
    await submitRuntimeSessionAction(id, expectedSessionId, action);
  }
  function eligibleRuntimeSession(id: string, expectedSessionId: string, action: "force" | "reconcile") {
    if (!nativeMode) return false;
    const snapshot = application.getSnapshot();
    if (snapshot.issue) return false;
    const session = snapshot.runtimeSessions?.[id];
    const networkSession = snapshot.networkResources?.[id];
    if (action === "force" ? !session || session.sessionId !== expectedSessionId : networkSession !== expectedSessionId && session?.sessionId !== expectedSessionId) { notify("这条记录属于旧会话，未操作现在的浏览器；请重新读取状态。", true); return false; }
    if (action === "force") {
      if (!application.forceStopRuntime || !session?.canForce || session.needsReconcile) { notify("尚未满足指定会话强制结束条件。请先正常关闭；不会按PID结束进程。", true); return false; }
    } else if (!application.reconcileRuntime) { notify("当前桌面版本未提供会话核对。", true); return false; }
    return true;
  }
  async function submitRuntimeSessionAction(id: string, expectedSessionId: string, action: "force" | "reconcile") {
    if (!eligibleRuntimeSession(id, expectedSessionId, action)) return;
    if (!beginRuntimeAction(id)) return;
    try {
      const request = { environmentId: id, sessionId: expectedSessionId, requestId: uid("request") };
      const result = action === "force" ? await application.forceStopRuntime!(request) : await application.reconcileRuntime!(request);
      if (!result.ok) notify(result.error.message, true);
      else notify(action === "force" ? "指定会话结束任务已受理；确认本次Job全部退出后才显示已停止。" : "核对任务已受理；以实际进程身份和目录锁结果为准。");
    } finally { endRuntimeAction(id); }
  }
  async function cancelQueuedRuntime() {
    const queued = Object.values(application.getSnapshot().runtimeSessions ?? {}).filter(session => session.state === "starting" && session.launchStage === "queued");
    for (const session of queued) {
      const result = await application.cancelOperation(session.operationId);
      if (!result.ok) notify(result.error.message, true);
    }
    await application.refresh?.();
  }
  async function checkProxy(ids: string[]) {
    if (nativeMode) { notify("真实代理检查尚未接入，不会返回模拟检测结果。", true); return; }
    if (checkingBusy.current) return;
    checkingBusy.current = true;
    let checkFailed = false;
    setChecking(ids);
    for (const id of ids) {
      await sleep(350);
      const p = current.current.proxies.find((p) => p.id === id);
      if (!p) continue;
      const failed = Boolean(p.simulateFailure);
      if (!update((s) => ({
        ...s,
        proxies: s.proxies.map((i) =>
          i.id === id
            ? {
                ...i,
                status: failed ? "failed" : "connected",
                latency: failed ? undefined : 120,
              }
            : i,
        ),
      }), log(
        "模拟代理检查",
        p.name,
        failed
          ? "预设失败场景；没有发起实际网络请求。"
          : "预设成功场景；真实出口与可用性待桌面服务检测。",
        failed ? "error" : "info",
      ))) { checkFailed = true; break; }
    }
    checkingBusy.current = false;
    setChecking([]);
    if (!checkFailed) notify("模拟检查完成，结果不代表真实网络状态");
  }
  function newBackup(): boolean {
    if (nativeMode) { notify("完整本地备份尚未接入，不会创建原型 JSON 冒充备份。", true); return false; }
    const snapshot = createSnapshot(state);
    const name = `工作区快照 ${time(snapshot.createdAt)}`;
    if (!update((s) => ({
      ...s,
      backups: [
        { id: uid("backup"), name, createdAt: snapshot.createdAt, snapshot },
        ...s.backups,
      ],
    }), log(
      "创建原型快照",
      name,
      "仅包含原型配置和示例 Cookie，排除代理密码，不包含 Chromium 用户目录。",
    ))) return false;
    notify("原型快照已保存到当前浏览器");
    return true;
  }
  function confirmRestore() {
    if (nativeMode) { notify("原型快照不能恢复到真实工作区，原数据未修改。", true); return; }
    if (dialog?.kind !== "restore") return;
    setFormError("");
    try {
      const next = restoreSnapshot(state, dialog.snapshot);
      const result = updateDemo(() => next, log(
        "恢复原型快照",
        dialog.name,
        "配置已恢复；代理凭据需重新填写，所有代理需重新检查。",
      ));
      if (!result.ok) { setFormError(result.error.message); return; }
      setSelected([]);
      setDialog(null);
      notify("快照已恢复，代理需要重新检查");
    } catch (e) {
      setFormError((e as Error).message);
    }
  }
  function openCookies(e: Environment) {
    if (nativeMode) { setNativeCookieEnvironment(e); setMenu(null); return; }
    setDialog({ kind: "cookies", id: e.id });
    setCookieText("");
    setCookieResult(null);
    setFormError("");
    setMenu(null);
  }
  function saveDemoCookie() {
    if (dialog?.kind !== "cookies" || !cookieResult || !cookieResult.cookies.length || cookieResult.errors.length) return;
    if (!update((s) => ({
      ...s,
      environments: s.environments.map((e) => e.id === dialog.id
        ? { ...e, cookies: mergeCookies(e.cookies, cookieResult.cookies) } : e),
    }), log("导入示例 Cookie", state.environments.find((e) => e.id === dialog.id)?.name || "",
      `已将 ${cookieResult.cookies.length} 条 Cookie 保真写入原型记录；未写真实浏览器。`))) return;
    setDialog(null);
    notify(`已导入 ${cookieResult.cookies.length} 条示例 Cookie`);
  }
  function openProxyImport() {
    if (environmentConfirmationRef.current) return;
    if (drawerRef.current && !drawerSuspended) {
      proxyReturnFocus.current = document.activeElement as HTMLElement;
      setDraftProxyImportBusy(proxyImportSession.getSnapshot().busy);
      setDraftProxyImportOpen(true);
      return;
    }
    if (nativeMode) { setNativeProxyImportOpen(true); return; }
    navigate("proxies");
  }
  function closeDraftProxyImport() {
    if (proxyImportSession.getSnapshot().busy) return;
    proxyImportSession.setShowText(false);
    setDraftProxyImportOpen(false);
  }
  function openGroupAssignment(ids: string[], name = "") {
    setMenu(null); setGroupName(name); setFormError(""); setDialog({ kind: "group", ids: [...new Set(ids)] });
  }
  async function assignSelectedGroup(retryIds?: string[], retryName?: string) {
    const ids = retryIds ?? (dialog?.kind === "group" ? dialog.ids : []), name = (retryName ?? groupName).trim();
    if (groupPending || !name || !ids.length) return;
    setGroupPending(true); setFormError("");
    try {
      if (!nativeMode) {
        const result = updateDemo(s => ({ ...s, environments: s.environments.map(environment => ids.includes(environment.id) ? { ...environment, group: name } : environment) }), log("调整分组", `${ids.length} 个环境`, `已归入分组 ${name}。`));
        if (!result.ok) { setFormError(result.error.message); return; }
      } else {
        let failed = 0;
        for (const id of ids) {
          const preview = await application.previewEnvironment({ kind: "edit", sourceId: id });
          if (!preview.ok) { failed++; setOutcome({ id, name: current.current.environments.find(item => item.id === id)?.name ?? "所选环境", action: "分组", state: "error", message: preview.error.message, group: name }); continue; }
          const draft = preview.data;
          try {
            const result = await application.updateEnvironment({ previewId: draft.previewId, configuration: { ...draft.environment, group: name }, expectedRevision: draft.expectedRevision!, requestId: uid("request"), profileHash: draft.fingerprint?.previewProfile.configHash });
            if (!result.ok) failed++;
            setOutcome({ id, name: draft.environment.name, action: "分组", state: result.ok ? "success" : "error", message: result.ok ? `已归入 ${name}，指纹保持不变` : result.error.message, group: name });
          } finally { void application.discardPreview(draft.previewId); }
        }
        if (failed) { setDialog(null); notify(`${ids.length - failed} 项分组已保存，${failed} 项失败；查看逐项结果并重试。`, true); return; }
      }
      setDialog(null); notify(`已将 ${ids.length} 个环境归入 ${name}`);
    } finally { setGroupPending(false); }
  }
  function removeEnvironments() {
    if (dialog?.kind !== "delete") return;
    if (nativeMode) { setNativeRecycleSelection([...dialog.ids]); setDialog(null); return; }
    if (
      state.environments.some(
        (e) =>
          dialog.ids.includes(e.id) &&
          ["running", "starting", "stopping"].includes(e.status),
      )
    ) {
      setFormError("所选环境仍在运行，请先关闭。");
      return;
    }
    if (!update((s) => ({
      ...s,
      environments: s.environments.filter((e) => !dialog.ids.includes(e.id)),
    }), log(
      "删除环境",
      `${dialog.ids.length} 个环境`,
      "仅删除示例记录及示例 Cookie；不操作真实文件。",
    ))) return;
    setDialog(null);
    setSelected([]);
    notify("环境已从工作区移除");
  }
  const canConfigureProfileField = (field: string) => !profileBusy && (!nativeMode || !!selectedKernelRecord?.report.capabilities.some(capability => capability.field === field && capability.status === "configurable" && capability.source === "observed"));
  const outcomePanel = outcomes.length > 0 && <section className="environment-outcomes" aria-label="逐项操作结果">
    <div className="outcome-heading"><strong>操作结果 · {outcomes.length} 项</strong><span>{nativeMode ? "本机服务反馈" : "模拟操作，不启动真实浏览器"}</span><button className="text-button" disabled={outcomes.some(item => ["pending", "accepted"].includes(item.state))} onClick={() => setOutcomes([])}>收起结果</button></div>
    <ul>{outcomes.map(item => <li key={item.id} className={`outcome-${item.state}`} role={item.state === "error" ? "alert" : "status"}><span>{["pending", "accepted"].includes(item.state) ? <LoaderCircle size={14} className="spin" /> : item.state === "error" ? <TriangleAlert size={14} /> : <CheckCircle2 size={14} />}<strong>{item.name}</strong></span><span>{item.created && item.action === "打开" ? item.state === "error" ? "已创建，打开失败；" : "已创建；" : ""}{item.message}</span>{item.state === "error" && <button className="text-button" disabled={runtimeActionIds.includes(item.id) || groupPending} aria-label={`${item.name} 重试${item.action}`} onClick={() => item.action === "分组" ? void assignSelectedGroup([item.id], item.group) : item.action === "打开" ? void launch([item.id], !!item.created) : void stop([item.id])}>重试{item.action}</button>}</li>)}</ul>
  </section>;
  return (
    <div className="app-shell">
      <aside
        className="sidebar"
        inert={appInert} data-app-inert={String(appInert)}
      >
        <a className="brand" href="#/environments">
          <div className="brand-mark">
            <Fingerprint size={25} />
          </div>
          <div>
            <strong>棱镜浏览器</strong>
            <span>PRISM BROWSER</span>
          </div>
        </a>
        <nav aria-label="主导航">
          {(["environments", "groups", "proxies", "kernels", "backups", "activity", "guide"] as Route[]).map(
            (r) => {
              const Icon = routeInfo[r].icon;
              return (
                <a
                  key={r}
                  href={`#/${r}`}
                  className={`nav-item ${route === r ? "active" : ""}`}
                >
                  <Icon size={15} />
                  <span>{routeInfo[r].label}</span>
                </a>
              );
            },
          )}
        </nav>
      </aside>
      <div
        className="main-shell"
        inert={appInert} data-app-inert={String(appInert)}
      >
        <header className="topbar">
          <span className="topbar-local"><Monitor size={15} />本机工作区</span>
          <div className="topbar-right">
            <span className="prototype-label" title={nativeMode ? "本机服务接入；真实桌面验收范围不随页面测试扩大" : "仅供交互验收，不启动真实浏览器；请勿输入真实凭据"}>
              <span />
              {nativeMode ? "本机桌面" : "交互原型"}
            </span>
            <button
              className="icon-button"
              aria-label="查看操作记录"
              onClick={() => navigate("activity")}
            >
              <Bell size={19} />
              {errors > 0 && <i />}
            </button>
            <button
              className="icon-button"
              aria-label="使用说明与产品文档"
              onClick={() => navigate("guide")}
            >
              <CircleHelp size={19} />
            </button>
          </div>
        </header>
        <main className={route === "environments" ? "environment-workspace" : undefined}>
          {drawerSuspended && drawer && <div className="prototype-notice" role="status"><span>环境草稿已保留，安装内核不会清空正在填写的内容。</span><Button onClick={() => { const available = usableKernels.find(kernel => kernel.id === workspace.defaultKernel?.kernelId) ?? usableKernels[0]; if (drawer.kind === "create" && !canGenerateProfile && available) patchDraft({ coreId: available.id }); setDrawerSuspended(false); navigate("environments"); }}>继续环境草稿</Button><Button onClick={closeDrawer}>放弃草稿</Button></div>}
          {nativeMode && workspace.maintenance && <div className="prototype-notice" role="status">完整恢复正在维护保护中，配置修改与新启动暂不可用。<Button onClick={() => navigate("backups")}>查看恢复任务</Button></div>}
          {nativeMode && workspace.migrationMaintenance && <div className="prototype-notice" role="status">内核迁移维护中，原环境保持停止。<Button onClick={() => navigate("kernels")}>查看试用与迁移</Button></div>}
          {nativeMode && workspace.recycleMaintenance && <div className="prototype-notice" role="status">回收维护保护中，请核实原任务。<Button onClick={() => setNativeRecycleSelection([])}>打开回收区</Button></div>}
          {route === "environments" && (
            <>
              <div className="environment-list-panel">
                <EnvironmentFilters search={search} group={group} status={status} groups={groups} total={environmentTotal} running={running} errors={errors} selected={selected.length} pageSelected={pageItems.filter(item => selected.includes(item.id)).length} native={nativeMode} blocked={uiBlocked} searchRef={searchRef}
                  onSearch={value => { setSearch(value); setPage(1); }} onGroup={value => { setGroup(value); setPage(1); }} onStatus={value => { setStatus(value); setPage(1); }} onClear={() => { setSearch(""); setGroup("全部分组"); setStatus("all"); setPage(1); }}
                  onCreate={() => void openCreate()} onOpen={() => void launch([...selected])} onStop={() => void stop([...selected])} onAssign={() => openGroupAssignment(selected)} onCancelSelection={() => setSelected([])}
                  onRemove={() => { if (nativeMode) setNativeRecycleSelection([...selected]); else { setDialog({ kind: "delete", ids: [...selected] }); setFormError(""); } }}
                  onRefresh={() => { void application.refresh?.().then(result => { if (!result.ok) notify(result.error.message, true); else notify("环境列表已刷新"); }); }}
                  onBackup={() => { setNativeBackupSelection([...selected]); navigate("backups"); }} onHistory={() => setNativeBatchInput({ kind: "history" })} onRecycle={() => setNativeRecycleSelection([])} onClone={() => setNativeBatchInput({ kind: "clone", sourceIds: [...selected] })} onProxyAssign={() => setNativeBatchInput({ kind: "assign", sourceIds: [...selected] })}
                  onRetryReleased={() => void launch(selected.filter(id => { const session = workspace.runtimeSessions?.[id]; return session?.state === "error" && !session.pid && !session.resourcesPending && !session.needsReconcile && !session.persistencePending && !workspace.networkResources?.[id]; }))} />
                <section className="environment-table-panel" aria-label="环境列表">
                {nativeMode && Object.values(workspace.runtimeSessions ?? {}).some(session => session.state === "starting") && <div className="selection-bar" role="status">
                  <span>启动队列：{Object.values(workspace.runtimeSessions ?? {}).filter(session => session.state === "starting" && session.launchStage === "queued").length} 项等待，按受理顺序启动；已运行环境不占队列名额。</span>
                  <Button onClick={() => void cancelQueuedRuntime()}>取消排队启动</Button>
                </div>}
                {outcomePanel}
                <div className="environment-table-scroll">
                  <table className="environment-table">
                    <thead>
                      <tr>
                        <th className="check-cell">
                          <input
                            type="checkbox"
                            aria-label="选择当前页全部环境"
                            ref={element => { if (element) element.indeterminate = pageItems.some(item => selected.includes(item.id)) && !pageItems.every(item => selected.includes(item.id)); }}
                            disabled={nativeMode && environmentQueryBusy}
                            checked={
                              pageItems.length > 0 &&
                              pageItems.every((e) => selected.includes(e.id))
                            }
                            onChange={(e) =>
                              setSelected(
                                e.target.checked
                                  ? [
                                      ...new Set([
                                        ...selected,
                                        ...pageItems.map((i) => i.id),
                                      ]),
                                    ]
                                  : selected.filter(
                                      (id) =>
                                        !pageItems.some((i) => i.id === id),
                                    ),
                              )
                            }
                          />
                        </th>
                         <th>序号</th>
                         <th>分组</th>
                         <th>环境名称</th>
                         <th>直连 / 代理</th>
                         <th>内核 / 状态</th>
                         <th>创建时间</th>
                         <th>打开</th>
                      </tr>
                    </thead>
                    <tbody>
                      {pageItems.map((e) => {
                        const p = state.proxies.find((p) => p.id === e.proxyId);
                        const runtimeSession = nativeMode ? workspace.runtimeSessions?.[e.id] : undefined;
                        const runtimeActionPending = runtimeActionIds.includes(e.id);
                        const demoWriteRetry = !nativeMode && !runtimeActionPending && (e.status === "stopping" || e.status === "starting" && !batch);
                        const core = state.kernels.find(
                          (k) => k.id === e.coreId,
                        );
                        return (
                          <tr
                            key={e.id}
                            className={
                              selected.includes(e.id) ? "row-selected" : ""
                            }
                          >
                            <td className="check-cell">
                              <input
                                type="checkbox"
                                aria-label={`选择 ${e.name}`}
                                disabled={nativeMode && environmentQueryBusy}
                                checked={selected.includes(e.id)}
                                onChange={(event) =>
                                  setSelected(
                                    event.target.checked
                                      ? [...selected, e.id]
                                      : selected.filter((id) => id !== e.id),
                                  )
                                }
                              />
                            </td>
                            <td className="environment-code">{e.code}<Fingerprint size={12} /></td>
                            <td className="environment-group-cell" title={e.group || "未分组"}>{e.group || "未分组"}</td>
                            <td><button className="name-button" title={e.name} onClick={() => void openEdit(e)}>{e.name}</button></td>
                            <td>
                              {p ? (
                                <>
                                  <div className="proxy-title">
                                    <span className="proxy-type-mark" title={p.type.toUpperCase()}>{p.type === "socks5" ? "S" : p.type === "https" ? "H" : "P"}</span>
                                    <span title={`${p.type}://${p.host}:${p.port}`}>{p.host}<small>{p.country || "未检测"}</small></span>
                                  </div>
                                </>
                              ) : e.proxyId ? (
                                <span className="proxy-title danger-text">已绑定代理不可用<br />不会改为直连</span>
                              ) : (
                                <>
                                  <span className="proxy-title">
                                    <Globe2 size={15} />
                                    本机直连
                                  </span>
                                </>
                               )}
                             </td>
                             <td>
                               <span className="device-line">
                                <Monitor size={14} />
                                  {core?.version || "不可用"}
                               </span>
                                <div className="environment-status-line"><span
                                className={`status ${e.status}`}
                                title={e.error}
                              >
                                {["starting", "stopping"].includes(e.status) && !demoWriteRetry ? (
                                  <LoaderCircle size={12} className="spin" />
                                ) : (
                                  <span className="status-dot" />
                                )}
                                 {demoWriteRetry ? "模拟结果待保存" : runtimeSession?.needsReconcile ? "待核对" : nativeMode && !core?.available ? "未就绪" : statusLabels[e.status]}
                                </span><EnvironmentRuntimeDetails environment={e} session={runtimeSession} blocked={uiBlocked} /></div>
                            </td>
                            <td>
                              <span className="last-open">
                                {environmentTime(e.createdAt)}
                              </span>
                            </td>
                            <td>
                              <div className="row-actions">
                                {demoWriteRetry ? (
                                  <Button className="soft-primary compact" onClick={() => e.status === "starting" ? void launch([e.id]) : void stop([e.id])}>重试{e.status === "starting" ? "打开" : "关闭"}</Button>
                                ) : runtimeSession?.persistencePending ? (
                                  <span className="cell-secondary runtime-recovery-note" role="status">结果待保存 · 修复存储后自动核对</span>
                                ) : workspace.networkResources?.[e.id] ? (
                                  <Button className="soft-primary compact" disabled={runtimeActionPending} onClick={() => void handleRuntimeSessionAction(e.id, workspace.networkResources![e.id], "reconcile")}>重试资源清理</Button>
                                ) : runtimeSession?.needsReconcile ? (
                                  <Button className="soft-primary compact" disabled={runtimeActionPending} onClick={() => void handleRuntimeSessionAction(e.id, runtimeSession.sessionId, "reconcile")}>核对会话</Button>
                                 ) : runtimeSession?.canForce ? (
                                   <>
                                     <Button className="stop-button compact" disabled={runtimeActionPending || e.status === "stopping"} onClick={() => void stop([e.id])}>重试关闭</Button>
                                     <Button className="danger compact" disabled={runtimeActionPending || e.status === "stopping"} onClick={() => void handleRuntimeSessionAction(e.id, runtimeSession.sessionId, "force")}>强制结束</Button>
                                   </>
                                ) : e.status === "running" || (nativeMode && (e.status === "starting" || !!runtimeSession?.pid || runtimeSession?.resourcesPending)) ? (
                                  <Button
                                      className="stop-button compact"
                                      aria-label={e.status === "starting" ? "取消启动" : runtimeSession?.resourcesPending && e.status === "error" ? "重试关闭" : "关闭"}
                                      title="关闭此环境，保留身份和浏览数据"
                                     disabled={e.status === "stopping" || runtimeActionPending}
                                    onClick={() => stop([e.id])}
                                  >
                                     <Square size={12} />
                                     {e.status === "starting" ? "取消启动" : runtimeSession?.resourcesPending && e.status === "error" ? "重试关闭" : "已打开"}
                                  </Button>
                                ) : (
                                  <Button
                                    className="launch-button compact"
                                    disabled={runtimeActionPending || ["starting", "stopping"].includes(
                                      e.status,
                                    )}
                                    onClick={() => launch([e.id])}
                                  >
                                    {["starting", "stopping"].includes(
                                      e.status,
                                    ) ? (
                                      <LoaderCircle
                                        size={13}
                                        className="spin"
                                      />
                                    ) : (
                                      <Monitor size={13} />
                                    )}
                                    {runtimeActionPending ? "处理中" : e.status === "starting" ? "打开中" : e.status === "stopping" ? "关闭中" : "打开"}
                                  </Button>
                                )}
                                <div className="menu-wrap">
                                  <button
                                    className="icon-button"
                                    aria-label={`${e.name} 更多操作`}
                                    aria-expanded={menu === e.id}
                                    onClick={event => { setMenuAnchor(event.currentTarget); setMenu(menu === e.id ? null : e.id); }}
                                  >
                                    <EllipsisVertical size={14} />
                                  </button>
                                  {menu === e.id && menuAnchor && <ReferencePopover anchor={menuAnchor} label={`${e.name} 操作菜单`} className="environment-row-menu" onClose={() => setMenu(null)}>
                                      <button onClick={() => openEdit(e)}>
                                        <Settings2 size={15} />
                                        编辑环境
                                      </button>
                                      <button onClick={() => { setMenu(null); if (nativeMode) setNativeBatchInput({ kind: "clone", sourceIds: [e.id] }); else void openCreate(e); }}>
                                        <Copy size={15} />
                                        按模板新建
                                      </button>
                                      <button onClick={() => openCookies(e)}>
                                        <FileJson size={15} />
                                        导入 Cookie
                                      </button>
                                      <button onClick={() => openGroupAssignment([e.id], e.group)}><Folder size={15} />调整分组</button>
                                      {nativeMode && <><button onClick={() => { setMenu(null); setNativeBatchInput({ kind: "assign", sourceIds: [e.id] }); }}><Network size={15} />明确分配代理</button><button onClick={() => { setMenu(null); setNativeBackupSelection([e.id]); navigate("backups"); }}><HardDrive size={15} />完整备份此环境</button></>}
                                      <button
                                        className="danger-text"
                                        onClick={() => {
                                          if (nativeMode) { setNativeRecycleSelection([e.id]); setMenu(null); return; }
                                          setDialog({
                                            kind: "delete",
                                            ids: [e.id],
                                          });
                                          setFormError("");
                                          setMenu(null);
                                        }}
                                      >
                                        <Trash2 size={15} />
                                        移除环境
                                      </button>
                                  </ReferencePopover>}
                                </div>
                              </div>
                            </td>
                          </tr>
                        );
                      })}
                      {visible.length === 0 && <tr className="environment-empty-row"><td colSpan={8}><Empty title={environmentTotal === 0 ? "还没有浏览器环境" : "没有符合条件的环境"} text={environmentTotal === 0 ? "新建一个环境，选择网络与内核即可开始。" : "试试其他关键词或清除筛选，已保存环境没有被删除。"} action={environmentTotal === 0 ? <Button className="primary" onClick={() => void openCreate()}>创建第一个环境</Button> : <Button onClick={() => { setSearch(""); setGroup("全部分组"); setStatus("all"); setPage(1); }}>清除筛选</Button>} /></td></tr>}
                    </tbody>
                  </table>
                </div>
                <div className="table-footer environment-pagination">
                  <span>
                    共 {filteredTotal} 个环境{nativeMode && environmentQueryBusy ? " · 正在读取分页…" : ""}
                    <span className="footer-separator">·</span>每页 10 条
                  </span>
                  <div className="pagination">
                    <button
                      className="icon-button"
                      aria-label="上一页"
                      disabled={page <= 1 || environmentQueryBusy}
                      onClick={() => setPage((p) => p - 1)}
                    >
                      <ChevronLeft size={16} />
                    </button>
                    <span>{Math.min(page, pageCount)}</span>
                    <span className="subtle-text">/ {pageCount}</span>
                    <button
                      className="icon-button"
                      aria-label="下一页"
                      disabled={page >= pageCount || environmentQueryBusy}
                      onClick={() => setPage((p) => p + 1)}
                    >
                      <ChevronRight size={16} />
                    </button>
                  </div>
                </div>
                </section>
              </div>
            </>
          )}
          {route === "groups" && <>{outcomePanel}<EnvironmentGroups groups={groups} environments={state.environments} native={nativeMode} onFilter={value => { setGroup(value); setStatus("all"); navigate("environments"); }} onAssign={openGroupAssignment} /></>}
          {route === "proxies" && nativeMode && <NativeProxyManager application={application} workspace={workspace}
            importOpen={nativeProxyImportOpen} onImportOpenChange={setNativeProxyImportOpen}
            onAssign={ids => {
              if (ids?.length) setNativeBatchInput({ kind: "assign", sourceIds: [...new Set(ids)] });
              else { navigate("environments"); notify("请先明确选择要分配代理的环境，再使用“批量分配代理”。"); }
            }} />}
          {route === "proxies" && !nativeMode && <DemoProxyManager application={application} workspace={workspace} checking={checking} onCheck={checkProxy} />}
          {route === "kernels" && nativeMode && <><NativeKernelManager application={application} workspace={workspace} onMigration={() => setMigrationOpen(true)} /><NativeMigrationManager application={application} workspace={workspace} open={migrationOpen} onOpenChange={setMigrationOpen} /></>}
          {route === "kernels" && !nativeMode && <DemoKernelManager workspace={workspace} />}
          {route === "backups" && (
            <BackupManagementPage key={nativeMode ? `native:${nativeBackupSelection.join(",")}` : "demo"}
              application={application} workspace={workspace} selectedIds={nativeBackupSelection}
              demo={{ state, disabled: Boolean(storageIssue), onCreate: newBackup,
                onRestore: (snapshot, name) => { setFormError(""); setDialog({ kind: "restore", snapshot, name }); },
                onDownload: backup => download(`prism-snapshot-${backup.id}.json`, JSON.stringify(backup.snapshot, null, 2)) }} />
          )}
          {route === "activity" && (
            <ActivityPage activities={state.activities} native={nativeMode} runtimeSessions={workspace.runtimeSessions}
              blockedIds={runtimeActionIds} diagnostics={nativeMode ? <NativeDiagnostics application={application} /> : undefined}
              onExport={nativeMode ? undefined : () => download("prism-activity.json", JSON.stringify(state.activities, null, 2))}
              onSessionAction={(environmentId, sessionId, action) => void handleRuntimeSessionAction(environmentId, sessionId, action)} />
          )}
          {route === "guide" && (
            <HelpPage native={nativeMode} docTab={docTab}
              text={docTab === "user" ? userGuideText : docTab === "prd" ? prdText : docTab === "kernel" ? kernelText : developmentText}
              onDocTab={setDocTab} onNavigate={navigate} download={download} docHref={docHref} />
          )}
        </main>
      </div>
      {drawerVisible && drawer && (
        <div
          className="overlay environment-overlay"
          inert={Boolean(legacyDialog || draftProxyImportOpen || environmentConfirmation || storageIssue)}
          data-app-inert={String(Boolean(legacyDialog || draftProxyImportOpen || environmentConfirmation || storageIssue))}
        >
          <EnvironmentEditorWindow dialogRef={drawerOverlayRef} error={formError}
            saving={savePending} canSave={canSaveProfile}
            recoveringCreation={!!drawer.creationOperationId || !!drawer.creationUnconfirmed}
            onClose={closeDrawer} onSave={open => void saveEnvironment(open)}
            form={{ environment: drawer.environment, kind: drawer.kind, groups,
              kernels: usableKernels, proxies: state.proxies, native: nativeMode,
              busy: savePending || !!drawer.creationOperationId || !!drawer.creationUnconfirmed,
              generating, profileBusy,
              kernelLocked: drawer.kind === "edit" && (!nativeMode || state.environments.find(e => e.id === drawer.environment.id)?.coreId !== "kernel-pending"),
              canGenerate: canGenerateProfile, fresh: profileIsFresh, preview: drawer.fingerprint,
              history: drawer.history, dataRef: drawer.userDataRef, quantity, previewError,
              onChange: patchDraft, onQuantity: value => { if (!environmentConfirmationRef.current) setQuantity(value); },
              onGenerate: regenerate => void generateProfile(regenerate),
              onRestore: revision => void previewProfileRestore(revision),
              onImportProxy: openProxyImport, canConfigure: canConfigureProfileField,
              onKernels: () => { setDrawerSuspended(true); navigate("kernels"); } }} />
        </div>
      )}
      {nativeMode && nativeCookieEnvironment && <NativeCookieImport key={nativeCookieEnvironment.id} application={application} workspace={workspace} environment={nativeCookieEnvironment} onClose={() => setNativeCookieEnvironment(null)} />}
      {nativeMode && nativeRecycleSelection && <NativeRecycleManager application={application} workspace={workspace} selectedIds={nativeRecycleSelection} onClose={() => { setNativeRecycleSelection(null); setSelected([]); void application.refresh?.(); }} />}
      {nativeMode && nativeBatchInput && <NativeBatchDialog key={`${nativeBatchInput.kind}:${nativeBatchInput.initialPage?.planId ?? nativeBatchInput.sourceIds?.join(",") ?? "history"}`} application={application} workspace={workspace} input={nativeBatchInput} onClose={() => { setNativeBatchInput(null); setMenu(null); }} />}
      {draftProxyImportOpen && <div className="overlay modal-overlay stacked-overlay" ref={draftProxyOverlayRef} tabIndex={-1} inert={Boolean(environmentConfirmation || storageIssue)} data-app-inert={String(Boolean(environmentConfirmation || storageIssue))}>
        <ProxyImportWindow application={application} session={proxyImportSession} open lifecycle="parent" context="environment"
          onClose={closeDraftProxyImport} onBusyChange={setDraftProxyImportBusy}
          onImported={ids => { if (ids[0]) patchDraft({ proxyId: ids[0] }); notify(`已导入 ${ids.length} 条代理，环境草稿已保留`); }} />
      </div>}
      {!nativeMode && dialog?.kind === "restore" && <DemoRestoreWindow snapshot={dialog.snapshot} name={dialog.name}
        runningCount={state.environments.filter(environment => ["running", "starting", "stopping"].includes(environment.status)).length}
        error={formError} onClose={() => setDialog(null)} onConfirm={confirmRestore} />}
      {legacyDialog?.kind === "cookies" && <div className={`overlay modal-overlay ${drawerVisible ? "stacked-overlay" : ""}`} inert={Boolean(environmentConfirmation || storageIssue)} data-app-inert={String(Boolean(environmentConfirmation || storageIssue))}>
        <DemoCookieImportWindow environment={state.environments.find(e => e.id === legacyDialog.id)!}
          text={cookieText} result={cookieResult} error={formError} dialogRef={overlayRef}
          onText={text => { setCookieText(text); setCookieResult(null); }}
          onParse={() => setCookieResult(parseCookies(cookieText))}
          onClose={() => setDialog(null)} onSave={saveDemoCookie} />
      </div>}
      {legacyDialog?.kind === "delete" && <div className="overlay modal-overlay" inert={Boolean(environmentConfirmation || storageIssue)} data-app-inert={String(Boolean(environmentConfirmation || storageIssue))}>
        <DemoEnvironmentRemoveWindow count={legacyDialog.ids.length} error={formError}
          dialogRef={overlayRef} onClose={() => setDialog(null)} onRemove={removeEnvironments} />
      </div>}
      {legacyDialog?.kind === "group" && (
        <div className={`overlay modal-overlay ${drawerVisible ? "stacked-overlay" : ""}`} inert={Boolean(environmentConfirmation || storageIssue)} data-app-inert={String(Boolean(environmentConfirmation || storageIssue))}>
          <div
            className="modal group-modal"
            ref={overlayRef}
            tabIndex={-1}
            role="dialog"
            aria-modal="true"
            aria-labelledby="modal-title"
          >
            <div className="modal-header">
              <h2 id="modal-title">调整环境分组</h2>
              <button
                className="icon-button"
                aria-label="关闭对话框"
                disabled={groupPending}
                onClick={() => setDialog(null)}
              >
                <X size={20} />
              </button>
            </div>
            <div className="modal-body">
              <Field label="分组名称">
                <input
                  aria-label="分组名称"
                  value={groupName}
                  disabled={groupPending}
                  onChange={(e) => setGroupName(e.target.value)}
                  placeholder="例如：新品测试"
                  list="existing-groups"
                />
                <datalist id="existing-groups">
                  {groups.map((g) => (
                    <option key={g}>{g}</option>
                  ))}
                </datalist>
              </Field>
              <p className="group-scope-note">将明确选定的 <strong>{legacyDialog.ids.length}</strong> 个环境归入此名称。<br />筛选与翻页不会扩大这次范围。</p>
              <p>这是环境记录的分组标签，不创建独立空分组。普通分组修改保留 seed、内核、代理与浏览数据。</p>
              {formError && (
                <div className="form-error" role="alert">
                  <TriangleAlert size={16} />
                  {formError}
                </div>
              )}
            </div>
            <div className="modal-footer">
              <Button disabled={groupPending} onClick={() => setDialog(null)}>取消</Button>
              <Button
                className="primary"
                disabled={groupPending || !legacyDialog.ids.length || !groupName.trim()}
                onClick={() => void assignSelectedGroup()}
              >
                应用到所选环境
              </Button>
            </div>
          </div>
        </div>
      )}
      {batch && (
        <div className="batch-progress" role="status">
          <LoaderCircle size={18} className="spin" />
          <span>
            {batch.label} · {batch.done} / {batch.total}
          </span>
          <Button
            onClick={async () => {
              if (creationOperation.current) {
                const result = await application.cancelOperation(creationOperation.current);
                if (!result.ok) { notify(result.error.message, true); return; }
              } else batchCancelled.current = true;
              notify("已请求取消，当前项目结束后停止余下任务。");
            }}
          >
            取消余下任务
          </Button>
        </div>
      )}
      {environmentConfirmation && <div className="overlay modal-overlay environment-confirmation-overlay" style={{ zIndex: confirmationLayer }} inert={Boolean(storageIssue)} data-app-inert={String(Boolean(storageIssue))}>
        {environmentConfirmation.kind === "dirty" ? <EnvironmentConfirmation title="放弃未保存修改？" titleId="environment-dirty-title"
          dialogRef={confirmationOverlayRef} onCancel={cancelEnvironmentConfirmation} onConfirm={confirmEnvironmentAction} confirmLabel="放弃修改">
          <p>本次编辑未保存，放弃后已保存身份与数据保持不变。</p>
          <p>取消可继续编辑当前草稿。</p>
        </EnvironmentConfirmation> : <EnvironmentConfirmation title="强制结束指定会话？" titleId="environment-force-title"
          dialogRef={confirmationOverlayRef} onCancel={cancelEnvironmentConfirmation} onConfirm={confirmEnvironmentAction} confirmLabel="确认强制结束" danger>
          <p>仅强制结束这份已确认会话，可能丢失尚未保存的网页内容。不会结束其他环境，也不会清空浏览数据。</p>
          <p>{environmentConfirmation.name}<br />环境 ID：{environmentConfirmation.environmentId}<br />会话 ID：{environmentConfirmation.sessionId}</p>
        </EnvironmentConfirmation>}
      </div>}
      {storageIssue && (
        <div className="overlay modal-overlay workspace-blocker">
          <div
            className="modal"
            ref={workspaceOverlayRef}
            tabIndex={-1}
            role="alertdialog"
            aria-modal="true"
            aria-label="工作区需要处理"
          >
            <div className="modal-header">
              <h2>工作区需要处理</h2>
            </div>
            <div className="modal-body">
              <p>{storageIssue}</p>
              {nativeMode && workspace.issue?.code !== "WORKSPACE_LOADING" && <NativeDiagnostics application={application} />}
            </div>
            <div className="modal-footer">
              {workspace.damagedRecord !== undefined ? (
                <>
                  <Button
                    onClick={() =>
                      download(
                        "prism-original-record.txt",
                        workspace.damagedRecord || "",
                        "text/plain",
                      )
                    }
                  >
                    导出原始记录
                  </Button>
                  <Button
                    className="primary"
                    onClick={async () => {
                      const result = await application.compatibility?.resetDamagedWorkspace();
                      if (result?.ok) location.reload();
                      else if (result) notify(result.error.message, true);
                    }}
                  >
                    重置演示工作区
                  </Button>
                </>
              ) : (
                <Button className="primary" disabled={workspace.issue?.code === "WORKSPACE_LOADING"} onClick={() => nativeMode ? void application.refresh?.() : location.reload()}>
                  {nativeMode ? "重新读取本机工作区" : "重新载入"}
                </Button>
              )}
            </div>
          </div>
        </div>
      )}
      {toast && (
        <div
          className={`toast ${toast.error ? "toast-error" : ""}`}
          role="status"
        >
          {toast.error ? (
            <TriangleAlert size={18} />
          ) : (
            <CheckCircle2 size={18} />
          )}
          <span>{toast.text}</span>
          <button
            className="icon-button"
            aria-label="关闭提示"
            onClick={() => setToast(null)}
          >
            <X size={15} />
          </button>
        </div>
      )}
    </div>
  );
}
