package wsserver

import (
	"context"
	"errors"
	"log"
	"net/http"
	"sync"

	"smart-speaker/internal/graph"
	types "smart-speaker/internal/types"
)

type serverStage struct {
	server *http.Server
	once   sync.Once
}

// NewStage は WebSocket 用 HTTP サーバーを起動します。
func NewStage(server *http.Server) *graph.Stage {
	stage := &serverStage{server: server}
	downstream := make(chan types.Event)
	close(downstream)
	return &graph.Stage{
		Downstream: downstream,
		Run:        stage.run,
		CloseFn:    stage.close,
	}
}

func (s *serverStage) run(context.Context) {
	if s.server == nil {
		return
	}
	go func() {
		if err := s.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("wsserver listen error: %v", err)
		}
	}()
}

func (s *serverStage) close() error {
	var err error
	s.once.Do(func() {
		if s.server != nil {
			err = s.server.Close()
		}
	})
	return err
}
