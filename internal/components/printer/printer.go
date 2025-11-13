package printer

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"

	"smart-speaker/internal/graph"
	"smart-speaker/internal/interfaces"
	types "smart-speaker/internal/types"
)

// Printer は graph.Stage と interfaces.Processor を兼ねる出力シンク。
type Printer struct {
	writer   *bufio.Writer
	upstream chan interface{}
	ctx      context.Context
	cancel   context.CancelFunc
}

func NewPrinter() *Printer {
	ctx, cancel := context.WithCancel(context.Background())
	p := &Printer{
		writer:   bufio.NewWriter(os.Stdout),
		upstream: make(chan interface{}),
		ctx:      ctx,
		cancel:   cancel,
	}
	go p.run()
	return p
}

func (p *Printer) run() {
	for {
		select {
		case <-p.ctx.Done():
			return
		case data, ok := <-p.upstream:
			if !ok {
				return
			}
			evt, ok := data.(types.Event)
			if !ok {
				log.Printf("unexpected upstream data type: %T", data)
				continue
			}
			if evt.Kind != types.EventRealtimeOutput {
				continue
			}
			line, ok := evt.Payload.(types.OutputLine)
			if !ok {
				log.Printf("unexpected event payload type: %T", evt.Payload)
				continue
			}
			if err := p.Process(p.ctx, line); err != nil {
				if errors.Is(err, context.Canceled) {
					return
				}
				log.Printf("printer stage error: %v", err)
				return
			}
		}
	}
}

func (p *Printer) Upstream() chan<- interface{} { return p.upstream }

func (p *Printer) Downstream() <-chan interface{} { return nil }

func (p *Printer) Close() error {
	p.cancel()
	close(p.upstream)
	if err := p.writer.Flush(); err != nil {
		log.Printf("flush error: %v", err)
	}
	return nil
}

func (p *Printer) Process(ctx context.Context, line types.OutputLine) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	label := renderRoleLabel(line.Role)
	if label == "" {
		return nil
	}
	if _, err := fmt.Fprintf(p.writer, "%s: %s\n", label, line.Text); err != nil {
		return err
	}
	return p.writer.Flush()
}

// コンパイル時にインターフェース実装漏れを検出するためのダミー代入。
var _ graph.Stage = (*Printer)(nil)
var _ interfaces.Processor[types.OutputLine] = (*Printer)(nil)

func renderRoleLabel(role string) string {
	switch role {
	case "assistant":
		return "Assistant"
	case "error":
		return strings.Title(role)
	case "user":
		return ""
	default:
		if role == "" {
			return "Assistant"
		}
		return strings.Title(role)
	}
}
