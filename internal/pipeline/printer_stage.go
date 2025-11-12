package pipeline

import (
	"context"
	"errors"
	"log"

	"smart-speaker/internal/components/printer"
	types "smart-speaker/internal/types"
)

// PrinterStage consumes assistant outputs and prints them.
type PrinterStage struct {
	printer *printer.Printer
}

func NewPrinterStage() *PrinterStage {
	return &PrinterStage{printer: printer.New()}
}

func (p *PrinterStage) Process(ctx context.Context, upstream <-chan interface{}) <-chan interface{} {
	if upstream == nil {
		log.Printf("printer stage requires an upstream source")
		return nil
	}
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case data, ok := <-upstream:
				if !ok {
					return
				}
				line, ok := data.(types.OutputLine)
				if !ok {
					log.Printf("unexpected upstream data type: %T", data)
					continue
				}
				if err := p.printer.Process(ctx, line); err != nil {
					if errors.Is(err, context.Canceled) {
						return
					}
					log.Printf("printer error: %v", err)
					return
				}
			}
		}
	}()
	return nil
}

func (p *PrinterStage) Close() error {
	return p.printer.Close()
}
