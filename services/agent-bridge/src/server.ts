import { timingSafeEqual } from "node:crypto";
import { createServer, type IncomingMessage, type Server, type ServerResponse } from "node:http";
import type { PermissionBroker } from "./permissions.ts";
import { HttpError } from "./protocol.ts";
import type { SessionReader } from "./sessions.ts";
import type { TaskManager } from "./tasks.ts";

type Deps = {
  token: string;
  tasks: Pick<TaskManager, "start" | "cancel" | "cancelSession" | "get">;
  broker: Pick<PermissionBroker, "answer" | "pendingFor">;
  sessions: Pick<SessionReader, "list" | "messages">;
  log?: (message: string) => void;
};

const MAX_BODY = 64 * 1024;

// createBridgeServer は smart-speaker から呼ばれるHTTP APIを作る。すべてBearerトークンで認証する。
export function createBridgeServer(deps: Deps): Server {
  const log = deps.log ?? ((message) => console.log(message));
  return createServer((req, res) => {
    handle(deps, req, res).catch((error) => {
      const status = error instanceof HttpError ? error.status : 500;
      if (status === 500) log(`http: ${req.method} ${req.url}: ${String(error)}`);
      send(res, status, { error: status === 500 ? "内部エラーが発生しました" : String(error.message) });
    });
  });
}

async function handle(deps: Deps, req: IncomingMessage, res: ServerResponse): Promise<void> {
  const url = new URL(req.url ?? "/", "http://bridge");
  const path = url.pathname;
  const method = req.method ?? "GET";
  if (method === "GET" && path === "/healthz") return send(res, 200, { ok: true });
  if (!authorized(req.headers.authorization, deps.token)) throw new HttpError(401, "認証に失敗しました");

  let match: RegExpMatchArray | null;
  if (method === "POST" && path === "/tasks") {
    const body = await readJSON(req);
    const input = stringField(body, "input", true);
    const sessionId = stringField(body, "session_id", false);
    const workdir = stringField(body, "workdir", false);
    return send(res, 202, await deps.tasks.start(input, sessionId, workdir));
  }
  if (method === "POST" && (match = path.match(/^\/permissions\/([^/]+)$/))) {
    const body = await readJSON(req);
    if (typeof body.allow !== "boolean") throw new HttpError(400, "allow は true か false で指定してください");
    deps.broker.answer(match[1], body.allow, stringField(body, "message", false));
    return send(res, 204);
  }
  if (method === "POST" && (match = path.match(/^\/tasks\/([^/]+)\/cancel$/))) {
    deps.tasks.cancel(match[1]);
    return send(res, 204);
  }
  if (method === "GET" && (match = path.match(/^\/tasks\/([^/]+)$/))) {
    const task = deps.tasks.get(match[1]);
    if (!task) throw new HttpError(404, "作業が見つかりません");
    return send(res, 200, { status: task.status, session_id: task.sessionId, pending_permissions: deps.broker.pendingFor(task.id) });
  }
  if (method === "GET" && path === "/sessions") {
    const limit = intParam(url, "limit", 20, 1, 100);
    return send(res, 200, await deps.sessions.list(limit));
  }
  if (method === "POST" && (match = path.match(/^\/sessions\/([^/]+)\/cancel$/))) {
    deps.tasks.cancelSession(pathParam(match[1]));
    return send(res, 204);
  }
  if (method === "GET" && (match = path.match(/^\/sessions\/([^/]+)\/messages$/))) {
    const limit = intParam(url, "limit", 10, 1, 50);
    const offset = intParam(url, "offset", 0, 0, 10_000);
    return send(res, 200, await deps.sessions.messages(pathParam(match[1]), limit, offset));
  }
  throw new HttpError(404, "見つかりません");
}

function pathParam(value: string): string {
  try {
    return decodeURIComponent(value);
  } catch {
    throw new HttpError(400, "パスの形式が正しくありません");
  }
}

function authorized(header: string | undefined, token: string): boolean {
  const expected = Buffer.from(`Bearer ${token}`);
  const actual = Buffer.from(header ?? "");
  return actual.length === expected.length && timingSafeEqual(actual, expected);
}

async function readJSON(req: IncomingMessage): Promise<Record<string, unknown>> {
  const chunks: Buffer[] = [];
  let size = 0;
  for await (const chunk of req) {
    size += chunk.length;
    if (size > MAX_BODY) throw new HttpError(413, "本文が大きすぎます");
    chunks.push(chunk);
  }
  try {
    const value = JSON.parse(Buffer.concat(chunks).toString("utf8"));
    if (value && typeof value === "object" && !Array.isArray(value)) return value;
  } catch {
    // 下でまとめて400にする。
  }
  throw new HttpError(400, "JSONオブジェクトを指定してください");
}

function stringField(body: Record<string, unknown>, name: string, required: true): string;
function stringField(body: Record<string, unknown>, name: string, required: false): string | undefined;
function stringField(body: Record<string, unknown>, name: string, required: boolean): string | undefined {
  const value = body[name];
  if (value === undefined || value === null) {
    if (required) throw new HttpError(400, `${name} を指定してください`);
    return undefined;
  }
  if (typeof value !== "string") throw new HttpError(400, `${name} は文字列で指定してください`);
  return value;
}

function intParam(url: URL, name: string, fallback: number, min: number, max: number): number {
  const raw = url.searchParams.get(name);
  if (raw === null || raw === "") return fallback;
  const value = Number(raw);
  if (!Number.isInteger(value) || value < min || value > max) throw new HttpError(400, `${name} は${min}〜${max}の整数で指定してください`);
  return value;
}

function send(res: ServerResponse, status: number, body?: unknown): void {
  if (body === undefined) {
    res.writeHead(status).end();
    return;
  }
  res.writeHead(status, { "content-type": "application/json; charset=utf-8" }).end(JSON.stringify(body));
}
