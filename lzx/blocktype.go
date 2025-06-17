package lzx

type blocktype int

const (
	undefined blocktype = iota
	verbatim
	aligned
	uncompressed
)
