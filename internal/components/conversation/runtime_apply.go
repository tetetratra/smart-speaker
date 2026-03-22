package conversation

func (r *runner) applyEffects(effects []effect) {
	for _, eff := range effects {
		switch e := eff.(type) {
		case emitEventEffect:
			r.emit(e.event)
		case startTimerEffect:
			r.startTimer(e.duration)
		case stopTimerEffect:
			r.stopTimer()
		case requestResponseEffect:
			r.applyRequestResponseEffect(e)
		case logRecordEffect:
			r.applyLogRecordEffect(e)
		case runtimeLogEffect:
			r.applyRuntimeLogEffect(e)
		}
	}
}
