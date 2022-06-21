package cab

import (
	"encoding/binary"
	"errors"
	"io"
)

// openFileData returns an io.ReadCloser that verifies the checksum of the entry, if it exists.
func openFileData(entry *cabinetFileData) (io.ReadCloser, error) {
	if entry.UncompressedBytes == 0 {
		// continued entry, see https://docs.microsoft.com/en-us/previous-versions//bb267310(v=vs.85)#cfdata
		return nil, errors.New("continued entries are not supported")
	}
	// Open a separate section reader for this file data reader to prevent race conditions on the underlying section reader
	reader := io.NewSectionReader(entry.compressedData, 0, entry.compressedData.Size())
	return &dataEntryReader{entry, reader, checksumWriter{}}, nil
}

type dataEntryReader struct {
	entry    *cabinetFileData
	reader   io.Reader
	checksum checksumWriter
}

func (d *dataEntryReader) Read(data []byte) (n int, err error) {
	n, err = d.reader.Read(data)
	if d.entry.Checksum != 0 {
		// Write data for later checksum check
		d.checksum.Write(data[:n])
	}
	return n, err
}

func (d *dataEntryReader) Close() (err error) {
	if d.entry.Checksum == 0 {
		return nil // No checksum set for this entry
	}
	if d.checksum.Checksum == 0 { // No data read yet - no reason to verify checksum
		return nil
	}
	// Copy remaining data from underlying reader to ensure we can verify the checksum
	_, err = io.Copy(&d.checksum, d.reader)
	if err != nil {
		return err
	}
	d.checksum.Flush()
	// After calculating the checksum over the data, we must feed the data entry header (minus checksum) to the checksum
	binary.Write(&d.checksum, binary.LittleEndian, checksumlessEntry{
		d.entry.CompressedBytes,
		d.entry.UncompressedBytes,
	})
	d.checksum.Write(d.entry.reservedData)
	d.checksum.Flush()
	if d.checksum.Checksum != d.entry.Checksum && d.entry.Checksum != 0 {
		return errors.New("checksum mismatch")
	} else {
		// Set checksum on entry to 0 to avoid checking it again later
		d.entry.Checksum = 0
	}
	return nil
}

type checksumlessEntry struct {
	CompressedBytes   uint16
	UncompressedBytes uint16
}
