import fs from "node:fs";
import path from "node:path";
import assert from "node:assert/strict";

const docs = [...new Set([
  "README.md",
  "AGENTS.md",
  "docs/ENGINEERING.md",
  "docs/SPEC.md",
  "docs/ISSUES.md",
  ".github/pull_request_template.md",
  "docs/PRD.md",
  "docs/DEVELOPMENT.md",
  "docs/KERNEL.md",
  "docs/TRACEABILITY.md",
  "docs/ACCEPTANCE.md",
  "THIRD_PARTY_NOTICES.md",
  ...fs.readdirSync("docs", { recursive: true }).filter(file => file.endsWith(".md")).map(file => path.join("docs", file)),
])];
const ids = [
  "ENV-001",
  "ENV-002",
  "ENV-003",
  "FP-001",
  "FP-002",
  "PRX-001",
  "CK-001",
  "CORE-001",
  "BKP-001",
  "DATA-001",
  "UX-001",
  "DOC-001",
];
for (const file of docs) {
  const content = fs.readFileSync(file, "utf8");
  assert(
    !/[A-Z]:[\\/]Users[\\/]/i.test(content),
    `${file}: personal absolute path`,
  );
  for (const match of content.matchAll(/\[[^\]]*\]\(([^)]+)\)/g)) {
    const href = match[1].split("#")[0];
    if (!href || /^(?:https?:|mailto:|\/)/.test(href)) continue;
    assert(
      fs.existsSync(path.resolve(path.dirname(file), decodeURIComponent(href))),
      `${file}: broken link ${href}`,
    );
  }
}
for (const file of ["docs/PRD.md", "docs/TRACEABILITY.md"]) {
  const content = fs.readFileSync(file, "utf8");
  for (const id of ids) assert(content.includes(id), `${file}: missing ${id}`);
}
const app = fs.readFileSync("src/App.tsx", "utf8");
for (const route of [
  "environments",
  "proxies",
  "kernels",
  "backups",
  "activity",
  "guide",
]) {
  assert(
    app.includes(`'${route}'`) || app.includes(`"${route}"`),
    `missing route ${route}`,
  );
  assert(
    fs.readFileSync("docs/TRACEABILITY.md", "utf8").includes(route),
    `unmapped route ${route}`,
  );
}
for (const doc of ["PRD.md", "DEVELOPMENT.md", "KERNEL.md"])
  assert(app.includes(`${doc}?raw`), `${doc} is not linked into the app`);
console.log(
  `PASS: ${docs.length} documents, local links, 12 requirements, 6 routes, 3 embedded documents.`,
);
