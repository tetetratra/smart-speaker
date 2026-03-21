package conversation

import (
	"log"

	types "smart-speaker/internal/types"
)

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
			messages := r.contexts.WithSystemContexts(r.ctx, e.messages)
			if len(messages) == 0 {
				continue
			}
			r.emit(types.Event{
				Kind: types.EventResponsesRequest,
				Payload: types.ResponsesRequest{
					RequestID: e.requestID,
					Messages:  messages,
					Tools:     e.tools,
				},
			})
		case logRecordEffect:
			r.logger.Write(e.record)
		case runtimeLogEffect:
			if e.message != "" {
				log.Print(e.message)
			}
		}
	}
}
