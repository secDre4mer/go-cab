package lzx

import (
	"errors"
	"io"
)

type bitStream struct {
	Internal io.Reader

	// Currently cached bits
	cachedData int64
	// Number of bits cached in cachedData
	cacheSize int
}

func (b *bitStream) ReadBits(c int) (int64, error) {
	bits, err := b.PeekBits(c)
	if err != nil {
		return 0, err
	}
	b.cacheSize -= c
	b.cachedData = b.cachedData & (1<<b.cacheSize - 1)
	return bits, nil
}

func (b *bitStream) PeekBits(c int) (int64, error) {
	if c > 32 {
		return 0, errors.New("invalid bit read")
	}
	for c > b.cacheSize {
		// Read new data
		var nextData [2]byte
		if _, err := io.ReadFull(b.Internal, nextData[:]); err != nil {
			if err == io.EOF {
				err = io.ErrUnexpectedEOF
			}
			return 0, err
		}
		b.cachedData = b.cachedData<<16 | int64(nextData[1])<<8 | int64(nextData[0])
		b.cacheSize += 16
	}
	result := b.cachedData >> (b.cacheSize - c)
	return result, nil
}

func (b *bitStream) Align() {
	b.cacheSize -= b.cacheSize % 16 // Align to 16 bits
	b.cachedData = b.cachedData & (1<<b.cacheSize - 1)
}

func (b *bitStream) BitsLeft() int {
	return b.cacheSize
}
