# One-File Video Streaming Service

A lightweight, single-file video streaming service written in Go that supports hardware-accelerated transcoding and multiple streaming formats. Upload videos and get instant HLS and DASH streaming URLs.

## Features

### Streaming Formats

- HLS (HTTP Live Streaming)
- DASH (Dynamic Adaptive Streaming over HTTP)

### Codec Support

- AVC (H.264) - Best compatibility
- HEVC (H.265) - Better compression
- AV1 - Next-gen codec with superior compression

### Hardware Acceleration

- macOS: VideoToolbox
- NVIDIA: NVENC
- AMD: AMF
- Intel: QuickSync

## Prerequisites

- Go 1.19 or later
- FFmpeg with hardware acceleration support
- 8GB+ RAM recommended
- Storage space for video processing

## Quick Start

1. Clone the repository:

```bash
git clone https://github.com/yourusername/agomi-video-go.git
cd agomi-video-go
```

2. Install FFmpeg:

```bash
# macOS
brew install ffmpeg

# Ubuntu/Debian
sudo apt update
sudo apt install ffmpeg

# Windows (using Chocolatey)
choco install ffmpeg
```

3. Create a `.env` file:

```plaintext
PORT=3000
STORAGE_PATH=./videos
HW_ACCEL=macos  # Options: macos, nvidia, amd, intel
```

4. Run the server:

```bash
go run main.go
```

5. Open `index.html` in your browser or serve it using a static file server.

## API Endpoints

### Upload Video

```http
POST /api/videos?codec=avc
```

- Supported codec query parameters: `avc`, `hevc`, `av1`
- Returns HLS and DASH URLs

### Stream Access

- HLS: `http://localhost:3000/hls/{videoId}/playlist.m3u8`
- DASH: `http://localhost:3000/dash/{videoId}/manifest.mpd`

## Hardware Acceleration Setup

### macOS (VideoToolbox)

- No additional setup required
- Set `HW_ACCEL=macos` in `.env`

### NVIDIA (NVENC)

- Install NVIDIA drivers
- Install CUDA toolkit
- Set `HW_ACCEL=nvidia` in `.env`

### AMD (AMF)

- Install AMD drivers
- Set `HW_ACCEL=amd` in `.env`

### Intel (QuickSync)

- Enable Intel QuickSync
- Set `HW_ACCEL=intel` in `.env`

## Performance Notes

- Hardware acceleration significantly improves transcoding speed
- AV1 encoding is currently software-only and slower
- HEVC offers better compression than AVC at the cost of compatibility
- Default settings prioritize speed over quality

## License

MIT License

## Contributing

Pull requests are welcome. For major changes, please open an issue first to discuss what you would like to change.
