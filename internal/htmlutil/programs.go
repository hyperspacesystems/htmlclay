package htmlutil

import (
	"bytes"
	"strings"

	"github.com/panphora/htmlclay/internal/config"
	"golang.org/x/net/html"
)

// ProgramType is the script type of a carried program tag.
const ProgramType = "text/x-clay-program"

const (
	// ProgramScanLimit is how much of a document carried programs are read
	// from. A program tag that starts past it, or whose text runs past it, is
	// reported as not read.
	ProgramScanLimit = 5 << 19
	// MaxPrograms is how many program tags one document may carry.
	MaxPrograms = 8
	// MaxProgramBytes is the largest program text.
	MaxProgramBytes = 256 << 10
	maxNotRead      = 32
)

// Why a program tag is in ProgramScan.NotRead instead of Programs.
const (
	NotReadPastLimit = "past-limit"
	NotReadCut       = "cut"
	NotReadTooLarge  = "too-large"
	NotReadTooMany   = "too-many"
	NotReadDuplicate = "duplicate"
	NotReadBadName   = "bad-name"
)

// Program is one carried program tag: its name, its can attribute as
// written, and its text exactly as the file holds it.
type Program struct {
	Name string
	Can  string
	Text string
}

// NotRead is a program tag the scan saw and did not read, and why.
type NotRead struct {
	Name   string
	Reason string
}

// ProgramScan is every program tag a document carries: the ones read, in
// document order, and the ones not read.
type ProgramScan struct {
	Programs []Program
	NotRead  []NotRead
}

// Program returns the program named name.
func (s ProgramScan) Program(name string) (Program, bool) {
	for _, p := range s.Programs {
		if p.Name == name {
			return p, true
		}
	}
	return Program{}, false
}

// Reason returns why the tag named name was not read.
func (s ProgramScan) Reason(name string) (string, bool) {
	for _, n := range s.NotRead {
		if n.Name == name {
			return n.Reason, true
		}
	}
	return "", false
}

// ReadPrograms returns the carried program tags in data. Only the first
// ProgramScanLimit bytes are read; every tag after or across that line is
// still named in NotRead, so a program is never dropped without a word. The
// document is read as a browser parses it, so a tag inside a template or in a
// foreign namespace is not one. Two tags with one name are both refused, so no
// reader has to guess which runs.
func ReadPrograms(data []byte) ProgramScan {
	var scan ProgramScan
	seen := make(map[string]bool)
	twice := make(map[string]bool)
	notRead := func(name, reason string) {
		if len(scan.NotRead) < maxNotRead {
			scan.NotRead = append(scan.NotRead, NotRead{Name: name, Reason: reason})
		}
	}
	full := parseProgramTags(data)
	var prefix []Program
	if len(data) > ProgramScanLimit {
		prefixBuf := make([]byte, ProgramScanLimit+len("<!--end-->"))
		copy(prefixBuf, data[:ProgramScanLimit])
		copy(prefixBuf[ProgramScanLimit:], "<!--end-->")
		prefix = parseProgramTags(prefixBuf)
	}
	for i, p := range full {
		switch {
		case !config.ValidHelperName(p.Name) || config.IsReservedHelperName(p.Name):
			notRead(clip(p.Name), NotReadBadName)
		case seen[p.Name]:
			if !twice[p.Name] {
				notRead(p.Name, NotReadDuplicate)
			}
			twice[p.Name] = true
		case len(data) > ProgramScanLimit && i >= len(prefix):
			notRead(p.Name, NotReadPastLimit)
		case len(data) > ProgramScanLimit && (prefix[i].Name != p.Name || prefix[i].Can != p.Can || prefix[i].Text != p.Text):
			notRead(p.Name, NotReadCut)
		case len(p.Text) > MaxProgramBytes:
			notRead(p.Name, NotReadTooLarge)
		case len(scan.Programs) == MaxPrograms:
			notRead(p.Name, NotReadTooMany)
		default:
			scan.Programs = append(scan.Programs, p)
		}
		seen[p.Name] = true
	}
	return dropTwice(scan, twice)
}

// parseProgramTags returns the carried program tags in data in document order.
// It parses data the way a browser does: a script inside a template or inside
// an svg or math namespace is not a program the page runs, so it is not one
// here either. The break comment keeps a truncated prefix from swallowing the
// tags after it.
func parseProgramTags(data []byte) []Program {
	doc, err := html.Parse(bytes.NewReader(data))
	if err != nil {
		return nil
	}
	var programs []Program
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Namespace == "" {
			tag := strings.ToLower(n.Data)
			if tag == "template" || tag == "noscript" {
				return
			}
			if tag == "script" {
				var typ, name, can string
				var hasType, hasName, hasCan bool
				for _, a := range n.Attr {
					if a.Namespace != "" {
						continue
					}
					switch strings.ToLower(a.Key) {
					case "type":
						if !hasType {
							typ, hasType = a.Val, true
						}
					case "name":
						if !hasName {
							name, hasName = a.Val, true
						}
					case "can":
						if !hasCan {
							can, hasCan = a.Val, true
						}
					}
				}
				if strings.EqualFold(strings.TrimSpace(typ), ProgramType) {
					programs = append(programs, Program{Name: name, Can: can, Text: scriptText(n)})
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return programs
}

// scriptText is the DOM's textContent: every descendant text node in order.
func scriptText(n *html.Node) string {
	var b strings.Builder
	var visit func(*html.Node)
	visit = func(n *html.Node) {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			switch c.Type {
			case html.TextNode:
				b.WriteString(c.Data)
			case html.ElementNode:
				visit(c)
			}
		}
	}
	visit(n)
	return b.String()
}

// dropTwice takes every program whose name two tags share out of the
// programs read.
func dropTwice(scan ProgramScan, twice map[string]bool) ProgramScan {
	if len(twice) == 0 {
		return scan
	}
	kept := scan.Programs[:0]
	for _, p := range scan.Programs {
		if !twice[p.Name] {
			kept = append(kept, p)
		}
	}
	scan.Programs = kept
	return scan
}

func clip(s string) string {
	if len(s) > 64 {
		return s[:64]
	}
	return s
}
