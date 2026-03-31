# Sonos Status Utility

A lightweight, multi-room status dashboard for Sonos systems built with Go and the Fyne GUI toolkit.

## Features

- **Multi-Room Overview**: Automatically discovers and groups all Sonos speakers in your network.
- **Real-time Playback Status**: Shows current track, artist, and album art.
- **Interactive Controls**:
    - Play/Pause toggle
    - Volume slider
    - Mute/Unmute
- **Audio Modes**:
    - Night Mode
    - Speech Enhancement
    - **Loudness** (New)
- **Advanced Diagnostics**:
    - **Track Progress Bar**: Real-time position and duration (e.g., `01:45 / 03:20`).
    - **Interface Quality**: Detects connection type (Wired/Wireless) and reports signal quality (SNR/RSSI/SonosNet Quality).
    - **Hardware Details**: Displays all physical units in a room and their IP addresses.
- **Clean UI**: Dark-themed, responsive layout that suppresses common macOS linker warnings.

## Installation

Ensure you have [Go](https://go.dev/doc/install) installed (v1.26.1 or later).

### Prerequisites (macOS/Linux)
You may need standard development headers for Fyne (X11/OpenGL).

## Usage

### Local Development
Use the provided `Makefile` to avoid duplicate library warnings during development and build:

#### Run the application
```bash
make run
```

#### Build the binary
```bash
make build
```

## Automated Builds & Packaging

This project uses **GitHub Actions** to automatically build and package the application for multiple platforms on every push to the `main` branch.

### macOS
- **Format**: Standard macOS Installer Package (`.pkg`).
- **Installation**: Run the `.pkg` file to install **SonosStatus** directly into your `/Applications` folder.

### Linux
- **Format**: Compressed Archive (`.tar.gz`).
- **Installation**: Extract the archive and run the `SonosStatus` binary. Ensure you have the necessary graphics libraries (Mesa/X11) installed.

### Windows
- **Format**: Compressed Zip (`.zip`).
- **Installation**: Extract the archive and run `SonosStatus.exe`.

### How to Download
1. Go to the **Actions** tab in your GitHub repository.
2. Select the latest successful workflow run.
3. Scroll down to the **Artifacts** section to download the builds for your platform.

## How it Works

- **Discovery**: Uses SSDP (Simple Service Discovery Protocol) to find Sonos speakers on your local network.
- **Status Updates**: Polls individual speakers for volume, mute, and playback state.
- **Diagnostics**: Scrapes hidden internal Sonos diagnostic pages (on port 1400) to fetch high-fidelity signal strength (SNR) and mesh network (SonosNet) quality metrics.

## Dependencies

- [Fyne v2](https://fyne.io/)
- [Gonos](https://github.com/HandyGold75/Gonos)

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
