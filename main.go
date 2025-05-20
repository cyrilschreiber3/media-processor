package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

const useHwaccel = true

func isMediaFile(filePath string) bool {
	mediaExtensions := []string{".mp4", ".avi", ".mkv", ".mov", ".flv", ".wmv", ".mp3", ".wav", ".aac", ".ogg", ".flac"}
	for _, ext := range mediaExtensions {
		lowercaseFilePath := strings.ToLower(filePath)
		if strings.HasSuffix(lowercaseFilePath, ext) {
			return true
		}
	}

	return false
}

type mediaInfo struct {
	Format struct {
		FilePath string `json:"filename"`
		Duration string `json:"duration"`
		Bitrate  string `json:"bit_rate"`
	} `json:"format"`
	Streams []struct {
		Index        int    `json:"index"`
		CodecType    string `json:"codec_type"`
		CodecName    string `json:"codec_name"`
		CodecProfile string `json:"profile"`
		Width        int    `json:"width"`
		Height       int    `json:"height"`
		Bitrate      string `json:"bit_rate"`
		PixelFormat  string `json:"pix_fmt"`
	} `json:"streams"`
}

func getMediaInfo(filePath string) (mediaInfo, error) {
	var info mediaInfo

	cmd := exec.Command("ffprobe",
		"-hide_banner",
		"-loglevel", "fatal",
		"-show_error",
		"-show_format",
		"-show_streams",
		"-show_private_data",
		"-print_format", "json",
		filePath)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return info, fmt.Errorf("error executing ffprobe: %w", err)
	}

	if err := json.Unmarshal(output, &info); err != nil {
		return info, fmt.Errorf("error unmarshalling ffprobe output: %w", err)
	}

	return info, nil
}

func processPixelFormatMatch(match []string, pixelFormat string, pixelFormatExp *regexp.Regexp) (int, error) {
	result := make(map[string]string)

	for i, name := range pixelFormatExp.SubexpNames() {
		if i != 0 && name != "" {
			result[name] = match[i]
		}
	}

	if result["name"] == pixelFormat {
		bitDepthStr, ok := result["bit_depth"]
		if !ok {
			return -1, fmt.Errorf("bit depth not found for pixel format %s", pixelFormat)
		}

		nbComponentsStr, ok := result["nb_components"]
		if !ok {
			return -1, fmt.Errorf("number of components not found for pixel format %s", pixelFormat)
		}

		nbComponentsInt, err := strconv.Atoi(nbComponentsStr)
		if err != nil {
			return -1, fmt.Errorf("error converting number of components to int: %w", err)
		}

		if nbComponentsInt > 1 {
			bitDepthStr = strings.Split(bitDepthStr, "-")[0]
		}

		bitDepth, _ := strconv.Atoi(bitDepthStr)

		return bitDepth, nil
	}

	return -1, fmt.Errorf("pixel format %s not found", pixelFormat)
}

func getBitDepth(pixelFormat string) (int, error) {
	cmd := exec.Command("ffmpeg", "-hide_banner", "-pix_fmts")

	output, err := cmd.CombinedOutput()
	if err != nil {
		return -1, fmt.Errorf("error executing ffmpeg: %w", err)
	}

	outputArr := strings.Split(string(output), "\n")

	const pixelFormatExpStr = `^.{5}\s(?<name>[^\r\n\s=]*)\s*(?P<nb_components>[0-9])\s*(?P<bpp>[0-9]*)\s*(?P<bit_depth>(?:[0-9]+-)*[0-9]+)$` //nolint:lll
	pixelFormatExp := regexp.MustCompile(pixelFormatExpStr)

	for _, line := range outputArr {
		match := pixelFormatExp.FindStringSubmatch(line)
		if len(match) > 0 {
			bitDepth, err := processPixelFormatMatch(match, pixelFormat, pixelFormatExp)
			if err == nil {
				return bitDepth, nil
			}
		}
	}

	return -1, fmt.Errorf("pixel format %s not found", pixelFormat)
}

func isAudioCodecSupported(codecName string) bool {
	supportedFormats := []string{"mp3", "opus", "flac", "ac3"}

	switch {
	case strings.Contains(codecName, "pcm_"):
		return true
	case slices.Contains(supportedFormats, codecName):
		return true
	default:
		return false
	}
}

type mediaProperties struct {
	hasVideoStream         bool
	hasAudioStream         bool
	isVertical             bool
	unsupportedAudioFormat bool
	highestBitDepth        int
}

func analyzeMediaInfo(info mediaInfo) mediaProperties {
	var mediaProperties mediaProperties

	for _, stream := range info.Streams {
		if stream.CodecType == "video" {
			mediaProperties.hasVideoStream = true

			bitDepth, err := getBitDepth(stream.PixelFormat)
			if err != nil {
				log.Printf("Error getting bit depth for pixel format %s: %v\n", stream.PixelFormat, err)
				log.Printf("Using default bit depth of 8\n")

				bitDepth = 8 // Default to 8 if error occurs
			}

			fmt.Printf("Bit depth for pixel format %s: %d\n", stream.PixelFormat, bitDepth)

			if bitDepth > mediaProperties.highestBitDepth {
				mediaProperties.highestBitDepth = bitDepth
			}

			if stream.Width > stream.Height {
				mediaProperties.isVertical = false
			} else {
				mediaProperties.isVertical = true
			}
		}

		if stream.CodecType == "audio" {
			mediaProperties.hasAudioStream = true

			mediaProperties.unsupportedAudioFormat = !isAudioCodecSupported(stream.CodecName)
		}
	}

	return mediaProperties
}

func createProxyDirectory(filePath string) (string, error) {
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

func createFfmpegCommandForProxy(filePath string, proxyFilePath string, media mediaProperties) []string {
	var cmd []string

	cmd = append(cmd, "ffmpeg", "-y", "-hide_banner", "-loglevel", "error")

	if useHwaccel {
		cmd = append(cmd, "-hwaccel", "cuda")
	}

	cmd = append(cmd, "-i", filePath)

	if media.hasVideoStream { //nolint:nestif
		if useHwaccel {
			cmd = append(cmd, "-c:v", "h264_nvenc")
		} else {
			cmd = append(cmd, "-c:v", "libx264")
		}

		if media.highestBitDepth > 8 {
			cmd = append(cmd, "-pix_fmt", "yuv420p")
		}

		cmd = append(cmd, "-maxrate", "6M", "-preset", "fast")

		if media.isVertical {
			cmd = append(cmd, "-vf", "scale=540:-2")
		} else {
			cmd = append(cmd, "-vf", "scale=960:-2")
		}
	}

	if media.hasAudioStream {
		if media.unsupportedAudioFormat {
			cmd = append(cmd, "-c:a", "pcm_s16le")
		}
	}

	cmd = append(cmd, proxyFilePath)

	return cmd
}

func createFfmpegCommandForOriginal(filePath string) []string {
	var cmd []string

	fileName := filepath.Base(filePath)
	parentDir := filepath.Dir(filePath)

	inputFilePath := filepath.Join(parentDir, "Originals", fileName)

	cmd = append(cmd, "ffmpeg", "-y", "-hide_banner", "-loglevel", "error")
	cmd = append(cmd, "-i", inputFilePath, "-c:v", "copy", "-c:a", "pcm_s16le", filePath)

	return cmd
}

func processUnsupportedAudioOriginal(filePath string) (bool, error) {
	log.Printf("Moving unsupported audio file to Originals: %s\n", filePath)

	parentDir := filepath.Dir(filePath)

	parentDirInfo, err := os.Stat(parentDir)
	if err != nil {
		return false, fmt.Errorf("error getting parent directory info: %w", err)
	}

	originalsDir := filepath.Join(parentDir, "Originals")
	if _, err := os.Stat(originalsDir); err != nil {
		if err := os.MkdirAll(originalsDir, parentDirInfo.Mode()); err != nil {
			return false, fmt.Errorf("error creating Originals directory: %w", err)
		}
	}

	fileName := filepath.Base(filePath)

	inputFilePath := filepath.Join(originalsDir, fileName)
	if _, err := os.Stat(inputFilePath); err == nil {
		return false, fmt.Errorf("original file already exists: %s", inputFilePath)
	}

	if err := os.Rename(filePath, inputFilePath); err != nil {
		return false, fmt.Errorf("error moving file to Originals: %w", err)
	}

	cmd := createFfmpegCommandForOriginal(filePath)
	if len(cmd) == 0 {
		return false, errors.New("could not generate ffmpeg command for original file")
	}

	log.Printf("Executing ffmpeg command for original file: %s\n", strings.Join(cmd, " "))
	cmdExec := exec.Command(cmd[0], cmd[1:]...)
	cmdExec.Stdout = os.Stdout
	cmdExec.Stderr = os.Stderr

	if err := cmdExec.Run(); err != nil {
		return false, fmt.Errorf("error executing ffmpeg command for original file: %w", err)
	}
	// TODO: parse the output

	return true, nil
}

func processFile(file os.DirEntry, filePath string) (bool, error) {
	log.Printf("Processing file: %s\n", filePath)

	parentDir := filepath.Dir(filePath)
	fileName := strings.TrimSuffix(file.Name(), filepath.Ext(file.Name()))

	proxyFilePath := filepath.Join(parentDir, "Proxy", fileName+".mov")
	if _, err := os.Stat(proxyFilePath); err == nil {
		log.Printf("Proxy file already exists: %s\n", proxyFilePath)

		return false, nil
	}

	mediaInfo, err := getMediaInfo(filePath)
	if err != nil {
		return false, fmt.Errorf("error getting media info: %w", err)
	}

	if len(mediaInfo.Streams) == 0 {
		return false, errors.New("no streams found in media file")
	}

	mediaProperties := analyzeMediaInfo(mediaInfo)
	if !mediaProperties.hasVideoStream && !mediaProperties.hasAudioStream {
		return false, errors.New("no video or audio stream found")
	}

	_, err = createProxyDirectory(filePath)
	if err != nil {
		return false, fmt.Errorf("error creating proxy directory: %w", err)
	}

	ffmpegCmd := createFfmpegCommandForProxy(filePath, proxyFilePath, mediaProperties)
	if len(ffmpegCmd) == 0 {
		return false, errors.New("could not generate ffmpeg command")
	}

	log.Printf("Executing ffmpeg command: %s\n", strings.Join(ffmpegCmd, " "))
	cmdExec := exec.Command(ffmpegCmd[0], ffmpegCmd[1:]...)
	cmdExec.Stdout = os.Stdout
	cmdExec.Stderr = os.Stderr

	if err := cmdExec.Run(); err != nil {
		return false, fmt.Errorf("error executing ffmpeg command for original file: %w", err)
	}
	// TODO: parse the output

	if mediaProperties.unsupportedAudioFormat {
		log.Printf("Unsupported audio format detected. Converting to PCM for file: %s\n", filePath)

		_, err = processUnsupportedAudioOriginal(filePath)
		if err != nil {
			return false, fmt.Errorf("error processing unsupported audio source file: %w", err)
		}
	}

	return true, nil
}

func main() {
	if len(os.Args) < 2 {
		log.Fatal("Usage: go run main.go <path>")
	}

	if _, err := exec.LookPath("ffmpeg"); err != nil {
		log.Fatal("ffmpeg is not installed. Please install ffmpeg to use this program.")
	}

	watchPath := os.Args[1]

	files, err := os.ReadDir(watchPath)
	if err != nil {
		log.Fatal(err)
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		filePath := strings.TrimRight(watchPath, "/") + "/" + file.Name()

		if !isMediaFile(filePath) {
			log.Printf("Skipping non-media file: %s\n", filePath)

			continue
		}

		parentDir := filepath.Dir(filePath)
		if parentDir == "Proxy" {
			log.Printf("Skipping proxy file: %s\n", filePath)

			continue
		}

		changed, err := processFile(file, filePath)
		if err != nil {
			log.Printf("Error processing file %s: %v\n", filePath, err)

			continue
		}

		if changed {
			log.Printf("File %s has been processed and saved as %s\n", filePath, filepath.Join(parentDir, "Proxy", file.Name()))
		} else {
			log.Printf("File %s has not been changed\n", filePath)
		}
	}
}
