import { readFileSync } from "node:fs";

export type Config = {
  listen: { host: string; port: number };
  token: string;
  workdir: string;
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
    workdir: required("AGENT_BRIDGE_WORKDIR"),
    eventsURL: required("AGENT_BRIDGE_EVENTS_URL"),
    claudePath: env.AGENT_BRIDGE_CLAUDE_PATH?.trim() || undefined,
    permissionTimeoutMs: seconds(env.AGENT_BRIDGE_PERMISSION_TIMEOUT_SECONDS, 300),
    taskTimeoutMs: seconds(env.AGENT_BRIDGE_TASK_TIMEOUT_SECONDS, 3600),
    maxTasks: positiveInt(env.AGENT_BRIDGE_MAX_TASKS, 1),
  };
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
