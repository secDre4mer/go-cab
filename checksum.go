package cab

func computeChecksum(pv []byte, seed uint32) uint32 {
	csum := seed // Init checksum
	pb := pv     // Start at front of data block

	//** Checksum integral multiple of ULONGs
	for len(pb) >= 4 {
		ul := uint32(pb[0])           // Get low-order byte
		ul |= ((uint32)(pb[1])) << 8  // Add 2nd byte
		ul |= ((uint32)(pb[2])) << 16 // Add 3rd byte
		ul |= ((uint32)(pb[3])) << 24 // Add 4th byte
		pb = pb[4:]
		csum ^= ul // Update checksum
	}

	//** Checksum remainder bytes
	ul := uint32(0)
	for len(pb) > 0 {
		ul = (ul << 8) | uint32(pb[0])
		pb = pb[1:]
	}
	csum ^= ul // Update checksum

	//** Return computed checksum
	return csum
}

type checksumWriter struct {
	Checksum uint32

	remainder []byte
}

func (c *checksumWriter) Write(data []byte) (n int, err error) {
	var initialLength = len(data)
	if len(c.remainder) != 0 {
		neededBytes := 4 - len(c.remainder)
		if len(data) < neededBytes {
			c.remainder = append(c.remainder, data...)
			return len(data), nil
		} else {
			// Fill up remainder from data and add it to checksum
			c.remainder = append(c.remainder, data[:neededBytes]...)
			data = data[neededBytes:]

			c.Checksum = computeChecksum(c.remainder, c.Checksum)
			c.remainder = nil
		}
	}
	if len(data)%4 != 0 {
		var remainderSize = len(data) % 4
		c.remainder = make([]byte, remainderSize)
		copy(c.remainder, data[len(data)-remainderSize:])
		data = data[:len(data)-remainderSize]
	}
	if len(data) > 0 {
		c.Checksum = computeChecksum(data, c.Checksum)
	}
	return initialLength, nil
}

func (c *checksumWriter) Flush() {
	if len(c.remainder) > 0 {
		c.Checksum = computeChecksum(c.remainder, c.Checksum)
		c.remainder = nil
	}
}
