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
  Ellipsis,
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
  Sparkles,
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
} from "./application/contract";
import { applyFingerprint, fingerprintMatchesConfiguration } from "./application/fingerprint-model";
import prdText from "../docs/PRD.md?raw";
import developmentText from "../docs/DEVELOPMENT.md?raw";
import kernelText from "../docs/KERNEL.md?raw";
const Markdown = lazy(() => import("react-markdown"));
import remarkGfm from "remark-gfm";
import { NativeKernelManager } from "./components/NativeKernelManager";
import { NativeProxyManager } from "./components/NativeProxyManager";
import { FingerprintRevisionPanel } from "./components/FingerprintRevisionPanel";

type Route =
  "environments" | "proxies" | "kernels" | "backups" | "activity" | "guide";
type Drawer = {
  kind: "create" | "edit";
  environment: Environment;
  tab: "basic" | "fingerprint" | "preferences";
  previewId: string;
  requestId: string;
  expectedRevision?: number;
  fingerprint?: FingerprintPreview;
  history?: ProfileRevision[];
  userDataRef?: string;
};
type Dialog =
  | { kind: "delete"; ids: string[] }
  | { kind: "cookies"; id: string }
  | { kind: "proxy" }
  | { kind: "proxy-edit"; proxy: ProxyNode }
  | { kind: "restore"; snapshot: Snapshot; name: string }
  | { kind: "kernel"; core: Kernel }
  | { kind: "group" };
const routeInfo = {
  environments: {
    label: "浏览器环境",
    icon: LayoutGrid,
    description: "每一份环境，都有独立的工作空间。",
    req: "ENV-001 · ENV-002 · FP-001",
  },
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
  if (/(?:PRD|DEVELOPMENT|KERNEL)\.md/.test(href)) return "#/guide";
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
  const drawerRef = useRef(drawer);
  drawerRef.current = drawer;
  const drawerRuntime = drawer?.kind === "edit" ? workspace.runtimeSessions?.[drawer.environment.id] : undefined;
  const profileBusy = nativeMode && !!drawerRuntime && (["starting", "running", "stopping"].includes(drawerRuntime.state) || !!drawerRuntime.pid || drawerRuntime.needsReconcile || drawerRuntime.persistencePending);
  const previewOpenSequence = useRef(0);
  const fingerprintBusy = useRef(false);
  const [dialog, setDialog] = useState<Dialog | null>(null);
  const [nativeProxyImportOpen, setNativeProxyImportOpen] = useState(false);
  const [formError, setFormError] = useState("");
  const [generating, setGenerating] = useState(false);
  const [savePending, setSavePending] = useState(false);
  const [quantity, setQuantity] = useState(1);
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
  const [deleteData, setDeleteData] = useState(false);
  const [docTab, setDocTab] = useState<"prd" | "development" | "kernel">("prd");
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
  const initialDraft = useRef("");
  const initialFingerprintHash = useRef("");
  const backupFile = useRef<HTMLInputElement>(null);
  const cookieFile = useRef<HTMLInputElement>(null);
  const proxyFile = useRef<HTMLInputElement>(null);
  const overlayRef = useRef<HTMLDivElement>(null);
  const searchRef = useRef<HTMLInputElement>(null);
  const notify = (text: string, error = false) => {
    setToast({ text, error });
    if (toastTimer.current) clearTimeout(toastTimer.current);
    toastTimer.current = setTimeout(() => setToast(null), 4200);
  };
  const update = (fn: (s: State) => State, activity?: Pick<Activity, "action" | "target" | "detail" | "result">) => {
    const result = application.compatibility?.update((s) => {
      const next = fn(s);
      return activity ? { ...next, activities: [{ ...activity, id: uid("log"), time: now() }, ...next.activities] } : next;
    });
    if (!result) notify("此演示操作尚未接入桌面服务。", true);
    else if (!result.ok) notify(result.error.message, true);
    return Boolean(result?.ok);
  };
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
  const nativeRuntimeActive = nativeMode && Object.values(workspace.runtimeSessions ?? {}).some(session => ["starting", "running", "stopping"].includes(session.state) || !!session.pid || session.needsReconcile || session.persistencePending);
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
  const closeDrawer = () => {
    if (generating || saving.current) return;
    if (
      drawer &&
      (JSON.stringify(drawer.environment) !== initialDraft.current || (drawer.fingerprint?.previewProfile.configHash ?? "") !== initialFingerprintHash.current) &&
      !window.confirm("配置尚未保存。确定放弃本次编辑吗？")
    )
      return;
    previewOpenSequence.current++;
    if (drawer) void application.discardPreview(drawer.previewId);
    setDrawer(null);
  };
  useEffect(() => {
    const key = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key === "k") {
        e.preventDefault();
        searchRef.current?.focus();
      }
      if (e.key === "Escape") {
        setMenu(null);
        if (!generating) {
          closeDrawer();
          setDialog(null);
        }
      }
    };
    window.addEventListener("keydown", key);
    return () => window.removeEventListener("keydown", key);
  }, [generating, drawer]);
  useEffect(() => {
    if (!drawer && !dialog) return;
    const oldOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    const previous = document.activeElement as HTMLElement;
    const timer = setTimeout(
      () =>
        overlayRef.current
          ?.querySelector<HTMLElement>("button, input, select, textarea")
          ?.focus(),
      30,
    );
    const trap = (e: KeyboardEvent) => {
      if (e.key !== "Tab" || !overlayRef.current) return;
      const elements = [
        ...overlayRef.current.querySelectorAll<HTMLElement>(
          "button:not(:disabled), input:not(:disabled), select:not(:disabled), textarea:not(:disabled), a[href]",
        ),
      ].filter((el) => el.offsetParent !== null);
      const first = elements[0],
        last = elements.at(-1);
      if (e.shiftKey && document.activeElement === first) {
        e.preventDefault();
        last?.focus();
      } else if (!e.shiftKey && document.activeElement === last) {
        e.preventDefault();
        first?.focus();
      }
    };
    document.addEventListener("keydown", trap);
    return () => {
      document.body.style.overflow = oldOverflow;
      clearTimeout(timer);
      document.removeEventListener("keydown", trap);
      previous?.focus();
    };
  }, [Boolean(drawer || dialog)]);
  const navigate = (r: Route) => {
    location.hash = `/${r}`;
  };
  const groups = [
    ...new Set(state.environments.map((e) => e.group).filter(Boolean)),
  ];
  const visible = state.environments.filter(
    (e) =>
      (!search ||
        `${e.name} ${e.code} ${e.note}`
          .toLowerCase()
          .includes(search.toLowerCase())) &&
      (group === "全部分组" || e.group === group) &&
      (status === "all" || e.status === status),
  );
  const pageCount = Math.max(1, Math.ceil(visible.length / 8));
  const pageItems = visible.slice(
    (Math.min(page, pageCount) - 1) * 8,
    Math.min(page, pageCount) * 8,
  );
  const running = state.environments.filter(
    (e) => e.status === "running",
  ).length;
  const errors = state.environments.filter((e) => e.status === "error").length;
  const patchDraft = (value: Partial<Environment>) => {
    if (saving.current) return;
    if (profileBusy && Object.keys(value).some(key => !["name", "group", "note"].includes(key))) return;
    setFormError("");
    setDrawer((d) =>
      d ? { ...d, requestId: uid("request"), environment: { ...d.environment, ...value } } : d,
    );
  };
  async function openCreate(template?: Environment) {
    const sequence = ++previewOpenSequence.current;
    const result = await application.previewEnvironment({ kind: "create", sourceId: template?.id });
    if (sequence !== previewOpenSequence.current) { if (result.ok) void application.discardPreview(result.data.previewId); return; }
    if (!result.ok) { notify(result.error.message, true); return; }
    const preview = result.data;
    initialDraft.current = JSON.stringify(preview.environment);
    initialFingerprintHash.current = preview.fingerprint?.previewProfile.configHash ?? "";
    setDrawer({ kind: "create", ...preview, requestId: uid("request"), tab: "basic" });
    setFormError("");
    setQuantity(1);
    setMenu(null);
  }
  async function openEdit(e: Environment) {
    const sequence = ++previewOpenSequence.current;
    const result = await application.previewEnvironment({ kind: "edit", sourceId: e.id });
    if (sequence !== previewOpenSequence.current) { if (result.ok) void application.discardPreview(result.data.previewId); return; }
    if (!result.ok) { notify(result.error.message, true); return; }
    const preview = result.data;
    initialDraft.current = JSON.stringify(preview.environment);
    initialFingerprintHash.current = preview.fingerprint?.previewProfile.configHash ?? "";
    setDrawer({ kind: "edit", ...preview, requestId: uid("request"), tab: "basic" });
    setQuantity(1);
    setFormError("");
    setMenu(null);
    const history = await application.listFingerprintRevisions(e.id);
    setDrawer(current => current?.previewId === preview.previewId ? { ...current, history: history.ok ? history.data : undefined } : current);
    if (!history.ok && drawerRef.current?.previewId === preview.previewId) setFormError(history.error.message);
  }
  function applyProfilePreview(previewId: string, preview: EnvironmentPreview) {
    setDrawer(current => {
      if (!current || current.previewId !== previewId || !preview.fingerprint) return current;
      return { ...current, requestId: uid("request"), fingerprint: preview.fingerprint, userDataRef: preview.userDataRef ?? current.userDataRef, environment: applyFingerprint(current.environment, preview.fingerprint.previewProfile) };
    });
  }
  async function generateProfile(regenerate = false) {
    if (!drawer || fingerprintBusy.current || saving.current || profileBusy) return;
    const target = drawer.previewId;
    fingerprintBusy.current = true;
    setGenerating(true);
    setFormError("");
    try {
      const result = await application.generateFingerprint({ previewId: target, kernelId: drawer.environment.coreId, templateId: drawer.environment.fingerprintVersion, overrides: drawer.environment, regenerate });
      if (drawerRef.current?.previewId !== target) return;
      if (result.ok) { applyProfilePreview(target, result.data); notify(regenerate ? "新的种子只在预览中；查看变更后保存才生效。" : "档案预览已更新，没有保存或启动浏览器。"); }
      else setFormError(result.error.message);
    } finally { fingerprintBusy.current = false; setGenerating(false); }
  }
  async function previewProfileRestore(revision: number) {
    if (!drawer || fingerprintBusy.current || saving.current || profileBusy) return;
    const target = drawer.previewId;
    fingerprintBusy.current = true; setGenerating(true); setFormError("");
    try {
      const result = await application.previewFingerprintRestore(target, revision);
      if (drawerRef.current?.previewId !== target) return;
      if (result.ok) { applyProfilePreview(target, result.data); notify("旧档案已加载为回滚预览；保存才生效，名称、代理和数据保持不变。"); }
      else setFormError(result.error.message);
    } finally { fingerprintBusy.current = false; setGenerating(false); }
  }
  async function saveEnvironment() {
    if (batchBusy.current || saving.current || fingerprintBusy.current || !drawer) return;
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
      const accepted = await application.createBatch({ ...request, count: quantity });
      if (drawerRef.current?.previewId !== target) return;
      if (!accepted.ok) { setFormError(accepted.error.message); return; }
      batchBusy.current = true;
      creationOperation.current = accepted.data.operation.id;
      latestOperationEvent.current = undefined;
      setDrawer(null);
      let inspected = await application.getOperation(creationOperation.current);
      while (inspected.ok && !operationIsTerminal(inspected.data)) {
        setBatch({ label: "创建环境", done: inspected.data.completedIds.length, total: inspected.data.total });
        await sleep(30);
        inspected = await application.getOperation(creationOperation.current);
      }
      if (!inspected.ok) notify(inspected.error.message, true);
      else {
        const operation = inspected.data;
        const prefix = operation.state === "cancelled" ? "已取消余下任务；" : "";
        notify(`${operation.error?.message || prefix}已创建 ${operation.completedIds.length} 个环境`, operation.state === "failed");
      }
      setGroup("全部分组"); setStatus("all"); setSearch(""); setPage(1);
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
  async function launch(ids: string[]) {
    if (nativeMode) {
      if (!application.startRuntime) { notify("当前桌面版本未接入真实启停。", true); return; }
      const targets = [...new Set(ids)].filter(id => !runtimeActions.current.has(id));
      if (!targets.length) return;
      if (!window.confirm("当前仅支持本机直连，网站可看到本机网络出口。已绑定代理的环境将被阻止，不会绕过代理。确认使用本机直连启动所选环境？")) return;
      for (const id of targets) {
        if (application.getSnapshot().runtimeSessions?.[id]?.needsReconcile) { notify("此环境的原会话仍待核对，不会新开浏览器或绕过目录保护。", true); continue; }
        if (!beginRuntimeAction(id)) continue;
        try {
          const result = await application.startRuntime({ environmentId: id, requestId: uid("request"), networkPolicy: "direct" });
          if (!result.ok) { notify(result.error.message, true); continue; }
          notify("启动任务已受理；内核和控制通道就绪后才显示运行中。");
        } finally { endRuntimeAction(id); }
      }
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
    for (const id of ids) {
      if (batchCancelled.current) break;
      const e = current.current.environments.find((i) => i.id === id);
      if (!e || ["running", "starting", "stopping"].includes(e.status))
        continue;
      setBatch({ label: "启动环境", done, total: ids.length });
      const failure = launchError(e, current.current);
      if (failure) {
        if (!update((s) => ({
          ...s,
          environments: s.environments.map((i) =>
            i.id === id ? { ...i, status: "error", error: failure } : i,
          ),
        }), log("启动被阻止", e.name, failure, "error"))) break;
        notify(failure, true);
        continue;
      }
      if (!update((s) => ({
        ...s,
        environments: s.environments.map((i) =>
          i.id === id ? { ...i, status: "starting", error: undefined } : i,
        ),
      }))) break;
      await sleep(650);
      if (
        current.current.environments.find((i) => i.id === id)?.status ===
        "starting"
      ) {
        if (!update((s) => ({
          ...s,
          environments: s.environments.map((i) =>
            i.id === id && i.status === "starting"
              ? { ...i, status: "running", lastOpened: now() }
              : i,
          ),
        }), log(
          "模拟启动",
          e.name,
          "演示状态已变为运行中；未启动真实浏览器进程。",
        ))) break;
      }
      done++;
    }
    batchBusy.current = false;
    setBatch(null);
  }
  async function stop(ids: string[]) {
    if (nativeMode) {
      if (!application.stopRuntime) { notify("当前桌面版本未接入真实停止。", true); return; }
      for (const id of [...new Set(ids)]) {
        if (application.getSnapshot().runtimeSessions?.[id]?.needsReconcile) { notify("此环境的会话仍待核对；批量关闭不会自动按PID结束或升级成强制结束。", true); continue; }
        if (!beginRuntimeAction(id)) continue;
        try {
          const result = await application.stopRuntime({ environmentId: id, requestId: uid("request") });
          if (!result.ok) notify(result.error.message, true);
          else notify("停止任务已受理；等待本次会话退出，不会清除浏览数据。");
        } finally { endRuntimeAction(id); }
      }
      setMenu(null);
      return;
    }
    batchCancelled.current = true;
    for (const id of ids) {
      const e = current.current.environments.find((i) => i.id === id);
      if (!e || !["running", "starting"].includes(e.status)) continue;
      if (!update((s) => ({
        ...s,
        environments: s.environments.map((i) =>
          i.id === id ? { ...i, status: "stopping" } : i,
        ),
      }))) return;
      await sleep(350);
      if (!update((s) => ({
        ...s,
        environments: s.environments.map((i) =>
          i.id === id ? { ...i, status: "ready" } : i,
        ),
      }), log("模拟关闭", e.name, "固定指纹和示例 Cookie 已保留。"))) return;
    }
  }
  async function handleRuntimeSessionAction(id: string, expectedSessionId: string, action: "force" | "reconcile") {
    if (!nativeMode) return;
    const session = application.getSnapshot().runtimeSessions?.[id];
    if (!session || session.sessionId !== expectedSessionId) { notify("这条记录属于旧会话，未操作现在的浏览器；请重新读取状态。", true); return; }
    if (action === "force") {
      if (!application.forceStopRuntime || !session.canForce || session.needsReconcile) { notify("尚未满足指定会话强制结束条件。请先正常关闭；不会按PID结束进程。", true); return; }
      if (!window.confirm("仅强制结束这份已确认会话，可能丢失尚未保存的网页内容。不会结束其他环境，也不会清空浏览数据。确认强制结束？")) return;
    } else if (!application.reconcileRuntime) { notify("当前桌面版本未提供会话核对。", true); return; }
    if (!beginRuntimeAction(id)) return;
    try {
      const request = { environmentId: id, sessionId: expectedSessionId, requestId: uid("request") };
      const result = action === "force" ? await application.forceStopRuntime!(request) : await application.reconcileRuntime!(request);
      if (!result.ok) notify(result.error.message, true);
      else notify(action === "force" ? "指定会话结束任务已受理；确认本次Job全部退出后才显示已停止。" : "核对任务已受理；以实际进程身份和目录锁结果为准。");
    } finally { endRuntimeAction(id); }
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
    if (nativeMode) { notify("真实 Cookie 写入尚未接入，不会写入示例记录。", true); return; }
    setDialog({ kind: "cookies", id: e.id });
    setCookieText("");
    setCookieResult(null);
    setFormError("");
    setMenu(null);
  }
  function openProxyImport() {
    if (nativeMode) { setNativeProxyImportOpen(true); return; }
    setDialog({ kind: "proxy" });
    setProxyText("");
    setProxyRows([]);
    setFormError("");
  }
  function removeEnvironments() {
    if (dialog?.kind !== "delete") return;
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
      deleteData
        ? "原型记录及模拟数据已移除；无真实文件操作。"
        : "移除原型记录；桌面版应保留孤立数据目录并提供找回入口。",
    ))) return;
    setDialog(null);
    setSelected([]);
    notify("环境已从工作区移除");
  }
  const summary = routeInfo[route];
  const selectedKernelRecord = workspace.kernelRecords?.find(record => record.id === drawer?.environment.coreId);
  const canGenerateProfile = !nativeMode || selectedKernelRecord?.status === "verified";
  const profileIsFresh = !!drawer?.fingerprint && fingerprintMatchesConfiguration(drawer.fingerprint.previewProfile, drawer.environment);
  // Preserve T02's explicit, non-launchable pending configuration workflow.
  // Selecting an installed build still requires a complete matching preview.
  const pendingConfiguration = nativeMode && drawer?.environment.coreId === "kernel-pending";
  const canSaveProfile = pendingConfiguration || profileIsFresh;
  const canConfigureProfileField = (field: string) => !profileBusy && (!nativeMode || !!selectedKernelRecord?.report.capabilities.some(capability => capability.field === field && capability.status === "configurable" && capability.source === "observed"));
  return (
    <div className="app-shell">
      <aside
        className="sidebar"
        inert={Boolean(drawer || dialog || storageIssue)}
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
        <div className="workspace">
          <span className="workspace-icon">
            <Monitor size={18} />
          </span>
          <div>
            <strong>本机工作区</strong>
            <span>Windows · 本地模式</span>
          </div>
          <ChevronDown size={14} />
        </div>
        <div className="nav-caption">工作空间</div>
        <nav aria-label="主导航">
          {(["environments", "proxies", "kernels", "backups"] as Route[]).map(
            (r) => {
              const Icon = routeInfo[r].icon;
              return (
                <a
                  key={r}
                  href={`#/${r}`}
                  className={`nav-item ${route === r ? "active" : ""}`}
                >
                  <Icon size={18} />
                  <span>{routeInfo[r].label}</span>
                  {r === "environments" && (
                    <span className="nav-count">
                      {state.environments.length}
                    </span>
                  )}
                </a>
              );
            },
          )}
        </nav>
        <div className="nav-caption second">工作区工具</div>
        <nav>
          {(["activity", "guide"] as Route[]).map((r) => {
            const Icon = routeInfo[r].icon;
            return (
              <a
                key={r}
                href={`#/${r}`}
                className={`nav-item ${route === r ? "active" : ""}`}
              >
                <Icon size={18} />
                {routeInfo[r].label}
              </a>
            );
          })}
        </nav>
        <div className="sidebar-bottom">
          <div className="local-note">
            <ShieldCheck size={20} />
            <div>
              <strong>数据留在本地</strong>
              <span>每个环境，独立保存</span>
            </div>
          </div>
          <div className="sidebar-footer">
            <span className="avatar">本</span>
            <div>
              <strong>本地工作区</strong>
              <span>无需登录 · 无数量配额</span>
            </div>
            <LockKeyhole size={15} />
          </div>
        </div>
      </aside>
      <div
        className="main-shell"
        inert={Boolean(drawer || dialog || storageIssue)}
      >
        <header className="topbar">
          <div className="breadcrumb">
            工作空间
            <ChevronRight size={14} />
            <strong>{summary.label}</strong>
          </div>
          <div className="topbar-right">
            <span className="prototype-label">
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
            <span className="topbar-divider" />
            <span className="mini-avatar">P</span>
          </div>
        </header>
        <main>
          <div className="page-heading">
            <div>
              <div className="eyebrow">
                {route === "environments"
                  ? "ENVIRONMENT WORKSPACE"
                  : route.toUpperCase()}
              </div>
              <h1>
                {summary.label}
                {route === "environments" && (
                  <span className="heading-count">
                    {state.environments.length}
                  </span>
                )}
              </h1>
              <p>{summary.description}</p>
            </div>
            <div className="heading-actions">
              {route === "environments" ? (
                <>
                  <Button
                    onClick={() => {
                      navigate("backups");
                    }}
                  >
                    <ArrowDownToLine size={16} />
                    导入 / 备份
                  </Button>
                  <Button className="primary" onClick={() => openCreate()}>
                    <Plus size={18} />
                    新建环境
                  </Button>
                </>
              ) : route === "proxies" ? (
                <Button className="primary" onClick={openProxyImport}>
                  <Plus size={18} />
                  添加代理
                </Button>
              ) : route === "backups" ? (
                <>
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
          </div>
          {route === "environments" && (
            <>
              <section className="stats-grid" aria-label="环境概览">
                <div className="stat-card">
                  <div className="stat-icon blue">
                    <Layers3 size={20} />
                  </div>
                  <div>
                    <span>全部环境</span>
                    <strong>
                      {state.environments.length}
                      <small>个</small>
                    </strong>
                  </div>
                  <span className="stat-foot">按需创建，无数量配额</span>
                </div>
                <div className="stat-card">
                  <div className="stat-icon green">
                    <Monitor size={20} />
                  </div>
                  <div>
                    <span>运行中</span>
                    <strong>
                      {running}
                      <small>个</small>
                    </strong>
                  </div>
                  <span className="stat-foot">
                    <span className="status-dot green-dot" />
                    {nativeMode ? "本机真实会话 · 仅明确直连" : "模拟运行状态"}
                  </span>
                </div>
                <div className="stat-card">
                  <div className="stat-icon violet">
                    <Network size={20} />
                  </div>
                  <div>
                    <span>已配置代理</span>
                    <strong>
                      {state.proxies.length}
                      <small>个</small>
                    </strong>
                  </div>
                  <button
                    className="stat-link"
                    onClick={() => navigate("proxies")}
                  >
                    管理代理
                    <ArrowUpRight size={14} />
                  </button>
                </div>
                <div className="stat-card">
                  <div className="stat-icon amber">
                    <TriangleAlert size={20} />
                  </div>
                  <div>
                    <span>需要处理</span>
                    <strong>
                      {errors}
                      <small>个</small>
                    </strong>
                  </div>
                  <button
                    className="stat-link"
                    onClick={() => {
                      setStatus("error");
                      setPage(1);
                    }}
                  >
                    查看异常环境
                    <ArrowUpRight size={14} />
                  </button>
                </div>
              </section>
              <section className="work-card">
                <div className="list-tabs">
                  <button
                    className={status === "all" ? "selected" : ""}
                    onClick={() => {
                      setStatus("all");
                      setPage(1);
                    }}
                  >
                    全部环境<span>{state.environments.length}</span>
                  </button>
                  <button
                    className={status === "running" ? "selected" : ""}
                    onClick={() => {
                      setStatus("running");
                      setPage(1);
                    }}
                  >
                    运行中<span>{running}</span>
                  </button>
                  <button
                    className={status === "error" ? "selected" : ""}
                    onClick={() => {
                      setStatus("error");
                      setPage(1);
                    }}
                  >
                    需处理
                    {errors > 0 && <span className="amber-text">{errors}</span>}
                  </button>
                  <div className="list-tabs-tail">
                    <span className="subtle-text">独立配置 · 固定指纹</span>
                    <button
                      className="icon-button"
                      title="重新载入列表"
                      aria-label="刷新环境列表"
                      onClick={async () => {
                        setSearch("");
                        setGroup("全部分组");
                        setStatus("all");
                        setPage(1);
                        const result = await application.refresh?.();
                        if (result && !result.ok) notify(result.error.message, true);
                        else notify(nativeMode ? "已重新读取本机 SQLite 档案" : "环境列表已刷新");
                      }}
                    >
                      <RefreshCw size={16} />
                    </button>
                  </div>
                </div>
                <div className="filters">
                  <div className="search-box">
                    <Search size={17} />
                    <input
                      ref={searchRef}
                      aria-label="搜索环境"
                      placeholder="搜索环境名称、编号或备注"
                      value={search}
                      onChange={(e) => {
                        setSearch(e.target.value);
                        setPage(1);
                      }}
                    />
                    <kbd>Ctrl K</kbd>
                  </div>
                  <div className="select-wrap">
                    <Folder size={16} />
                    <select
                      aria-label="筛选分组"
                      value={group}
                      onChange={(e) => {
                        setGroup(e.target.value);
                        setPage(1);
                      }}
                    >
                      <option>全部分组</option>
                      {groups.map((g) => (
                        <option key={g}>{g}</option>
                      ))}
                    </select>
                  </div>
                  <Button
                    className="filter-button"
                    onClick={() => {
                      setStatus(status === "ready" ? "all" : "ready");
                      setPage(1);
                    }}
                  >
                    <ListFilter size={16} />
                    {status === "ready" ? "显示全部状态" : "待启动环境"}
                  </Button>
                  <span className="filter-spacer" />
                  <button
                    className="icon-button"
                    aria-label="分组管理"
                    onClick={() => {
                      setDialog({ kind: "group" });
                      setGroupName("");
                      setFormError("");
                    }}
                  >
                    <FolderPlus size={18} />
                  </button>
                </div>
                {selected.length > 0 && (
                  <div className="selection-bar">
                    <span>
                      已选择 <strong>{selected.length}</strong> 个环境
                    </span>
                    <Button onClick={() => launch(selected)}>
                      <Monitor size={14} />
                      批量启动
                    </Button>
                    <Button onClick={() => stop(selected)}>
                      <Square size={13} />
                      批量关闭
                    </Button>
                    <Button
                      onClick={() => {
                        setDialog({ kind: "delete", ids: selected });
                        setFormError("");
                        setDeleteData(false);
                      }}
                    >
                      <Trash2 size={14} />
                      移除
                    </Button>
                    <button
                      className="text-button"
                      onClick={() => setSelected([])}
                    >
                      取消选择
                    </button>
                  </div>
                )}
                <div className="table-scroll">
                  <table className="environment-table">
                    <thead>
                      <tr>
                        <th className="check-cell">
                          <input
                            type="checkbox"
                            aria-label="选择当前页全部环境"
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
                        <th>环境名称 / 编号</th>
                        <th>代理网络</th>
                        <th>设备档案</th>
                        <th>状态</th>
                        <th>最近打开</th>
                        <th className="actions-head">操作</th>
                      </tr>
                    </thead>
                    <tbody>
                      {pageItems.map((e) => {
                        const p = state.proxies.find((p) => p.id === e.proxyId);
                        const runtimeSession = nativeMode ? workspace.runtimeSessions?.[e.id] : undefined;
                        const runtimeActionPending = runtimeActionIds.includes(e.id);
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
                            <td>
                              <div className="environment-identity">
                                <div
                                  className={`environment-avatar avatar-${(+e.code || 1) % 4}`}
                                >
                                  <Fingerprint size={22} />
                                </div>
                                <div>
                                  <button
                                    className="name-button"
                                    onClick={() => openEdit(e)}
                                  >
                                    {e.name}
                                  </button>
                                  <div className="cell-secondary">
                                    <span className="mono">#{e.code}</span>
                                    <span className="tiny-divider" />
                                    <span>{e.group}</span>
                                  </div>
                                </div>
                              </div>
                            </td>
                            <td>
                              {p ? (
                                <>
                                  <div className="proxy-title">
                                    <span className="country-code">
                                      {p.country}
                                    </span>
                                    {p.name.split(" · ")[1] || p.name}
                                  </div>
                                  <div className="cell-secondary mono">
                                    {p.type.toUpperCase()} · {p.host}
                                  </div>
                                </>
                              ) : (
                                <>
                                  <span className="proxy-title">
                                    <Globe2 size={15} />
                                    本机网络
                                  </span>
                                  <div className="cell-secondary">
                                    已明确选择直连
                                  </div>
                                </>
                              )}
                            </td>
                            <td>
                              <span className="device-line">
                                <Monitor size={14} />
                                Windows · Chromium{" "}
                                {core?.version.split(".")[0] || "—"}
                              </span>
                              <div className="cell-secondary">
                                <Fingerprint size={12} />
                                <span className="mono">{e.seed}</span>
                                <span className="locked-label">
                                  <LockKeyhole size={10} />
                                  固定
                                </span>
                              </div>
                            </td>
                            <td>
                              <span
                                className={`status ${e.status}`}
                                title={e.error}
                              >
                                {["starting", "stopping"].includes(e.status) ? (
                                  <LoaderCircle size={12} className="spin" />
                                ) : (
                                  <span className="status-dot" />
                                )}
                                 {runtimeSession?.needsReconcile ? "待核对" : nativeMode && !core?.available ? "未就绪" : statusLabels[e.status]}
                               </span>
                               {nativeMode && e.error && <div className="cell-secondary runtime-recovery-note" role="status">{e.error}</div>}
                               {runtimeSession?.nextAction && <div className="cell-secondary runtime-recovery-note">{runtimeSession.nextAction}</div>}
                               {runtimeSession?.lastExitCode !== undefined && <div className="cell-secondary">上次退出码：{runtimeSession.lastExitCode}</div>}
                               {runtimeSession?.reconciledAt && <div className="cell-secondary">核对：{time(runtimeSession.reconciledAt)}</div>}
                            </td>
                            <td>
                              <span className="last-open">
                                {time(e.lastOpened)}
                              </span>
                            </td>
                            <td>
                              <div className="row-actions">
                                {runtimeSession?.persistencePending ? (
                                  <span className="cell-secondary runtime-recovery-note" role="status">结果待保存 · 修复存储后自动核对</span>
                                ) : runtimeSession?.needsReconcile ? (
                                  <Button className="soft-primary compact" disabled={runtimeActionPending} onClick={() => void handleRuntimeSessionAction(e.id, runtimeSession.sessionId, "reconcile")}>核对会话</Button>
                                ) : runtimeSession?.canForce ? (
                                  <Button className="danger compact" disabled={runtimeActionPending || e.status === "stopping"} onClick={() => void handleRuntimeSessionAction(e.id, runtimeSession.sessionId, "force")}>强制结束</Button>
                                ) : e.status === "running" || (nativeMode && (e.status === "starting" || !!runtimeSession?.pid)) ? (
                                  <Button
                                     className="stop-button compact"
                                     disabled={e.status === "stopping" || runtimeActionPending}
                                    onClick={() => stop([e.id])}
                                  >
                                    <Square size={12} />
                                     {e.status === "starting" ? "取消启动" : "关闭"}
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
                                    启动
                                  </Button>
                                )}
                                <div className="menu-wrap">
                                  <button
                                    className="icon-button"
                                    aria-label={`${e.name} 更多操作`}
                                    onClick={() =>
                                      setMenu(menu === e.id ? null : e.id)
                                    }
                                  >
                                    <Ellipsis size={19} />
                                  </button>
                                  {menu === e.id && (
                                    <div className="dropdown">
                                      <button onClick={() => openEdit(e)}>
                                        <Settings2 size={15} />
                                        编辑环境
                                      </button>
                                      <button onClick={() => openCreate(e)}>
                                        <Copy size={15} />
                                        按模板新建
                                      </button>
                                      <button onClick={() => openCookies(e)}>
                                        <FileJson size={15} />
                                        导入 Cookie
                                      </button>
                                      <button
                                        className="danger-text"
                                        onClick={() => {
                                          setDialog({
                                            kind: "delete",
                                            ids: [e.id],
                                          });
                                          setFormError("");
                                          setDeleteData(false);
                                          setMenu(null);
                                        }}
                                      >
                                        <Trash2 size={15} />
                                        移除环境
                                      </button>
                                    </div>
                                  )}
                                </div>
                              </div>
                            </td>
                          </tr>
                        );
                      })}
                    </tbody>
                  </table>
                </div>
                {visible.length === 0 && (
                  <Empty
                    title="没有符合条件的环境"
                    text="试试其他关键词，或创建一个新的浏览器环境。"
                    action={
                      <Button
                        onClick={() => {
                          setSearch("");
                          setGroup("全部分组");
                          setStatus("all");
                        }}
                      >
                        清除筛选
                      </Button>
                    }
                  />
                )}
                <div className="table-footer">
                  <span>
                    共 {visible.length} 个环境
                    <span className="footer-separator">·</span>每页 8 条
                  </span>
                  <div className="pagination">
                    <button
                      className="icon-button"
                      aria-label="上一页"
                      disabled={page <= 1}
                      onClick={() => setPage((p) => p - 1)}
                    >
                      <ChevronLeft size={16} />
                    </button>
                    <span>{Math.min(page, pageCount)}</span>
                    <span className="subtle-text">/ {pageCount}</span>
                    <button
                      className="icon-button"
                      aria-label="下一页"
                      disabled={page >= pageCount}
                      onClick={() => setPage((p) => p + 1)}
                    >
                      <ChevronRight size={16} />
                    </button>
                  </div>
                </div>
              </section>
              <div className="bottom-tip">
                <ShieldCheck size={17} />
                <span>
                  设备档案创建后固定保存。更换代理、关闭窗口或重新打开，都不会自动更换指纹。
                </span>
                <button onClick={() => navigate("guide")}>
                  了解环境规则
                  <ChevronRight size={14} />
                </button>
              </div>
            </>
          )}
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
          {route === "kernels" && nativeMode && <NativeKernelManager application={application} workspace={workspace} />}
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
            <>
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
            <section className="work-card">
              <div className="section-toolbar">
                <h2>最近操作</h2>
                <Button
                  onClick={() =>
                    download(
                      "prism-activity.json",
                      JSON.stringify(state.activities, null, 2),
                    )
                  }
                >
                  <Download size={15} />
                  导出记录
                </Button>
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
            </section>
          )}
          {route === "guide" && (
            <>
              <div className="guide-intro">
                <div className="guide-symbol">
                  <BookOpen size={28} />
                </div>
                <div>
                  <h2>从产品需求，走到可实现的页面</h2>
                  <p>
                    {nativeMode ? "环境配置由本机 SQLite 持久保存。真实内核、代理、Cookie 和完整恢复按后续任务接入；未接入功能不会使用模拟成功。" : "需求编号贯穿页面、数据模型与验收项。当前交互原型全部使用本地示例数据。"}
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
                          docTab === "prd"
                            ? "PRD.md"
                            : docTab === "kernel"
                              ? "KERNEL.md"
                              : "DEVELOPMENT.md",
                          docTab === "prd"
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
                                /(?:PRD|DEVELOPMENT|KERNEL)\.md/.test(href)
                              ) {
                                ev.preventDefault();
                                setDocTab(
                                  href.includes("KERNEL")
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
                      {docTab === "prd"
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
          <footer className="page-footer">
            <span>
              <Monitor size={13} />
              Windows 本地版<span className="footer-separator">·</span>
              {nativeMode ? "SQLite 本机持久化 · 真实启停开发中，待验收 · 仅直连" : "仅供交互验收，请勿输入真实凭据"}
            </span>
            <button
              onClick={() => {
                setDocTab("prd");
                navigate("guide");
              }}
            >
              <Link2 size={12} />
              {summary.req}
            </button>
          </footer>
        </main>
      </div>
      {drawer && (
        <div
          className="overlay"
          onMouseDown={(e) => {
            if (e.target === e.currentTarget && !generating) closeDrawer();
          }}
        >
          <div
            className="drawer"
            ref={overlayRef}
            role="dialog"
            aria-modal="true"
            aria-labelledby="drawer-title"
          >
            <div className="drawer-header">
              <div className="drawer-heading">
                <div className="small-icon">
                  <Fingerprint size={22} />
                </div>
                <div>
                  <h2 id="drawer-title">
                    {drawer.kind === "create"
                      ? "新建浏览器环境"
                      : "编辑浏览器环境"}
                  </h2>
                  <p>独立数据 · 固定指纹 · 专属网络配置</p>
                </div>
              </div>
              <button
                className="icon-button"
                aria-label="关闭环境配置"
                disabled={generating}
                onClick={closeDrawer}
              >
                <X size={20} />
              </button>
            </div>
            <div className="drawer-tabs">
              {(
                [
                  { id: "basic", name: "基础与代理", icon: Layers3 },
                  { id: "fingerprint", name: "设备指纹", icon: Fingerprint },
                  { id: "preferences", name: "浏览器偏好", icon: Settings2 },
                ] as const
              ).map((t) => (
                <button
                  key={t.id}
                  className={drawer.tab === t.id ? "active" : ""}
                  onClick={() => setDrawer({ ...drawer, tab: t.id })}
                >
                  <t.icon size={16} />
                  {t.name}
                </button>
              ))}
            </div>
            <div className="drawer-body">
              {drawer.tab === "basic" && (
                <>
                  <div className="form-section-title">
                    <span>01</span>
                    <h3>环境信息</h3>
                  </div>
                  <Field label="环境名称 *">
                    <input
                      autoComplete="off"
                      aria-label="环境名称"
                      value={drawer.environment.name}
                      placeholder="例如：北美主店"
                      onChange={(e) => patchDraft({ name: e.target.value })}
                    />
                  </Field>
                  <div className="field-row">
                    <Field label="环境分组">
                      <input
                        list="group-options"
                        value={drawer.environment.group}
                        onChange={(e) => patchDraft({ group: e.target.value })}
                      />
                      <datalist id="group-options">
                        {groups.map((g) => (
                          <option key={g} value={g} />
                        ))}
                      </datalist>
                    </Field>
                    {drawer.kind === "create" && (
                      <Field
                        label="创建数量"
                        hint={nativeMode ? "目前仅接入单条创建；持久批量任务待接入，不是总数配额。" : "每个实例生成独立种子与数据记录"}
                      >
                        <input
                          aria-label="创建数量"
                          type="number"
                          min="1"
                          step="1"
                          value={quantity}
                          onChange={(e) => setQuantity(Number(e.target.value))}
                        />
                      </Field>
                    )}
                  </div>
                  <Field label="备注">
                    <textarea
                      value={drawer.environment.note}
                      onChange={(e) => patchDraft({ note: e.target.value })}
                      rows={2}
                      placeholder="添加店铺用途或操作提醒"
                    />
                  </Field>
                  <div className="form-section-title">
                    <span>02</span>
                    <h3>代理网络</h3>
                  </div>
                  <Field
                    label="绑定代理"
                    hint="修改代理不会自动更换已保存的指纹和地区设置。"
                  >
                    <select
                      aria-label="绑定代理"
                      disabled={profileBusy || savePending}
                      value={drawer.environment.proxyId}
                      onChange={(e) => patchDraft({ proxyId: e.target.value })}
                    >
                      <option value="">本机网络（直连）</option>
                      {state.proxies.map((p) => (
                        <option key={p.id} value={p.id}>
                          {p.name} ·{" "}
                          {p.status === "failed"
                            ? "连接失败"
                            : p.status === "unchecked"
                              ? "待检查"
                              : p.type.toUpperCase()}
                        </option>
                      ))}
                    </select>
                  </Field>
                  <div className="form-note">
                    <ShieldCheck size={17} />
                    <span>
                      {drawer.environment.proxyId
                        ? "已绑定代理。未通过连接检查时将阻止启动，不自动直连。"
                        : "当前明确选择本机直连。需要独立出口时，请为该环境绑定代理。"}
                    </span>
                  </div>
                  <div className="form-section-title">
                    <span>03</span>
                    <h3>浏览器内核</h3>
                  </div>
                  <Field label="固定内核版本">
                    <select
                      aria-label="固定内核版本"
                      disabled={generating || savePending || profileBusy || (drawer.kind === "edit" && (!nativeMode || state.environments.find(environment => environment.id === drawer.environment.id)?.coreId !== "kernel-pending"))}
                      value={drawer.environment.coreId}
                      onChange={(e) => patchDraft({ coreId: e.target.value })}
                    >
                      {state.kernels.map((k) => (
                        <option key={k.id} value={k.id}>
                          {nativeMode ? `fingerprint-chromium ${k.version} · ${k.available ? "已核验" : "未就绪"} · ${k.id.slice(0, 8)}` : `Chromium ${k.version} · ${k.available ? "演示基线" : "待验证"}`}
                        </option>
                      ))}
                    </select>
                  </Field>
                  <p className="field-hint">
                    {nativeMode ? "先在内核页安装核验，再显式选择并生成档案预览。普通编辑不能切换已有精确构建；旧档案不自动重绑定，保存不代表已可启动。" : "实际内核：fingerprint-chromium。当前仅演示配置绑定，未安装浏览器内核。"}
                  </p>
                </>
              )}
              {drawer.tab === "fingerprint" && (
                <>
                  <div className="fingerprint-hero">
                    <div className="fingerprint-hero-icon">
                      <Fingerprint size={36} />
                    </div>
                    <div>
                      <Tag kind="blue-tag">Windows 桌面档案</Tag>
                      <h3>一次生成，持续使用</h3>
                      <p>配置保存后，重新打开继续沿用。</p>
                    </div>
                    <Button
                      className="soft-primary"
                      disabled={generating || savePending || profileBusy || !canGenerateProfile}
                      onClick={() => void generateProfile(true)}
                    >
                      {generating ? (
                        <LoaderCircle size={16} className="spin" />
                      ) : (
                        <Sparkles size={16} />
                      )}
                      重新生成
                    </Button>
                  </div>
                  <Field
                    label="固定指纹种子"
                    hint="重生成仅修改当前草稿，保存后生效；不会清除 Cookie。"
                  >
                    <div className="seed-input">
                      <Fingerprint size={16} />
                      <input
                        aria-label="固定指纹种子"
                        readOnly
                        value={drawer.environment.seed}
                      />
                      <LockKeyhole size={15} />
                    </div>
                  </Field>
                  <div className="field-row">
                    <Field label="操作系统">
                      <input readOnly value="Windows 桌面" />
                    </Field>
                    <Field label="CPU 线程数">
                      <select
                        disabled={generating || savePending || !canConfigureProfileField("cpu")}
                        aria-label="CPU线程偏好"
                        value={drawer.environment.cpu}
                        onChange={(e) => patchDraft({ cpu: e.target.value })}
                      >
                        <option value="auto">由内核按种子生成</option>
                        {["4", "8", "12", "16"].map((n) => (
                          <option key={n}>{n}</option>
                        ))}
                      </select>
                    </Field>
                  </div>
                  <div className="form-section-title">
                    <span>01</span>
                    <h3>语言与地区</h3>
                    <button
                      className="text-button"
                      disabled={generating || savePending || !canConfigureProfileField("acceptLanguages") || !canConfigureProfileField("timezone")}
                      onClick={() => {
                        const p = state.proxies.find(
                          (p) => p.id === drawer.environment.proxyId,
                        );
                        if (!p) {
                          notify("请先绑定代理，再采用其预设地区建议。", true);
                          return;
                        }
                        const r = regions[p.country];
                        if (!r) {
                          notify(
                            "该地区尚无预设，请手动选择语言和时区。",
                            true,
                          );
                          return;
                        }
                        patchDraft({
                          language: r.language,
                          timezone: r.timezone,
                        });
                        notify(
                          "已采用代理预设地区；真实 IP 归属待桌面服务检查。",
                        );
                      }}
                    >
                      采用代理地区
                    </button>
                  </div>
                  <div className="field-row">
                    <Field label="网站语言">
                      <select
                        disabled={generating || savePending || !canConfigureProfileField("acceptLanguages")}
                        aria-label="网站语言"
                        value={drawer.environment.language}
                        onChange={(e) =>
                          patchDraft({ language: e.target.value })
                        }
                      >
                        {Object.values(regions).map((r) => (
                          <option key={r.language} value={r.language}>
                            {r.language}
                          </option>
                        ))}
                      </select>
                    </Field>
                    <Field label="时区">
                      <select
                        disabled={generating || savePending || !canConfigureProfileField("timezone")}
                        aria-label="设备时区"
                        value={drawer.environment.timezone}
                        onChange={(e) =>
                          patchDraft({ timezone: e.target.value })
                        }
                      >
                        {Object.values(regions).map((r) => (
                          <option key={r.timezone}>{r.timezone}</option>
                        ))}
                      </select>
                    </Field>
                  </div>
                  {profileBusy && <p className="field-hint">环境正在运行、停止或等待会话核对，只能保存名称、分组和备注；实际目录空闲确认前不能改设备、代理及启动偏好。</p>}
                  <FingerprintRevisionPanel preview={drawer.fingerprint} history={drawer.history} native={nativeMode} stale={!profileIsFresh && !pendingConfiguration} busy={generating || savePending || profileBusy} canGenerate={canGenerateProfile} dataRef={drawer.userDataRef} onPreview={() => void generateProfile()} onRestore={revision => void previewProfileRestore(revision)} />
                </>
              )}
              {drawer.tab === "preferences" && (
                <>
                  <div className="form-section-title">
                    <span>01</span>
                    <h3>打开与显示</h3>
                  </div>
                  <Field
                    label="启动网址"
                    hint="每行一个完整网址，支持 http 和 https。"
                  >
                    <textarea
                      rows={4}
                      aria-label="启动网址"
                      disabled={profileBusy || savePending}
                      value={drawer.environment.urls}
                      onChange={(e) => patchDraft({ urls: e.target.value })}
                      placeholder="https://example.com"
                    />
                  </Field>
                  <div className="field-row">
                    <Field label="窗口宽度">
                      <input
                        disabled={generating || savePending || profileBusy}
                        type="number"
                        value={drawer.environment.width}
                        onChange={(e) =>
                          patchDraft({ width: Number(e.target.value) })
                        }
                      />
                    </Field>
                    <Field label="窗口高度">
                      <input
                        disabled={generating || savePending || profileBusy}
                        type="number"
                        value={drawer.environment.height}
                        onChange={(e) =>
                          patchDraft({ height: Number(e.target.value) })
                        }
                      />
                    </Field>
                  </div>
                  <div className="form-note">
                    <Info size={17} />
                    <span>
                      窗口尺寸控制可见窗口大小，不代表网站读取的完整屏幕指纹。
                    </span>
                  </div>
                  <label className="toggle-row">
                    <div>
                      <strong>恢复上次标签页</strong>
                      <p>再次打开时继续之前的工作。</p>
                    </div>
                    <input
                      type="checkbox"
                      checked={drawer.environment.restoreTabs}
                      disabled={profileBusy || savePending}
                      onChange={(e) =>
                        patchDraft({ restoreTabs: e.target.checked })
                      }
                    />
                  </label>
                  <div className="form-section-title">
                    <span>02</span>
                    <h3>数据保存</h3>
                  </div>
                  <div className="policy-item">
                    <ShieldCheck size={19} />
                    <div>
                      <strong>独立数据目录</strong>
                      <p>{nativeMode ? "首次启动创建本环境独立目录；停止与档案修改保留数据，不与其他环境共享。" : "桌面版为此环境分配独立浏览器目录。"}</p>
                    </div>
                  </div>
                  <div className="policy-item">
                    <LockKeyhole size={19} />
                    <div>
                      <strong>保留登录数据</strong>
                      <p>{nativeMode ? "浏览器自身保存本环境数据；管理端 Cookie 导入仍待接入。实际隔离与重开保留待统一验收。" : "关闭窗口和修改代理时保留原有 Cookie。"}</p>
                    </div>
                  </div>
                </>
              )}
              {formError && (
                <div className="form-error" role="alert">
                  <TriangleAlert size={16} />
                  {formError}
                </div>
              )}
            </div>
            <div className="drawer-footer">
              <span>
                <ShieldCheck size={14} />
                {pendingConfiguration ? "仅保存待绑定配置 · 未生成可用档案，不能启动" : !canSaveProfile ? "请在设备指纹页选择内核、生成并查看预览" : nativeMode ? "保存到本机 SQLite · 提交成功才完成" : "仅保存在当前浏览器的演示数据"}
              </span>
              <div>
                <Button disabled={generating || savePending} onClick={closeDrawer}>
                  取消
                </Button>
                <Button
                  className="primary"
                  disabled={generating || savePending || !canSaveProfile}
                  onClick={saveEnvironment}
                >
                  <Check size={16} />
                  {drawer.kind === "create"
                    ? `创建${quantity > 1 ? ` ${quantity} 个` : ""}环境`
                    : "保存配置"}
                </Button>
              </div>
            </div>
          </div>
        </div>
      )}
      {dialog && (
        <div className="overlay modal-overlay">
          <div
            className={`modal ${["proxy", "cookies"].includes(dialog.kind) ? "wide-modal" : ""}`}
            ref={overlayRef}
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
                    : dialog.kind === "cookies"
                      ? "导入 Cookie"
                      : dialog.kind === "delete"
                        ? "移除浏览器环境"
                        : dialog.kind === "restore"
                          ? "恢复原型快照"
                          : dialog.kind === "kernel"
                            ? "内核能力与接入"
                            : "调整环境分组"}
              </h2>
              <button
                className="icon-button"
                aria-label="关闭对话框"
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
                  <Field label="代理内容">
                    <textarea
                      aria-label="代理内容"
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
                      解析并预览
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
              {dialog.kind === "cookies" && (
                <>
                  <div className="target-info">
                    <Fingerprint size={17} />
                    {state.environments.find((e) => e.id === dialog.id)?.name}
                    <small className="mono">{dialog.id.slice(0, 12)}</small>
                    <Tag>示例数据导入</Tag>
                  </div>
                  <p className="modal-intro">
                    粘贴 JSON 数组或 Netscape
                    文本。保留空值、过期时间及分区字段；当前只写入原型记录，不会写入真实浏览器。
                  </p>
                  <Field label="Cookie 内容（JSON / Netscape）">
                    <textarea
                      aria-label="Cookie 内容"
                      className="code-input"
                      rows={7}
                      value={cookieText}
                      onChange={(e) => {
                        setCookieText(e.target.value);
                        setCookieResult(null);
                      }}
                      placeholder={
                        '[{"name":"session","value":"demo","domain":"example.com","path":"/"}]'
                      }
                    />
                  </Field>
                  <input
                    ref={cookieFile}
                    type="file"
                    accept=".json,.txt"
                    hidden
                    onChange={async (e) => {
                      const f = e.target.files?.[0];
                      if (f) {
                        setCookieText(await f.text());
                        setCookieResult(null);
                      }
                      e.target.value = "";
                    }}
                  />
                  <div className="inline-actions">
                    <Button onClick={() => cookieFile.current?.click()}>
                      <ArrowUpFromLine size={15} />
                      选择文件
                    </Button>
                    <button
                      className="text-button"
                      onClick={() => {
                        setCookieText(
                          JSON.stringify(
                            [
                              {
                                name: "example_session",
                                value: "demo-only",
                                domain: "example.com",
                                path: "/",
                                secure: true,
                              },
                              {
                                name: "empty_preference",
                                value: "",
                                domain: "example.com",
                                path: "/",
                              },
                            ],
                            null,
                            2,
                          ),
                        );
                        setCookieResult(null);
                      }}
                    >
                      填入示例
                    </button>
                    <Button
                      className="soft-primary"
                      onClick={() => setCookieResult(parseCookies(cookieText))}
                    >
                      校验并预览
                    </Button>
                  </div>
                  {cookieResult && (
                    <div className="import-preview">
                      <h3>
                        校验结果{" "}
                        <span>
                          {cookieResult.cookies.length} 条有效 /{" "}
                          {cookieResult.errors.length} 条错误
                        </span>
                      </h3>
                      {cookieResult.cookies.map((c, i) => (
                        <div key={i}>
                          <CheckCircle2 size={15} />
                          <strong>{c.name}</strong>
                          <span>
                            {c.domain} ·{" "}
                            {c.value === "" ? "空值保留" : "值已隐藏"}
                            {Number(c.expires ?? c.expirationDate) > 0 &&
                            Number(c.expires ?? c.expirationDate) <
                              Date.now() / 1000
                              ? " · 已过期，仅保留解析记录"
                              : ""}
                          </span>
                        </div>
                      ))}
                      {cookieResult.errors.map((err) => (
                        <div className="invalid-row" key={err}>
                          <TriangleAlert size={15} />
                          {err}
                        </div>
                      ))}
                    </div>
                  )}
                </>
              )}
              {dialog.kind === "delete" && (
                <>
                  <div className="warning-illustration">
                    <Trash2 size={26} />
                  </div>
                  <p>
                    将从工作区移除 <strong>{dialog.ids.length}</strong>{" "}
                    个环境。正在运行的环境必须先关闭。
                  </p>
                  <label className="checkbox-card">
                    <input
                      type="checkbox"
                      checked={deleteData}
                      onChange={(e) => setDeleteData(e.target.checked)}
                    />
                    <span>
                      <strong>同时删除浏览器数据</strong>
                      <small>
                        桌面版将删除对应独立目录。当前原型仅移除示例记录，不操作真实文件。
                      </small>
                    </span>
                  </label>
                  <p className="field-hint">建议先到备份页面保存快照。</p>
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
                  <p>
                    将选中的 <strong>{selected.length}</strong>{" "}
                    个环境归入指定分组。没有选中环境时，请先在列表勾选。
                  </p>
                  <Field label="分组名称">
                    <input
                      aria-label="分组名称"
                      value={groupName}
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
              <Button onClick={() => setDialog(null)}>取消</Button>
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
                    setDialog(null);
                    notify(`已导入 ${nodes.length} 条代理，待检查`);
                  }}
                >
                  导入 {proxyRows.filter((r) => r.node).length} 条有效代理
                </Button>
              )}
              {dialog.kind === "cookies" && (
                <Button
                  className="primary"
                  disabled={
                    !cookieResult ||
                    !cookieResult.cookies.length ||
                    cookieResult.errors.length > 0
                  }
                  onClick={() => {
                    if (!cookieResult) return;
                    if (!update((s) => ({
                      ...s,
                      environments: s.environments.map((e) =>
                        e.id === dialog.id
                          ? {
                              ...e,
                              cookies: mergeCookies(
                                e.cookies,
                                cookieResult.cookies,
                              ),
                            }
                          : e,
                      ),
                    }), log(
                      "导入示例 Cookie",
                      state.environments.find((e) => e.id === dialog.id)
                        ?.name || "",
                      `已将 ${cookieResult.cookies.length} 条 Cookie 保真写入原型记录；未写真实浏览器。`,
                    ))) return;
                    setDialog(null);
                    notify(
                      `已导入 ${cookieResult.cookies.length} 条示例 Cookie`,
                    );
                  }}
                >
                  导入到原型记录
                </Button>
              )}
              {dialog.kind === "delete" && (
                <Button className="danger" onClick={removeEnvironments}>
                  确认移除
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
                  disabled={!selected.length || !groupName.trim()}
                  onClick={() => {
                    if (!update((s) => ({
                      ...s,
                      environments: s.environments.map((e) =>
                        selected.includes(e.id)
                          ? { ...e, group: groupName.trim() }
                          : e,
                      ),
                    }), log(
                      "调整分组",
                      `${selected.length} 个环境`,
                      `已归入分组 ${groupName.trim()}。`,
                    ))) return;
                    setDialog(null);
                    notify("分组已更新");
                  }}
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
      {storageIssue && (
        <div className="overlay modal-overlay">
          <div
            className="modal"
            role="alertdialog"
            aria-modal="true"
            aria-label="工作区需要处理"
          >
            <div className="modal-header">
              <h2>工作区需要处理</h2>
            </div>
            <div className="modal-body">
              <p>{storageIssue}</p>
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
