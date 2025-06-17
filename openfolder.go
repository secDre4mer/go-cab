package cab

import (
	"errors"
	"io"

	"github.com/secDre4mer/go-cab/lzx"
	"github.com/secDre4mer/go-cab/mszip"
)

const compressionTypeMask = 0xF

func (folder cabinetFileFolder) open() (io.ReadCloser, error) {
	var dataReaders = make([]io.ReadCloser, len(folder.dataEntries))
	for i := range folder.dataEntries {
		dataReader, err := openFileData(&folder.dataEntries[i])
		if err != nil {
			return nil, err
		}
		dataReaders[i] = dataReader
	}
	switch folder.CompressionType & compressionTypeMask {
	case compressionTypeNone:
		return &multiReader{Readers: dataReaders}, nil
	case compressionTypeMszip:
		return mszip.New(dataReaders), nil
	case compressionTypeQuantum:
		return nil, errors.New("quantum compression is not supported yet")
	case compressionTypeLzx:
		windowSize := 1 << int((folder.CompressionType>>8)&0x1F)
		lzxReader, err := lzx.New(&multiReader{Readers: dataReaders}, int(windowSize), 0)
		if err != nil {
			return nil, err
		}
		return io.NopCloser(lzxReader), nil
	default:
		return nil, errors.New("unknown compression type")
	}
}

const (
	compressionTypeNone    = 0
	compressionTypeMszip   = 1
	compressionTypeQuantum = 2
	compressionTypeLzx     = 3
)
