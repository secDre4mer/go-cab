package cab

import (
	"io"
	"sync"
)

// cabinetFileFolder represents a folder in the CAB file, which contains compressed data for one or more files.
// The data exists in a single compression stream, with the files being sequentially concatenated in the uncompressed data.
//
// To allow efficient access to the content, this struct contains a cachedReader. If the files are read
// in sequential order, the reader is reused, allowing the stream to be read in a single decompression pass
// instead of having to decompress it for each file.
type cabinetFileFolder struct {
	cabinetFileFolderHeader
	reservedData []byte

	dataEntries []cabinetFileData

	// cachedReader for more efficient sequential access
	cachedReader      *folderReader
	cachedReaderMutex sync.Mutex
}

// OpenAt opens a reader that reads the folder content starting from the given offset.
func (folder *cabinetFileFolder) OpenAt(offset int64) (io.ReadCloser, error) {
	reader, err := folder.openReaderFor(offset)
	if err != nil {
		return nil, err
	}

	// Skip forward to requested offset
	if _, err := io.CopyN(io.Discard, reader, offset-reader.Offset); err != nil {
		_ = reader.Close()
		return nil, err
	}

	return reader, nil
}

// openReaderFor opens a folderReader that can be used to read from the specified offset.
// It reuses the cached reader if possible; otherwise it opens a new reader. A newly opened
// reader is cached for later reuse when it is closed.
//
// It does not place the reader at the specified offset; it only guarantees that the returned
// reader is at an offset less than or equal to the requested one (so it can skip forwards to
// the requested offset).
func (folder *cabinetFileFolder) openReaderFor(offset int64) (*folderReader, error) {
	folder.cachedReaderMutex.Lock()
	defer folder.cachedReaderMutex.Unlock()

	if folder.cachedReader != nil && folder.cachedReader.Offset <= offset {
		// Can reuse cached reader
		cachedReader := folder.cachedReader
		folder.cachedReader = nil
		return cachedReader, nil
	}
	return newFolderReader(folder)
}

// cacheReader stores the passed reader for later reuse.
// If there is an old cached reader, it is replaced.
func (folder *cabinetFileFolder) cacheReader(reader *folderReader) {
	// Swap the new (to be cached) reader and the currently cached reader
	folder.cachedReaderMutex.Lock()
	oldCached := folder.cachedReader
	folder.cachedReader = reader
	folder.cachedReaderMutex.Unlock()

	if oldCached != nil {
		// Close the old cached reader.
		// Drop errors since it's unrelated to the caching of the new reader.
		_ = oldCached.Inner.Close()
	}
}
