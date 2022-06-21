package cab

import "testing"

func TestChecksumWriter(t *testing.T) {
	var checksum checksumWriter
	n, err := checksum.Write([]byte{0x1, 0x2, 0x3, 0x4, 0x5, 0x6, 0x7})
	if err != nil {
		t.Fatal(err)
	}
	if n != 7 {
		t.Fatal("not all bytes written")
	}
	checksum.Flush()
	if checksum.Checksum != 67503110 {
		t.Fatal("incorrect checksum", checksum.Checksum)
	}
}

func TestChecksumWriterFlush(t *testing.T) {
	var checksum checksumWriter
	checksum.Write([]byte{0x1, 0x2, 0x3})
	checksum.Write([]byte{0x4, 0x5})
	checksum.Flush()
	checksum.Write([]byte{0x6, 0x7})
	checksum.Flush()
	if checksum.Checksum != 67306499 {
		t.Fatal("incorrect checksum", checksum.Checksum)
	}
}
