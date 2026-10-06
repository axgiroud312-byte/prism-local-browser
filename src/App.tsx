import {
  useEffect,
  useRef,
  useState,
  lazy,
  Suspense,
  useSyncExternalStore,
  type ReactNode,
} from "react";
import {
  Activity as ActivityIcon,
  ArrowDownToLine,
  ArrowUpFromLine,
  ArrowUpRight,
  Bell,
  BookOpen,
  Box,
  Check,
  CheckCheck,
  CheckCircle2,
  ChevronDown,
  ChevronLeft,
  ChevronRight,
  CircleHelp,
  Copy,
  Download,
  EllipsisVertical,
  ExternalLink,
  FileJson,
  Fingerprint,
  Folder,
  FolderPlus,
  Globe2,
  HardDrive,
  History,
  Info,
  Layers3,
  LayoutGrid,
  Link2,
  ListFilter,
  LoaderCircle,
  LockKeyhole,
  Monitor,
  Network,
  Plus,
  RefreshCw,
  Search,
  Settings2,
  ShieldCheck,
  SlidersHorizontal,
  Square,
  Terminal,
  Trash2,
  TriangleAlert,
  Wifi,
  X,
} from "lucide-react";
import {
  createSnapshot,
  launchError,
  mergeCookies,
  now,
  parseCookies,
  parseProxyText,
  parseSnapshot,
  regions,
  restoreSnapshot,
  uid,
  type Environment,
  type Activity,
  type Kernel,
  type ProxyNode,
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
const Markdown = lazy(() => import("react-markdown"));
import remarkGfm from "remark-gfm";
import { NativeKernelManager } from "./components/NativeKernelManager";
import { NativeMigrationManager } from "./components/NativeMigrationManager";
import { NativeProxyManager } from "./components/NativeProxyManager";
import { NativeCookieImport } from "./components/NativeCookieImport";
import { NativeBatchDialog, type NativeBatchDialogInput } from "./components/NativeBatchDialog";
import { readRuntimeStartPlan } from "./application/runtime-start-plan";
import { NativeBackupManager } from "./components/NativeBackupManager";
import { NativeRecycleManager } from "./components/NativeRecycleManager";
import { NativeDiagnostics } from "./components/NativeDiagnostics";
import { EnvironmentEditorWindow } from "./components/EnvironmentEditorWindow";
import { DemoCookieImportWindow, DemoEnvironmentRemoveWindow } from "./components/DemoEnvironmentWindows";
import { EnvironmentConfirmation } from "./components/EnvironmentDialogParts";
import { EnvironmentFilters } from "./components/EnvironmentFilters";
import { EnvironmentGroups } from "./components/EnvironmentGroups";
import { ReferencePopover } from "./components/ReferenceUi";
import { EnvironmentRuntimeDetails } from "./components/EnvironmentRuntimeDetails";

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
  | { kind: "proxy" }
  | { kind: "proxy-edit"; proxy: ProxyNode }
  | { kind: "restore"; snapshot: Snapshot; name: string }
  | { kind: "kernel"; core: Kernel }
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
function Tag({ children, kind = "" }: { children: ReactNode; kind?: string }) {
  return <span className={`tag ${kind}`}>{children}</span>;
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

function topModalElement() {
  const modals = [...document.querySelectorAll<HTMLElement>('[aria-modal="true"]')]
    .filter(element => element.getClientRects().length > 0 && !element.closest("[inert]"));
  const layer = (element: HTMLElement) => {
    let highest = 0;
    for (let node: HTMLElement | null = element; node; node = node.parentElement) {
      const zIndex = Number(getComputedStyle(node).zIndex);
      if (Number.isFinite(zIndex)) highest = Math.max(highest, zIndex);
    }
    return highest;
  };
  return modals.reduce<HTMLElement | null>((top, candidate) => !top || layer(candidate) >= layer(top) ? candidate : top, null);
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
  const [environmentConfirmation, setEnvironmentConfirmation] = useState<PendingEnvironmentConfirmation | null>(null);
  const environmentConfirmationRef = useRef<PendingEnvironmentConfirmation | null>(null);
  const [nativeProxyImportOpen, setNativeProxyImportOpen] = useState(false);
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
  const [proxyText, setProxyText] = useState("");
  const [proxyRows, setProxyRows] = useState<ReturnType<typeof parseProxyText>>(
    [],
  );
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
  const backupFile = useRef<HTMLInputElement>(null);
  const proxyFile = useRef<HTMLInputElement>(null);
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
      if (eligible) target.focus();
      else if (fallback?.isConnected && !fallback.closest("[inert]") && !fallback.matches(":disabled")) fallback.focus();
    }, 0);
    return () => clearTimeout(timer);
  }, [Boolean(environmentConfirmation)]);
  useEffect(() => {
    if (drawerVisible || drawer || environmentConfirmation) return;
    const target = drawerReturnFocus.current;
    if (!target) return;
    const timer = setTimeout(() => { if (target.isConnected && !target.closest("[inert]") && !target.matches(":disabled")) target.focus(); }, 0);
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
      if (top && ![drawerOverlayRef.current, overlayRef.current, draftProxyOverlayRef.current, confirmationOverlayRef.current].includes(top as HTMLDivElement)) {
        if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "k") e.preventDefault();
        return;
      }
      if (environmentConfirmationRef.current) return; // Capture listener owns warning keys, not the lower form/manager.
      if (nativeBatchInput || nativeCookieEnvironment || nativeRecycleSelection) {
        if ((e.ctrlKey || e.metaKey) && e.key === "k") e.preventDefault();
        return;
      }
      if (draftProxyImportOpen || dialog) {
        if ((e.ctrlKey || e.metaKey) && e.key === "k") e.preventDefault();
        if (e.key === "Escape" && !draftProxyImportBusy && !groupPending) {
          e.preventDefault();
          if (draftProxyImportOpen) setDraftProxyImportOpen(false);
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
  }, [generating, drawer, drawerVisible, dialog, environmentConfirmation, draftProxyImportOpen, draftProxyImportBusy, groupPending, nativeBatchInput, nativeCookieEnvironment, nativeRecycleSelection, storageIssue]);
  useEffect(() => {
    if (dialog?.kind === "proxy" || draftProxyImportOpen || !drawerVisible || !proxyReturnFocus.current) return;
    const timer = setTimeout(() => proxyReturnFocus.current?.focus(), 40);
    return () => clearTimeout(timer);
  }, [dialog?.kind, draftProxyImportOpen, drawerVisible]);
  useEffect(() => {
    if (!formError) return;
    const overlay = dialog ? overlayRef.current : drawerVisible ? drawerOverlayRef.current : null;
    overlay?.querySelector('[role="alert"]')?.scrollIntoView({ block: "nearest" });
  }, [formError, Boolean(dialog), drawerVisible]);
  useEffect(() => {
    if (!drawerVisible && !dialog && !environmentConfirmation && !draftProxyImportOpen && !storageIssue) return;
    const oldOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    const previous = document.activeElement as HTMLElement;
    const topOverlay = () => storageIssue ? workspaceOverlayRef.current : environmentConfirmationRef.current ? confirmationOverlayRef.current : draftProxyImportOpen ? draftProxyOverlayRef.current : dialog ? overlayRef.current : drawerOverlayRef.current;
    const focusable = (overlay: HTMLElement) => [...overlay.querySelectorAll<HTMLElement>(
      'button:not(:disabled), input:not(:disabled), select:not(:disabled), textarea:not(:disabled), a[href], summary, [tabindex="0"]',
    )].filter(element => element.getClientRects().length > 0 && !element.closest("[inert]"));
    const focusFirst = () => {
      const overlay = topOverlay();
      if (overlay && topModalElement() === overlay) (focusable(overlay)[0] ?? overlay).focus();
    };
    const timer = setTimeout(focusFirst, 30);
    const trap = (e: KeyboardEvent) => {
      const overlay = topOverlay();
      if (e.defaultPrevented || !overlay || topModalElement() !== overlay) return;
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
        overlay.focus();
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
      if (overlay && topModalElement() === overlay && !overlay.contains(event.target as Node)) focusFirst();
    };
    document.addEventListener("keydown", trap, true);
    document.addEventListener("focusin", repairFocus);
    return () => {
      document.body.style.overflow = oldOverflow;
      clearTimeout(timer);
      document.removeEventListener("keydown", trap, true);
      document.removeEventListener("focusin", repairFocus);
      if (previous?.isConnected && !previous.closest("[inert]")) previous.focus();
      else if (menuAnchor?.isConnected && !menuAnchor.closest("[inert]")) menuAnchor.focus();
    };
  }, [drawerVisible, Boolean(dialog), Boolean(environmentConfirmation), draftProxyImportOpen, Boolean(storageIssue)]);
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
      if (!application.forceStopRuntime || !session?.canForce || !session.canControl || session.needsReconcile) { notify("尚未满足指定会话强制结束条件。请先正常关闭；不会按PID结束进程。", true); return false; }
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
  function newBackup() {
    if (nativeMode) { notify("完整本地备份尚未接入，不会创建原型 JSON 冒充备份。", true); return; }
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
    ))) return;
    notify("原型快照已保存到当前浏览器");
  }
  function confirmRestore() {
    if (nativeMode) { notify("原型快照不能恢复到真实工作区，原数据未修改。", true); return; }
    if (dialog?.kind !== "restore") return;
    try {
      const next = restoreSnapshot(state, dialog.snapshot);
      if (!update(() => next, log(
        "恢复原型快照",
        dialog.name,
        "配置已恢复；代理凭据需重新填写，所有代理需重新检查。",
      ))) return;
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
      if (nativeMode) { setDraftProxyImportBusy(false); setDraftProxyImportOpen(true); return; }
    }
    if (nativeMode) { setNativeProxyImportOpen(true); return; }
    setDialog({ kind: "proxy" });
    setProxyText("");
    setProxyRows([]);
    setFormError("");
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
  const summary = routeInfo[route];
  const canConfigureProfileField = (field: string) => !profileBusy && (!nativeMode || !!selectedKernelRecord?.report.capabilities.some(capability => capability.field === field && capability.status === "configurable" && capability.source === "observed"));
  const outcomePanel = outcomes.length > 0 && <section className="environment-outcomes" aria-label="逐项操作结果">
    <div className="outcome-heading"><strong>操作结果 · {outcomes.length} 项</strong><span>{nativeMode ? "本机服务反馈" : "模拟操作，不启动真实浏览器"}</span><button className="text-button" disabled={outcomes.some(item => ["pending", "accepted"].includes(item.state))} onClick={() => setOutcomes([])}>收起结果</button></div>
    <ul>{outcomes.map(item => <li key={item.id} className={`outcome-${item.state}`} role={item.state === "error" ? "alert" : "status"}><span>{["pending", "accepted"].includes(item.state) ? <LoaderCircle size={14} className="spin" /> : item.state === "error" ? <TriangleAlert size={14} /> : <CheckCircle2 size={14} />}<strong>{item.name}</strong></span><span>{item.created && item.action === "打开" ? item.state === "error" ? "已创建，打开失败；" : "已创建；" : ""}{item.message}</span>{item.state === "error" && <button className="text-button" disabled={runtimeActionIds.includes(item.id) || groupPending} aria-label={`${item.name} 重试${item.action}`} onClick={() => item.action === "分组" ? void assignSelectedGroup([item.id], item.group) : item.action === "打开" ? void launch([item.id], !!item.created) : void stop([item.id])}>重试{item.action}</button>}</li>)}</ul>
  </section>;
  return (
    <div className="app-shell">
      <aside
        className="sidebar"
        inert={uiBlocked}
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
        inert={uiBlocked}
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
          {!["environments", "groups"].includes(route) && <div className="page-toolbar">
            <h1>{summary.label}</h1>
            <div className="page-toolbar-actions">
              {route === "proxies" ? (
                <Button className="primary" onClick={openProxyImport}>
                  <Plus size={18} />
                  添加代理
                </Button>
              ) : route === "backups" ? (
                nativeMode ? <span className="subtle-text">原生完整包 · 恢复前只读预检与明确确认</span> : <>
                  <Button onClick={() => backupFile.current?.click()}>
                    <ArrowUpFromLine size={16} />
                    导入快照
                  </Button>
                  <Button className="primary" onClick={newBackup}>
                    <Plus size={18} />
                    创建快照
                  </Button>
                </>
              ) : route === "kernels" ? (
                <a
                  className="button"
                  href="https://github.com/adryfish/fingerprint-chromium/blob/main/README-ZH.md"
                  target="_blank"
                  rel="noreferrer"
                >
                  <ExternalLink size={16} />
                  查看官方内核
                </a>
              ) : null}
            </div>
          </div>}
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
          {route === "proxies" && nativeMode && <NativeProxyManager application={application} workspace={workspace} importOpen={nativeProxyImportOpen} onImportOpenChange={setNativeProxyImportOpen} />}
          {route === "proxies" && !nativeMode && (
            <>
              <div className="info-strip">
                <Info size={18} />
                <div>
                  <strong>独立出口，明确绑定</strong>
                  <p>
                    支持 HTTP、HTTPS 和
                    SOCKS5。检查失败时阻止环境启动；不自动切换直连。当前检查使用演示结果。
                  </p>
                </div>
              </div>
              <section className="work-card">
                <div className="section-toolbar">
                  <h2>
                    代理列表 <span>{state.proxies.length}</span>
                  </h2>
                  <Button
                    disabled={checking.length > 0}
                    onClick={() => checkProxy(state.proxies.map((p) => p.id))}
                  >
                    <RefreshCw
                      size={15}
                      className={checking.length ? "spin" : ""}
                    />
                    检查全部
                  </Button>
                </div>
                <div className="table-scroll">
                  <table>
                    <thead>
                      <tr>
                        <th>代理名称</th>
                        <th>协议与地址</th>
                        <th>地区</th>
                        <th>检查结果</th>
                        <th>关联环境</th>
                        <th>操作</th>
                      </tr>
                    </thead>
                    <tbody>
                      {state.proxies.map((p) => (
                        <tr key={p.id}>
                          <td>
                            <div className="proxy-title">
                              <div className="small-icon">
                                <Network size={17} />
                              </div>
                              <strong>{p.name}</strong>
                            </div>
                            <span className="cell-secondary">
                              {p.username
                                ? "已配置认证用户名"
                                : "无需用户名认证"}
                            </span>
                          </td>
                          <td>
                            <Tag>{p.type.toUpperCase()}</Tag>
                            <div className="cell-secondary mono">
                              {p.host}:{p.port}
                            </div>
                          </td>
                          <td>
                            <span className="country-code">{p.country}</span>{" "}
                            {regions[p.country]?.label || p.country}
                          </td>
                          <td>
                            <Tag
                              kind={
                                p.status === "connected"
                                  ? "success"
                                  : p.status === "failed"
                                    ? "warning"
                                    : ""
                              }
                            >
                              {checking.includes(p.id)
                                ? "检查中…"
                                : p.status === "connected"
                                  ? `模拟可连接 · ${p.latency} ms`
                                  : p.status === "failed"
                                    ? "模拟连接失败"
                                    : "待检查"}
                            </Tag>
                          </td>
                          <td>
                            {
                              state.environments.filter(
                                (e) => e.proxyId === p.id,
                              ).length
                            }{" "}
                            个环境
                          </td>
                          <td>
                            <div className="row-actions">
                              <Button
                                className="compact"
                                disabled={checking.length > 0}
                                onClick={() => checkProxy([p.id])}
                              >
                                <Wifi size={14} />
                                检查
                              </Button>
                              <button
                                className="icon-button"
                                aria-label={`编辑代理 ${p.name}`}
                                onClick={() => {
                                  if (
                                    state.environments.some(
                                      (e) =>
                                        e.proxyId === p.id &&
                                        [
                                          "running",
                                          "starting",
                                          "stopping",
                                        ].includes(e.status),
                                    )
                                  ) {
                                    notify(
                                      "请先关闭使用该代理的环境，再修改代理配置。",
                                      true,
                                    );
                                    return;
                                  }
                                  setDialog({
                                    kind: "proxy-edit",
                                    proxy: { ...p },
                                  });
                                  setFormError("");
                                }}
                              >
                                <Settings2 size={16} />
                              </button>
                              <button
                                className="icon-button"
                                aria-label={`删除代理 ${p.name}`}
                                onClick={() => {
                                  if (
                                    state.environments.some(
                                      (e) => e.proxyId === p.id,
                                    )
                                  ) {
                                    notify(
                                      "代理正在被环境引用，请先更改环境的代理绑定。",
                                      true,
                                    );
                                    return;
                                  }
                                  if (!update((s) => ({
                                    ...s,
                                    proxies: s.proxies.filter(
                                      (i) => i.id !== p.id,
                                    ),
                                  }), log(
                                    "删除代理",
                                    p.name,
                                    "未被引用的代理配置已删除。",
                                  ))) return;
                                }}
                              >
                                <Trash2 size={16} />
                              </button>
                            </div>
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
                {!state.proxies.length && (
                  <Empty
                    title="还没有代理"
                    text="导入代理后，可以在新建或编辑环境时进行绑定。"
                    action={
                      <Button className="primary" onClick={openProxyImport}>
                        添加代理
                      </Button>
                    }
                  />
                )}
              </section>
            </>
          )}
          {route === "kernels" && nativeMode && <><NativeKernelManager application={application} workspace={workspace} /><NativeMigrationManager application={application} workspace={workspace} /></>}
          {route === "kernels" && !nativeMode && (
            <>
              <div className="info-strip">
                <ShieldCheck size={19} />
                <div>
                  <strong>固定版本，明确来源</strong>
                  <p>
                    {nativeMode ? "指定 adryfish/fingerprint-chromium。当前未安装；安装与校验尚未接入，本机档案不能启动。" : "选用 adryfish/fingerprint-chromium。原型没有下载或运行内核；正式接入需核对可执行文件版本、哈希和参数实际读值。"}
                  </p>
                </div>
              </div>
              <div className="kernel-grid">
                {state.kernels.map((k) => (
                  <section
                    className={`kernel-card ${k.available ? "primary-kernel" : ""}`}
                    key={k.id}
                  >
                    <div className="kernel-card-top">
                      <div className="kernel-symbol">
                        <Box size={28} />
                      </div>
                      <Tag kind={k.available ? "blue-tag" : ""}>
                        {nativeMode ? (k.available ? "已核验" : "未安装 · 未就绪") : (k.available ? "演示基线" : "待验证")}
                      </Tag>
                    </div>
                    <h2>Chromium {k.version.split(".")[0]}</h2>
                    <div className="kernel-version mono">{k.version}</div>
                    <p>{k.note}</p>
                    <div className="kernel-meta">
                      <span>
                        适用系统<strong>Windows x64</strong>
                      </span>
                      <span>
                        关联环境
                        <strong>
                          {
                            state.environments.filter((e) => e.coreId === k.id)
                              .length
                          }{" "}
                          个
                        </strong>
                      </span>
                      <span>
                        真实运行验证<strong>尚未执行</strong>
                      </span>
                    </div>
                    <Button
                      className={k.available ? "soft-primary wide" : "wide"}
                      onClick={() => {
                        setDialog({ kind: "kernel", core: k });
                        setFormError("");
                      }}
                    >
                      <SlidersHorizontal size={16} />
                      查看能力与接入要求
                    </Button>
                  </section>
                ))}
              </div>
              <div className="work-card capability-card">
                <div>
                  <h2>参数能力分层</h2>
                  <p>只开放有明确实现入口的参数。</p>
                </div>
                <div className="capability-columns">
                  <div>
                    <Tag kind="success">可配置</Tag>
                    <p>语言、时区、CPU 线程数、网络策略</p>
                  </div>
                  <div>
                    <Tag kind="blue-tag">内核生成</Tag>
                    <p>GPU、内存、绘图与声音相关输出</p>
                  </div>
                  <div>
                    <Tag kind="warning">待实际核对</Tag>
                    <p>完整屏幕指纹、定位、跨版本输出</p>
                  </div>
                </div>
              </div>
            </>
          )}
          {route === "backups" && (
            nativeMode ? <NativeBackupManager key={nativeBackupSelection.join(",")} application={application} workspace={workspace} selectedIds={nativeBackupSelection} /> : <>
              <input
                ref={backupFile}
                type="file"
                accept=".json"
                hidden
                onChange={async (e) => {
                  const file = e.target.files?.[0];
                  e.target.value = "";
                  if (!file) return;
                  if (nativeMode) { notify("原型 JSON 不可导入真实工作区；完整恢复尚未接入。原数据未修改。", true); return; }
                  try {
                    if (file.size > 10 * 1024 * 1024)
                      throw new Error(
                        "原型文件请控制在 10 MB 内；这不是产品环境数量限制。",
                      );
                    const snapshot = parseSnapshot(await file.text());
                    setDialog({ kind: "restore", snapshot, name: file.name });
                    setFormError("");
                  } catch (err) {
                    notify((err as Error).message, true);
                  }
                }}
              />
              <div className="info-strip amber-strip">
                <Info size={19} />
                <div>
                  <strong>{nativeMode ? "完整本机备份与恢复尚未接入" : "当前保存的是原型数据快照"}</strong>
                  <p>
                    {nativeMode ? "本机环境保存在 SQLite。此页不会生成原型快照，也不会把原型 JSON 恢复为生产记录。" : "JSON 包含页面中的环境配置与示例 Cookie，排除代理密码。真实 Chromium 用户目录的备份与原子恢复在开发文档中单独定义。"}
                  </p>
                </div>
              </div>
              <section className="work-card">
                <div className="section-toolbar">
                  <h2>
                    本地快照 <span>{state.backups.length}</span>
                  </h2>
                  <span className="subtle-text">
                    保存在当前浏览器 · 建议导出文件留存
                  </span>
                </div>
                {state.backups.length ? (
                  <div className="table-scroll">
                    <table>
                      <thead>
                        <tr>
                          <th>快照名称</th>
                          <th>环境数量</th>
                          <th>创建时间</th>
                          <th>格式</th>
                          <th>操作</th>
                        </tr>
                      </thead>
                      <tbody>
                        {state.backups.map((b) => (
                          <tr key={b.id}>
                            <td>
                              <div className="proxy-title">
                                <div className="small-icon">
                                  <HardDrive size={18} />
                                </div>
                                <strong>{b.name}</strong>
                              </div>
                            </td>
                            <td>{b.snapshot.environments.length} 个</td>
                            <td>{time(b.createdAt)}</td>
                            <td>
                              <Tag>原型 JSON</Tag>
                            </td>
                            <td>
                              <div className="row-actions">
                                <Button
                                  className="compact"
                                  onClick={() =>
                                    download(
                                      `prism-snapshot-${b.id}.json`,
                                      JSON.stringify(b.snapshot, null, 2),
                                    )
                                  }
                                >
                                  <Download size={14} />
                                  导出
                                </Button>
                                <Button
                                  className="compact"
                                  onClick={() => {
                                    setDialog({
                                      kind: "restore",
                                      snapshot: b.snapshot,
                                      name: b.name,
                                    });
                                    setFormError("");
                                  }}
                                >
                                  <History size={14} />
                                  恢复
                                </Button>
                              </div>
                            </td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                ) : (
                  <Empty
                    title="为工作区保存第一份快照"
                    text="在批量修改或测试恢复流程前，先保留当前环境配置。"
                    action={
                      <Button className="primary" onClick={newBackup}>
                        <Plus size={16} />
                        创建快照
                      </Button>
                    }
                  />
                )}
              </section>
            </>
          )}
          {route === "activity" && (
            <>{nativeMode && <NativeDiagnostics application={application} />}<section className="work-card">
              <div className="section-toolbar">
                <h2>最近操作</h2>
                {!nativeMode && <Button
                  onClick={() =>
                    download(
                      "prism-activity.json",
                      JSON.stringify(state.activities, null, 2),
                    )
                  }
                >
                  <Download size={15} />
                  导出记录
                </Button>}
              </div>
              <div className="activity-list">
                {state.activities.map((a) => {
                  const session = a.environmentId ? workspace.runtimeSessions?.[a.environmentId] : undefined;
                  const currentSession = nativeMode && !!session && session.sessionId === a.sessionId;
                  return (
                  <div className="activity-item" key={a.id}>
                    <span className={`activity-symbol ${a.result}`}>
                      {a.result === "error" ? (
                        <TriangleAlert size={17} />
                      ) : a.result === "success" ? (
                        <Check size={17} />
                      ) : (
                        <Info size={17} />
                      )}
                    </span>
                    <div className="activity-content">
                      <strong>
                        {a.action}
                        <span>{a.target}</span>
                      </strong>
                      <p>{a.detail}</p>
                      {a.errorCode && <p className="mono">原因：{a.errorCode}</p>}
                      {a.nextAction && <p>{a.nextAction}</p>}
                      {currentSession && session.needsReconcile && <Button className="compact" disabled={runtimeActionIds.includes(a.environmentId!)} onClick={() => void handleRuntimeSessionAction(a.environmentId!, session.sessionId, "reconcile")}>核对会话</Button>}
                      {currentSession && session.canForce && !session.needsReconcile && <Button className="danger compact" disabled={runtimeActionIds.includes(a.environmentId!)} onClick={() => void handleRuntimeSessionAction(a.environmentId!, session.sessionId, "force")}>强制结束此会话</Button>}
                    </div>
                    <time>{time(a.time)}</time>
                  </div>
                ); })}
              </div>
            </section></>
          )}
          {route === "guide" && (
            <>
              <div className="guide-intro">
                <div className="guide-symbol">
                  <BookOpen size={28} />
                </div>
                <div>
                  <h2>{nativeMode ? "本机使用指南与排错" : "从产品需求，走到可实现的页面"}</h2>
                  <p>
                    {nativeMode ? "首版候选仅用于合成数据检查：代理逐会话保护已接入，本机受控能力已有证据；独立远端、人工流程及干净Windows验收仍待完成。按指南查看步骤与限制。" : "需求编号贯穿页面、数据模型与验收项。当前交互原型全部使用本地示例数据。"}
                  </p>
                </div>
                <Tag kind="blue-tag">v1.0 交付规格</Tag>
              </div>
              <div className="guide-links">
                {[
                  {
                    title: "环境与固定指纹",
                    text: "ENV-001 / ENV-002 / FP-001",
                    r: "environments",
                  },
                  {
                    title: "代理与故障阻断",
                    text: "PRX-001 / ENV-003",
                    r: "proxies",
                  },
                  {
                    title: "内核能力与版本",
                    text: "CORE-001 / FP-002",
                    r: "kernels",
                  },
                  {
                    title: "快照与恢复边界",
                    text: "BKP-001 / DATA-001",
                    r: "backups",
                  },
                ].map((item) => (
                  <button
                    key={item.r}
                    onClick={() => navigate(item.r as Route)}
                  >
                    <span>{item.title}</span>
                    <small>{item.text}</small>
                    <ArrowUpRight size={17} />
                  </button>
                ))}
              </div>
              <section className="work-card document-card">
                <div className="list-tabs">
                  {(
                    [
                      { id: "user", text: "本机使用指南" },
                      { id: "prd", text: "产品需求 PRD" },
                      { id: "development", text: "开发与验收" },
                      { id: "kernel", text: "内核适配合同" },
                    ] as const
                  ).map((d) => (
                    <button
                      className={docTab === d.id ? "selected" : ""}
                      key={d.id}
                      onClick={() => setDocTab(d.id)}
                    >
                      {d.text}
                    </button>
                  ))}
                  <div className="list-tabs-tail">
                    <Button
                      className="compact"
                      onClick={() =>
                        download(
                          docTab === "user" ? "USER_GUIDE.md" : docTab === "prd"
                            ? "PRD.md"
                            : docTab === "kernel"
                              ? "KERNEL.md"
                              : "DEVELOPMENT.md",
                          docTab === "user" ? userGuideText : docTab === "prd"
                            ? prdText
                            : docTab === "kernel"
                              ? kernelText
                              : developmentText,
                          "text/markdown;charset=utf-8",
                        )
                      }
                    >
                      <Download size={14} />
                      下载文档
                    </Button>
                  </div>
                </div>
                <div className="document-body">
                  <Suspense fallback={<p>正在载入文档…</p>}>
                    <Markdown
                      remarkPlugins={[remarkGfm]}
                      components={{
                        a: ({ href, children }) => (
                          <a
                            href={docHref(href)}
                            target={
                              docHref(href).startsWith("https://")
                                ? "_blank"
                                : undefined
                            }
                            rel="noreferrer"
                            onClick={(ev) => {
                              if (
                                href &&
                                /(?:PRD|DEVELOPMENT|KERNEL|USER_GUIDE)\.md/.test(href)
                              ) {
                                ev.preventDefault();
                                setDocTab(
                                  href.includes("USER_GUIDE") ? "user" : href.includes("KERNEL")
                                    ? "kernel"
                                    : href.includes("DEVELOPMENT")
                                      ? "development"
                                      : "prd",
                                );
                                document
                                  .querySelector(".document-body")
                                  ?.scrollTo(0, 0);
                              }
                            }}
                          >
                            {children}
                          </a>
                        ),
                      }}
                    >
                      {docTab === "user" ? userGuideText : docTab === "prd"
                        ? prdText
                        : docTab === "kernel"
                          ? kernelText
                          : developmentText}
                    </Markdown>
                  </Suspense>
                </div>
              </section>
            </>
          )}
        </main>
      </div>
      {drawerVisible && drawer && (
        <div
          className="overlay environment-overlay"
          inert={Boolean(dialog || draftProxyImportOpen || environmentConfirmation || storageIssue)}
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
      {nativeMode && draftProxyImportOpen && <div className="overlay modal-overlay stacked-overlay"><div className="modal wide-modal" ref={draftProxyOverlayRef} role="dialog" aria-modal="true" aria-labelledby="draft-proxy-import-title">
        <div className="modal-header"><h2 id="draft-proxy-import-title">导入代理</h2><button className="icon-button" aria-label="关闭代理导入并返回环境配置" disabled={draftProxyImportBusy} onClick={() => setDraftProxyImportOpen(false)}><X size={20} /></button></div>
        <div className="modal-body"><p className="modal-intro">环境草稿已保留。导入成功后可直接选择新代理，不会改变指纹。</p><NativeProxyManager application={application} workspace={workspace} importOnly importOpen onImportOpenChange={open => { setDraftProxyImportOpen(open); if (!open) setDraftProxyImportBusy(false); }} onBusyChange={setDraftProxyImportBusy} onImported={ids => { if (ids[0]) patchDraft({ proxyId: ids[0] }); notify(`已导入 ${ids.length} 条代理，环境草稿已保留`); }} /></div>
        <div className="modal-footer"><Button disabled={draftProxyImportBusy} onClick={() => setDraftProxyImportOpen(false)}>返回环境配置</Button></div>
      </div></div>}
      {dialog?.kind === "cookies" && <div className={`overlay modal-overlay ${drawerVisible ? "stacked-overlay" : ""}`} inert={Boolean(environmentConfirmation || storageIssue)}>
        <DemoCookieImportWindow environment={state.environments.find(e => e.id === dialog.id)!}
          text={cookieText} result={cookieResult} error={formError} dialogRef={overlayRef}
          onText={text => { setCookieText(text); setCookieResult(null); }}
          onParse={() => setCookieResult(parseCookies(cookieText))}
          onClose={() => setDialog(null)} onSave={saveDemoCookie} />
      </div>}
      {dialog?.kind === "delete" && <div className="overlay modal-overlay" inert={Boolean(environmentConfirmation || storageIssue)}>
        <DemoEnvironmentRemoveWindow count={dialog.ids.length} error={formError}
          dialogRef={overlayRef} onClose={() => setDialog(null)} onRemove={removeEnvironments} />
      </div>}
      {dialog && dialog.kind !== "cookies" && dialog.kind !== "delete" && (
        <div className={`overlay modal-overlay ${drawerVisible ? "stacked-overlay" : ""}`} inert={Boolean(environmentConfirmation || storageIssue)}>
          <div
            className={`modal ${dialog.kind === "proxy" ? "wide-modal" : ""} ${dialog.kind === "group" ? "group-modal" : ""}`}
            ref={overlayRef}
            tabIndex={-1}
            role="dialog"
            aria-modal="true"
            aria-labelledby="modal-title"
          >
            <div className="modal-header">
              <h2 id="modal-title">
                {dialog.kind === "proxy-edit"
                  ? "编辑代理配置"
                  : dialog.kind === "proxy"
                    ? "批量导入代理"
                    : dialog.kind === "restore"
                      ? "恢复原型快照"
                      : dialog.kind === "kernel"
                        ? "内核能力与接入"
                        : "调整环境分组"}
              </h2>
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
              {dialog.kind === "proxy" && (
                <>
                  <p className="modal-intro">
                    每行一个代理，支持
                    HTTP、HTTPS、SOCKS5。请使用示例数据，原型不进行真实网络检查。
                  </p>
                  <Field label="代理文本">
                    <textarea
                      aria-label="代理文本"
                      className="code-input"
                      rows={5}
                      placeholder="socks5://demo:password@192.0.2.10:1080"
                      value={proxyText}
                      onChange={(e) => {
                        setProxyText(e.target.value);
                        setProxyRows([]);
                      }}
                    />
                  </Field>
                  <input
                    ref={proxyFile}
                    type="file"
                    accept=".txt"
                    hidden
                    onChange={async (e) => {
                      const f = e.target.files?.[0];
                      if (f) {
                        setProxyText(await f.text());
                        setProxyRows([]);
                      }
                      e.target.value = "";
                    }}
                  />
                  <div className="inline-actions">
                    <Button onClick={() => proxyFile.current?.click()}>
                      <ArrowUpFromLine size={15} />
                      选择文本文件
                    </Button>
                    <button
                      className="text-button"
                      onClick={() => {
                        setProxyText(
                          "socks5://demo:example@192.0.2.90:1080\nhttp://198.51.100.90:8080",
                        );
                        setProxyRows([]);
                      }}
                    >
                      填入示例
                    </button>
                    <Button
                      className="soft-primary"
                      onClick={() => {
                        const rows = parseProxyText(proxyText);
                        setProxyRows(rows);
                        setFormError(
                          rows.length ? "" : "请先输入至少一条代理。",
                        );
                      }}
                    >
                      解析预览
                    </Button>
                  </div>
                  {proxyRows.length > 0 && (
                    <div className="import-preview">
                      <h3>
                        解析结果{" "}
                        <span>
                          {proxyRows.filter((r) => r.node).length} 条有效 /{" "}
                          {proxyRows.filter((r) => r.error).length} 条无效
                        </span>
                      </h3>
                      {proxyRows.map((r) => (
                        <div
                          key={r.line}
                          className={r.error ? "invalid-row" : ""}
                        >
                          {r.node ? (
                            <CheckCircle2 size={15} />
                          ) : (
                            <TriangleAlert size={15} />
                          )}
                          <span>第 {r.line} 行</span>
                          <strong>
                            {r.node
                              ? `${r.node.type}://${r.node.host}:${r.node.port}`
                              : r.error}
                          </strong>
                        </div>
                      ))}
                    </div>
                  )}
                </>
              )}
              {dialog.kind === "restore" && (
                <>
                  <div className="target-info">
                    <HardDrive size={18} />
                    {dialog.name}
                  </div>
                  <p>
                    该快照包含{" "}
                    <strong>{dialog.snapshot.environments.length}</strong>{" "}
                    个环境和 <strong>{dialog.snapshot.proxies.length}</strong>{" "}
                    个代理。恢复会替换当前原型工作区配置。
                  </p>
                  <div className="form-note">
                    <TriangleAlert size={18} />
                    <span>
                      请先关闭所有模拟运行的环境并备份当前配置。恢复保留原种子，代理密码需重新填写，代理状态重置为待检查。
                    </span>
                  </div>
                </>
              )}
              {dialog.kind === "kernel" && (
                <>
                  <div className="kernel-modal-title">
                    <Box size={30} />
                    <div>
                      <h3>fingerprint-chromium {dialog.core.version}</h3>
                      <p>
                        Windows x64 ·{" "}
                        {dialog.core.available ? "可审查源码基线" : "候选构建"}
                      </p>
                    </div>
                  </div>
                  <div className="generated-fields">
                    {[
                      ["可执行文件校验", "待真实桌面服务接入"],
                      ["浏览器版本与 UA 一致性", "必须在实际运行后核对"],
                      ["固定种子与独立目录", "作为启动必需参数"],
                      ["代理认证", "本地代理桥接，失败阻止启动"],
                      ["完整能力报告", "见内核适配合同"],
                    ].map(([a, b]) => (
                      <div key={a}>
                        <span>{a}</span>
                        <strong>{b}</strong>
                      </div>
                    ))}
                  </div>
                  <p className="field-hint">
                    原型没有下载、安装或检测内核。切换版本不自动迁移已有环境；升级需完整备份与兼容性验证。
                  </p>
                </>
              )}
              {dialog.kind === "proxy-edit" && (
                <>
                  <p className="modal-intro">
                    保存后需重新检查。修改网络不会自动改写环境指纹、语言或时区。
                  </p>
                  <Field label="代理名称">
                    <input
                      value={dialog.proxy.name}
                      onChange={(e) =>
                        setDialog({
                          ...dialog,
                          proxy: { ...dialog.proxy, name: e.target.value },
                        })
                      }
                    />
                  </Field>
                  <div className="field-row">
                    <Field label="协议">
                      <select
                        value={dialog.proxy.type}
                        onChange={(e) =>
                          setDialog({
                            ...dialog,
                            proxy: {
                              ...dialog.proxy,
                              type: e.target.value as ProxyNode["type"],
                            },
                          })
                        }
                      >
                        {["http", "https", "socks5"].map((t) => (
                          <option key={t}>{t}</option>
                        ))}
                      </select>
                    </Field>
                    <Field label="预设地区">
                      <select
                        value={dialog.proxy.country}
                        onChange={(e) =>
                          setDialog({
                            ...dialog,
                            proxy: { ...dialog.proxy, country: e.target.value },
                          })
                        }
                      >
                        {Object.entries(regions).map(([k, r]) => (
                          <option key={k} value={k}>
                            {r.label}
                          </option>
                        ))}
                      </select>
                    </Field>
                  </div>
                  <div className="field-row">
                    <Field label="地址">
                      <input
                        value={dialog.proxy.host}
                        onChange={(e) =>
                          setDialog({
                            ...dialog,
                            proxy: { ...dialog.proxy, host: e.target.value },
                          })
                        }
                      />
                    </Field>
                    <Field label="端口">
                      <input
                        type="number"
                        value={dialog.proxy.port}
                        onChange={(e) =>
                          setDialog({
                            ...dialog,
                            proxy: {
                              ...dialog.proxy,
                              port: Number(e.target.value),
                            },
                          })
                        }
                      />
                    </Field>
                  </div>
                  <div className="field-row">
                    <Field label="用户名">
                      <input
                        autoComplete="off"
                        value={dialog.proxy.username}
                        onChange={(e) =>
                          setDialog({
                            ...dialog,
                            proxy: {
                              ...dialog.proxy,
                              username: e.target.value,
                            },
                          })
                        }
                      />
                    </Field>
                    <Field label="密码（请使用演示值）">
                      <input
                        type="password"
                        autoComplete="new-password"
                        value={dialog.proxy.password}
                        onChange={(e) =>
                          setDialog({
                            ...dialog,
                            proxy: {
                              ...dialog.proxy,
                              password: e.target.value,
                            },
                          })
                        }
                      />
                    </Field>
                  </div>
                  <label className="checkbox-card">
                    <input
                      type="checkbox"
                      checked={Boolean(dialog.proxy.simulateFailure)}
                      onChange={(e) =>
                        setDialog({
                          ...dialog,
                          proxy: {
                            ...dialog.proxy,
                            simulateFailure: e.target.checked,
                          },
                        })
                      }
                    />
                    <span>
                      <strong>模拟连接失败</strong>
                      <small>用于验收故障阻断；关闭后检查展示模拟成功。</small>
                    </span>
                  </label>
                </>
              )}
              {dialog.kind === "group" && (
                <>
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
                  <p className="group-scope-note">将明确选定的 <strong>{dialog.ids.length}</strong> 个环境归入此名称。<br />筛选与翻页不会扩大这次范围。</p>
                  <p>这是环境记录的分组标签，不创建独立空分组。普通分组修改保留 seed、内核、代理与浏览数据。</p>
                </>
              )}
              {formError && (
                <div className="form-error" role="alert">
                  <TriangleAlert size={16} />
                  {formError}
                </div>
              )}
            </div>
            <div className="modal-footer">
              <Button disabled={groupPending} onClick={() => setDialog(null)}>取消</Button>
              {dialog.kind === "proxy-edit" && (
                <Button
                  className="primary"
                  onClick={() => {
                    const p = dialog.proxy;
                    const parsed = parseProxyText(
                      `${p.type}://${p.host}:${p.port}`,
                    )[0];
                    if (
                      !p.name.trim() ||
                      !parsed?.node ||
                      p.host.includes("@") ||
                      /[\s/?#]/.test(p.host) ||
                      !Number.isInteger(p.port) ||
                      p.port < 1 ||
                      p.port > 65535
                    ) {
                      setFormError("请填写有效的名称、地址和端口。");
                      return;
                    }
                    if (!update((s) => ({
                      ...s,
                      proxies: s.proxies.map((i) =>
                        i.id === p.id
                          ? {
                              ...p,
                              name: p.name.trim(),
                              status: "unchecked",
                              latency: undefined,
                            }
                          : i,
                      ),
                    }), log("修改代理", p.name, "配置已保存，等待重新检查。"))) return;
                    setDialog(null);
                    notify("代理已保存，请重新检查");
                  }}
                >
                  保存代理
                </Button>
              )}
              {dialog.kind === "proxy" && (
                <Button
                  className="primary"
                  disabled={!proxyRows.length || !proxyRows.some((r) => r.node)}
                  onClick={() => {
                    const nodes = proxyRows.flatMap((r) =>
                      r.node ? [r.node] : [],
                    );
                    if (!update((s) => ({
                      ...s,
                      proxies: [...s.proxies, ...nodes],
                    }), log(
                      "导入代理",
                      `${nodes.length} 条`,
                      "有效行已导入，无效行未保存；等待连接检查。",
                    ))) return;
                    if (drawerRef.current && nodes[0]) patchDraft({ proxyId: nodes[0].id });
                    setDialog(null);
                    notify(`已导入 ${nodes.length} 条代理，待检查`);
                  }}
                >
                  导入 {proxyRows.filter((r) => r.node).length} 个代理
                </Button>
              )}
              {dialog.kind === "restore" && (
                <Button className="primary" onClick={confirmRestore}>
                  确认恢复
                </Button>
              )}
              {dialog.kind === "kernel" && (
                <Button
                  className="primary"
                  onClick={() => {
                    setDialog(null);
                    setDocTab("kernel");
                    navigate("guide");
                  }}
                >
                  阅读适配文档
                </Button>
              )}
              {dialog.kind === "group" && (
                <Button
                  className="primary"
                  disabled={groupPending || !dialog.ids.length || !groupName.trim()}
                  onClick={() => void assignSelectedGroup()}
                >
                  应用到所选环境
                </Button>
              )}
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
      {environmentConfirmation && <div className="overlay modal-overlay environment-confirmation-overlay" inert={Boolean(storageIssue)}>
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
