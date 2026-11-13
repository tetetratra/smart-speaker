import { randomUUID } from "node:crypto";
import type { PermissionResult } from "@anthropic-ai/claude-agent-sdk";
import type { Notify } from "./notifier.ts";
import { HttpError } from "./protocol.ts";

type Pending = {
  taskId: string;
  requestId: string;
  tool: string;
  expiresAt: Date;
  resolve: (result: PermissionResult) => void;
};

export type PermissionAsk = {
  context?: Record<string, unknown>;
  taskId: string;
  sessionId: string;
  tool: string;
  input: Record<string, unknown>;
  title?: string;
  description?: string;
  signal: AbortSignal;
};

// PermissionBroker は canUseTool で止まっている承認依頼を、smart-speaker からの返事まで保留する。
export class PermissionBroker {
  #pending = new Map<string, Pending>();
  #notify: Notify;
  #timeoutMs: number;

  constructor(notify: Notify, timeoutMs: number) {
    this.#notify = notify;
    this.#timeoutMs = timeoutMs;
  }

  // request は返事、期限切れ、作業の中止のいずれかで決まる。返事がなければ拒否にする。
  request(ask: PermissionAsk): Promise<PermissionResult> {
    const requestId = randomUUID();
    const expiresAt = new Date(Date.now() + this.#timeoutMs);
    return new Promise<PermissionResult>((resolve) => {
      let timer: NodeJS.Timeout | undefined;
      const delivery = new AbortController();
      const finish = (result: PermissionResult) => {
        if (!this.#pending.delete(requestId)) return;
        clearTimeout(timer);
        // 片付いた依頼の通知を再送し続けると、後から届いて死んだ依頼を尋ねてしまう。
        delivery.abort();
        ask.signal.removeEventListener("abort", onAbort);
        resolve(result);
      };
      const onAbort = () => finish(deny("作業が中止されました"));
      this.#pending.set(requestId, { taskId: ask.taskId, requestId, tool: ask.tool, expiresAt, resolve: finish });
      timer = setTimeout(() => finish(deny("期限までにユーザーの返事がありませんでした")), this.#timeoutMs);
      if (ask.signal.aborted) return onAbort();
      ask.signal.addEventListener("abort", onAbort, { once: true });
      void this.#notify(
        {
          type: "permission_request",
          ...(ask.context === undefined ? {} : { context: ask.context }),
          task_id: ask.taskId,
          session_id: ask.sessionId,
          request_id: requestId,
          tool: ask.tool,
          title: ask.title,
          description: ask.description,
          input: ask.input,
          expires_at: expiresAt.toISOString(),
        },
        expiresAt,
        delivery.signal,
      ).then((delivered) => {
        // 通知が届かなければ、誰も答えられないので拒否する。
        if (!delivered) finish(deny("承認依頼をスマートスピーカーへ届けられませんでした"));
      });
    }).then((result) => (result.behavior === "allow" ? { behavior: "allow", updatedInput: ask.input } : result));
  }

  answer(requestId: string, allow: boolean, message?: string): void {
    const pending = this.#pending.get(requestId);
    if (!pending) throw new HttpError(404, "保留中の承認依頼が見つかりません");
    pending.resolve(allow ? { behavior: "allow" } : deny(message?.trim() || "ユーザーが拒否しました"));
  }

  pendingFor(taskId: string): { request_id: string; tool: string; expires_at: string }[] {
    return [...this.#pending.values()]
      .filter((p) => p.taskId === taskId)
      .map((p) => ({ request_id: p.requestId, tool: p.tool, expires_at: p.expiresAt.toISOString() }));
  }

  // rejectTask は作業が終わったとき、残っている承認依頼をすべて拒否する。
  rejectTask(taskId: string): void {
    for (const pending of [...this.#pending.values()]) {
      if (pending.taskId === taskId) pending.resolve(deny("作業が終了しました"));
    }
  }
}

function deny(message: string): PermissionResult {
  return { behavior: "deny", message };
}
