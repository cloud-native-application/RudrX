package util

import (
	"bytes"
	"fmt"
	"io"
	"os"
)

// IOStreams provides the standard names for iostreams.  This is useful for embedding and for unit testing.
// Inconsistent and different names make it hard to read and review code
type IOStreams struct {
	// In think, os.Stdin
	In io.Reader
	// Out think, os.Stdout
	Out io.Writer
	// ErrOut think, os.Stderr
	ErrOut io.Writer
}

func PrintErrorMessage(errorMessage string, exitCode int) {
	fmt.Println(errorMessage)
	os.Exit(exitCode)
}

// NewTestIOStreams returns a valid IOStreams and in, out, errout buffers for unit tests
func NewTestIOStreams() (IOStreams, *bytes.Buffer, *bytes.Buffer, *bytes.Buffer) {
	in := &bytes.Buffer{}
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}

	return IOStreams{
		In:     in,
		Out:    out,
		ErrOut: errOut,
	}, in, out, errOut
}

//Infonln compared to Info(), won't print new line
func (i *IOStreams) Infonln(a ...interface{}) {
	_, _ = i.Out.Write([]byte(fmt.Sprint(a...)))
}

func (i *IOStreams) Info(a ...interface{}) {
	_, _ = i.Out.Write([]byte(fmt.Sprintln(a...)))
}

func (i *IOStreams) Infof(format string, a ...interface{}) {
	_, _ = i.Out.Write([]byte(fmt.Sprintf(format, a...)))
}

func (i *IOStreams) Errorf(format string, a ...interface{}) {
	_, _ = i.ErrOut.Write([]byte(fmt.Sprintf(format, a...)))
}

func (i *IOStreams) Error(a ...interface{}) {
	_, _ = i.ErrOut.Write([]byte(fmt.Sprintln(a...)))
}
