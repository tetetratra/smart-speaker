package printer

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"

	"smart-speaker/internal/interfaces"
	types "smart-speaker/internal/types"
)

var _ interfaces.Processor[types.OutputLine] = (*Printer)(nil)

// 出力行を標準出力へ書き出す
type Printer struct {
	writer *bufio.Writer
	mu     sync.Mutex
}

// Printer を生成する
func New() *Printer {
	return &Printer{writer: bufio.NewWriter(os.Stdout)}
}

// 1 行分の出力を描画する
func (p *Printer) Process(ctx context.Context, line types.OutputLine) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	label := renderRoleLabel(line.Role)
	if label == "" {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, err := fmt.Fprintf(p.writer, "%s: %s\n", label, line.Text); err != nil {
		return err
	}
	return p.writer.Flush()
}

// バッファの内容をフラッシュする
func (p *Printer) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.writer.Flush(); err != nil {
		log.Printf("flush error: %v", err)
	}
	return nil
}

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
