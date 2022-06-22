package cab

import (
	"encoding/binary"
	"errors"
	"io"
	"strings"
	"time"
)

type Cabinet struct {
	Files               []*File
	ReservedHeaderBlock []byte
	MultiCabinetInfo
}

type MultiCabinetInfo struct {
	PreviousFile string
	PreviousDisk string
	NextFile     string
	NextDisk     string
	SetId        uint16 // ID of this multi-cabinet set; should be the same in all files in the set
	SetIndex     uint16 // Index of this cabinet in the multi-cabinet set
}

// Helper flag for fuzzing:
// cabextract assums that CFFILE structs are immediately after CFFOLDER, which is usually the case, but not necessary according to the specification.
// With this flag set, we return an error if this assumption is incorrect
var requireDirectCfFileFollow = false

func Open(reader io.ReaderAt, size int64) (*Cabinet, error) {
	fullReader := io.NewSectionReader(reader, 0, size)
	var cab Cabinet

	var cfHeader cabinetFileHeader
	if err := binary.Read(fullReader, binary.LittleEndian, &cfHeader); err != nil {
		return nil, err
	}

	if cfHeader.Signature != [4]byte{0x4D, 0x53, 0x43, 0x46} {
		return nil, errors.New("CAB signature did not match")
	}

	if cfHeader.VersionMajor != 1 {
		return nil, errors.New("unsupported major version")
	}
	if cfHeader.VersionMinor > 3 {
		return nil, errors.New("unsupported minor version")
	}
	cab.SetIndex = cfHeader.SetIndex
	cab.SetId = cfHeader.SetId

	cabinetReserve := cfHeader.Flags&cabinetReserveExists != 0
	var reservedSizes cabinetFileReservedSizes
	if cabinetReserve {
		if err := binary.Read(fullReader, binary.LittleEndian, &reservedSizes); err != nil {
			return nil, err
		}
	}
	var reservedHeaderBlock = make([]byte, reservedSizes.ReservedHeaderSize)
	if _, err := io.ReadFull(fullReader, reservedHeaderBlock); err != nil {
		return nil, err
	}
	cab.ReservedHeaderBlock = reservedHeaderBlock

	previousCabinet := cfHeader.Flags&previousCabinetExists != 0
	if previousCabinet {
		var err error
		if cab.PreviousFile, err = readZeroTerminatedString(fullReader); err != nil {
			return nil, err
		}
		if cab.PreviousDisk, err = readZeroTerminatedString(fullReader); err != nil {
			return nil, err
		}
	}

	nextCabinet := cfHeader.Flags&nextCabinetExists != 0
	if nextCabinet {
		var err error
		if cab.NextFile, err = readZeroTerminatedString(fullReader); err != nil {
			return nil, err
		}
		if cab.NextDisk, err = readZeroTerminatedString(fullReader); err != nil {
			return nil, err
		}
	}

	folders, err := readFolderEntries(fullReader, cfHeader.FolderCount, reservedSizes.ReservedFolderSize)
	if err != nil {
		return nil, err
	}

	postFolderOffset, _ := fullReader.Seek(0, io.SeekCurrent)

	// Look up data entries for each folder
	for i := range folders {
		folder := &folders[i]
		if _, err := fullReader.Seek(int64(folder.CoffCabStart), io.SeekStart); err != nil {
			return nil, err
		}
		dataEntries, err := readDataEntries(fullReader, folder.CfDataCount, reservedSizes.ReservedDatablockSize)
		if err != nil {
			return nil, err
		}
		folder.dataEntries = dataEntries
	}

	if requireDirectCfFileFollow {
		if int64(cfHeader.FirstFileEntryOffset) != postFolderOffset {
			return nil, errors.New("offset between CFFOLDER and CFFILE")
		}
	}

	_, err = fullReader.Seek(int64(cfHeader.FirstFileEntryOffset), io.SeekStart)
	if err != nil {
		return nil, err
	}

	fileEntries, err := readFileEntries(fullReader, cfHeader.FileCount)
	if err != nil {
		return nil, err
	}

	for _, fileEntry := range fileEntries {
		if int(fileEntry.FolderIndex) >= len(folders) {
			return nil, errors.New("invalid folder reference")
		}
		folder := &folders[fileEntry.FolderIndex]
		cab.Files = append(cab.Files, &File{
			Name:       fileEntry.fileName,
			Modified:   parseCabTimestamp(fileEntry.Date, fileEntry.Time),
			Attributes: fileEntry.Attributes,
			folder:     folder,
			header:     fileEntry.cabinetFileEntryHeader,
		})
	}

	return &cab, nil
}

const (
	previousCabinetExists = 0x0001
	nextCabinetExists     = 0x0002
	cabinetReserveExists  = 0x0004
)

// Cabinet file header according to https://docs.microsoft.com/en-us/previous-versions//bb267310(v=vs.85)?redirectedfrom=MSDN#cfheader
type cabinetFileHeader struct {
	Signature            [4]byte
	_                    uint32
	Filesize             uint32
	_                    uint32
	FirstFileEntryOffset uint32
	_                    uint32
	VersionMinor         byte
	VersionMajor         byte
	FolderCount          uint16
	FileCount            uint16
	Flags                uint16
	SetId                uint16
	SetIndex             uint16
	// Optional: cabinetFileReservedSizes, if cabinetReserveExists is set
	// Optional: Cabinet reserved area, if cabinetReserveExists is set
	// Optional: Name of previous cabinet file
	// Optional: Name of previous disk
	// Optional: Name of next cabinet file
	// Optional: Name of next disk
}

type cabinetFileReservedSizes struct {
	ReservedHeaderSize    uint16
	ReservedFolderSize    uint8
	ReservedDatablockSize uint8
}

type cabinetFileFolderHeader struct {
	CoffCabStart    uint32
	CfDataCount     uint16
	CompressionType uint16
	// Optional: Per-Folder reserved area
}

type cabinetFileFolder struct {
	cabinetFileFolderHeader
	reservedData []byte

	dataEntries []cabinetFileData
}

type cabinetFileEntryHeader struct {
	UncompressedFileSize       uint32
	UncompressedOffsetInFolder uint32
	FolderIndex                uint16
	Date                       uint16
	Time                       uint16
	Attributes                 uint16
	// Followed by fileName, which is a zero-terminated string
}

type cabinetFileEntry struct {
	cabinetFileEntryHeader
	fileName string
}

type cabinetFileDataHeader struct {
	Checksum          uint32
	CompressedBytes   uint16
	UncompressedBytes uint16
	// Optional: Per-Datablock reserved area
	// Followed by compressed data bytes
}

type cabinetFileData struct {
	cabinetFileDataHeader
	reservedData   []byte
	compressedData *io.SectionReader
}

func readZeroTerminatedString(reader *io.SectionReader) (string, error) {
	stringStartOffset, _ := reader.Seek(0, io.SeekCurrent)

	var currentBufferSize = 10
	var stringBuffer strings.Builder
	for {
		var buffer = make([]byte, currentBufferSize)
		n, err := io.ReadFull(reader, buffer)
		foundZeroByte := false
		for i := range buffer {
			if buffer[i] == 0 {
				n = i
				foundZeroByte = true
				break
			}
		}
		stringBuffer.Write(buffer[:n])
		if foundZeroByte {
			break
		}
		if err != nil {
			return "", err
		}
		currentBufferSize *= 2
	}
	// Adjust reader offset to the position after the string and terminating zero
	reader.Seek(stringStartOffset+int64(stringBuffer.Len())+1, io.SeekStart)
	return stringBuffer.String(), nil
}

func readFolderEntries(reader *io.SectionReader, folderCount uint16, reservedAreaSize uint8) ([]cabinetFileFolder, error) {
	var folders []cabinetFileFolder
	for i := 0; i < int(folderCount); i++ {
		var folder cabinetFileFolder
		var folderHeader cabinetFileFolderHeader
		if err := binary.Read(reader, binary.LittleEndian, &folderHeader); err != nil {
			return nil, err
		}
		folder.cabinetFileFolderHeader = folderHeader

		if reservedAreaSize != 0 {
			folder.reservedData = make([]byte, reservedAreaSize)
			if _, err := io.ReadFull(reader, folder.reservedData); err != nil {
				return nil, err
			}
		}

		folders = append(folders, folder)
	}
	return folders, nil
}

func readFileEntries(reader *io.SectionReader, fileCount uint16) ([]cabinetFileEntry, error) {
	var files []cabinetFileEntry
	for i := 0; i < int(fileCount); i++ {
		var file cabinetFileEntry

		var fileHeader cabinetFileEntryHeader
		if err := binary.Read(reader, binary.LittleEndian, &fileHeader); err != nil {
			return nil, err
		}
		file.cabinetFileEntryHeader = fileHeader

		filename, err := readZeroTerminatedString(reader)
		if err != nil {
			return nil, err
		}
		file.fileName = filename
		files = append(files, file)
	}
	return files, nil
}

func readDataEntries(reader *io.SectionReader, dataCount uint16, reservedAreaSize uint8) ([]cabinetFileData, error) {
	var dataEntries []cabinetFileData
	for i := 0; i < int(dataCount); i++ {
		var dataEntry cabinetFileData

		var dataEntryHeader cabinetFileDataHeader
		if err := binary.Read(reader, binary.LittleEndian, &dataEntryHeader); err != nil {
			return nil, err
		}
		dataEntry.cabinetFileDataHeader = dataEntryHeader

		if reservedAreaSize != 0 {
			dataEntry.reservedData = make([]byte, reservedAreaSize)
			if _, err := io.ReadFull(reader, dataEntry.reservedData); err != nil {
				return nil, err
			}
		}

		// Store the compressed data as a reader and skip it
		currentOffset, _ := reader.Seek(0, io.SeekCurrent)
		dataEntry.compressedData = io.NewSectionReader(reader, currentOffset, int64(dataEntry.CompressedBytes))
		reader.Seek(int64(dataEntry.CompressedBytes), io.SeekCurrent)

		dataEntries = append(dataEntries, dataEntry)
	}
	return dataEntries, nil
}

func parseCabTimestamp(cabDate uint16, cabTime uint16) time.Time {
	// See https://docs.microsoft.com/en-us/previous-versions//bb267310(v=vs.85)#cffile
	// cabDate is ((year–1980) << 9)+(month << 5)+(day)
	// cabTime is (hour << 11)+(minute << 5)+(seconds/2)
	year := int(cabDate>>9) + 1980
	month := int(cabDate>>5) & 0b1111
	day := int(cabDate) & 0b11111
	hour := int(cabTime >> 11)
	minute := int(cabTime>>5) & 0b111111
	seconds := int(cabTime&0b11111) * 2
	return time.Date(year, time.Month(month), day, hour, minute, seconds, 0, time.Local)
}
