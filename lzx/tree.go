package lzx

import (
	"errors"
	"math"
)

func readTree(stream *bitStream, lengthBits int, size int) (*Tree, error) {
	var lengths = make([]byte, size)
	for i := 0; i < len(lengths); i++ {
		treeEntry, err := stream.ReadBits(lengthBits)
		if err != nil {
			return nil, err
		}
		lengths[i] = uint8(treeEntry)
	}

	return buildTable(lengths)
}

func buildTable(lengths []byte) (*Tree, error) {
	var maxLength byte
	for _, length := range lengths {
		if length > maxLength {
			maxLength = length
		}
	}

	// Plausibility check: lengths should have a total sum of the size
	code := 0
	for _, cl := range lengths {
		if cl > 0 {
			code += 1 << (maxLength - cl)
		}
	}

	if code != 1<<maxLength {
		return nil, errors.New("invalid tree lengths")
	}

	huffmanTree := make([]uint16, 1<<maxLength)
	position := 0

	if len(lengths) > math.MaxUint16 {
		return nil, errors.New("too many codes")
	}

	for bit := uint8(1); bit <= maxLength; bit++ {
		amount := 1 << (maxLength - bit)
		for code := uint16(0); code < uint16(len(lengths)); code++ {
			if lengths[code] == bit {
				for j := 0; j < amount; j++ {
					if position >= len(huffmanTree) {
						return nil, errors.New("bad tree")
					}
					huffmanTree[position] = code
					position++
				}
			}
		}
	}
	if position != len(huffmanTree) {
		return nil, errors.New("bad tree")
	}

	return &Tree{
		PathLengths: lengths,
		HuffmanTree: huffmanTree,
		MaxDepth:    int(maxLength),
	}, nil
}

type Tree struct {
	PathLengths []byte
	MaxDepth    int
	HuffmanTree []uint16
}

func (t Tree) Decode(stream *bitStream) (uint16, error) {
	// At most, we need as many bits as the max depth. Peek at this many bits to determine the code.
	nextBits, err := stream.PeekBits(t.MaxDepth)
	if err != nil {
		return 0, err
	}
	code := t.HuffmanTree[nextBits]
	// The actual amount of bites the code took is t.PathLengths[code].
	// Read them (and ignore, since we already read them with PeekBits).
	_, _ = stream.ReadBits(int(t.PathLengths[code]))
	return code, nil
}

type interval struct {
	from int64
	last int64
}

func buildTree(stream *bitStream, lengths []byte, intervals []interval) (*Tree, error) {
	for _, interval := range intervals {
		preTree, err := readTree(stream, 4, preTreeSize)
		if err != nil {
			return nil, err
		}
		for i := interval.from; i < interval.last; {
			k, err := preTree.Decode(stream)
			if err != nil {
				return nil, err
			}
			switch k {
			case 17, 18:
				var j int64
				if k == 17 {
					j, err = stream.ReadBits(4)
					if err != nil {
						return nil, err
					}
					j += 4
				} else {
					j, err = stream.ReadBits(5)
					if err != nil {
						return nil, err
					}
					j += 20

				}
				if i+j >= interval.last {
					j = interval.last - i
				}
				for ; j > 0; j-- {
					lengths[i] = 0
					i++
				}
			case 19:
				j, err := stream.ReadBits(1)
				if err != nil {
					return nil, err
				}
				j += 4
				if i+j >= interval.last {
					j = interval.last - i
				}
				k, err := preTree.Decode(stream)
				if err != nil {
					return nil, err
				}
				m := (uint16(lengths[i]) - k + 17) % 17
				for ; j > 0; j-- {
					lengths[i] = uint8(m)
					i++
				}
			default:
				lengths[i] = uint8((uint16(lengths[i]) - k + 17) % 17)
				i++
			}
		}
	}
	return buildTable(lengths)
}
