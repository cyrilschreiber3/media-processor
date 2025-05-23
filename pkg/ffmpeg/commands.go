package ffmpeg

import (
	"os/exec"
	"path/filepath"

	"github.com/cyrilschreiber3/media-processor/pkg/media"
)

// UseHardwareAcceleration determines if CUDA hardware acceleration should be used.
const UseHardwareAcceleration = true

// CreateProxyCommand creates an FFmpeg command for generating a proxy file.
func CreateProxyCommand(filePath string, proxyFilePath string, props media.Properties) []string {
	var cmd []string

	cmd = append(cmd, "ffmpeg", "-y", "-hide_banner", "-loglevel", "error")

	if UseHardwareAcceleration {
		cmd = append(cmd, "-hwaccel", "cuda")
	}

	cmd = append(cmd, "-i", filePath)

	//nolint:nestif
	if props.HasVideoStream {
		if UseHardwareAcceleration {
			cmd = append(cmd, "-c:v", "h264_nvenc")
		} else {
			cmd = append(cmd, "-c:v", "libx264")
		}

		if props.HighestBitDepth > 8 {
			cmd = append(cmd, "-pix_fmt", "yuv420p")
		}

		cmd = append(cmd, "-maxrate", "7M", "-preset", "default")

		if props.IsVertical {
			cmd = append(cmd, "-vf", "scale=540:-2")
		} else {
			cmd = append(cmd, "-vf", "scale=960:-2")
		}
	}

	if props.HasAudioStream {
		if props.UnsupportedAudioFormat {
			cmd = append(cmd, "-c:a", "pcm_s16le")
		}
	}

	cmd = append(cmd, proxyFilePath)

	return cmd
}

// CreateConvertedOriginalCommand creates an FFmpeg command for converting original file.
func CreateConvertedOriginalCommand(filePath string) []string {
	var cmd []string

	fileName := filepath.Base(filePath)
	parentDir := filepath.Dir(filePath)
	inputFilePath := filepath.Join(parentDir, "Originals", fileName)

	cmd = append(cmd, "ffmpeg", "-y", "-hide_banner", "-loglevel", "error")
	cmd = append(cmd, "-i", inputFilePath, "-c:v", "copy", "-c:a", "pcm_s16le", filePath)

	return cmd
}

// IsFFmpegInstalled checks if FFmpeg is installed on the system.
func IsFFmpegInstalled() bool {
	_, err := exec.LookPath("ffmpeg")
	return err == nil //nolint:nlreturn
}
