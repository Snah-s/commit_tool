package main

import (
	"errors"
	"os"
)

func openControllingTerminal() (*os.File, error) {
	return nil, errors.New("Windows hooks await validation; prototype is non-interactive")
}
