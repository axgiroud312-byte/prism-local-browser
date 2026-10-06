import { Folder, Pencil, Search } from "lucide-react";
import { useState } from "react";
import type { Environment } from "../domain";
import { ReferenceButton as Button } from "./ReferenceUi";

/** Groups are labels on environments, NOT independent backend entities. */
export function EnvironmentGroups({ groups, environments, native, onFilter, onAssign }: {
  groups: string[]; environments: Environment[]; native: boolean;
  onFilter: (group: string) => void; onAssign: (ids: string[], group: string) => void;
}) {
  const [search, setSearch] = useState("");
  return <section className="environment-groups-page">
    <div className="group-category">环境分组</div>
    <div className="group-list-panel">
      <div className="group-list-tools"><label className="group-name-search">分组名称：<input aria-label="搜索分组名称" placeholder="请输入" value={search} onChange={event => setSearch(event.target.value)} /><Search size={14} /></label><span>分组来自环境记录；{native ? "成员数和修改范围仅指已读取的当前页" : "重命名会修改该分组的现有成员"}。</span></div>
      <table className="reference-group-table"><thead><tr><th>分组名称</th><th>{native ? "当前页环境数" : "分组环境数"}</th><th>操作</th></tr></thead><tbody>
        {groups.filter(group => group.toLowerCase().includes(search.toLowerCase())).map(group => {
          const members = environments.filter(environment => environment.group === group);
          return <tr key={group}><td><Folder size={14} />{group}</td><td>{members.length}{native && "（当前页）"}</td><td><Button className="compact" onClick={() => onFilter(group)}>查看环境</Button><button className="icon-button" disabled={!members.length} aria-label={`修改 ${group} 分组`} title={native ? "只修改已读取的当前页成员，不是重命名整组" : "修改现有成员的分组名称"} onClick={() => onAssign(members.map(environment => environment.id), group)}><Pencil size={15} /></button></td></tr>;
        })}
      </tbody></table>
      {!groups.filter(group => group.toLowerCase().includes(search.toLowerCase())).length && <p className="group-empty">没有符合条件的分组。为环境设置分组后会在这里出现。</p>}
      <p className="group-crop-note">没有独立空分组、归属用户或排序服务。新增名称请在新建环境或“调整分组”时填写；这里不提供虚假的独立分组删除。</p>
    </div>
  </section>;
}
