package media

import (
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// MediaInfo represents the structure of FFprobe output.
type MediaInfo struct {
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

// Properties contains analyzed media properties.
type Properties struct {
	HasVideoStream         bool
	HasAudioStream         bool
	IsVertical             bool
	UnsupportedAudioFormat bool
	HighestBitDepth        int
}

// IsMediaFile checks if a file has a media extension.
func IsMediaFile(filePath string) bool {
	mediaExtensions := []string{".mp4", ".avi", ".mkv", ".mov", ".flv", ".wmv", ".mp3", ".wav", ".aac", ".ogg", ".flac"}
	for _, ext := range mediaExtensions {
		lowercaseFilePath := strings.ToLower(filePath)
		if strings.HasSuffix(lowercaseFilePath, ext) {
			return true
		}
	}

	return false
}

// GetMediaInfo uses FFprobe to get information about a media file.
func GetMediaInfo(filePath string) (MediaInfo, error) {
	var info MediaInfo

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

// GetBitDepth determines the bit depth of a pixel format.
func GetBitDepth(pixelFormat string) (int, error) {
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

// IsAudioCodecSupported checks if an audio codec is supported.
func IsAudioCodecSupported(codecName string) bool {
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

// AnalyzeMediaInfo analyzes the media info and returns properties.
func AnalyzeMediaInfo(info MediaInfo) Properties {
	var props Properties

	for _, stream := range info.Streams {
		if stream.CodecType == "video" {
			props.HasVideoStream = true

			bitDepth, err := GetBitDepth(stream.PixelFormat)
			if err != nil {
				log.Printf("Error getting bit depth for pixel format %s: %v\n", stream.PixelFormat, err)
				log.Printf("Using default bit depth of 8\n")

				bitDepth = 8 // Default to 8 if error occurs
			}

			if bitDepth > props.HighestBitDepth {
				props.HighestBitDepth = bitDepth
			}

			if stream.Width > stream.Height {
				props.IsVertical = false
			} else {
				props.IsVertical = true
			}
		}

		if stream.CodecType == "audio" {
			props.HasAudioStream = true
			props.UnsupportedAudioFormat = !IsAudioCodecSupported(stream.CodecName)
		}
	}

	return props
}
