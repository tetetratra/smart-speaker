package conversation

import types "smart-speaker/internal/types"

func (r *runner) applyRequestResponseEffect(e requestResponseEffect) {
	messages := r.contexts.WithSystemContexts(r.ctx, e.messages)
	if len(messages) == 0 {
		return
	}
	r.emit(types.Event{
		Kind: types.EventResponsesRequest,
		Payload: types.ResponsesRequest{
			RequestID: e.requestID,
			Messages:  messages,
			Tools:     e.tools,
		},
	})
}
