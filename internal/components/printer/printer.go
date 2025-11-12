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

// Printer writes output lines to stdout.
type Printer struct {
	writer *bufio.Writer
	mu     sync.Mutex
}

// New constructs a Printer.
func New() *Printer {
	return &Printer{writer: bufio.NewWriter(os.Stdout)}
}

// Process prints a single line.
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

// Close flushes any buffered output.
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
