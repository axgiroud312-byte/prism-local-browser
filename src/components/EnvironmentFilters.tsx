import { useEffect, useState, type RefObject } from "react";
import { ChevronDown, Copy, HardDrive, History, ListFilter, Network, Plus, RefreshCw, Search, Trash2 } from "lucide-react";
import { ReferenceButton as Button, ReferencePopover } from "./ReferenceUi";

export function EnvironmentFilters({ search, group, status, groups, total, running, errors, selected, pageSelected, native, blocked, searchRef, onSearch, onGroup, onStatus, onClear, onCreate, onOpen, onStop, onAssign, onRemove, onCancelSelection, onRefresh, onBackup, onHistory, onRecycle, onClone, onProxyAssign, onRetryReleased }: {
  search: string; group: string; status: string; groups: string[]; total: number; running: number; errors: number;
  selected: number; pageSelected: number; native: boolean; blocked: boolean; searchRef: RefObject<HTMLInputElement | null>;
  onSearch: (value: string) => void; onGroup: (value: string) => void; onStatus: (value: string) => void; onClear: () => void;
  onCreate: () => void; onOpen: () => void; onStop: () => void; onAssign: () => void; onRemove: () => void; onCancelSelection: () => void;
  onRefresh: () => void; onBackup: () => void; onHistory: () => void; onRecycle: () => void; onClone: () => void; onProxyAssign: () => void; onRetryReleased: () => void;
}) {
  const [popup, setPopup] = useState<{ kind: "create" | "group" | "advanced" | "more"; anchor: HTMLElement } | null>(null);
  const [groupSearch, setGroupSearch] = useState("");
  const [draftSearch, setDraftSearch] = useState("");
  const [draftGroup, setDraftGroup] = useState(group);
  const [draftStatus, setDraftStatus] = useState(status);
  useEffect(() => { if (blocked) setPopup(null); }, [blocked]);
  const toggle = (kind: NonNullable<typeof popup>["kind"], anchor: HTMLElement) => setPopup(previous => previous?.kind === kind ? null : { kind, anchor });
  const act = (action: () => void) => { popup?.anchor.focus(); setPopup(null); action(); };
  return <>
    <section className="environment-search-toolbar" aria-label="环境搜索与创建">
      <div className="split-button"><Button className="primary" onClick={onCreate}><Plus size={20} />新建环境</Button><button className="primary split-tail" aria-label="创建菜单" aria-expanded={popup?.kind === "create"} onClick={event => toggle("create", event.currentTarget)}><ChevronDown size={12} /></button></div>
      <span className="environment-count">已保存 <b>{total}</b> · 运行 <b>{running}</b><span className="count-boundary"> · 无数量配额</span></span>
      <Button className="primary" onClick={onBackup}><HardDrive size={14} />导入 / 备份</Button>
      <label className="toolbar-group"><select aria-label="筛选分组" value={group} onChange={event => onGroup(event.target.value)}><option>全部分组</option>{groups.map(value => <option key={value}>{value}</option>)}</select></label>
      <label className="environment-search"><span>名称 / 编号 / 备注：</span><input ref={searchRef} aria-label="搜索环境" value={search} placeholder="请输入" onChange={event => onSearch(event.target.value)} /><Search size={15} /></label>
      <button className="text-button advanced-search-trigger" aria-expanded={popup?.kind === "advanced"} onClick={event => { setDraftSearch(search); setDraftGroup(group); setDraftStatus(status); toggle("advanced", event.currentTarget); }}><ListFilter size={14} />高级搜索</button>
    </section>
    <div className="environment-list-toolbar">
      <div className="environment-filter-tabs">
        <button className={status === "all" ? "selected" : ""} onClick={() => onStatus("all")}>全部</button>
        <button className={status === "ready" ? "selected" : ""} onClick={() => onStatus("ready")}>待启动</button>
        <button className={status === "running" ? "selected" : ""} onClick={() => onStatus("running")}>已打开</button>
        <button className={status === "error" ? "selected" : ""} onClick={() => onStatus("error")}>需处理{errors ? ` ${errors}` : ""}</button>
        <button aria-label="分组筛选" aria-expanded={popup?.kind === "group"} className={group !== "全部分组" ? "selected" : ""} onClick={event => toggle("group", event.currentTarget)}>分组<ListFilter size={13} /></button>
      </div>
      {selected > 0 && <div className="selection-bar"><span title={`已选择 ${selected} 个环境；当前页 ${pageSelected} 个；筛选和翻页不会扩大操作范围`}>已选：<strong>{selected}</strong></span><button className="text-button" onClick={onCancelSelection}>取消选择</button></div>}
      <div className="environment-toolbar-actions">
        {selected > 0 && <><Button className="primary" onClick={onOpen}>批量打开</Button><Button className="primary" onClick={onStop}>批量关闭</Button><Button onClick={onAssign}>调整分组</Button></>}
        {native && <button className="reference-square" aria-label="打开回收区" title="打开回收区" onClick={onRecycle}><Trash2 size={16} /></button>}
        <Button className="primary" aria-expanded={popup?.kind === "more"} onClick={event => toggle("more", event.currentTarget)}>更多操作<ChevronDown size={12} /></Button>
        <button className="reference-square" aria-label="刷新环境列表" title="刷新环境列表" onClick={onRefresh}><RefreshCw size={18} /></button>
      </div>
    </div>
    {popup?.kind === "group" && <ReferencePopover anchor={popup.anchor} align="end" onClose={() => setPopup(null)} width={260} label="分组筛选菜单" className="environment-group-popup" role="dialog">
      <div className="group-popup-search"><input aria-label="搜索分组" value={groupSearch} placeholder="请输入" onChange={event => setGroupSearch(event.target.value)} /><Button className="primary" onClick={() => { setGroupSearch(""); onGroup("全部分组"); }}>重置</Button></div>
      <button onClick={() => act(() => onGroup("全部分组"))}>全部分组</button>
      {groups.filter(value => value.toLowerCase().includes(groupSearch.toLowerCase())).map(value => <button key={value} onClick={() => act(() => onGroup(value))}>{value}</button>)}
      {groupSearch && !groups.some(value => value.toLowerCase().includes(groupSearch.toLowerCase())) && <p className="popup-empty">没有符合条件的分组</p>}
    </ReferencePopover>}
    {popup?.kind === "advanced" && <ReferencePopover anchor={popup.anchor} align="end" width={432} label="高级搜索" onClose={() => setPopup(null)} role="dialog" className="environment-advanced-search">
      <header>搜索项目<span>仅现有服务支持的条件</span></header>
      <div className="advanced-filter-body"><label className="reference-field"><span>关键词</span><input aria-label="高级搜索关键词" value={draftSearch} placeholder="名称 / 编号 / 备注" onChange={event => setDraftSearch(event.target.value)} /></label>
        <label className="reference-field"><span>分组名称</span><select aria-label="高级搜索分组" value={draftGroup} onChange={event => setDraftGroup(event.target.value)}><option>全部分组</option>{groups.map(value => <option key={value}>{value}</option>)}</select></label>
        <label className="reference-field"><span>运行状态</span><select aria-label="高级搜索状态" value={draftStatus} onChange={event => setDraftStatus(event.target.value)}><option value="all">全部状态</option><option value="ready">待启动</option><option value="running">运行中</option><option value="starting">启动中</option><option value="stopping">关闭中</option><option value="error">需处理</option></select></label>
        <p>关键词匹配名称、编号或备注；所有条件一起生效。筛选不改变已经勾选的环境。</p>
      </div><footer><button className="text-button" onClick={() => { setDraftSearch(""); setDraftGroup("全部分组"); setDraftStatus("all"); }}>重置</button><Button className="primary" onClick={() => act(() => { onSearch(draftSearch); onGroup(draftGroup); onStatus(draftStatus); })}>开始搜索</Button></footer>
    </ReferencePopover>}
    {popup?.kind === "create" && <ReferencePopover anchor={popup.anchor} onClose={() => setPopup(null)} align="end" label="创建菜单">
      <button onClick={() => act(onCreate)}><Plus />新建 / 批量创建</button><button onClick={() => act(onBackup)}><HardDrive />导入 / 备份</button>
      {native && <button onClick={() => act(onHistory)}><History />批次与逐项结果</button>}
    </ReferencePopover>}
    {popup?.kind === "more" && <ReferencePopover anchor={popup.anchor} onClose={() => setPopup(null)} align="end" label="更多操作">
      <button disabled={!selected} onClick={() => act(onAssign)}><ListFilter />调整分组</button>
      {native && <><button onClick={() => act(onHistory)}><History />批次与逐项结果</button><button disabled={!selected} onClick={() => act(onClone)}><Copy />复制配置（新身份）</button><button disabled={!selected} onClick={() => act(onProxyAssign)}><Network />明确分配代理</button><button disabled={!selected} onClick={() => act(onBackup)}><HardDrive />完整备份所选</button><button disabled={!selected} onClick={() => act(onRetryReleased)}><RefreshCw />仅重试已释放资源的失败项</button></>}
      <button disabled={!selected} className="danger-text" onClick={() => act(onRemove)}><Trash2 />移除所选环境</button>
      <button onClick={() => act(onClear)}><RefreshCw />清除筛选</button>
    </ReferencePopover>}
  </>;
}
