package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	// Create the directories using os.MkdirAll
	dirs := []string{"internal/server", "web"}
	
	for _, dir := range dirs {
		err := os.MkdirAll(dir, 0755)
		if err != nil {
			fmt.Printf("Error creating directory %s: %v\n", dir, err)
			os.Exit(1)
		}
		fmt.Printf("Created directory: %s\n", dir)
	}
	
	// List contents of created directories
	fmt.Println("\nContents:")
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			fmt.Printf("Error reading %s: %v\n", dir, err)
			continue
		}
		fmt.Printf("\n%s/:\n", dir)
		if len(entries) == 0 {
			fmt.Println("  (empty)")
		}
		for _, entry := range entries {
			info, _ := entry.Info()
			path := filepath.Join(dir, entry.Name())
			mode := info.Mode()
			size := info.Size()
			isDir := entry.IsDir()
			
			var typeStr string
			if isDir {
				typeStr = "d"
			} else {
				typeStr = "-"
			}
			
			fmt.Printf("  %s %d %s\n", typeStr, size, entry.Name())
			_ = mode
			_ = path
		}
	}
}
