package builtin

import "embed"

//go:embed examples/*.htmlclay
var examples embed.FS

// ExampleNames are the file names the example documents get on disk.
var ExampleNames = map[string]string{
	FileSearch:  "File Search.htmlclay",
	FileConvert: "Image Converter.htmlclay",
	FolderWatch: "Folder Activity.htmlclay",
}

// Example is the example document that shows a built-in program at work, and
// the name it is saved under.
func Example(id string) (name string, data []byte, ok bool) {
	if !Is(id) {
		return "", nil, false
	}
	data, err := examples.ReadFile("examples/" + id + ".htmlclay")
	return ExampleNames[id], data, err == nil
}
