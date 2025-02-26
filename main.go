package main

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
)

const base_url = "http://localhost:3000"

func main() {
	godotenv.Load()
	storagePath, port := getEnv("STORAGE_PATH", "./videos"), getEnv("PORT", "3000")
	ensureDirectories(storagePath)

	r := gin.Default()
	r.Use(corsMiddleware())
	r.MaxMultipartMemory = 8 << 20

	r.Static("/hls", filepath.Join(storagePath, "hls"))
	r.Static("/dash", filepath.Join(storagePath, "dash"))
	r.POST("/api/videos", handleVideoUpload(storagePath))
	r.Run(":" + port)
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}

func handleVideoUpload(storagePath string) gin.HandlerFunc {
	return func(c *gin.Context) {
		startTime := time.Now()

		// Get codec from query parameter, default to avc
		codec := c.DefaultQuery("codec", "avc")
		if !isValidCodec(codec) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid codec. Use av1, hevc, or avc"})
			return
		}

		file, err := c.FormFile("video")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "No video file uploaded"})
			return
		}

		videoID := uuid.New().String()
		filename := filepath.Join(storagePath, "uploads", videoID+filepath.Ext(file.Filename))
		uploadStartTime := time.Now()
		if err := c.SaveUploadedFile(file, filename); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save video"})
			return
		}

		urls, timings, err := transcodeVideo(filename, videoID, storagePath, codec)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to transcode video"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"id": videoID, "hlsUrl": urls["hls"], "dashUrl": urls["dash"],
			"timings": gin.H{
				"uploadDuration": time.Since(uploadStartTime).Seconds(),
				"hlsTranscode":   timings["hlsTranscode"],
				"dashTranscode":  timings["dashTranscode"],
				"totalDuration":  time.Since(startTime).Seconds(),
			},
		})
	}
}

func isValidCodec(codec string) bool {
	validCodecs := map[string]bool{
		"av1":  true,
		"hevc": true,
		"avc":  true,
	}
	return validCodecs[codec]
}

func transcodeVideo(inputPath, videoID, storagePath, codec string) (map[string]string, map[string]float64, error) {
	hlsPath, dashPath := filepath.Join(storagePath, "hls", videoID), filepath.Join(storagePath, "dash", videoID)
	os.MkdirAll(hlsPath, 0755)
	os.MkdirAll(dashPath, 0755)

	timings := make(map[string]float64)
	inputParams, outputParams := splitFFmpegParams(getEnv("HW_ACCEL", "macos"), codec)

	// Create channels for error handling and synchronization
	errChan := make(chan error, 2)
	timesChan := make(chan struct {
		key   string
		value float64
	}, 2)

	// Start HLS transcoding in a goroutine
	go func() {
		hlsStart := time.Now()
		hlsCmd := exec.Command("ffmpeg", append(append(inputParams, "-i", inputPath),
			append(outputParams, "-hls_time", "4", "-hls_playlist_type", "vod",
				"-hls_segment_filename", filepath.Join(hlsPath, "segment_%03d.ts"),
				filepath.Join(hlsPath, "playlist.m3u8"))...)...)
		hlsCmd.Stdout = os.Stdout
		hlsCmd.Stderr = os.Stderr

		if err := hlsCmd.Run(); err != nil {
			errChan <- fmt.Errorf("HLS transcoding error: %v", err)
			return
		}
		timesChan <- struct {
			key   string
			value float64
		}{key: "hlsTranscode", value: time.Since(hlsStart).Seconds()}
		errChan <- nil
	}()

	// Start DASH transcoding in a goroutine
	go func() {
		dashStart := time.Now()
		dashCmd := exec.Command("ffmpeg", append(append(inputParams, "-i", inputPath),
			append(outputParams, "-f", "dash", "-use_timeline", "1", "-use_template", "1",
				"-seg_duration", "4", "-adaptation_sets", "id=0,streams=v id=1,streams=a",
				filepath.Join(dashPath, "manifest.mpd"))...)...)
		dashCmd.Stdout = os.Stdout
		dashCmd.Stderr = os.Stderr

		if err := dashCmd.Run(); err != nil {
			errChan <- fmt.Errorf("DASH transcoding error: %v", err)
			return
		}
		timesChan <- struct {
			key   string
			value float64
		}{key: "dashTranscode", value: time.Since(dashStart).Seconds()}
		errChan <- nil
	}()

	// Wait for both transcoding processes to complete
	for i := 0; i < 2; i++ {
		if err := <-errChan; err != nil {
			return nil, nil, err
		}
	}

	// Collect timing information
	for i := 0; i < 2; i++ {
		timing := <-timesChan
		timings[timing.key] = timing.value
	}

	return map[string]string{
		"hls":  fmt.Sprintf("%s/hls/%s/playlist.m3u8", base_url, videoID),
		"dash": fmt.Sprintf("%s/dash/%s/manifest.mpd", base_url, videoID),
	}, timings, nil
}

func splitFFmpegParams(hwAccel, codec string) ([]string, []string) {
	var commonOutput []string
	switch codec {
	case "av1":
		commonOutput = []string{"-c:v", "libaom-av1", "-crf", "30", "-b:v", "0", "-strict", "experimental", "-c:a", "aac", "-b:a", "128k"}
	case "hevc":
		commonOutput = []string{"-c:v", "libx265", "-crf", "28", "-preset", "medium", "-c:a", "aac", "-b:a", "128k"}
	default: // avc
		commonOutput = []string{
			"-c:v", "libx264",
			"-preset", "ultrafast",
			"-tune", "fastdecode",
			"-profile:v", "baseline",
			"-level", "3.0",
			"-b:v", "2M",
			"-maxrate", "2.5M",
			"-bufsize", "5M",
			"-pix_fmt", "yuv420p",
			"-c:a", "aac",
			"-b:a", "128k",
			"-movflags", "+faststart",
			"-g", "48",
			"-keyint_min", "48",
			"-sc_threshold", "0",
			"-bf", "0",
		}
	}

	switch hwAccel {
	case "nvidia":
		codecParams := map[string][]string{
			"av1":  {"-c:v", "av1_nvenc"},
			"hevc": {"-c:v", "hevc_nvenc"},
			"avc":  {"-c:v", "h264_nvenc", "-preset", "p4", "-tune", "ll"},
		}
		return []string{"-hwaccel", "cuda"}, append(codecParams[codec], commonOutput...)
	case "intel":
		codecParams := map[string][]string{
			"hevc": {"-c:v", "hevc_qsv"},
			"avc":  {"-c:v", "h264_qsv", "-preset", "faster"},
		}
		if codec == "av1" {
			return []string{}, commonOutput // Fall back to software encoding for AV1
		}
		return []string{"-hwaccel", "qsv"}, append(codecParams[codec], commonOutput...)
	case "amd":
		codecParams := map[string][]string{
			"hevc": {"-c:v", "hevc_amf"},
			"avc":  {"-c:v", "h264_amf", "-quality", "speed"},
		}
		if codec == "av1" {
			return []string{}, commonOutput // Fall back to software encoding for AV1
		}
		return []string{"-hwaccel", "amf"}, append(codecParams[codec], commonOutput...)
	default: // macos
		codecParams := map[string][]string{
			"hevc": {"-c:v", "hevc_videotoolbox"},
			"avc":  {"-c:v", "h264_videotoolbox", "-allow_sw", "0", "-realtime", "1", "-profile:v", "high", "-tag:v", "avc1", "-threads", "0"},
		}
		if codec == "avc" {
			return []string{"-hwaccel", "videotoolbox"},
				[]string{
					"-c:v", "h264_videotoolbox",
					"-b:v", "2M",
					"-maxrate", "2.5M",
					"-bufsize", "5M",
					"-pix_fmt", "nv12",
					"-c:a", "aac",
					"-b:a", "128k",
				}
		}
		return []string{"-hwaccel", "videotoolbox", "-hwaccel_output_format", "videotoolbox_vld"}, append(codecParams[codec], commonOutput...)
	}
}

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}

func ensureDirectories(storagePath string) {
	for _, dir := range []string{"uploads", "transcoded", "hls", "dash"} {
		os.MkdirAll(filepath.Join(storagePath, dir), 0755)
	}
}
