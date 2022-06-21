package cab

import (
	"errors"
	"io"

	"github.com/secDre4mer/go-cab/mszip"
)

func (folder cabinetFileFolder) open() (io.ReadCloser, error) {
	var dataReaders = make([]io.ReadCloser, len(folder.dataEntries))
	for i := range folder.dataEntries {
		dataReader, err := openFileData(&folder.dataEntries[i])
		if err != nil {
			return nil, err
		}
		dataReaders[i] = dataReader
	}
	switch folder.CompressionType {
	case compressionTypeNone:
		return &multiReader{Readers: dataReaders}, nil
	case compressionTypeMszip:
		return mszip.New(dataReaders), nil
	default:
		return nil, errors.New("unknown compression type")
	}
}

const (
	compressionTypeNone  = 0
	compressionTypeMszip = 1
)
