// smart-speaker とやり取りするJSONの形。Go側は internal/agentbridge/types.go に同じ形を定義している。

export type PermissionRequestEvent = {
  type: "permission_request";
  event_id: string;
  task_id: string;
  session_id: string;
  request_id: string;
  tool: string;
  // title は Claude Code が組み立てた確認文（例: 「Claude wants to read foo.txt」）。
  title?: string;
  description?: string;
  input: Record<string, unknown>;
  expires_at: string;
};

export type CompletedEvent = {
  type: "completed";
  event_id: string;
  task_id: string;
  session_id: string;
  // request は依頼文の冒頭。会話が区切られた後に届いても、何の結果か分かるようにする。
  request: string;
  summary: string;
  details: string[];
  permission_denials?: string[];
};

export type FailedEvent = {
  type: "failed";
  event_id: string;
  task_id: string;
  session_id: string;
  request: string;
  error: string;
};

export type BridgeEvent = PermissionRequestEvent | CompletedEvent | FailedEvent;

// event_id は Notifier が付ける。
export type EventInput =
  | Omit<PermissionRequestEvent, "event_id">
  | Omit<CompletedEvent, "event_id">
  | Omit<FailedEvent, "event_id">;

export type SessionSummary = {
  session_id: string;
  title: string;
  custom_title?: string;
  first_prompt?: string;
  created_at?: string;
  last_modified: string;
  git_branch?: string;
  tag?: string;
  from_smart_speaker: boolean;
  running: boolean;
  // task_id は実行中の作業があるときだけ入る。cancel に使う。
  task_id?: string;
  pending_permissions: number;
};

export type SessionTurn = {
  role: "user" | "assistant";
  text: string;
  tools?: string[];
  at?: string;
};

export type SessionMessages = {
  session_id: string;
  total: number;
  offset: number;
  messages: SessionTurn[];
};

// HTTPのステータスコードを持つエラー。server.ts がそのまま返す。
export class HttpError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}
