package lzx

// SlidingWindow implements a window of n bytes where old bytes are replaced with new ones. Old bytes within the last
// window size can be looked up again.
type SlidingWindow struct {
	windowData []byte

	position int
}

func NewWindow(size int) *SlidingWindow {
	return &SlidingWindow{
		windowData: make([]byte, size),
		position:   0,
	}
}

func (s *SlidingWindow) Lookback(offset int) byte {
	return s.windowData[(s.position-offset+len(s.windowData))%len(s.windowData)]
}

func (s *SlidingWindow) Add(b byte) {
	s.windowData[s.position] = b
	s.position = (s.position + 1) % len(s.windowData)
}

func (s *SlidingWindow) Size() int {
	return len(s.windowData)
}
