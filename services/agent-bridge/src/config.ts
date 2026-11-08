import { readFileSync, realpathSync, statSync } from "node:fs";
import { isAbsolute } from "node:path";

// Workdir はClaude Codeに作業させる場所の候補。名前は音声エージェントが選ぶときに使う。
export type Workdir = { name: string; path: string };

export type Config = {
  listen: { host: string; port: number };
  token: string;
  // workdirs の先頭が既定の作業ディレクトリ。
  workdirs: Workdir[];
  eventsURL: string;
  claudePath?: string;
  permissionTimeoutMs: number;
  taskTimeoutMs: number;
  maxTasks: number;
};

type Env = Record<string, string | undefined>;

// loadConfig は環境変数を読み、足りない設定があれば起動時に止める。
export function loadConfig(env: Env = process.env): Config {
  const required = (name: string): string => {
    const value = env[name]?.trim();
    if (!value) throw new Error(`${name} を設定してください`);
    return value;
  };
  const listen = parseListen(required("AGENT_BRIDGE_LISTEN"));
  const token = env.AGENT_BRIDGE_TOKEN_FILE?.trim()
    ? readFileSync(env.AGENT_BRIDGE_TOKEN_FILE.trim(), "utf8").trim()
    : required("AGENT_BRIDGE_TOKEN");
  if (token.length < 16) throw new Error("トークンは16文字以上にしてください");
  return {
    listen,
    token,
    workdirs: parseWorkdirs(env),
    eventsURL: required("AGENT_BRIDGE_EVENTS_URL"),
    claudePath: env.AGENT_BRIDGE_CLAUDE_PATH?.trim() || undefined,
    permissionTimeoutMs: seconds(env.AGENT_BRIDGE_PERMISSION_TIMEOUT_SECONDS, 300),
    taskTimeoutMs: seconds(env.AGENT_BRIDGE_TASK_TIMEOUT_SECONDS, 3600),
    maxTasks: positiveInt(env.AGENT_BRIDGE_MAX_TASKS, 1),
  };
}

// parseWorkdirs は「名前=パス」をカンマ区切りで並べた AGENT_BRIDGE_WORKDIRS を読む。
function parseWorkdirs(env: Env): Workdir[] {
  const value = env.AGENT_BRIDGE_WORKDIRS?.trim();
  if (!value) {
    if (env.AGENT_BRIDGE_WORKDIR?.trim()) {
      throw new Error("AGENT_BRIDGE_WORKDIR は AGENT_BRIDGE_WORKDIRS（名前=パス をカンマ区切り）に変わりました");
    }
    throw new Error("AGENT_BRIDGE_WORKDIRS を設定してください");
  }
  const workdirs: Workdir[] = [];
  for (const entry of value.split(",").map((e) => e.trim()).filter(Boolean)) {
    const index = entry.indexOf("=");
    const name = entry.slice(0, index).trim();
    const path = entry.slice(index + 1).trim();
    if (index <= 0 || !name || !path) throw new Error(`AGENT_BRIDGE_WORKDIRS は 名前=パス をカンマ区切りで指定してください: ${entry}`);
    if (!isAbsolute(path)) throw new Error(`作業ディレクトリは絶対パスで指定してください: ${path}`);
    if (!isDirectory(path)) throw new Error(`作業ディレクトリが見つかりません: ${path}`);
    if (workdirs.some((w) => w.name === name)) throw new Error(`作業ディレクトリの名前が重複しています: ${name}`);
    // 同じ場所を2つの名前で登録すると、一覧に同じセッションが重ねて出る。
    if (workdirs.some((w) => realpathSync(w.path) === realpathSync(path))) throw new Error(`同じ作業ディレクトリが重複しています: ${path}`);
    workdirs.push({ name, path });
  }
  if (workdirs.length === 0) throw new Error("AGENT_BRIDGE_WORKDIRS を設定してください");
  return workdirs;
}

function isDirectory(path: string): boolean {
  try {
    return statSync(path).isDirectory();
  } catch {
    return false;
  }
}

function parseListen(value: string): { host: string; port: number } {
  const index = value.lastIndexOf(":");
  const port = Number(value.slice(index + 1));
  if (index < 0 || !Number.isInteger(port) || port <= 0 || port > 65535) {
    throw new Error("AGENT_BRIDGE_LISTEN は host:port の形で指定してください");
  }
  return { host: value.slice(0, index) || "127.0.0.1", port };
}

function seconds(value: string | undefined, fallback: number): number {
  return positiveInt(value, fallback) * 1000;
}

function positiveInt(value: string | undefined, fallback: number): number {
  if (!value?.trim()) return fallback;
  const parsed = Number(value);
  if (!Number.isInteger(parsed) || parsed <= 0) throw new Error(`正の整数を指定してください: ${value}`);
  return parsed;
}
