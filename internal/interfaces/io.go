package interfaces

import (
	"context"
	"io"
)

// ストリームから順番に要素を取り出す
type Reader[T any] interface {
	Read(ctx context.Context) (T, error)
	io.Closer
}

// 受け取った要素をその場で処理する
type Processor[T any] interface {
	Process(ctx context.Context, data T) error
}
