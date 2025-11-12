package interfaces

import "context"

// Reader synchronously retrieves the next item in a stream.
type Reader[T any] interface {
	Read(ctx context.Context) (T, error)
}

// Processor handles a single item synchronously.
type Processor[T any] interface {
	Process(ctx context.Context, data T) error
}
