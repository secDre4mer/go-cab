package cab

import (
	"errors"
	"io"
	"sync/atomic"

	"github.com/secDre4mer/go-cab/mszip"
	"github.com/secDre4mer/lzx"
)

// folderReader provides a reusable reader on a cabinetFileFolder.
//
// It wraps the actual compressed data and tracks the offset;
// when Close is called, it is cached so that later cabinetFileFolder.OpenAt calls may reuse it.
type folderReader struct {
	Inner  io.ReadCloser
	Offset int64

	folder *cabinetFileFolder
	closed atomic.Bool
}

func (f *folderReader) Read(p []byte) (n int, err error) {
	if f.closed.Load() {
		// Already closed
		return 0, errors.New("reader is closed")
	}
	n, err = f.Inner.Read(p)
	f.Offset += int64(n)
	return
}

func (f *folderReader) Close() error {
	if f.closed.Swap(true) {
		// Already closed
		return nil
	}

	// Cache an (unclosed) copy of this reader so it can be reused later on
	f.folder.cacheReader(&folderReader{
		Inner:  f.Inner,
		Offset: f.Offset,
		folder: f.folder,
	})
	return nil
}

const (
	compressionTypeMask = 0xF

	compressionTypeNone    = 0
	compressionTypeMszip   = 1
	compressionTypeQuantum = 2
	compressionTypeLzx     = 3
)

// newFolderReader opens a new reader for the decompressed data in the specified folder.
func newFolderReader(folder *cabinetFileFolder) (*folderReader, error) {
	var dataReaders = make([]io.ReadCloser, len(folder.dataEntries))
	for i := range folder.dataEntries {
		dataReader, err := openFileData(&folder.dataEntries[i])
		if err != nil {
			return nil, err
		}
		dataReaders[i] = dataReader
	}
	var compressedReader io.ReadCloser
	switch folder.CompressionType & compressionTypeMask {
	case compressionTypeNone:
		compressedReader = &multiReader{Readers: dataReaders}
	case compressionTypeMszip:
		compressedReader = mszip.New(dataReaders)
	case compressionTypeQuantum:
		return nil, errors.New("quantum compression is not supported yet")
	case compressionTypeLzx:
		windowSize := 1 << int((folder.CompressionType>>8)&0x1F)
		lzxReader, err := lzx.New(&multiReader{Readers: dataReaders}, int(windowSize), 0)
		if err != nil {
			return nil, err
		}
		compressedReader = io.NopCloser(lzxReader)
	default:
		return nil, errors.New("unknown compression type")
	}
	// Wrap the reader into a folderReader that can be reused
	return &folderReader{
		Inner:  compressedReader,
		folder: folder,
	}, nil
}
