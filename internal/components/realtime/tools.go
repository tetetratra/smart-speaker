package realtime

// instructions は、今の指示文を返す。
func (s *stage) instructions() string {
	if s.cfg.Instructions == nil {
		return ""
	}
	return s.cfg.Instructions()
}

// tools は、今のツール定義に会話制御のツールを足して返す。
func (s *stage) tools() []any {
	var definitions []any
	if s.cfg.Tools != nil {
		definitions = s.cfg.Tools()
	}
	return withWaitTool(definitions)
}

// markToolsChanged は、ツールが変わったことを受け取る。
// 接続していなければ何もしない（次に接続するときに今の値を読む）。接続中なら、静かになってから送り直す。
func (s *stage) markToolsChanged() {
	if s.transport == nil {
		return
	}
	s.toolsDirty = true
	s.flushToolsUpdate()
}

// flushToolsUpdate は、ツールが変わっていれば、接続中で静かなときに session.update で今の指示文とツールを送り直す。
// 応答の途中や、ツールの結果を待っている間は送らない。tick でも呼び、静かになるのを待つ。
// 送ったあとに届く session.updated は、ready のあとなので何もしない。
func (s *stage) flushToolsUpdate() {
	if !s.toolsDirty || s.transport == nil || !s.ready {
		return
	}
	if s.speaking || s.active != "" || s.wantResponse || len(s.repairs) > 0 || s.pendingTools() {
		return
	}
	s.toolsDirty = false
	s.transport.send(toolsUpdate(s.instructions(), s.tools()))
}
