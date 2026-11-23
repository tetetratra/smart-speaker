package realtime

import (
	"sync/atomic"
	"testing"
)

func toolNamesIn(t *testing.T, update map[string]any) []string {
	t.Helper()
	var names []string
	for _, definition := range update["session"].(map[string]any)["tools"].([]any) {
		names = append(names, definition.(map[string]any)["name"].(string))
	}
	return names
}

func withToolsForTest(s *stage, names *[]string, instructions *string) {
	s.cfg.Tools = func() []any {
		var definitions []any
		for _, name := range *names {
			definitions = append(definitions, map[string]any{"type": "function", "name": name})
		}
		return definitions
	}
	s.cfg.Instructions = func() string { return *instructions }
}

// 接続中で静かなら、ツールが変わったらすぐ指示文とツールだけを送り直す。
func TestToolsChangedSendsSessionUpdateWhenConnected(t *testing.T) {
	s := newToolResponseStage(t)
	names, instructions := []string{"old"}, "前の指示"
	withToolsForTest(s, &names, &instructions)
	s.calls["search"].finished = true

	names, instructions = []string{"old", "mcp__new"}, "新しい指示"
	s.markToolsChanged()

	update := popToolCommand(t, s, "session.update")
	session := update["session"].(map[string]any)
	if session["instructions"] != "新しい指示" {
		t.Fatalf("指示文 = %v", session["instructions"])
	}
	got := toolNamesIn(t, update)
	if len(got) != 3 || got[0] != "old" || got[1] != "mcp__new" || got[2] != waitToolName {
		t.Fatalf("ツール = %v", got)
	}
	if _, ok := session["audio"]; ok {
		t.Fatalf("音声の設定まで送り直しています: %v", session)
	}
	noToolCommand(t, s)
}

// 接続していなければ送らない。次に接続するときに今の値を読む。
func TestToolsChangedDoesNothingWhenDisconnected(t *testing.T) {
	s := newToolResponseStage(t)
	transport := s.transport
	s.transport = nil
	s.ready = false
	s.markToolsChanged()
	if s.toolsDirty {
		t.Fatal("接続していないのに、送り直しを待っています")
	}
	s.transport = transport
	noToolCommand(t, s)
}

// 応答の途中では送らず、静かになってから送る。
func TestToolsChangedWaitsUntilQuiet(t *testing.T) {
	s := newToolResponseStage(t)
	names, instructions := []string{"a"}, "指示"
	withToolsForTest(s, &names, &instructions)
	// ツールの結果を待っている間も送らない。
	s.markToolsChanged()
	noToolCommand(t, s)
	s.calls["search"].finished = true
	s.active = "response1"
	s.flushToolsUpdate()
	noToolCommand(t, s)

	s.active = ""
	s.flushToolsUpdate()
	popToolCommand(t, s, "session.update")
	// 一度送ったら、次に変わるまで送らない。
	s.flushToolsUpdate()
	noToolCommand(t, s)
}

// WatchTools で受けた知らせは stage のキューを通り、接続中のセッションへ session.update が届く。
func TestWatchToolsSendsSessionUpdateOverConnection(t *testing.T) {
	var notify atomic.Pointer[func()]
	var version atomic.Int32
	r := newRig(t, func(cfg *Config) {
		cfg.Tools = func() []any {
			if version.Load() == 0 {
				return nil
			}
			return []any{map[string]any{"type": "function", "name": "mcp__added"}}
		}
		cfg.WatchTools = func(f func()) { notify.Store(&f) }
	})
	f := notify.Load()
	if f == nil {
		t.Fatal("WatchTools が呼ばれていません")
	}
	version.Store(1)
	(*f)()
	got := toolNamesIn(t, r.wait("session.update"))
	if len(got) != 2 || got[0] != "mcp__added" {
		t.Fatalf("ツール = %v", got)
	}
}
