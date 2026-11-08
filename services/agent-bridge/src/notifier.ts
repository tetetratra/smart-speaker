import { randomUUID } from "node:crypto";
import type { BridgeEvent, EventInput } from "./protocol.ts";

export type Notify = (event: EventInput, deadline: Date, signal?: AbortSignal) => Promise<boolean>;

type Options = {
  url: string;
  token: string;
  fetch?: typeof fetch;
  sleep?: (ms: number) => Promise<void>;
  now?: () => number;
  log?: (message: string) => void;
};

// Notifier は smart-speaker へ通知をプッシュする。届くまで、期限内で間隔を広げながら再送する。
// 同じ event_id で再送するので、受け手は重複を捨てられる。
export class Notifier {
  #options: Required<Omit<Options, "fetch">> & { fetch: typeof fetch };

  constructor(options: Options) {
    this.#options = {
      fetch: options.fetch ?? fetch,
      sleep: options.sleep ?? ((ms) => new Promise((resolve) => setTimeout(resolve, ms))),
      now: options.now ?? Date.now,
      log: options.log ?? ((message) => console.log(message)),
      url: options.url,
      token: options.token,
    };
  }

  // send は届いたら true、期限切れや再送しても意味がない応答なら false を返す。
  // signal が中止されたら（承認依頼が片付いたなど）、それ以上は再送しない。
  send: Notify = async (input, deadline, signal) => {
    const event = { ...input, event_id: randomUUID() } as BridgeEvent;
    const { fetch, sleep, now, log, url, token } = this.#options;
    let wait = 1000;
    for (let attempt = 1; ; attempt++) {
      if (signal?.aborted) return false;
      try {
        const response = await fetch(url, {
          method: "POST",
          headers: { "content-type": "application/json", authorization: `Bearer ${token}` },
          body: JSON.stringify(event),
          signal: AbortSignal.timeout(10_000),
        });
        if (response.ok) return true;
        // 認証や形式の誤りは再送しても直らない。
        if (response.status >= 400 && response.status < 500 && response.status !== 429) {
          log(`notify: ${event.type} を受け付けてもらえませんでした (${response.status})`);
          return false;
        }
        log(`notify: ${event.type} の送信に失敗しました (${response.status}, ${attempt}回目)`);
      } catch (error) {
        log(`notify: ${event.type} の送信に失敗しました (${String(error)}, ${attempt}回目)`);
      }
      if (now() + wait > deadline.getTime()) return false;
      await sleep(wait);
      wait = Math.min(wait * 2, 30_000);
    }
  };
}
