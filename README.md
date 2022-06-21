# Golang Cabinet File Parser

This package provides a simple parser for Microsoft Cabinet (.cab) files,
written in Golang.

## Example

```go
package main

import (
	"io"
	"os"
	
	"github.com/secDre4mer/go-cab"
)

func main() {
	file, _ := os.Open("path/to/cabfile.cab")
	defer file.Close()
	info, _ := file.Stat()
	
	cabinetFile, _ := cab.Open(file, info.Size())

	for _, file := range cabinetFile.Files {
		reader, _ := file.Open()
		_, _ = io.Copy(os.Stdout, reader)
	}
}
```

For simplicity's sake, error handling is omitted in this example.

## Limitations

- Cabinet that span multiple files are not supported