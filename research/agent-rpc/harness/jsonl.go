package harness

import (
	"bufio"
	"bytes"
	"io"
)

func readJSONLLine(r *bufio.Reader) ([]byte, error) {
	var buf bytes.Buffer
	for {
		frag, err := r.ReadBytes('\n')
		if len(frag) > 0 {
			buf.Write(frag)
			if frag[len(frag)-1] == '\n' {
				line := bytes.TrimSuffix(buf.Bytes(), []byte{'\n'})
				line = bytes.TrimSuffix(line, []byte{'\r'})
				out := make([]byte, len(line))
				copy(out, line)
				return out, nil
			}
		}
		if err != nil {
			if err == io.EOF && buf.Len() > 0 {
				line := bytes.TrimSuffix(buf.Bytes(), []byte{'\r'})
				out := make([]byte, len(line))
				copy(out, line)
				return out, nil
			}
			return nil, err
		}
	}
}
