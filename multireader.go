package cab

import "io"

// multiReader is an equivalent of io.MultiReader, but for io.ReadCloser instead of io.Reader.
type multiReader struct {
	Readers []io.ReadCloser
}

func (t *multiReader) Read(b []byte) (n int, err error) {
	for len(t.Readers) > 0 {
		reader := t.Readers[0]
		n, err = reader.Read(b)
		if err == io.EOF {
			t.Readers = t.Readers[1:]
			if err := reader.Close(); err != nil {
				return n, err
			}
			if n == 0 { // We must not return 0 bytes with no error, try reading from next reader
				continue
			}
		} else {
			return n, err
		}
	}
	return n, io.EOF
}

func (t *multiReader) Close() (err error) {
	for _, reader := range t.Readers {
		if readerErr := reader.Close(); readerErr != nil {
			if err == nil {
				err = readerErr
			}
		}
	}
	t.Readers = nil
	return
}
