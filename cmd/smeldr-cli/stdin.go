package main

import (
	"bufio"
	"bytes"
	"io"
	"os"
)

// utf8BOM is the UTF-8 byte order mark. Windows PowerShell 5.1 puts one in
// front of what it pipes to a program when the console is at code page 65001,
// and a second one when $OutputEncoding is UTF-8 too; its Set-Content
// -Encoding utf8 writes one at the start of a file (core T1238).
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// stdinReader is stdin without leading byte order marks. Every read of stdin
// goes through it: secrets, paths, files given as "-", confirmations and the
// hook's JSON.
func stdinReader() io.Reader { return withoutBOM(stdin) }

// withoutBOM drops any number of leading UTF-8 byte order marks from r and
// leaves everything after the first other byte as it is.
func withoutBOM(r io.Reader) io.Reader {
	br := bufio.NewReader(r)
	for {
		b, err := br.Peek(len(utf8BOM))
		if err != nil || !bytes.Equal(b, utf8BOM) {
			return br
		}
		br.Discard(len(utf8BOM)) //nolint:errcheck // Peek just saw these bytes
	}
}

// trimBOM drops leading UTF-8 byte order marks from data.
func trimBOM(data []byte) []byte {
	for bytes.HasPrefix(data, utf8BOM) {
		data = data[len(utf8BOM):]
	}
	return data
}

// readFileNoBOM reads a file the way the cli reads stdin: a leading UTF-8 byte
// order mark is dropped. A UTF-16 file is not converted.
func readFileNoBOM(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return trimBOM(data), nil
}
