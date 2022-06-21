package mszip

import (
	"compress/flate"
	"errors"
	"io"
)

func New(blocks []io.ReadCloser) io.ReadCloser {
	return &msZipReader{
		blocks: blocks,
	}
}

type msZipReader struct {
	currentBlock *memoryReader
	blocks       []io.ReadCloser
	dict         []byte
}

func checkBlockHeader(block io.Reader) error {
	var header [2]byte
	if _, err := io.ReadFull(block, header[:]); err != nil {
		return err
	}
	if header[0] != 0x43 || header[1] != 0x4B {
		return errors.New("invalid MS-ZIP header")
	}
	return nil
}

func (t *msZipReader) openCurrentBlock() error {
	if t.currentBlock != nil {
		return nil // Block already loaded
	}
	if len(t.blocks) == 0 {
		// No block we can use
		return io.EOF
	}
	block := t.blocks[0]
	if err := checkBlockHeader(block); err != nil {
		return err
	}
	t.currentBlock = &memoryReader{
		Reader: flate.NewReaderDict(block, t.dict),
	}
	return nil
}

func (t *msZipReader) closeCurrentBlock() error {
	// Pop current block from list
	finishedBlock := t.blocks[0]
	t.blocks = t.blocks[1:]

	// Update dict, remove block from read
	t.dict = t.currentBlock.B
	t.currentBlock = nil

	if err := finishedBlock.Close(); err != nil {
		return err
	}
	return nil
}

func (t *msZipReader) Read(b []byte) (n int, err error) {
	for {
		if err := t.openCurrentBlock(); err != nil {
			return 0, err
		}
		n, err = t.currentBlock.Read(b)
		if err == io.EOF {
			if err := t.closeCurrentBlock(); err != nil {
				return n, err
			}
			if n == 0 { // We must not return 0 bytes with no error, try reading from next reader
				continue
			}
		}
		return n, err
	}
}

func (t *msZipReader) Close() (err error) {
	for _, reader := range t.blocks {
		if readerErr := reader.Close(); readerErr != nil {
			if err == nil {
				err = readerErr
			}
		}
	}
	t.blocks = nil
	t.currentBlock = nil
	return
}

// memoryReader wraps an io.Reader remembers up to 32KiB
// of the last bytes read.
type memoryReader struct {
	io.Reader
	B []byte
}

func (mr *memoryReader) Read(b []byte) (int, error) {
	const maxWindow = 1 << 15 // Maximum size of a DEFLATE window
	n, err := mr.Reader.Read(b)

	var readData = b[:n]
	if len(readData) >= maxWindow {
		if len(mr.B) != maxWindow {
			mr.B = make([]byte, maxWindow)
		}
		copy(mr.B, readData[len(readData)-maxWindow:])
	} else {
		mr.B = append(mr.B, readData...)
		if len(mr.B) > maxWindow {
			mr.B = mr.B[len(mr.B)-maxWindow:]
		}
	}
	return n, err
}
