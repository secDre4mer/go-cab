package cab

import (
	"io"
	"io/fs"
	"path"
	"time"
)

type File struct {
	Name       string
	Modified   time.Time
	Attributes uint16

	header cabinetFileEntryHeader
	folder *cabinetFileFolder
}

const (
	AttributeReadOnly = 0x1
	AttributeHidden   = 0x2
	AttributeSystem   = 0x4
	AttributeArch     = 0x20
	AttributeExec     = 0x40
	AttributeNameUtf  = 0x80
)

func (f *File) Open() (io.ReadCloser, error) {
	folderReader, err := f.folder.OpenAt(int64(f.header.UncompressedOffsetInFolder))
	if err != nil {
		return nil, err
	}
	return limitReadCloser(folderReader, int64(f.header.UncompressedFileSize)), nil
}

func limitReadCloser(rc io.ReadCloser, limit int64) io.ReadCloser {
	return struct {
		io.Reader
		io.Closer
	}{
		Reader: io.LimitReader(rc, limit),
		Closer: rc,
	}
}

func (f *File) Stat() fs.FileInfo {
	return FileInfo{f}
}

type FileInfo struct {
	File *File
}

func (f FileInfo) Name() string {
	return path.Base(f.File.Name)
}

func (f FileInfo) Size() int64 {
	return int64(f.File.header.UncompressedFileSize)
}

func (f FileInfo) Mode() fs.FileMode {
	return 0
}

func (f FileInfo) ModTime() time.Time {
	return f.File.Modified
}

func (f FileInfo) IsDir() bool {
	return false
}

func (f FileInfo) Sys() any {
	return f.File
}
