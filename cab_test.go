package cab

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

func TestOpen(t *testing.T) {
	testfileData, err := os.ReadFile("testdata/simple.cab")
	if err != nil {
		t.Fatal(err)
	}
	cabFile, err := Open(bytes.NewReader(testfileData), int64(len(testfileData)))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range cabFile.Files {
		if file.Name != "test.yml" {
			t.Fatal(file.Name)
		}
		if !file.Modified.Equal(time.Date(2021, 11, 02, 14, 34, 56, 0, time.Local)) {
			t.Fatal(file.Modified)
		}
		reader, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.Copy(os.Stdout, reader); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDecompressLargeFile(t *testing.T) {
	testfileData, err := os.ReadFile("testdata/large.cab")
	if err != nil {
		t.Fatal(err)
	}
	cabFile, err := Open(bytes.NewReader(testfileData), int64(len(testfileData)))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range cabFile.Files {
		if file.Name != "explorer.exe" {
			t.Fatal(file.Name)
		}
		sha256Hash := sha256.New()
		reader, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.Copy(sha256Hash, reader); err != nil {
			t.Fatal(err)
		}
		if fmt.Sprintf("%X", sha256Hash.Sum(nil)) != "549529425FF33EF261DD93418F906A2A15A85890733D82EAA65AEDBBB2712442" {
			t.Fatal("hash mismatch on unpacked data")
		}
	}
}

func TestDecompressMultipleFiles(t *testing.T) {
	testfileData, err := os.ReadFile("testdata/drivers.cab")
	if err != nil {
		t.Fatal(err)
	}
	cabFile, err := Open(bytes.NewReader(testfileData), int64(len(testfileData)))
	if err != nil {
		t.Fatal(err)
	}
	expectedHashes, err := os.ReadFile("testdata/driverhashes.txt")
	if err != nil {
		t.Fatal(err)
	}
	expectedHashReader := bufio.NewReader(bytes.NewReader(expectedHashes))
	for _, file := range cabFile.Files {
		expectedHash, _ := expectedHashReader.ReadString('\n')
		expectedHash = strings.TrimSpace(expectedHash)
		sha256Hash := sha256.New()
		reader, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.Copy(sha256Hash, reader); err != nil {
			t.Fatal(err)
		}
		hashline := fmt.Sprintf("%X %s", sha256Hash.Sum(nil), file.Name)
		if expectedHash != hashline {
			t.Fatalf("Expected %s, got: %s", expectedHash, hashline)
		}
	}
}
