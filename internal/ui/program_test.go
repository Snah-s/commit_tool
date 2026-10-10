package ui

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"testing/iotest"
	"time"
)

func TestInlineSelectorEOF(t *testing.T) {
	for _, source := range []string{"reader", "pipe"} {
		t.Run(source, func(t *testing.T) {
			var input io.Reader = strings.NewReader("")
			if source == "pipe" {
				reader, writer, err := os.Pipe()
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { reader.Close() })
				if err := writer.Close(); err != nil {
					t.Fatal(err)
				}
				input = reader
			}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			draft, err := Run(ctx, input, io.Discard, testCatalog(t), Draft{}, Options{Accessible: true, InlineSelectors: true})
			if !errors.Is(err, io.EOF) || ctx.Err() != nil || draft != (Draft{}) {
				t.Fatalf("EOF: draft=%+v, err=%v, context=%v", draft, err, ctx.Err())
			}
		})
	}
}

func TestInlineSelectorCancelBeforeEOF(t *testing.T) {
	for _, key := range []string{"\x03", "\x1b"} {
		t.Run(key, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			input := iotest.DataErrReader(strings.NewReader(key))
			_, err := Run(ctx, input, io.Discard, testCatalog(t), Draft{}, Options{Accessible: true, InlineSelectors: true})
			if !errors.Is(err, ErrCanceled) || ctx.Err() != nil {
				t.Fatalf("last key: err=%v, context=%v", err, ctx.Err())
			}
		})
	}
}
