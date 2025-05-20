package audio

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/cyrilschreiber3/media-processor/pkg/ffmpeg"
)

// ProcessUnsupportedAudio moves the original file to the "Originals" directory
// and creates a converted version with supported audio format.
func ProcessUnsupportedAudio(filePath string) error {
	log.Printf("Moving unsupported audio file to Originals: %s\n", filePath)

	parentDir := filepath.Dir(filePath)

	parentDirInfo, err := os.Stat(parentDir)
	if err != nil {
		return fmt.Errorf("error getting parent directory info: %w", err)
	}

	// Create Originals directory if it doesn't exist
	originalsDir := filepath.Join(parentDir, "Originals")
	if _, err := os.Stat(originalsDir); err != nil {
		if err := os.MkdirAll(originalsDir, parentDirInfo.Mode()); err != nil {
			return fmt.Errorf("error creating Originals directory: %w", err)
		}
	}

	fileName := filepath.Base(filePath)
	inputFilePath := filepath.Join(originalsDir, fileName)

	// Check if file already exists in Originals
	if _, err := os.Stat(inputFilePath); err == nil {
		return fmt.Errorf("original file already exists: %s", inputFilePath)
	}

	// Move original file to Originals directory
	if err := os.Rename(filePath, inputFilePath); err != nil {
		return fmt.Errorf("error moving file to Originals: %w", err)
	}

	// Create and execute FFmpeg command to convert audio
	cmd := ffmpeg.CreateConvertedOriginalCommand(filePath)
	if len(cmd) == 0 {
		return errors.New("could not generate ffmpeg command for original file")
	}

	log.Printf("Executing ffmpeg command for original file: %s\n", strings.Join(cmd, " "))
	cmdExec := exec.Command(cmd[0], cmd[1:]...) //nolint:gosec
	cmdExec.Stdout = os.Stdout
	cmdExec.Stderr = os.Stderr

	if err := cmdExec.Run(); err != nil {
		return fmt.Errorf("error executing ffmpeg command for original file: %w", err)
	}

	return nil
}
