package clireporter

import "bufio"

type LineBufferedWriter struct {
	*bufio.Writer
}

func (w *LineBufferedWriter) Write(p []byte) (n int, err error) {
	for _, c := range p {
		if err = w.WriteByte(c); err != nil {
			break
		}

		n++
		if c == '\n' {
			w.Flush()
		}
	}
	return n, err
}
