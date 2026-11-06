package main

import (
	"bytes"
	"strings"
	"testing"
)

// mediaListBody is a GET /media body as smeldr.dev/media v1.7.1 writes it:
// MediaRecord without JSON tags, so Go field names.
const mediaListBody = `[{"ID":"NhKQ8YifFmdMBnIuxOH0kA","Filename":"c2d4e81c-x.svg","OriginalFilename":"x.svg","MediaType":"image","MIMEType":"image/svg+xml","Description":"test","SizeBytes":176,"UploadedAt":"2026-11-06T08:14:46.123Z","URL":"http://127.0.0.1:18098/media/c2d4e81c-x.svg"}]`

// TestMediaList_ReadsMediaKeys: media list fills every column from the keys
// GET /media sends, and from snake_case keys too; an empty list says so
// (T1149).
func TestMediaList_ReadsMediaKeys(t *testing.T) {
	snake := `[{"id":"NhKQ8YifFmdMBnIuxOH0kA","media_type":"image","original_filename":"x.svg","uploaded_at":"2026-11-06T08:14:46.123Z","url":"http://127.0.0.1:18098/media/c2d4e81c-x.svg"}]`
	for name, body := range map[string]string{"media v1.7.1": mediaListBody, "snake_case": snake} {
		t.Run(name, func(t *testing.T) {
			var out bytes.Buffer
			if err := printMediaList(&out, []byte(body)); err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{"NhKQ8YifFmdMBnIuxOH0kA", "image", "2026-11-06 08:14", "x.svg", "http://127.0.0.1:18098/media/c2d4e81c-x.svg"} {
				if !strings.Contains(out.String(), want) {
					t.Errorf("output lacks %q:\n%s", want, out.String())
				}
			}
		})
	}
	var out bytes.Buffer
	if err := printMediaList(&out, []byte(`[]`)); err != nil || out.String() != "no media records found\n" {
		t.Errorf("empty: %q %v", out.String(), err)
	}
	if err := printMediaList(&out, []byte(`{`)); err == nil {
		t.Error("a bad body decoded")
	}
}
