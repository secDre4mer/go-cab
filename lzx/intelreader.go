package lzx

import (
	"encoding/binary"
	"io"
)

type intelReader struct {
	Internal io.Reader

	buffer []byte

	*persistentData
}

func (i *intelReader) Read(b []byte) (int, error) {
	if i.IntelFileSize == 0 || !i.IntelStarted {
		n, err := i.Internal.Read(b)
		i.IntelCursorPosition += n
		return n, err
	}
	bufferedBytes := copy(b, i.buffer)
	i.buffer = i.buffer[bufferedBytes:]
	b = b[bufferedBytes:]
	n := bufferedBytes
	i.IntelCursorPosition += bufferedBytes
	if len(b) == 0 {
		return n, nil
	}
	readBytes, err := i.Internal.Read(b)
	n += readBytes
	for index := 0; index < readBytes; index++ {
		readByte := b[index]
		if readByte == 0xE8 {
			callBytes := make([]byte, 4)
			alreadyReadBytecount := copy(callBytes, b[index+1:readBytes])
			if alreadyReadBytecount < 4 {
				// Need to read extra bytes for conversion
				if _, err := io.ReadFull(i.Internal, callBytes[alreadyReadBytecount:]); err != nil {
					if err == io.ErrUnexpectedEOF || err == io.EOF { // Not enough bytes; ignore bad call instruction
						continue
					}
					return n, err
				}
			}

			absoluteOffset := int32(binary.LittleEndian.Uint32(callBytes))
			if absoluteOffset >= -int32(i.IntelCursorPosition+index) && absoluteOffset < int32(i.IntelFileSize) {
				var relativeOffset int32
				if absoluteOffset >= 0 {
					relativeOffset = absoluteOffset - int32(i.IntelCursorPosition+index)
				} else {
					relativeOffset = absoluteOffset + int32(i.IntelFileSize)
				}
				binary.LittleEndian.PutUint32(callBytes, uint32(relativeOffset))
			}

			copiedBackBytes := copy(b[index+1:readBytes], callBytes)
			i.buffer = callBytes[copiedBackBytes:]
			index += 4
		}
	}
	i.IntelCursorPosition += readBytes
	return n, err
}
