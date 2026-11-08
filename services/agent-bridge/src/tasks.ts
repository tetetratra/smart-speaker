import { randomUUID } from "node:crypto";
import type { CanUseTool, Options, Query, SDKMessage, SDKResultMessage } from "@anthropic-ai/claude-agent-sdk";
import type { Workdir } from "./config.ts";
import type { Notify } from "./notifier.ts";
import type { PermissionBroker } from "./permissions.ts";
import { HttpError, type TaskAccepted } from "./protocol.ts";
import { SMART_SPEAKER_TAG } from "./sessions.ts";

export type QueryFn = (params: { prompt: string; options?: Options }) => Query | AsyncIterable<SDKMessage>;
export type TagFn = (sessionId: string, tag: string | null, options?: { dir?: string }) => Promise<void>;

type Task = {
  id: string;
  sessionId: string;
  workdir: Workdir;
  abort: AbortController;
  status: "running" | "completed" | "failed";
};

type Deps = {
  query: QueryFn;
  tagSession: TagFn;
  // findSession はセッションを始めた作業ディレクトリを返す。候補で始めたセッションでなければ undefined。
  findSession: (sessionId: string) => Promise<Workdir | undefined>;
  broker: PermissionBroker;
  notify: Notify;
  // workdirs の先頭が既定の作業ディレクトリ。
  workdirs: Workdir[];
  claudePath?: string;
  maxTasks: number;
  taskTimeoutMs: number;
  log?: (message: string) => void;
};

const MAX_INPUT = 20_000;
// セッションの記録に残す起動元。SDKは指定がなければ「sdk-ts」にするが、Claude Code のCLIは
// 起動元が sdk-ts・sdk-py・sdk-cli のセッションを `claude --resume` の一覧から外すため、PCで続きを開けなくなる。
// 「cli」を指定しても非対話の実行では「sdk-cli」に書き換えられるので、独自の値にする。
export const ENTRYPOINT = "smart-speaker-agent-bridge";
// 音読用の本文の上限文字数。Go側（internal/agentbridge/service.go の maxReadAloud）と合わせる。
const MAX_READ_ALOUD = 6000;
const TIMEOUT = "timeout";
// 完了・失敗の通知を再送し続ける時間。
const RESULT_NOTIFY_WINDOW_MS = 30 * 60 * 1000;

const OUTPUT_SCHEMA = {
  type: "object",
  properties: {
    summary: { type: "string", description: "音声で読み上げる結果の要点。話し言葉で2〜3文まで。" },
    details: { type: "array", items: { type: "string" }, maxItems: 10, description: "画面に表示する補足。1項目1行。" },
    read_aloud: {
      type: "string",
      description: `ユーザーが文章の読み上げ（音読）を求めたときだけ入れる、読み上げる本文。要約・省略せず原文のまま。${MAX_READ_ALOUD}文字まで。`,
    },
  },
  required: ["summary", "details"],
  additionalProperties: false,
};

const APPEND_SYSTEM_PROMPT = `この作業はスマートスピーカーの音声アシスタント経由で依頼されています。
ユーザーは画面を見ていない可能性があります。結果の summary は、そのまま読み上げられる前提で、話し言葉で2〜3文にまとめてください。
細かい内容やファイル名、URLなどは details に1項目1行で書いてください。実行していない操作を実行したと書かないでください。

ユーザーが本や資料の文章の読み上げ（音読）を求めたときは、読み上げる本文を要約・省略せずに原文のまま read_aloud に入れてください（${MAX_READ_ALOUD}文字まで。超える場合は区切りのよいところまでにし、どこまで入れたかを summary に書きます）。
読み上げる資料はユーザーが自分で入手して手元に持っているもので、家庭内のスマートスピーカーがユーザー本人に向けて読み上げます。
日本の著作権法では、公衆に聞かせることを目的としない朗読は口述権（第24条、「公に」の意味は第22条）の対象外で、そのための文章の抽出は私的使用のための複製（第30条第1項）に当たります。
ただし、DRM などの技術的保護手段を回避しないと文章を取り出せない場合は、私的使用でも認められない（第30条第1項第2号）ため、抽出せずにその旨を伝えてください。`;

// TaskManager は Claude Code への依頼を管理する。1つのセッションで同時に動く作業は1つだけにする。
export class TaskManager {
  #tasks = new Map<string, Task>();
  #deps: Deps;
  #log: (message: string) => void;

  constructor(deps: Deps) {
    this.#deps = deps;
    this.#log = deps.log ?? ((message) => console.log(message));
  }

  // start は作業を始める。workdirName は作業ディレクトリの候補の名前で、空なら先頭の候補を使う。
  // 続きの依頼（sessionId あり）は、workdirName にかかわらずセッションを始めた場所で再開する。
  async start(input: string, sessionId?: string, workdirName?: string): Promise<TaskAccepted> {
    input = input?.trim() ?? "";
    if (!input) throw new HttpError(400, "input を指定してください");
    if (input.length > MAX_INPUT) throw new HttpError(400, `input は${MAX_INPUT}文字以内にしてください`);
    const resume = sessionId?.trim() || undefined;
    if (resume && !isUUID(resume)) throw new HttpError(400, "session_id の形式が正しくありません");
    let workdir = this.#workdir(workdirName);
    if (resume) {
      const found = await this.#deps.findSession(resume);
      if (!found) throw new HttpError(404, "セッションが見つかりません");
      workdir = found;
    }
    // await の後に確かめ、同時に届いた依頼が上限をすり抜けないようにする。
    const running = [...this.#tasks.values()].filter((t) => t.status === "running");
    if (resume && running.some((t) => t.sessionId === resume)) throw new HttpError(409, "このセッションは作業中です");
    if (running.length >= this.#deps.maxTasks) throw new HttpError(429, "同時に動かせる作業の上限に達しています");

    const task: Task = { id: randomUUID(), sessionId: resume ?? randomUUID(), workdir, abort: new AbortController(), status: "running" };
    this.#tasks.set(task.id, task);
    void this.#run(task, input, !resume);
    return { task_id: task.id, session_id: task.sessionId, workdir: workdir.name };
  }

  #workdir(name: string | undefined): Workdir {
    const { workdirs } = this.#deps;
    name = name?.trim();
    if (!name) return workdirs[0];
    const found = workdirs.find((w) => w.name === name);
    if (!found) throw new HttpError(400, `作業ディレクトリ「${name}」はありません。候補: ${workdirs.map((w) => w.name).join(", ")}`);
    return found;
  }

  cancel(taskId: string): void {
    const task = this.#tasks.get(taskId);
    if (!task) throw new HttpError(404, "作業が見つかりません");
    task.abort.abort();
  }

  // cancelSession は、task_id が分からなくなった後でも、セッションで実行中の作業を止められるようにする。
  cancelSession(sessionId: string): void {
    const task = this.runningTask(sessionId);
    if (!task) throw new HttpError(404, "このセッションで実行中の作業はありません");
    task.abort.abort();
  }

  runningTask(sessionId: string): Task | undefined {
    return [...this.#tasks.values()].find((t) => t.sessionId === sessionId && t.status === "running");
  }

  get(taskId: string): Task | undefined {
    return this.#tasks.get(taskId);
  }

  pendingCount(sessionId: string): number {
    return [...this.#tasks.values()]
      .filter((t) => t.sessionId === sessionId && t.status === "running")
      .reduce((sum, t) => sum + this.#deps.broker.pendingFor(t.id).length, 0);
  }

  shutdown(): void {
    for (const task of this.#tasks.values()) task.abort.abort();
  }

  async #run(task: Task, input: string, isNew: boolean): Promise<void> {
    const { broker, notify } = this.#deps;
    const timer = setTimeout(() => task.abort.abort(TIMEOUT), this.#deps.taskTimeoutMs);
    const canUseTool: CanUseTool = (toolName, toolInput, options) =>
      broker.request({
        taskId: task.id,
        sessionId: task.sessionId,
        tool: toolName,
        input: toolInput,
        title: options.title,
        description: options.description,
        signal: options.signal,
      });
    let result: SDKResultMessage | undefined;
    let error: unknown;
    try {
      const messages = this.#deps.query({
        prompt: input,
        options: {
          cwd: task.workdir.path,
          ...(isNew ? { sessionId: task.sessionId } : { resume: task.sessionId }),
          permissionMode: "auto",
          canUseTool,
          outputFormat: { type: "json_schema", schema: OUTPUT_SCHEMA },
          systemPrompt: { type: "preset", preset: "claude_code", append: APPEND_SYSTEM_PROMPT },
          abortController: task.abort,
          pathToClaudeCodeExecutable: this.#deps.claudePath,
          env: { ...childEnv(), CLAUDE_CODE_ENTRYPOINT: ENTRYPOINT },
          // 声では答えられない問い合わせ（選択肢の質問や計画の承認）は使わせない。
          disallowedTools: ["AskUserQuestion", "EnterPlanMode", "ExitPlanMode"],
        },
      });
      for await (const message of messages) {
        if (message.type === "result") result = message;
      }
    } catch (e) {
      error = e;
    } finally {
      clearTimeout(timer);
      broker.rejectTask(task.id);
    }
    if (isNew) {
      // タグはセッションの記録ができてから付ける。失敗しても作業の結果には影響させない。
      await this.#deps.tagSession(task.sessionId, SMART_SPEAKER_TAG, { dir: task.workdir.path }).catch((e) => {
        this.#log(`task ${task.id}: タグを付けられませんでした: ${String(e)}`);
      });
    }
    const outcome = interpret(result, error, task.abort.signal.aborted, task.abort.signal.reason === TIMEOUT);
    task.status = outcome.type;
    this.#log(`task ${task.id}: ${outcome.type}`);
    const deadline = new Date(Date.now() + RESULT_NOTIFY_WINDOW_MS);
    await notify({ ...outcome, task_id: task.id, session_id: task.sessionId, request: short(input, 100) }, deadline);
    // 終わった作業は動作確認用にしばらく残してから消す。
    setTimeout(() => this.#tasks.delete(task.id), 10 * 60 * 1000).unref();
  }
}

type Outcome =
  | { type: "completed"; summary: string; details: string[]; read_aloud?: string; permission_denials?: string[] }
  | { type: "failed"; error: string };

// interpret は result メッセージから、完了か失敗かを決める。
export function interpret(result: SDKResultMessage | undefined, error: unknown, aborted: boolean, timedOut = false): Outcome {
  if (aborted && result?.subtype !== "success") {
    return { type: "failed", error: timedOut ? "時間の上限に達したので作業を止めました" : "作業を中止しました" };
  }
  if (!result) return { type: "failed", error: error ? short(String(error), 500) : "Claude Codeから結果が返りませんでした" };
  if (result.subtype !== "success" || result.is_error) {
    const reason = "errors" in result && result.errors?.length ? result.errors.join(" / ") : result.subtype;
    return { type: "failed", error: short(reason, 500) };
  }
  const output = result.structured_output as { summary?: unknown; details?: unknown; read_aloud?: unknown } | undefined;
  const summary = typeof output?.summary === "string" && output.summary.trim() ? output.summary.trim() : short(result.result ?? "", 300);
  const details = Array.isArray(output?.details) ? output.details.filter((d): d is string => typeof d === "string") : [];
  const readAloud = typeof output?.read_aloud === "string" ? short(output.read_aloud.trim(), MAX_READ_ALOUD) : "";
  const denials = result.permission_denials.map((d) => `${d.tool_name}: ${short(JSON.stringify(d.tool_input), 120)}`);
  return {
    type: "completed",
    summary: short(summary, 600) || "作業が終わりました。",
    details: details.slice(0, 10).map((d) => short(d, 300)),
    ...(readAloud ? { read_aloud: readAloud } : {}),
    ...(denials.length ? { permission_denials: denials.slice(0, 10) } : {}),
  };
}

// childEnv は、Claude Code に引き継がない環境変数を除いた残りを返す。
// - ANTHROPIC_ と CLAUDE で始まるもの（ANTHROPIC_BASE_URL、CLAUDECODE など）。
//   引き継ぐと、CLIのログイン情報ではなく別の接続先を使おうとして失敗することがある。
// - AGENT_BRIDGE_ で始まるもの。共有トークンがあれば、Claude Code が自分の承認依頼を自分で許可できてしまう。
// - 名前が _TOKEN、_KEY、_SECRET、_PASSWORD で終わるもの。ブリッジは .envrc を direnv で読んで起動するため、
//   サーバー用のシークレット（OPENAI_API_KEY など）も環境変数に入っている。
// ほかの変数は、ユーザーのMCP設定が参照することがあるので残す。
const HIDDEN_ENV = /^(ANTHROPIC_|CLAUDE|AGENT_BRIDGE_)|_(TOKEN|KEY|SECRET|PASSWORD)$/i;

export function childEnv(source: NodeJS.ProcessEnv = process.env): Record<string, string> {
  const env: Record<string, string> = {};
  for (const [key, value] of Object.entries(source)) {
    if (value !== undefined && !HIDDEN_ENV.test(key)) env[key] = value;
  }
  return env;
}

function short(text: string, max: number): string {
  return text.length > max ? `${text.slice(0, max)}…` : text;
}

function isUUID(value: string): boolean {
  return /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(value);
}
