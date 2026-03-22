package conversation

import "log"

func (r *runner) applyLogRecordEffect(e logRecordEffect) {
	r.logger.Write(e.record)
}

func (r *runner) applyRuntimeLogEffect(e runtimeLogEffect) {
	if e.message != "" {
		log.Print(e.message)
	}
}
