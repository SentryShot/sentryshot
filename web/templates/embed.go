package templates

import (
	"embed"
	"log"
	"path/filepath"
)

//go:embed *
var files embed.FS

// PageFiles page template files. map[fileName]fileText.
var PageFiles map[string]string

// IncludeFiles sub template files. map[fileName]fileText.
var IncludeFiles map[string]string

func init() {
	PageFiles = parseDir(".")
	IncludeFiles = parseDir("includes")
}

func parseDir(dir string) map[string]string {
	entries, err := files.ReadDir(dir)
	if err != nil {
		log.Fatalf("could not read templates: %v", err)
	}

	fileList := make(map[string]string)
	for _, entry := range entries {
		fileName := entry.Name()

		fileIsHidden := ([]rune(fileName)[0] == []rune(".")[0])
		if entry.IsDir() || fileIsHidden {
			continue
		}

		data, err := files.ReadFile(filepath.Join(dir, fileName))
		if err != nil {
			log.Fatalf("could not read file: %v", err)
		}

		fileList[fileName] = string(data)
	}
	return fileList
}
