# Go Fireworks

An audio-reactive terminal fireworks show written in Go using tcell for terminal rendering and using go-particle and real-time audio analysis.

![tim particle screenshot](screenshot.png)


## Features

- Beautiful braille-based particle effects
- Real-time audio reactivity (responds to music playing on your system)
- Bass-triggered firework explosions
- Volume-influenced particle counts
- Frequency-based color selection
- Smooth physics simulation with gravity and air resistance
- Live audio visualization bars

## Audio Reactivity

The fireworks respond to system audio in four primary ways:

- **Volume** → **Color**: Louder sounds cycle through brighter colors
- **Pitch** (dominant frequency) → **Speed**: Higher pitched sounds create faster-moving particles
- **Rolling Average** (smoothed volume) → **Size**: Sustained loud sounds create larger explosions with more particles
- **Energy** (spectral energy) → **Density**: Higher overall energy increases the number of particles on screen by spawning fireworks more frequently

Additionally:
- **Bass peaks** trigger new firework explosions
- **Instant volume spikes** can also trigger explosions

## Requirements

- Go 1.21+
- PulseAudio or PipeWire (for audio capture)
- Terminal with Unicode/braille character support

## Installation & Running

### Using Nix (recommended)

```bash
# Build and run
nix run

# Or build first
nix build
./result/bin/go-fireworks

# Development shell
nix develop
```

### Using Go directly

```bash
# Install dependencies
go mod download

# Run
go run *.go

# Build
go build -o go-fireworks
./go-fireworks
```

### Using Make

```bash
make run       # Build and run
make nix-run   # Run with Nix
make build     # Build only
```

## Controls

- **ESC** or **Q**: Exit the show
- **D**: Toggle debug mode (shows audio values, firework stats, and effect mappings)

## Debug Mode

Press **D** to toggle debug information that displays:

**Audio Data:**
- Volume, Pitch, Rolling Average, Energy, Bass (with 4 decimal precision)
- Last bass peak value

**Firework Statistics:**
- Total fireworks on screen
- Number of rockets launching
- Number of exploded fireworks
- Total particle count

**Effects Mapping:**
- Quick reference showing which audio feature controls which visual effect

This is helpful for understanding how the music affects the fireworks in real-time.

## How It Works

The application:

1. Captures system audio output using `pacat` (PulseAudio/PipeWire monitor)
2. Performs FFT (Fast Fourier Transform) analysis on audio samples
3. Extracts audio features:
   - Frequency band energies (bass, mid, high)
   - Overall volume (RMS)
   - Dominant frequency (pitch)
   - Rolling average of volume over time
   - Spectral energy (combined energy across all frequencies)
4. Maps audio features to firework properties:
   - **Volume** → Color selection (cycles through color palette)
   - **Pitch** → Particle speed (high pitch = fast particles)
   - **Rolling average** → Particle count/explosion size
   - **Energy** → Spawn rate/particle density (high energy = more fireworks on screen)
   - **Bass peaks** → Trigger new explosions
5. Renders particles using braille Unicode characters for detailed effects
6. Displays real-time audio levels and their effects on screen

## Audio System Compatibility

The application auto-detects your audio system and works with:

- PulseAudio
- PipeWire (via PulseAudio compatibility layer)

If no audio system is detected, the fireworks will still display with automatic random timing.

## Development

The project consists of:

- `main.go`: Fireworks rendering and physics engine
- `audio.go`: Audio capture and FFT analysis
- `flake.nix`: Nix build configuration
- `Makefile`: Convenience build targets

## License

MIT
