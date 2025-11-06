package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
)

type ResponsePrinter struct {
	ctx    context.Context
	cancel context.CancelFunc
	in     <-chan OutputLine
	once   sync.Once
	writer *bufio.Writer
}

func NewResponsePrinter(ctx context.Context, in <-chan OutputLine) *ResponsePrinter {
	runCtx, cancel := context.WithCancel(ctx)
	return &ResponsePrinter{
		ctx:    runCtx,
		cancel: cancel,
		in:     in,
		writer: bufio.NewWriter(os.Stdout),
	}
}

func (p *ResponsePrinter) Run() {
	p.once.Do(func() {
		go p.loop()
	})
}

func (p *ResponsePrinter) loop() {
	defer p.flush()
	for {
		select {
		case <-p.ctx.Done():
			return
		case line, ok := <-p.in:
			if !ok {
				return
			}
			label := renderRoleLabel(line.Role)
			if label != "" {
				fmt.Fprintf(p.writer, "%s: %s\n", label, line.Text)
			} else {
				fmt.Fprintln(p.writer, line.Text)
			}
			p.flush()
		}
	}
}

func (p *ResponsePrinter) flush() {
	if err := p.writer.Flush(); err != nil {
		log.Printf("flush error: %v", err)
	}
}

func (p *ResponsePrinter) Close() {
	p.cancel()
}

func renderRoleLabel(role string) string {
	switch role {
	case "assistant":
		return "Assistant"
	case "user":
		return "You"
	case "error":
		return strings.Title(role)
	default:
		if role == "" {
			return "Assistant"
		}
		return strings.Title(role)
	}
}
