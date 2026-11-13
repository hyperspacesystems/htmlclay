package htmlutil

import (
	"bytes"
	"strings"

	"github.com/panphora/htmlclay/internal/config"
	"golang.org/x/net/html"
)

const (
	maxHelperNames = 8
	// Declarations after this prefix are ignored, so authors should put them
	// first in the head of a single-file application.
	helperScanLimit = 512 << 10
)

// ReadHelperNames returns the helper names a document declares, in document
// order, deduplicated, at most maxHelperNames. Invalid names are skipped rather
// than erroring: a bad meta tag is ignored the way an unknown meta tag is.
func ReadHelperNames(data []byte) []string {
	names, _, _ := readHelperDeclarations(data)
	return names
}

// ReadHelperFolders returns, per declared helper name, the folder its
// declaration suggests for the program picker (data-program-folder), relative
// to the document or, with a ~/ prefix, to the home folder. Only the first declaration of a name counts, as for
// ReadHelperNames, and a name without the attribute is absent.
func ReadHelperFolders(data []byte) map[string]string {
	_, folders, _ := readHelperDeclarations(data)
	return folders
}

// ReadHelperFiles returns, per declared helper name, the program file name its
// declaration expects (data-program-file), such as "clay-search" for the name
// "search". It is a bare file name, never a path: HTML Clay looks for it only in
// folders the person already keeps programs in. An invalid value is ignored.
func ReadHelperFiles(data []byte) map[string]string {
	_, _, files := readHelperDeclarations(data)
	return files
}

// ValidProgramFileName reports whether s is a bare file name of 1 to 64 ASCII
// letters, digits, dots, hyphens and underscores that does not start with a dot.
func ValidProgramFileName(s string) bool {
	if len(s) == 0 || len(s) > 64 || s[0] == '.' {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') && c != '.' && c != '-' && c != '_' {
			return false
		}
	}
	return true
}

func readHelperDeclarations(data []byte) ([]string, map[string]string, map[string]string) {
	if len(data) > helperScanLimit {
		data = data[:helperScanLimit]
	}
	z := html.NewTokenizer(bytes.NewReader(data))
	seen := make(map[string]bool)
	names := make([]string, 0, maxHelperNames)
	folders := make(map[string]string)
	files := make(map[string]string)
	templateDepth := 0
	rawElement := ""
	for {
		tokenType := z.Next()
		if tokenType == html.ErrorToken {
			return names, folders, files
		}
		token := z.Token()
		if rawElement != "" {
			if tokenType == html.EndTagToken && token.Data == rawElement {
				rawElement = ""
			}
			continue
		}
		if templateDepth > 0 {
			switch tokenType {
			case html.StartTagToken, html.SelfClosingTagToken:
				if token.Data == "template" {
					templateDepth++
				}
			case html.EndTagToken:
				if token.Data == "template" {
					templateDepth--
				}
			}
			continue
		}
		switch tokenType {
		case html.TextToken:
			if strings.Trim(token.Data, " \t\n\r\f") != "" {
				return names, folders, files
			}
		case html.EndTagToken:
			if token.Data == "head" || token.Data == "body" || token.Data == "html" || token.Data == "br" {
				return names, folders, files
			}
		case html.StartTagToken, html.SelfClosingTagToken:
			if token.Data == "template" {
				templateDepth = 1
				continue
			}
			if !isHeadElement(token.Data) {
				return names, folders, files
			}
			if isRawHeadElement(token.Data) {
				rawElement = token.Data
				continue
			}
			if token.Data != "meta" {
				continue
			}
			var metaName, content, folder, file string
			for _, attr := range token.Attr {
				switch attr.Key {
				case "name":
					metaName = attr.Val
				case "content":
					content = attr.Val
				case "data-program-folder":
					folder = strings.TrimSpace(attr.Val)
				case "data-program-file":
					file = strings.TrimSpace(attr.Val)
				}
			}
			if metaName != "htmlclay-helper" || !config.ValidHelperName(content) || seen[content] {
				continue
			}
			seen[content] = true
			names = append(names, content)
			if folder != "" {
				folders[content] = folder
			}
			if ValidProgramFileName(file) {
				files[content] = file
			}
			if len(names) == maxHelperNames {
				return names, folders, files
			}
		}
	}
}

func isRawHeadElement(name string) bool {
	switch name {
	case "noscript", "script", "title", "noframes", "style":
		return true
	default:
		return false
	}
}

func isHeadElement(name string) bool {
	switch name {
	case "html", "head", "base", "basefont", "bgsound", "link", "meta", "noscript", "script", "title", "noframes", "style":
		return true
	default:
		return false
	}
}
