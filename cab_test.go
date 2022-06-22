package cab

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strconv"
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

func FuzzPanic(f *testing.F) {
	simple, err := os.ReadFile("testdata/simple.cab")
	if err != nil {
		f.Fatal(err)
	}
	f.Add(simple)

	drivers, err := os.ReadFile("testdata/drivers.cab")
	if err != nil {
		f.Fatal(err)
	}
	f.Add(drivers)
	f.Fuzz(func(t *testing.T, data []byte) {
		defer func() {
			if err := recover(); err != nil {
				t.Fatal(err)
			}
		}()
		cabFile, err := Open(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			t.Log(err)
			return
		}
		for _, file := range cabFile.Files {
			reader, err := file.Open()
			if err != nil {
				t.Log(err)
				continue
			}
			if _, err := io.Copy(io.Discard, reader); err != nil {
				t.Log(err)
				continue
			}
		}
	})
}

var cabextract, _ = exec.LookPath("cabextract")

func FuzzFileCorrectness(f *testing.F) {
	requireDirectCfFileFollow = true
	if cabextract == "" {
		f.Skip("cabextract is not installed")
	}
	simple, err := os.ReadFile("testdata/simple.cab")
	if err != nil {
		f.Fatal(err)
	}
	f.Add(simple)

	drivers, err := os.ReadFile("testdata/drivers.cab")
	if err != nil {
		f.Fatal(err)
	}
	f.Add(drivers)

	f.Fuzz(func(t *testing.T, data []byte) {
		tempFile, err := os.CreateTemp("", "gocab-fuzz")
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(tempFile.Name())
		defer tempFile.Close()
		tempFile.Write(data)

		cabFile, err := Open(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			t.Log(err)
			return
		}
		cabextractListing, err := runCabExtract(tempFile.Name())
		if err != nil {
			t.Log(err)
			return
		}
		if len(cabFile.Files) != len(cabextractListing) {
			t.Fatal("File count mismatch", cabFile.Files, cabextractListing)
			return
		}

		for i := range cabFile.Files {
			ourFile := cabFile.Files[i]
			cabextractFile := cabextractListing[i]
			if !cabextractFile.Modified.IsZero() {
				if ourFile.Modified != cabextractFile.Modified {
					t.Fatal("time differed", ourFile, cabextractFile)
				}
			}
			if ourFile.Stat().Size() != int64(cabextractFile.Filesize) {
				t.Fatal("file size differed", ourFile, cabextractFile)
			}
			if !printableAscii.MatchString(cabextractFile.Name) {
				continue
			}
			cleanedOurName := nameReplacer.Replace(ourFile.Name)
			cleanedCabextractName := nameReplacer.Replace(cabextractFile.Name)
			if len(cleanedCabextractName) >= 10 { // cabextract only shows at most 10 chars in that line
				if len(cleanedOurName) >= 10 {
					cleanedOurName = cleanedOurName[:10]
					cleanedCabextractName = cleanedCabextractName[:10]
				}
			}
			if cleanedCabextractName != "x" && !strings.Contains(ourFile.Name, "\n") && cleanedOurName != cleanedCabextractName {
				t.Fatal("name differed", ourFile.Name, cabextractFile.Name)
			}
			cabextractFileContent, err := getCabExtractFile(tempFile.Name(), cabextractFile.Name)
			if err != nil {
				continue
			}
			if len(cabextractFileContent) == 0 {
				continue
			}
			if i == 64 {
				t.Log("bad")
			}
			f, err := ourFile.Open()
			if err != nil {
				if err.Error() == "unknown compression type" {
					t.Skip()
				}
				t.Fatal(err)
			}
			var b bytes.Buffer
			if _, err := io.Copy(&b, f); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(b.Bytes(), cabextractFileContent) {
				os.WriteFile("own", b.Bytes(), 0644)
				os.WriteFile("cab", cabextractFileContent, 0644)
				t.Fatal("Content mismatch")
			}
		}
	})
}

var nameReplacer = strings.NewReplacer("/", "", `\`, "")

var printableAscii = regexp.MustCompile(`^[\x20-\x7E]+$`)

type cabExtractListedFile struct {
	Filesize int
	Modified time.Time
	Name     string
}

func runCabExtract(path string) ([]cabExtractListedFile, error) {
	cmd := exec.Command(cabextract, "-l", path)
	var b bytes.Buffer
	cmd.Stdout = &b
	err := cmd.Run()
	if err != nil {
		return nil, err
	}
	reader := bufio.NewReader(&b)
	var files []cabExtractListedFile
	for {
		line, err := reader.ReadString('\n')
		if err == io.EOF {
			if line == "" {
				break
			}
		} else if err != nil {
			return nil, err
		}
		line = strings.TrimSuffix(line, "\n")
		var parts = strings.SplitN(line, " | ", 3)
		if len(parts) < 3 {
			continue
		}
		filesize, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil {
			continue
		}
		date, _ := time.ParseInLocation("02.01.2006 15:04:05", strings.TrimSpace(parts[1]), time.Local)
		name := parts[2]
		files = append(files, cabExtractListedFile{
			Filesize: filesize,
			Modified: date,
			Name:     name,
		})
	}
	return files, nil
}

func getCabExtractFile(path string, file string) ([]byte, error) {
	cmd := exec.Command(cabextract, "-s", "-F", file, "-p", path)
	var b bytes.Buffer
	cmd.Stdout = &b
	err := cmd.Run()
	if err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}
