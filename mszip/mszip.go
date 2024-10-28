package mszip

import (
	"compress/flate"
	"errors"
	"io"
)

func New(blocks []io.ReadCloser) io.ReadCloser {
	return &msZipReader{
		blocks: blocks,
		lastReadBytes: ringBuffer{
			buf: make([]byte, 0, maxWindow),
		},
	}
}

type msZipReader struct {
	currentBlock  io.Reader
	lastReadBytes ringBuffer

	blocks []io.ReadCloser
	dict   []byte
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
	t.currentBlock = io.TeeReader(flate.NewReaderDict(block, t.dict), &t.lastReadBytes)
	return nil
}

func (t *msZipReader) closeCurrentBlock() error {
	// Pop current block from list
	finishedBlock := t.blocks[0]
	t.blocks = t.blocks[1:]

	// Update dict, remove block from read
	t.dict = t.lastReadBytes.Data()
	t.lastReadBytes.Clear()
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
			} else {
				return n, nil
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

const maxWindow = 1 << 15 // Maximum size of a DEFLATE window

type ringBuffer struct {
	buf         []byte
	rotateIndex int // Index of the first byte in the buffer
}

func (r *ringBuffer) Write(p []byte) (n int, err error) {
	n = len(p) // We always write all data; store the length for return value later (since we may modify p)

	if len(p) > cap(r.buf) {
		// Data is larger than the buffer, only keep the last cap(r.buf) bytes
		p = p[len(p)-cap(r.buf):]
	}

	// Expand buffer if we haven't reached full capacity yet
	if len(r.buf) < cap(r.buf) {
		if len(r.buf)+len(p) < cap(r.buf) {
			// Buffer has enough space to fit all data
			r.buf = append(r.buf, p...)
			return n, nil
		}
		// Fit as much data as possible into the buffer, then start rotating
		fittingData := cap(r.buf) - len(r.buf)
		r.buf = append(r.buf, p[:fittingData]...)
		p = p[fittingData:]
	}
	// Buffer is full, write the data to the buffer and rotate
	for len(p) > 0 {
		dataWritten := copy(r.buf[r.rotateIndex:], p)
		p = p[dataWritten:]
		r.rotateIndex = (r.rotateIndex + dataWritten) % len(r.buf)
	}
	return n, nil
}

func (r *ringBuffer) Data() []byte {
	return append(r.buf[r.rotateIndex:], r.buf[:r.rotateIndex]...)
}

func (r *ringBuffer) Clear() {
	r.buf = r.buf[:0]
	r.rotateIndex = 0
}
