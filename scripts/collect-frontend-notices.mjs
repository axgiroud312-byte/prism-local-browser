import { readFile, readdir, mkdir, writeFile } from "node:fs/promises";
import { resolve, join } from "node:path";
import { fileURLToPath } from "node:url";

const root = fileURLToPath(new URL("../", import.meta.url));
const lock = JSON.parse(await readFile(join(root, "package-lock.json"), "utf8"));
const lines = ["Prism Browser - frontend runtime component licenses", "Generated from locked installed production packages (including transitive packages) and Vite's injected runtime helpers. The Vite CLI, test tools and Chromium are not distributed."];
let count = 0;
for (const [relative, entry] of Object.entries(lock.packages).sort(([a], [b]) => a.localeCompare(b))) {
  if (!relative || entry.link || entry.dev && relative !== "node_modules/vite") continue;
  const directory = resolve(root, relative);
  const pkg = JSON.parse(await readFile(join(directory, "package.json"), "utf8"));
  if (pkg.version !== entry.version) throw new Error(`Installed version differs from lock: ${pkg.name}`);
  const notices = (await readdir(directory, { withFileTypes: true })).filter(file => file.isFile() && /^(?:licen[sc]e|copying|notice|copyright)(?:$|[-.])/i.test(file.name)).sort((a, b) => a.name.localeCompare(b.name));
  if (!notices.length) throw new Error(`Missing runtime license: ${pkg.name}@${pkg.version}`);
  for (const notice of notices) {
    lines.push(`\n=== ${pkg.name}@${pkg.version} / ${notice.name} ===\n`, await readFile(join(directory, notice.name), "utf8"));
  }
  count++;
}
const destination = join(root, "build/bin/FRONTEND-THIRD-PARTY-NOTICES.txt");
await mkdir(join(root, "build/bin"), { recursive: true });
await writeFile(destination, lines.join("\n"), "utf8");
console.log(`Collected licenses for ${count} locked frontend runtime packages.`);
