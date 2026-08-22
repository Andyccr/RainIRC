package protocol

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

var (
	ErrTooLarge  = errors.New("message exceeds size limit")
	ErrMalformed = errors.New("malformed message")
)

// Encode returns one NDJSON line (JSON object + newline).
func Encode(m *Message) ([]byte, error) {
	if m == nil {
		return nil, fmt.Errorf("nil message")
	}
	data, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	if len(data) > MaxMessageSize {
		return nil, ErrTooLarge
	}
	out := make([]byte, 0, len(data)+1)
	out = append(out, data...)
	out = append(out, '\n')
	return out, nil
}

func Write(w io.Writer, m *Message) error {
	data, err := Encode(m)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

// Read reads one NDJSON message from r, capped at max bytes (excluding newline).
func Read(r *bufio.Reader, max int) (*Message, error) {
	if max <= 0 {
		max = MaxMessageSize
	}
	line, err := readLineLimited(r, max)
	if err != nil {
		return nil, err
	}
	line = bytes.TrimSpace(line)
	if len(line) == 0 {
		return nil, ErrMalformed
	}
	var m Message
	if err := json.Unmarshal(line, &m); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMalformed, err)
	}
	if err := m.ValidateEnvelope(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMalformed, err)
	}
	return &m, nil
}

func readLineLimited(r *bufio.Reader, max int) ([]byte, error) {
	var buf []byte
	for {
		part, err := r.ReadSlice('\n')
		if len(buf)+len(part) > max+1 { // +1 allows the terminating newline
			return nil, ErrTooLarge
		}
		if err == bufio.ErrBufferFull {
			buf = append(buf, part...)
			continue
		}
		if err != nil {
			if errors.Is(err, io.EOF) && len(buf)+len(part) > 0 {
				return nil, io.ErrUnexpectedEOF
			}
			return nil, err
		}
		// part includes the newline
		n := len(part)
		if n > 0 && part[n-1] == '\n' {
			n--
			if n > 0 && part[n-1] == '\r' {
				n--
			}
		}
		buf = append(buf, part[:n]...)
		if len(buf) > max {
			return nil, ErrTooLarge
		}
		return buf, nil
	}
}
