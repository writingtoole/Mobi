// The mobi package reads and writes .mobi format ebooks.
package mobi

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/writingtoole/pdb"
	"github.com/writingtoole/pdb/lz77"
)

type Mobi struct {
	// The name of the book
	Name string
	// The text contents of the book. MOBI books have only a single file of text in them.
	Contents []byte
	header   *mobiHeaderData
}

// Compression types
const (
	CompressionNone     = 1
	CompressionPalmDOC  = 2
	CompressionHuffCDIC = 17480
)

// Book types
const (
	TypeMobipocket   = 2
	TypePalmDoc      = 3
	TypeAudio        = 4
	TypeKindlegen    = 232
	TypeKF8          = 248
	TypeNews         = 257
	TypeNewsFeed     = 258
	TypeNewsMagazine = 259
	TypePICS         = 513
	TypeWORD         = 514
	TypeXLS          = 515
	TypePPD          = 516
	TypeText         = 517
	TypeHTML         = 518
)

// Encoding types
const (
	EncodingWinLatin1 = 1252
	EncodingUTF8      = 65001
)

type header struct {
	Compression    uint16
	Unused         uint16
	TextLength     uint32
	RecordCount    uint16
	RecordSize     uint16
	EncrpytionType uint16
	Unknown        uint16
}

type mobiHeaderID struct {
	Identifier   [4]byte
	HeaderLength uint32
}

const mhdSize = 248

type mobiHeaderData struct {
	MobiType                    uint32
	TextEncoding                uint32
	UniqueId                    uint32
	FileFormatVersion           uint32
	OrthographicIndex           uint32
	InflectionIndex             uint32
	IndexNames                  uint32
	IndexKeys                   uint32
	ExtraIndex                  [6]uint32
	FirstNonBookRecord          uint32
	NameOffset                  uint32
	NameLength                  uint32
	Locale                      uint32
	InputLanguage               uint32
	OutputLanguage              uint32
	MinVersion                  uint32
	FirstImage                  uint32
	HuffmanRecordOffset         uint32
	HuffmanRecordCount          uint32
	HuffmanTableOffset          uint32
	HuffmanTableLength          uint32
	EXTHFlags                   uint32
	Padding                     [32]byte
	Unknown1                    uint32
	DRMOffset                   uint32
	DRMCount                    uint32
	DRMLength                   uint32
	DRMFlags                    uint32
	Unknown2                    [2]uint32
	FirstTextRecord             uint16
	LastContentRecord           uint16
	Unknown3                    uint32
	FCISRecordNumber            uint32
	Unknown4                    uint32
	FLISRecordNumber            uint32
	Unknown5                    uint32
	Unknown6                    [2]uint32
	Unknown7                    uint32
	FirstCompDataSectionCount   uint32
	NumberOfCompilationSections uint32
	Unknown8                    uint32
	ExtraFlags                  uint32
	INDXRecordOffset            uint32
}

func Read(name string) (*Mobi, error) {
	fh, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	return ReadFH(fh)
}

func ReadFH(fh io.ReadSeeker) (*Mobi, error) {
	p, err := pdb.ReadFH(fh)
	if err != nil {
		return nil, fmt.Errorf("pdb read error: %v", err)
	}
	return Parse(p)

}

func Parse(p *pdb.Pdb) (*Mobi, error) {
	m := &Mobi{}
	err := m.parseHeader(p)
	if err != nil {
		return nil, fmt.Errorf("Header parsing error: %v", err)
	}
	return m, nil
}

// reverseDecodeInt takes the end of the passed-in buffer and returns
// the int that's reverse-decoded in it. returns the calculated value
// and the new end of buffer location after the int is removed.
func reverseDecodeInt(b []byte) (int, int, int, error) {
	collection := []byte{}
	var i int
	var l int
	for i = len(b) - 1; i >= 0; i = i - 1 {
		l++
		c := b[i]
		sc := c & 0x7f
		collection = append([]byte{sc}, collection...)
		if c&0x80 != 0 {
			break
		}
	}
	if i == -1 {
		return 0, 0, 0, fmt.Errorf("No high bit found")
	}

	ret := 0
	for _, c := range collection {
		ret = ret << 7
		ret = ret + int(c)
	}
	return ret, i, l, nil
}

// parseHeader parses the book header data.
func (m *Mobi) parseHeader(p *pdb.Pdb) error {
	h := header{}
	rd := p.Records[0].Data
	b := bytes.NewReader(rd)

	err := binary.Read(b, binary.BigEndian, &h)
	if err != nil {
		return err
	}

	mhi := mobiHeaderID{}
	err = binary.Read(b, binary.BigEndian, &mhi)
	if err != nil {
		return err
	}

	endOffset := mhi.HeaderLength + 24
	rawMobi := rd[24:endOffset]

	mhd := &mobiHeaderData{}
	m.header = mhd
	if extra := mhdSize - len(rawMobi); extra > 0 {

	}

	mb := bytes.NewReader(rawMobi)
	err = binary.Read(mb, binary.BigEndian, mhd)
	if err != nil {
		return fmt.Errorf("Error reading mhd: %v", err)
	}

	m.Name = string(rd[mhd.NameOffset : mhd.NameOffset+mhd.NameLength])

	switch h.Compression {
	case CompressionNone:
		rawBookText := make([]byte, 0, h.RecordCount*4096)
		for i := 1; i < int(mhd.FirstNonBookRecord); i++ {
			rawBookText = append(rawBookText, m.trailStrip(p, i)...)
		}
		m.Contents = rawBookText
	case CompressionPalmDOC:
		rawBookText := make([]byte, 0, h.RecordCount*4096)
		for i := 1; i < int(mhd.FirstNonBookRecord); i++ {
			c, err := lz77.Decompress(m.trailStrip(p, i))
			if err != nil {
				return fmt.Errorf("Error decompressing record %v: %v", i, err)
			}
			rawBookText = append(rawBookText, c...)
		}
		m.Contents = rawBookText
	default:
		return fmt.Errorf("Unknown compression type %v", h.Compression)
	}

	return nil
}

// trailStrip strips off any trailing data from the record that
// doesn't actually count as part of the record data.
func (m *Mobi) trailStrip(p *pdb.Pdb, rec int) []byte {
	d := p.Records[rec].Data

	// Occasionally there are all-null records. We ignore those.
	if len(d) == 3 && bytes.Equal(d, []byte{0, 0, 0}) {
		return []byte{}
	}
	if m.header.ExtraFlags != 0 {
		for i := 15; i >= 0; i-- {
			b := uint32(1 << uint(i))
			if m.header.ExtraFlags&b != 0 {
				extra := 0
				switch i {
				case 0:
					// Bit 0 is special.
					l := int(d[len(d)-1])
					extra = 1 + (l & 3)
					d = d[0 : len(d)-extra]
				default:
					in, off, l, err := reverseDecodeInt(d)
					if err != nil {
						log.Printf("Int decode error for int %v, rec %v: %v", rec, i, err)
						return d
					}
					extra = in - l
					d = d[0 : off-extra]
				}
			}
		}
	}
	return d
}
