package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/cyrilschreiber3/media-processor/pkg/audio"
	"github.com/cyrilschreiber3/media-processor/pkg/ffmpeg"
	"github.com/cyrilschreiber3/media-processor/pkg/media"
	"github.com/cyrilschreiber3/media-processor/pkg/proxy"
)

// processFile handles the processing of a single media file.
func processFile(file os.DirEntry, filePath string) (bool, error) {
	log.Printf("Processing file: %s\n", filePath)

	// Generate proxy file
	changed, err := proxy.GenerateProxy(filePath, file)
	if err != nil {
		return false, fmt.Errorf("error generating proxy: %w", err)
	}

	// Get media information for audio processing
	mediaInfo, err := media.GetMediaInfo(filePath)
	if err != nil {
		return false, fmt.Errorf("error getting media info: %w", err)
	}

	// Check if file has unsupported audio format
	props := media.AnalyzeMediaInfo(mediaInfo)
	if props.UnsupportedAudioFormat {
		log.Printf("Unsupported audio format detected. Converting to PCM for file: %s\n", filePath)

		err = audio.ProcessUnsupportedAudio(filePath)
		if err != nil {
			return false, fmt.Errorf("error processing unsupported audio source file: %w", err)
		}
	}

	return changed, nil
}

func main() {
	// Check command line arguments
	if len(os.Args) < 2 {
		log.Fatal("Usage: go run main.go <path>")
	}

	// Check if FFmpeg is installed
	if !ffmpeg.IsFFmpegInstalled() {
		log.Fatal("ffmpeg is not installed. Please install ffmpeg to use this program.")
	}

	// Get the watch path from command line arguments
	watchPath := os.Args[1]

	// Read the files in the watch path
	files, err := os.ReadDir(watchPath)
	if err != nil {
		log.Fatal(err)
	}

	// Process each file
	for _, file := range files {
		// Skip directories
		if file.IsDir() {
			continue
		}

		filePath := filepath.Join(watchPath, file.Name())

		// Skip non-media files
		if !media.IsMediaFile(filePath) {
			log.Printf("Skipping non-media file: %s\n", filePath)

			continue
		}

		// Skip files in the Proxy directory
		parentDir := filepath.Dir(filePath)
		if filepath.Base(parentDir) == "Proxy" {
			log.Printf("Skipping proxy file: %s\n", filePath)

			continue
		}

		// Process the file
		changed, err := processFile(file, filePath)
		if err != nil {
			log.Printf("Error processing file %s: %v\n", filePath, err)

			continue
		}

		// Log the result
		if changed {
			log.Printf("File %s has been processed successfully\n", filePath)
		} else {
			log.Printf("File %s has not been changed\n", filePath)
		}
	}
}
