package proxy

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/cyrilschreiber3/media-processor/pkg/ffmpeg"
	"github.com/cyrilschreiber3/media-processor/pkg/media"
)

// CreateProxyDirectory creates a proxy directory within the parent directory.
func CreateProxyDirectory(filePath string) (string, error) {
	parentDir, err := os.Stat(filepath.Dir(filePath))
	if err != nil {
		return "", fmt.Errorf("error getting parent directory: %w", err)
	}

	proxyDir := filepath.Join(filepath.Dir(filePath), "Proxy")

	if _, err := os.Stat(proxyDir); err == nil {
		return proxyDir, nil
	}

	if err := os.MkdirAll(proxyDir, parentDir.Mode()); err != nil {
		return "", fmt.Errorf("error creating proxy directory: %w", err)
	}

	return proxyDir, nil
}

// GenerateProxy creates a proxy file from the original media.
func GenerateProxy(filePath string, fileInfo os.DirEntry) (bool, error) {
	parentDir := filepath.Dir(filePath)
	fileName := strings.TrimSuffix(fileInfo.Name(), filepath.Ext(fileInfo.Name()))
	proxyFilePath := filepath.Join(parentDir, "Proxy", fileName+".mov")

	// Check if proxy already exists
	if _, err := os.Stat(proxyFilePath); err == nil {
		log.Printf("Proxy file already exists: %s\n", proxyFilePath)

		return false, nil
	}

	// Get media information
	mediaInfo, err := media.GetMediaInfo(filePath)
	if err != nil {
		return false, fmt.Errorf("error getting media info: %w", err)
	}

	if len(mediaInfo.Streams) == 0 {
		return false, errors.New("no streams found in media file")
	}

	// Analyze media properties
	props := media.AnalyzeMediaInfo(mediaInfo)
	if !props.HasVideoStream && !props.HasAudioStream {
		return false, errors.New("no video or audio stream found")
	}

	// Create proxy directory
	_, err = CreateProxyDirectory(filePath)
	if err != nil {
		return false, fmt.Errorf("error creating proxy directory: %w", err)
	}

	// Create and run ffmpeg command
	ffmpegCmd := ffmpeg.CreateProxyCommand(filePath, proxyFilePath, props)
	if len(ffmpegCmd) == 0 {
		return false, errors.New("could not generate ffmpeg command")
	}

	log.Printf("Executing ffmpeg command: %s\n", strings.Join(ffmpegCmd, " "))
	cmdExec := exec.Command(ffmpegCmd[0], ffmpegCmd[1:]...) //nolint:gosec
	cmdExec.Stdout = os.Stdout
	cmdExec.Stderr = os.Stderr

	if err := cmdExec.Run(); err != nil {
		return false, fmt.Errorf("error executing ffmpeg command: %w", err)
	}

	return true, nil
}
