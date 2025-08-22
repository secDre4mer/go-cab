package lzx

import (
	"encoding/binary"
	"fmt"
	"io"
)

type persistentData struct {
	Window     *SlidingWindow
	R0, R1, R2 uint32

	IntelStarted  bool
	IntelFileSize uint32

	ResetInterval int
}

func readBlockHeader(stream *bitStream, data *persistentData) (io.Reader, error) {
	// Read next block header
	blockType, err := stream.ReadBits(3)
	if err != nil {
		return nil, err
	}
	currentBlockType := blocktype(blockType)
	uncompressedBlockSize, err := stream.ReadBits(24)
	if err != nil {
		return nil, err
	}

	var blockReader io.Reader

	switch currentBlockType {
	case uncompressed:
		stream.Align()
		var r0, r1, r2 uint32
		if err := binary.Read(stream.Internal, binary.LittleEndian, &r0); err != nil {
			return nil, err
		}
		if err := binary.Read(stream.Internal, binary.LittleEndian, &r1); err != nil {
			return nil, err
		}
		if err := binary.Read(stream.Internal, binary.LittleEndian, &r2); err != nil {
			return nil, err
		}
		blockReader = &uncompressedReader{
			Reader: stream.Internal,
		}
		data.R0, data.R1, data.R2 = r0, r1, r2
		data.IntelStarted = true
	case aligned, verbatim:
		compressed := compressedReader{
			Reader:         stream,
			persistentData: data,
		}
		if currentBlockType == aligned {
			alignedTree, err := readTree(stream, alignedTreeLengthSize, alignedTreeSize)
			if err != nil {
				return nil, err
			}
			compressed.alignedTree = *alignedTree
		}
		windowBits := 0
		size := data.Window.Size()
		for size > 1 {
			windowBits++
			size = size >> 1
		}
		mainTree, err := buildTree(stream, mainTreeSize, []interval{{0, numChars}, {numChars, numChars + int64(positionSlots[windowBits-15])<<3}})
		if err != nil {
			return nil, err
		}
		compressed.mainTree = *mainTree
		if compressed.mainTree.PathLengths[0xE8] != 0 {
			data.IntelStarted = true
		}

		lengthTree, err := buildTree(stream, secondaryNumElements, []interval{{0, secondaryNumElements}})
		if err != nil {
			return nil, err
		}
		compressed.lengthTree = *lengthTree
		blockReader = &compressed
	default:
		return nil, fmt.Errorf("invalid block type %d", blockType)
	}
	return io.LimitReader(expectNoEofReader{blockReader}, uncompressedBlockSize), nil
}

type expectNoEofReader struct {
	Internal io.Reader
}

func (e expectNoEofReader) Read(d []byte) (int, error) {
	n, err := e.Internal.Read(d)
	if err == io.EOF {
		err = io.ErrUnexpectedEOF
	}
	return n, err
}

type uncompressedReader struct {
	Reader io.Reader

	*persistentData
}

func (u uncompressedReader) Read(p []byte) (n int, err error) {
	return u.Reader.Read(p)
}

type compressedReader struct {
	Reader *bitStream

	mainTree    Tree
	lengthTree  Tree
	alignedTree Tree

	remainingBytes int

	processedBytes int

	*persistentData
}

func (c *compressedReader) Read(data []byte) (n int, err error) {
	for n < len(data) {
		if c.remainingBytes == 0 {
			if c.ResetInterval > 0 && c.processedBytes%c.ResetInterval == 0 && c.processedBytes != 0 {
				c.Reader.Align()
			}
			c.remainingBytes, err = c.ReadElement()
			if err != nil {
				return n, err
			}
		}
		for c.remainingBytes > 0 && n < len(data) {
			data[n] = c.Window.Lookback(c.remainingBytes)
			c.remainingBytes--
			c.processedBytes++
			n++
		}
	}
	return n, nil
}

func (c *compressedReader) ReadElement() (n int, err error) {
	mainElement, err := c.mainTree.Decode(c.Reader)
	if err != nil {
		return n, err
	}
	if mainElement < numChars {
		c.Window.Add(uint8(mainElement))
		n++
		return n, nil
	}
	mainElement -= numChars
	matchLength := mainElement & numPrimaryLengths
	if matchLength == numPrimaryLengths {
		encodedLength, err := c.lengthTree.Decode(c.Reader)
		if err != nil {
			return n, err
		}
		matchLength += encodedLength
	}
	matchLength += minMatch

	positionSlot := mainElement >> 3
	var matchOffset uint32
	switch positionSlot {
	case 0:
		matchOffset = c.R0
	case 1:
		matchOffset = c.R1
		c.R0, c.R1 = c.R1, c.R0
	case 2:
		matchOffset = c.R2
		c.R0, c.R2 = c.R2, c.R0
	default: // Not a repeated offset
		var (
			verbatimBits int64
			alignedBits  uint16
		)
		extra := extraBits[positionSlot]
		switch {
		case extra >= 3 && c.alignedTree.MaxDepth > 0: // aligned bits are present
			verbatimBits, err = c.Reader.ReadBits(extra - 3)
			if err != nil {
				return n, err
			}
			alignedBits, err = c.alignedTree.Decode(c.Reader)
			if err != nil {
				return n, err
			}
			verbatimBits = verbatimBits << 3
		case extra > 0: // only verbatim bits
			verbatimBits, err = c.Reader.ReadBits(extra)
			if err != nil {
				return n, err
			}
		default: // no verbatim bits
		}
		matchOffset = positionBase[positionSlot] + uint32(verbatimBits) + uint32(alignedBits) - 2
		c.R0, c.R1, c.R2 = matchOffset, c.R0, c.R1
	}
	for matchLength > 0 {
		lookbackByte := c.Window.Lookback(int(matchOffset))
		c.Window.Add(lookbackByte)
		matchLength--
		n++
	}
	return n, nil
}
