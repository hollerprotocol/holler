package wire

import (
	"bufio"
	"errors"
	"io"
	"sync"
)

// ErrLineTooLong means a line exceeded MaxLine (too_large).
var ErrLineTooLong = errors.New("line exceeds 1 MiB")

// Reader reads newline-terminated lines of at most MaxLine bytes.
type Reader struct {
	br  *bufio.Reader
	max int
}

// NewReader returns a Reader over r.
func NewReader(r io.Reader) *Reader {
	return &Reader{br: bufio.NewReaderSize(r, 64<<10), max: MaxLine}
}

// ReadLine returns the next line without its trailing newline. The returned
// slice is owned by the caller. A final line without a newline is reported
// as io.ErrUnexpectedEOF, since a well-formed peer always terminates lines.
func (r *Reader) ReadLine() ([]byte, error) {
	var line []byte
	for {
		frag, err := r.br.ReadSlice('\n')
		if len(line)+len(frag) > r.max+1 {
			return nil, ErrLineTooLong
		}
		line = append(line, frag...)
		switch {
		case err == nil:
			return line[:len(line)-1], nil
		case errors.Is(err, bufio.ErrBufferFull):
			continue
		case errors.Is(err, io.EOF) && len(line) > 0:
			return nil, io.ErrUnexpectedEOF
		default:
			return nil, err
		}
	}
}

// Writer writes whole lines. It is safe for concurrent use; each line is
// written with a single Write call so lines never interleave.
type Writer struct {
	mu sync.Mutex
	w  io.Writer
}

// NewWriter returns a Writer over w.
func NewWriter(w io.Writer) *Writer {
	return &Writer{w: w}
}

// WriteLine writes line followed by a newline.
func (w *Writer) WriteLine(line []byte) error {
	if len(line) > MaxLine {
		return ErrLineTooLong
	}
	buf := make([]byte, len(line)+1)
	copy(buf, line)
	buf[len(line)] = '\n'
	w.mu.Lock()
	defer w.mu.Unlock()
	_, err := w.w.Write(buf)
	return err
}
