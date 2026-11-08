import { getSessionInfo, getSessionMessages, listSessions, query, tagSession } from "@anthropic-ai/claude-agent-sdk";
import { loadConfig } from "./config.ts";
import { Notifier } from "./notifier.ts";
import { PermissionBroker } from "./permissions.ts";
import { createBridgeServer } from "./server.ts";
import { SessionReader } from "./sessions.ts";
import { TaskManager } from "./tasks.ts";

const config = loadConfig();
const notifier = new Notifier({ url: config.eventsURL, token: config.token });
const broker = new PermissionBroker(notifier.send, config.permissionTimeoutMs);
let sessions: SessionReader;
const tasks = new TaskManager({
  query,
  tagSession,
  findSession: (id) => sessions.find(id),
  broker,
  notify: notifier.send,
  workdirs: config.workdirs,
  claudePath: config.claudePath,
  maxTasks: config.maxTasks,
  taskTimeoutMs: config.taskTimeoutMs,
});
sessions = new SessionReader({ listSessions, getSessionInfo, getSessionMessages }, config.workdirs, tasks);

const server = createBridgeServer({ token: config.token, tasks, broker, sessions });
server.listen(config.listen.port, config.listen.host, () => {
  console.log(`agent-bridge: ${config.listen.host}:${config.listen.port} で待ち受けています（作業ディレクトリ: ${config.workdirs.map((w) => `${w.name}=${w.path}`).join(", ")}）`);
});

for (const signal of ["SIGINT", "SIGTERM"] as const) {
  process.on(signal, () => {
    // 実行中の作業は止める。完了通知は送れないので、smart-speaker側には届かない。
    tasks.shutdown();
    server.close(() => process.exit(0));
    setTimeout(() => process.exit(0), 3000).unref();
  });
}
