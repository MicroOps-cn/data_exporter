package common

import (
	"bufio"
	"bytes"
	"fmt"
	"gopkg.in/yaml.v3"
	"io"
)

type SliceString []string

func (s *SliceString) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.SequenceNode {
		type plain SliceString
		return value.Decode((*plain)(s))
	} else if value.Kind == yaml.MappingNode || value.ShortTag() == "!!str" {
		var c string
		if err := value.Decode(&c); err != nil {
			return err
		}
		*s = append(*s, c)
		return nil
	} else if value.Kind == yaml.AliasNode {
		return value.Alias.Decode(s)
	} else {
		return fmt.Errorf("unsupport type, expected map, list, or string, position: Line: %d,Column:%d", value.Line, value.Column)
	}
}

type ReadLineCloser interface {
	io.Closer
	ReadLine() ([]byte, error)
}
type LineBuffer struct {
	io.Closer
	buf *bufio.Scanner
}

func (r *LineBuffer) ReadLine() ([]byte, error) {
	fmt.Println(">>>>>>>> scan", r, r.buf)
	if r.buf.Scan() {

		fmt.Println(">>>>>>>> scan", r, r.buf.Bytes())
		return r.buf.Bytes(), nil
	} else {
		return nil, io.EOF
	}
}

var _ ReadLineCloser = &LineBuffer{}

func NewLineBuffer(rc io.ReadCloser, maxRead int64, lineMaxRead int, lineSep SliceString, endOf []byte) ReadLineCloser {
	buf := &LineBuffer{
		Closer: rc,
	}
	if maxRead > 0 {
		buf.buf = bufio.NewScanner(io.LimitReader(rc, maxRead))
	} else {
		buf.buf = bufio.NewScanner(rc)
	}
	fmt.Println("..........", maxRead, lineMaxRead, lineSep, endOf)
	if len(endOf) > 0 || len(lineSep) > 1 || (len(lineSep) == 1 && lineSep[0] != "\n") {
		buf.buf.Split(func(data []byte, atEOF bool) (advance int, token []byte, err error) {
			if len(endOf) > 0 {
				if i := bytes.Index(data, endOf); i >= 0 {
					data = data[:i]
				}
			}
			if atEOF && len(data) == 0 {
				return 0, nil, nil
			}
			var minIndex int = -1
			var minSep string
			for _, sep := range lineSep {
				if i := bytes.Index(data, []byte(sep)); i >= 0 {
					if minIndex < 0 || i < minIndex {
						minIndex = i
						minSep = sep
					}
				}
			}
			if minIndex >= 0 {
				if minIndex+len([]byte(minSep)) <= lineMaxRead || lineMaxRead <= 0 {
					return minIndex + len([]byte(minSep)), dropCR(data[0:minIndex]), nil
				} else {
					return lineMaxRead, dropCR(data[:lineMaxRead]), nil
				}
			}

			// If we're at EOF, we have a final, non-terminated line. Return it.
			if atEOF {
				if len(data) <= lineMaxRead || lineMaxRead <= 0 {
					return len(data), dropCR(data), nil
				} else {
					return lineMaxRead, dropCR(data[:lineMaxRead]), nil
				}
			}
			// Request more data.
			return 0, nil, nil
		})
	}
	return buf
}

func dropCR(data []byte) []byte {
	if len(data) > 0 && data[len(data)-1] == '\r' {
		return data[0 : len(data)-1]
	}
	return data
}
