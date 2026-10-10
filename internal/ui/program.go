package ui

import (
	"context"
	"errors"
	"io"
	"reflect"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/term"
	"github.com/muesli/cancelreader"
)

var errInputEOF = errors.New("terminal input ended")

type programReader struct {
	io.Reader
	eof bool
}

func (r *programReader) Read(p []byte) (int, error) {
	if r.eof {
		return 0, errInputEOF
	}
	n, err := r.Reader.Read(p)
	// Ultraviolet discards io.EOF; a distinct error reaches the event loop after pending input.
	if errors.Is(err, io.EOF) {
		r.eof = true
		if n > 0 {
			return n, nil
		}
		err = errInputEOF
	}
	return n, err
}

type programFile struct {
	cancelreader.File
	reader *programReader
}

func (f programFile) Read(p []byte) (int, error) {
	return f.reader.Read(p)
}

func runProgram(ctx context.Context, model tea.Model, input io.Reader, output io.Writer) error {
	if input != nil {
		reader := &programReader{Reader: input}
		if file, ok := input.(cancelreader.File); ok {
			input = programFile{file, reader}
		} else {
			input = reader
		}
	}
	// Graceful quit joins the terminal reader; Bubble Tea's forced shutdown does not.
	options := []tea.ProgramOption{tea.WithInput(input), tea.WithOutput(output),
		tea.WithContext(context.WithoutCancel(ctx)), tea.WithoutSignalHandler()}
	var resizeErr error
	if file, ok := output.(term.File); ok && term.IsTerminal(file.Fd()) {
		requestType := reflect.TypeOf(tea.RequestWindowSize())
		options = append(options, tea.WithFilter(func(_ tea.Model, msg tea.Msg) tea.Msg {
			if reflect.TypeOf(msg) != requestType {
				return msg
			}
			// Bubble Tea otherwise starts an unjoined resize goroutine that can outlive the terminal.
			width, height, err := term.GetSize(file.Fd())
			if err != nil {
				resizeErr = err
				return tea.Quit()
			}
			return tea.WindowSizeMsg{Width: width, Height: height}
		}))
	}
	p := tea.NewProgram(model, options...)
	done := make(chan struct{})
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		select {
		case <-ctx.Done():
			p.Quit()
		case <-done:
		}
	}()
	_, err := p.Run()
	close(done)
	<-stopped
	if ctx.Err() != nil {
		return ErrCanceled
	}
	if errors.Is(err, errInputEOF) {
		return io.EOF
	}
	if err == nil {
		return resizeErr
	}
	return err
}
