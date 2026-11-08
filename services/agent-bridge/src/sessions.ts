import { realpathSync } from "node:fs";
import { resolve } from "node:path";
import type { SDKSessionInfo, SessionMessage } from "@anthropic-ai/claude-agent-sdk";
import type { Workdir } from "./config.ts";
import { HttpError, type SessionList, type SessionMessages, type SessionTurn } from "./protocol.ts";

export const SMART_SPEAKER_TAG = "smart-speaker";

export type SessionAPI = {
  listSessions(options: { dir: string; limit?: number; includeWorktrees?: boolean }): Promise<SDKSessionInfo[]>;
  getSessionInfo(sessionId: string, options: { dir: string }): Promise<SDKSessionInfo | undefined>;
  getSessionMessages(sessionId: string, options: { dir: string }): Promise<SessionMessage[]>;
};

export type TaskState = {
  runningTask(sessionId: string): { id: string } | undefined;
  pendingCount(sessionId: string): number;
};

const MAX_TURN_TEXT = 1500;
const STRUCTURED_OUTPUT = "StructuredOutput";
const MAX_TOTAL_TEXT = 12000;

// SessionReader は作業ディレクトリの候補で始めたセッションだけを読む。ほかのプロジェクトのセッションは見せない。
export class SessionReader {
  #api: SessionAPI;
  #workdirs: Workdir[];
  #tasks: TaskState;

  constructor(api: SessionAPI, workdirs: Workdir[], tasks: TaskState) {
    this.#api = api;
    this.#workdirs = workdirs;
    this.#tasks = tasks;
  }

  // find はセッションを始めた作業ディレクトリを返す。候補で始めたセッションでなければ undefined。
  // worktreeのセッションは同じ場所で再開できないので、見つからない扱いにする。
  async find(sessionId: string): Promise<Workdir | undefined> {
    for (const workdir of this.#workdirs) {
      const info = await this.#api.getSessionInfo(sessionId, { dir: workdir.path });
      if (info !== undefined && (!info.cwd || samePath(info.cwd, workdir.path))) return workdir;
    }
    return undefined;
  }

  // list はすべての候補のセッションを新しい順に並べ、limit 件まで返す。
  async list(limit: number): Promise<SessionList> {
    const all = await Promise.all(
      this.#workdirs.map(async (workdir) => {
        // worktreeのセッションは同じ作業ディレクトリで再開できないので含めない。
        const sessions = await this.#api.listSessions({ dir: workdir.path, limit, includeWorktrees: false });
        return sessions.map((s) => ({ workdir, s }));
      }),
    );
    const picked = all
      .flat()
      .sort((a, b) => b.s.lastModified - a.s.lastModified)
      .slice(0, limit);
    const sessions = picked.map(({ workdir, s }) => ({
      session_id: s.sessionId,
      workdir: workdir.name,
      title: s.summary,
      custom_title: s.customTitle,
      first_prompt: truncate(s.firstPrompt, 200),
      created_at: s.createdAt ? new Date(s.createdAt).toISOString() : undefined,
      last_modified: new Date(s.lastModified).toISOString(),
      git_branch: s.gitBranch,
      tag: s.tag,
      from_smart_speaker: s.tag === SMART_SPEAKER_TAG,
      running: this.#tasks.runningTask(s.sessionId) !== undefined,
      task_id: this.#tasks.runningTask(s.sessionId)?.id,
      pending_permissions: this.#tasks.pendingCount(s.sessionId),
    }));
    return { workdirs: this.#workdirs.map((w) => w.name), sessions };
  }

  // messages の offset は新しい側から数える。SDKは古い側から数えるので、全件を読んでから切り出す。
  async messages(sessionId: string, limit: number, offset: number): Promise<SessionMessages> {
    const workdir = await this.find(sessionId);
    if (!workdir) throw new HttpError(404, "セッションが見つかりません");
    const turns = toTurns(await this.#api.getSessionMessages(sessionId, { dir: workdir.path }));
    const end = Math.max(turns.length - offset, 0);
    const picked = turns.slice(Math.max(end - limit, 0), end);
    return { session_id: sessionId, workdir: workdir.name, total: turns.length, offset, messages: capTotal(picked) };
  }
}

type Block = { type?: string; text?: string; name?: string; input?: Record<string, unknown> };

// toTurns はSDKのメッセージを、ユーザーの発話とClaudeの返答の組に整える。
// 続けて届くassistantのメッセージは1つにまとめ、ツールの結果と思考は捨てる。
export function toTurns(messages: SessionMessage[]): SessionTurn[] {
  const turns: SessionTurn[] = [];
  for (const m of messages) {
    if (m.type !== "user" && m.type !== "assistant") continue;
    const blocks = contentBlocks((m.message as { content?: unknown } | undefined)?.content);
    let text = blocks
      .filter((b) => b.type === "text" && b.text)
      .map((b) => stripReminders(b.text!))
      .filter(Boolean)
      .join("\n");
    const tools = blocks.filter((b) => b.type === "tool_use" && b.name !== STRUCTURED_OUTPUT).map(toolLine);
    // 構造化出力で終わるターンは、最後の返答がツール呼び出しになる。その summary を返答の本文として扱う。
    const structured = blocks.find((b) => b.type === "tool_use" && b.name === STRUCTURED_OUTPUT)?.input?.summary;
    if (typeof structured === "string" && structured.trim()) {
      text = text ? `${text}\n${structured.trim()}` : structured.trim();
    }
    const at = (m as { timestamp?: string }).timestamp;
    if (m.type === "user") {
      // ツールの結果だけのuserメッセージは会話ではない。
      if (text) turns.push({ role: "user", text: truncate(text, MAX_TURN_TEXT)!, at });
      continue;
    }
    if (!text && tools.length === 0) continue;
    const last = turns.at(-1);
    if (last?.role === "assistant") {
      if (text) last.text = truncate(last.text ? `${last.text}\n${text}` : text, MAX_TURN_TEXT)!;
      if (tools.length) last.tools = [...(last.tools ?? []), ...tools].slice(-20);
      last.at = at ?? last.at;
      continue;
    }
    turns.push({ role: "assistant", text: truncate(text, MAX_TURN_TEXT) ?? "", tools: tools.length ? tools : undefined, at });
  }
  return turns;
}

// samePath は末尾の / やシンボリックリンク（/tmp と /private/tmp など）の違いを無視して比べる。
function samePath(a: string, b: string): boolean {
  const real = (p: string) => {
    try {
      return realpathSync(p);
    } catch {
      return resolve(p);
    }
  };
  return real(a) === real(b);
}

function contentBlocks(content: unknown): Block[] {
  if (typeof content === "string") return [{ type: "text", text: content }];
  return Array.isArray(content) ? (content as Block[]) : [];
}

// CLIが差し込むタグ（system-reminder やスラッシュコマンドの記録）は会話から除く。
function stripReminders(text: string): string {
  return text
    .replace(/<(system-reminder|command-name|command-message|command-args|local-command-stdout|local-command-caveat)>[\s\S]*?<\/\1>/g, "")
    .trim();
}

// toolLine はツール呼び出しを「[Bash] git status」のような1行にする。
function toolLine(block: Block): string {
  const input = block.input ?? {};
  const keys = ["command", "file_path", "path", "pattern", "url", "query", "description", "prompt"];
  const key = keys.find((k) => typeof input[k] === "string");
  const detail = key ? String(input[key]) : "";
  return `[${block.name ?? "tool"}]${detail ? " " + truncate(detail.replace(/\s+/g, " "), 80) : ""}`;
}

function capTotal(turns: SessionTurn[]): SessionTurn[] {
  // 新しい側を優先して、全体の長さを抑える。
  let total = 0;
  const kept: SessionTurn[] = [];
  for (const turn of [...turns].reverse()) {
    total += turn.text.length;
    if (total > MAX_TOTAL_TEXT && kept.length > 0) break;
    kept.unshift(turn);
  }
  return kept;
}

function truncate(text: string | undefined, max: number): string | undefined {
  if (text === undefined) return undefined;
  return text.length > max ? `${text.slice(0, max)}…` : text;
}
