package ui

import (
	"context"
	"errors"
	"io"
	"os"
	"reflect"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/term"
	"github.com/creack/pty"
)

type eofTerminal struct {
	*os.File
	initial *term.State
	raw     bool
}

func (f *eofTerminal) Read([]byte) (int, error) {
	state, err := term.GetState(f.Fd())
	if err != nil {
		return 0, err
	}
	f.raw = !reflect.DeepEqual(state, f.initial)
	return 0, io.EOF
}

func TestInlineSelectorEOFRestoresTerminal(t *testing.T) {
	master, slave, err := pty.Open()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { master.Close(); slave.Close() })
	initial, err := term.GetState(slave.Fd())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(master, "\n"); err != nil {
		t.Fatal(err)
	}
	input := &eofTerminal{File: slave, initial: initial}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err = Run(ctx, input, io.Discard, testCatalog(t), Draft{}, Options{Accessible: true, InlineSelectors: true})
	if !errors.Is(err, io.EOF) || ctx.Err() != nil || !input.raw {
		t.Fatalf("terminal EOF: err=%v, context=%v, raw=%v", err, ctx.Err(), input.raw)
	}
	state, err := term.GetState(slave.Fd())
	if err != nil || !reflect.DeepEqual(state, initial) {
		t.Fatalf("terminal was not restored: %v", err)
	}
}

type resizeThenQuit struct{}

func (resizeThenQuit) Init() tea.Cmd                         { return tea.Sequence(tea.RequestWindowSize, tea.Quit) }
func (m resizeThenQuit) Update(tea.Msg) (tea.Model, tea.Cmd) { return m, nil }
func (resizeThenQuit) View() tea.View                        { return tea.NewView("") }

func TestWindowSizeRequestBeforeTerminalClose(t *testing.T) {
	for range 20 {
		master, slave, err := pty.Open()
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		err = runProgram(ctx, resizeThenQuit{}, slave, slave)
		cancel()
		slave.Close()
		master.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
}
