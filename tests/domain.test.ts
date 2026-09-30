import assert from "node:assert/strict";
import test from "node:test";
import {
  createSnapshot,
  launchError,
  mergeCookies,
  parseCookies,
  parseProxyText,
  parseSnapshot,
  restoreSnapshot,
  seedState,
  uniqueSeed,
} from "../src/domain.ts";
import type { Cookie, Snapshot, State, Status } from "../src/domain.ts";

function stoppedState(): State {
  const state = seedState();
  state.environments = state.environments.map((env) => ({
    ...env,
    status: "ready",
  }));
  return state;
}

function snapshotText(edit?: (snapshot: Snapshot) => void): string {
  const snapshot = createSnapshot(seedState());
  edit?.(snapshot);
  return JSON.stringify(snapshot);
}

test("snapshot rejects malformed render fields and normalizes cookie paths", () => {
  assert.throws(() =>
    parseSnapshot(
      snapshotText((s) => {
        s.environments[0].lastOpened = "bad-date";
      }),
    ),
  );
  assert.throws(() =>
    parseSnapshot(
      snapshotText((s) => {
        (s.kernels[0] as unknown as Record<string, unknown>).note = {
          bad: true,
        };
      }),
    ),
  );
  assert.throws(() =>
    parseSnapshot(
      snapshotText((s) => {
        s.proxies[0].country = "UNKNOWN";
      }),
    ),
  );
  const input = snapshotText((s) => {
    s.environments[0].cookies = [
      { name: "session", value: "", domain: "example.com" } as Cookie,
    ];
  });
  assert.equal(parseSnapshot(input).environments[0].cookies[0].path, "/");
});

test("uniqueSeed retries a collision and returns an in-range, unused integer string", (t) => {
  const randomValues = [0, 1];
  const random = t.mock.method(
    globalThis.crypto,
    "getRandomValues",
    (array: Uint32Array) => {
      assert.ok(
        randomValues.length,
        "seed generation should finish after the unused candidate",
      );
      array[0] = randomValues.shift()!;
      return array;
    },
  );
  const seed = uniqueSeed(["1", "900000"]);
  assert.match(seed, /^\d+$/);
  assert.ok(Number(seed) >= 1 && Number(seed) <= 2147483647);
  assert.ok(!["1", "900000"].includes(seed));
  assert.equal(random.mock.callCount(), 2);
});

test("uniqueSeed keeps repeated generations within the supported range and distinct", () => {
  const seeds: string[] = [];
  for (let i = 0; i < 100; i++) {
    const seed = uniqueSeed(seeds);
    assert.ok(Number.isInteger(Number(seed)));
    assert.ok(Number(seed) >= 1 && Number(seed) <= 2147483647);
    assert.ok(!seeds.includes(seed));
    seeds.push(seed);
  }
});

test("launchError blocks a missing or unavailable kernel", () => {
  const state = stoppedState();
  for (const coreId of ["missing-core", "core-150"]) {
    assert.match(
      launchError({ ...state.environments[0], coreId }, state)!,
      /内核/,
    );
  }
});

test("launchError fails closed for missing, failed, and unchecked assigned proxies", () => {
  const state = stoppedState();
  for (const proxyId of ["missing-proxy", "px-de", "px-sg"]) {
    const error = launchError({ ...state.environments[0], proxyId }, state);
    assert.ok(
      error,
      `assigned proxy ${proxyId} must not silently become direct`,
    );
    assert.match(error, /不会切换为直连/);
  }
});

test("launchError permits a checked proxy and an explicitly direct environment", () => {
  const state = stoppedState();
  assert.equal(
    launchError({ ...state.environments[0], proxyId: "px-us" }, state),
    null,
  );
  assert.equal(
    launchError({ ...state.environments[0], proxyId: "" }, state),
    null,
  );
});

test("proxy import supports HTTP, HTTPS, SOCKS5, and a bare host with default ports", () => {
  const rows = parseProxyText(
    "http://proxy.example\nhttps://proxy.example\nsocks5://proxy.example\nproxy.example:8080",
  );
  assert.equal(rows.length, 4);
  assert.deepEqual(
    rows.map((row) => row.error),
    [undefined, undefined, undefined, undefined],
  );
  assert.deepEqual(
    rows.map((row) => [row.node?.type, row.node?.port]),
    [
      ["http", 80],
      ["https", 443],
      ["socks5", 1080],
      ["http", 8080],
    ],
  );
  assert.ok(rows.every((row) => row.node?.status === "unchecked"));
  assert.equal(new Set(rows.map((row) => row.node?.id)).size, 4);
});

test("proxy import decodes escaped credentials without confusing URL delimiters", () => {
  const [row] = parseProxyText(
    "socks5://test%40operator:p%3Aa%23s%25s@proxy.example:1081",
  );
  assert.equal(row.error, undefined);
  assert.equal(row.node?.host, "proxy.example");
  assert.equal(row.node?.username, "test@operator");
  assert.equal(row.node?.password, "p:a#s%s");
  assert.equal(row.node?.port, 1081);
});

test("proxy import retains an IPv6 host and its separate port", () => {
  const [row] = parseProxyText("https://[2001:db8::1]:8443");
  assert.equal(row.error, undefined);
  assert.equal(row.node?.host.replace(/^\[|\]$/g, ""), "2001:db8::1");
  assert.equal(row.node?.port, 8443);
  assert.equal(row.node?.type, "https");
});

test("proxy import skips comments and blank lines while retaining original error line numbers", () => {
  const rows = parseProxyText(
    "# Example proxies\n\nhttp://proxy.example:8080\nftp://proxy.example:21\nhttps://proxy.example/path",
  );
  assert.deepEqual(
    rows.map((row) => row.line),
    [3, 4, 5],
  );
  assert.ok(rows[0].node);
  assert.ok(rows[1].error);
  assert.ok(rows[2].error);
  assert.equal(rows[1].node, undefined);
  assert.equal(rows[2].node, undefined);
});

test("proxy import rejects invalid ports, malformed credentials, and URL query or fragment fields", () => {
  for (const input of [
    "http://proxy.example:0",
    "http://proxy.example:65536",
    "http://proxy.example:abc",
    "http://%ZZ:example@proxy.example:8080",
    "http://proxy.example:8080?extra=true",
    "http://proxy.example:8080#extra",
    "socks5://",
  ]) {
    const [row] = parseProxyText(input);
    assert.ok(row.error, `invalid proxy should be rejected: ${input}`);
    assert.equal(row.node, undefined);
  }
});

test("JSON cookies preserve empty values, session metadata, expiry fields, and partition keys", () => {
  const input = [
    { name: "empty", value: "", domain: ".example.com", session: true },
    {
      name: "session",
      value: "example",
      domain: "example.com",
      path: "/",
      expires: -1,
      httpOnly: true,
    },
    {
      name: "persistent",
      value: "example",
      domain: "example.com",
      path: "/shop",
      expires: 2000000000,
      secure: true,
      sameSite: "None",
    },
    {
      name: "exported",
      value: "example",
      domain: "example.com",
      path: "/",
      expirationDate: 2000000001,
      hostOnly: true,
    },
    {
      name: "partitioned",
      value: "example",
      domain: "example.com",
      path: "/",
      partitionKey: {
        topLevelSite: "https://store.example",
        hasCrossSiteAncestor: false,
      },
    },
  ];
  const result = parseCookies(JSON.stringify(input));
  assert.deepEqual(result.errors, []);
  assert.deepEqual(
    result.cookies,
    input.map((cookie) => ({ ...cookie, path: cookie.path || "/" })),
  );
  assert.equal(result.cookies[0].value, "");
  assert.equal(Object.hasOwn(result.cookies[0], "expires"), false);
  assert.equal(Object.hasOwn(result.cookies[3], "expires"), false);
});

test("JSON cookies report invalid records while keeping valid records available for review", () => {
  const input = [
    { name: "valid", value: "", domain: "example.com" },
    { name: "invalid-domain", value: "example", domain: "https://example.com" },
    {
      name: "invalid-path",
      value: "example",
      domain: "example.com",
      path: "shop",
    },
    {
      name: "invalid-expiry",
      value: "example",
      domain: "example.com",
      expires: "tomorrow",
    },
    { name: "invalid-value", value: 123, domain: "example.com" },
  ];
  const result = parseCookies(JSON.stringify(input));
  assert.equal(result.cookies.length, 1);
  assert.equal(result.cookies[0].name, "valid");
  assert.equal(result.errors.length, 4);
  assert.ok(parseCookies("{}").errors.length);
  assert.ok(parseCookies("[broken-json").errors.length);
});

test("Netscape cookies parse seven tab-separated columns, HttpOnly, and an empty session value", () => {
  const input = [
    "# Netscape HTTP Cookie File",
    "#HttpOnly_.example.com\tTRUE\t/\tTRUE\t2000000000\tsid\texample",
    "example.com\tFALSE\t/cart\tFALSE\t0\tempty\t",
  ].join("\n");
  const result = parseCookies(input);
  assert.deepEqual(result.errors, []);
  assert.equal(result.cookies.length, 2);
  const [persistent, session] = result.cookies;
  assert.equal(persistent.domain, ".example.com");
  assert.equal(persistent.httpOnly, true);
  assert.equal(persistent.secure, true);
  assert.equal(persistent.expires ?? persistent.expirationDate, 2000000000);
  assert.equal(session.name, "empty");
  assert.equal(session.value, "");
  assert.equal(session.path, "/cart");
  assert.ok(
    session.session === true ||
      (session.expires === undefined && session.expirationDate === undefined) ||
      session.expires === -1,
    "Netscape expiry 0 must retain session semantics, not become an expired persistent cookie",
  );
});

test("Netscape cookie import rejects an incomplete row", () => {
  assert.ok(
    parseCookies("example.com\tFALSE\t/\tFALSE\t0\tmissing-value-column").errors
      .length,
  );
});

test("cookie merge replaces only the matching domain, path, name, and partition", () => {
  const base: Cookie = {
    name: "sid",
    value: "old",
    domain: "example.com",
    path: "/",
  };
  const existing: Cookie[] = [
    base,
    { ...base, path: "/shop" },
    { ...base, domain: "other.example" },
    { ...base, name: "other" },
    { ...base, partitionKey: { topLevelSite: "https://one.example" } },
    { ...base, partitionKey: { topLevelSite: "https://two.example" } },
  ];
  const updated = { ...base, value: "replacement", httpOnly: true };
  const partitionUpdated = { ...existing[4], value: "partition-replacement" };
  const merged = mergeCookies(existing, [updated, partitionUpdated]);
  assert.equal(merged.length, existing.length);
  assert.deepEqual(merged[0], updated);
  assert.deepEqual(merged[4], partitionUpdated);
  assert.deepEqual(
    merged.filter((_, index) => ![0, 4].includes(index)),
    existing.filter((_, index) => ![0, 4].includes(index)),
  );
  assert.equal(existing[0].value, "old");
});

test("snapshot creation removes proxy passwords and resets transient states without changing the source", () => {
  const state = seedState();
  state.proxies[0].password = "example-only-password";
  const original = structuredClone(state);
  const snapshot = createSnapshot(state);
  assert.deepEqual(state, original);
  assert.ok(!JSON.stringify(snapshot).includes("example-only-password"));
  assert.ok(
    snapshot.proxies.every(
      (proxy) =>
        proxy.password === "" &&
        proxy.status === "unchecked" &&
        proxy.latency === undefined,
    ),
  );
  assert.ok(
    snapshot.environments.every(
      (env) => env.status === "ready" && env.error === undefined,
    ),
  );
  assert.deepEqual(
    snapshot.environments.map((env) => env.seed),
    state.environments.map((env) => env.seed),
  );
});

test("snapshot parser rejects malformed JSON, unsupported format or version, and missing collections", () => {
  for (const text of ["not-json", "null", "{}", "[]"])
    assert.throws(() => parseSnapshot(text));
  const base = JSON.parse(snapshotText());
  for (const mutation of [
    { format: "another-app" },
    { schemaVersion: 2 },
    { environments: null },
    { proxies: {} },
    { kernels: undefined },
  ]) {
    assert.throws(() =>
      parseSnapshot(JSON.stringify({ ...base, ...mutation })),
    );
  }
});

test("snapshot parser rejects duplicate environment, proxy, or kernel IDs", () => {
  const appendDuplicate: Array<(snapshot: Snapshot) => void> = [
    (snapshot) => {
      snapshot.environments.push(structuredClone(snapshot.environments[0]));
    },
    (snapshot) => {
      snapshot.proxies.push(structuredClone(snapshot.proxies[0]));
    },
    (snapshot) => {
      snapshot.kernels.push(structuredClone(snapshot.kernels[0]));
    },
  ];
  for (const edit of appendDuplicate) {
    const text = snapshotText(edit);
    assert.throws(() => parseSnapshot(text), /重复 ID/);
  }
});

test("snapshot parser rejects dangling core and proxy references", () => {
  for (const field of ["coreId", "proxyId"] as const) {
    const text = snapshotText((snapshot) => {
      snapshot.environments[0][field] = "missing-reference";
    });
    assert.throws(() => parseSnapshot(text), /引用无效/);
  }
});

test("snapshot parser accepts a valid round trip and rejects malformed embedded cookies", () => {
  const snapshot = createSnapshot(seedState());
  assert.deepEqual(
    parseSnapshot(JSON.stringify(snapshot)),
    JSON.parse(JSON.stringify(snapshot)),
  );
  const invalid = snapshotText((data) => {
    data.environments[0].cookies = [
      {
        name: "bad",
        value: "example",
        domain: "https://example.com",
        path: "/",
      },
    ];
  });
  assert.throws(() => parseSnapshot(invalid));
});

test("snapshot restore blocks every active lifecycle state and leaves the workspace intact", () => {
  const snapshot = createSnapshot(seedState());
  for (const status of ["running", "starting", "stopping"] as Status[]) {
    const state = stoppedState();
    state.environments[0].status = status;
    const original = structuredClone(state);
    assert.throws(() => restoreSnapshot(state, snapshot), /先停止所有环境/);
    assert.deepEqual(state, original);
  }
});

test("snapshot restore retains saved seeds and cookie data, resets states, and strips even injected proxy passwords", () => {
  const state = stoppedState();
  const snapshot = createSnapshot(seedState());
  snapshot.environments[0].seed = "2147483647";
  snapshot.environments[0].status = "running";
  snapshot.environments[0].error = "stale example error";
  snapshot.environments[0].cookies = [
    {
      name: "empty",
      value: "",
      domain: "example.com",
      path: "/",
      partitionKey: { topLevelSite: "https://store.example" },
    },
  ];
  snapshot.proxies[0].password = "injected-example-password";
  snapshot.proxies[0].status = "connected";
  snapshot.proxies[0].latency = 12;
  const originalState = structuredClone(state);
  const originalSnapshot = structuredClone(snapshot);
  const restored = restoreSnapshot(state, snapshot);
  assert.deepEqual(
    restored.environments.map((env) => env.seed),
    snapshot.environments.map((env) => env.seed),
  );
  assert.deepEqual(
    restored.environments[0].cookies,
    snapshot.environments[0].cookies,
  );
  assert.ok(
    restored.environments.every(
      (env) => env.status === "ready" && env.error === undefined,
    ),
  );
  assert.ok(
    restored.proxies.every(
      (proxy) =>
        proxy.password === "" &&
        proxy.status === "unchecked" &&
        proxy.latency === undefined,
    ),
  );
  assert.ok(!JSON.stringify(restored).includes("injected-example-password"));
  assert.deepEqual(state, originalState);
  assert.deepEqual(snapshot, originalSnapshot);
  assert.ok(
    launchError(restored.environments[0], restored),
    "restored proxies require a fresh check",
  );
});
