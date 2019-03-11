package mobi

import (
	"strings"
	"testing"
)

// The sample file is a copy of Alice in Wonderland from Project Gutenberg.
const sampleFile = "testdata/pg11-images.mobi"
const bookName = "Alice's Adventures in Wonderland"

func TestBasic(t *testing.T) {
	b, err := Read(sampleFile)
	if err != nil {
		t.Fatalf("Unable to open %q: %v", sampleFile, err)
	}

	// Did we decode the name right?
	if b.Name != bookName {
		t.Errorf("Book name error: got %v, want %v", b.Name, bookName)
	}

	// The book should end with "</html>" so make sure it does.
	if !strings.HasSuffix(string(b.Contents), "</html>") {
		t.Errorf("Book contents error: expected trailing string </html>, have %v", string(b.Contents[len(b.Contents)-7:len(b.Contents)]))
	}

	// The sample book should have a single image.
	if len(b.Images) != 1 {
		t.Errorf("Book image count error: got %v, want 1", len(b.Images))
	}
}
