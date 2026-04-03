# Go Fireworks Library

This project provides reusable Go packages for creating audio-reactive particle effects. You can use it as a standalone application or integrate the packages into your own Go projects.

## Installation

```bash
go get github.com/timlinux/go-fireworks/pkg/display    # High-level API (recommended)
go get github.com/timlinux/go-fireworks/pkg/particles  # Low-level particle physics
go get github.com/timlinux/go-fireworks/pkg/audio      # Audio analysis
go get github.com/timlinux/go-fireworks/pkg/fireworks  # Fireworks logic
```

Or add to your `go.mod`:

```go
require go-fireworks v0.0.0
```

## Quick Start (Easiest)

Drop audio-reactive fireworks into any tcell application with just 3 lines:

```go
import "go-fireworks/pkg/display"

// In your app:
fw, _ := display.New(screen)
defer fw.Stop()
fw.Start()  // Runs in background at 50fps
```

That's it! See [Example 1](#example-1-drop-in-fireworks-3-lines) for complete code.

## Package Overview

### `pkg/display` - High-Level Drop-in API (Recommended)

The easiest way to add fireworks to your app:

- **3-line integration** - Just create, start, and stop
- **Automatic background rendering** - Runs at 50fps in a goroutine
- **Region support** - Render to a portion of your screen
- **Manual control option** - Call Update/Render yourself if needed
- **Trigger API** - Manually spawn fireworks on demand

### `pkg/particles` - Particle Physics Engine

The core particle simulation package providing:

- **Particle type** - Individual particles with position, velocity, life, and color
- **ParticleSystem** - Manages collections of particles with physics simulation
- **Explosion generators** - Create radial, directional, sideways, and spiral explosion patterns
- **Collision detection** - Elastic collisions between particles
- **Cluster scattering** - Automatic re-explosion of tightly grouped particles

### `pkg/audio` - Real-time Audio Analysis

Audio capture and analysis via PulseAudio:

- **FFT-based spectral analysis** - Bass, mid, high frequency bands
- **Advanced features** - Spectral centroid, flux, rolloff
- **Beat detection** - Adaptive threshold beat detection with BPM estimation
- **Onset detection** - Sudden event detection for transients

### `pkg/fireworks` - Fireworks Show Manager

High-level fireworks display system combining particles and audio:

- **Audio-reactive spawning** - Fireworks synchronized to beats
- **Configurable physics** - Gravity, air resistance, explosion parameters
- **Color palettes** - Customizable color schemes
- **Statistics tracking** - Explosion counts by type

## Quick Start Examples

### Example 1: Drop-in Fireworks (3 lines)

The simplest way to add fireworks to an existing tcell app:

```go
package main

import (
    "time"

    "github.com/gdamore/tcell/v2"
    "go-fireworks/pkg/display"
)

func main() {
    // Your existing tcell setup
    screen, _ := tcell.NewScreen()
    screen.Init()
    defer screen.Fini()
    screen.SetStyle(tcell.StyleDefault.Background(tcell.ColorBlack))

    // Add fireworks - just 3 lines!
    fw, _ := display.New(screen)
    defer fw.Stop()
    fw.Start()

    // Your app continues running - fireworks render in background
    // Press any key to exit
    screen.PollEvent()
}
```

### Example 2: Fireworks in a Region

Render fireworks to only part of your screen:

```go
package main

import (
    "github.com/gdamore/tcell/v2"
    "go-fireworks/pkg/display"
)

func main() {
    screen, _ := tcell.NewScreen()
    screen.Init()
    defer screen.Fini()

    width, height := screen.Size()

    // Fireworks in the right half of the screen
    config := display.DefaultConfig()
    config.ShowTitle = true
    config.Title = "Fireworks Zone"

    fw, _ := display.NewWithConfig(screen, config)
    fw.SetBounds(width/2, 0, width, height)
    defer fw.Stop()
    fw.Start()

    // Draw your UI in the left half
    style := tcell.StyleDefault.Foreground(tcell.ColorWhite)
    msg := "Your app UI here"
    for i, ch := range msg {
        screen.SetContent(i+5, height/2, ch, nil, style)
    }
    screen.Show()

    screen.PollEvent()
}
```

### Example 3: Manual Control Loop

For apps that need to control the render loop:

```go
package main

import (
    "time"

    "github.com/gdamore/tcell/v2"
    "go-fireworks/pkg/display"
)

func main() {
    screen, _ := tcell.NewScreen()
    screen.Init()
    defer screen.Fini()

    fw, _ := display.New(screen)
    defer fw.Stop()

    // Don't call fw.Start() - we'll control the loop

    ticker := time.NewTicker(20 * time.Millisecond)
    running := true

    go func() {
        for {
            ev := screen.PollEvent()
            if _, ok := ev.(*tcell.EventKey); ok {
                running = false
                return
            }
        }
    }()

    for running {
        <-ticker.C

        screen.Clear()

        // Your app rendering here...

        // Update and render fireworks
        fw.Update(0) // 0 = auto-calculate dt
        fw.Render()

        screen.Show()
    }
}
```

### Example 4: Trigger Fireworks on Events

Spawn fireworks in response to user actions:

```go
package main

import (
    "github.com/gdamore/tcell/v2"
    "go-fireworks/pkg/display"
)

func main() {
    screen, _ := tcell.NewScreen()
    screen.Init()
    defer screen.Fini()

    fw, _ := display.New(screen)
    defer fw.Stop()
    fw.Start()

    for {
        ev := screen.PollEvent()
        switch ev := ev.(type) {
        case *tcell.EventKey:
            switch ev.Rune() {
            case ' ':
                fw.TriggerFirework() // Single firework
            case 'b':
                fw.TriggerFireworks(5) // Burst of 5
            case 'c':
                fw.Clear() // Clear all
            case 'q':
                return
            }
        }
    }
}
```

### Example 5: Basic Particle System

Use the particle system for any kind of particle effects:

```go
package main

import (
    "time"
    "go-fireworks/pkg/particles"
)

func main() {
    // Create a particle system with bounds
    ps := particles.NewParticleSystem(800, 600)

    // Create an explosion at coordinates (400, 300)
    config := particles.DefaultExplosionConfig()
    config.NumParticles = 50
    config.Speed = 20.0
    config.Type = particles.ExplosionRadial
    config.Color = 0xFF0000 // Red

    explosion := particles.CreateExplosion(400, 300, config)
    ps.AddParticles(explosion)

    // Game loop
    lastTime := time.Now()
    for {
        now := time.Now()
        dt := now.Sub(lastTime).Seconds()
        lastTime = now

        // Update physics
        ps.Update(dt)

        // Render particles
        for _, p := range ps.Particles {
            if p.Life > 0 {
                // Draw particle at (p.X, p.Y) with color p.Color
                // Use your rendering library here
            }
        }

        // Clean up dead particles periodically
        if ps.ActiveCount() == 0 {
            break
        }

        time.Sleep(16 * time.Millisecond) // ~60 FPS
    }
}
```

### Example 2: Audio-Reactive Visualization

Create visualizations that respond to music:

```go
package main

import (
    "fmt"
    "time"
    "go-fireworks/pkg/audio"
)

func main() {
    // Create and start audio analyzer
    analyzer := audio.NewAnalyzer()
    if err := analyzer.Start(); err != nil {
        fmt.Println("Audio initialization failed:", err)
    }
    defer analyzer.Stop()

    // Check if audio is available
    if !analyzer.IsEnabled() {
        fmt.Println("Audio disabled:", analyzer.GetErrorMsg())
        fmt.Println("Running without audio reactivity...")
    }

    // Main loop
    ticker := time.NewTicker(20 * time.Millisecond)
    for range ticker.C {
        data := analyzer.GetData()

        // Use audio features for visualization
        if data.IsBeat {
            fmt.Printf("BEAT! Confidence: %.2f, BPM: %.1f\n",
                data.BeatConfidence, data.EstimatedBPM)
        }

        // Map audio features to visual properties
        brightness := data.SpectralCentroid // 0-1, warm to cool
        intensity := data.Energy            // 0-1, overall energy
        dynamics := data.SpectralFlux       // 0-1, rate of change

        // Use these values to control your visualization
        _ = brightness
        _ = intensity
        _ = dynamics
    }
}
```

### Example 3: Full Fireworks Show

Create a complete audio-reactive fireworks display:

```go
package main

import (
    "time"
    "go-fireworks/pkg/audio"
    "go-fireworks/pkg/fireworks"
    "go-fireworks/pkg/particles"
)

func main() {
    // Setup
    width, height := 800, 600

    // Create audio analyzer
    analyzer := audio.NewAnalyzer()
    analyzer.Start()
    defer analyzer.Stop()

    // Create fireworks show with custom config
    config := fireworks.DefaultConfig()
    config.Gravity = 2.5        // Stronger gravity
    config.MaxParticles = 50    // More particles per explosion
    config.BaseExplosion = 25.0 // Faster explosions

    show := fireworks.NewShowWithConfig(width, height, config)

    // Main loop
    ticker := time.NewTicker(20 * time.Millisecond)
    lastTime := time.Now()
    spawnTimer := time.Now()

    for range ticker.C {
        now := time.Now()
        dt := now.Sub(lastTime).Seconds()
        lastTime = now

        // Get audio data
        audioData := analyzer.GetData()
        hasAudio := analyzer.IsEnabled()

        // Spawn fireworks based on audio
        shouldSpawn, count := show.ShouldSpawn(audioData, hasAudio)
        particleCount := show.ActiveParticleCount()
        threshold := show.DynamicParticleThreshold(audioData, hasAudio)

        if particleCount < threshold {
            if shouldSpawn && hasAudio {
                for i := 0; i < count; i++ {
                    show.CreateFirework(audioData)
                }
            } else if now.Sub(spawnTimer).Seconds() > 1.5 {
                // Fallback: spawn periodically without audio
                show.CreateFirework(audioData)
                spawnTimer = now
            }
        }

        // Update physics
        show.Update(dt)

        // Render
        renderShow(show)
    }
}

func renderShow(show *fireworks.Show) {
    // Draw launching rockets
    for _, fw := range show.GetLaunchingRockets() {
        // Draw rocket at (fw.RocketX, fw.RocketY) with color fw.Color
    }

    // Draw particles
    for _, p := range show.GetAllParticles() {
        if p.Life > 0 {
            // Draw particle at (p.X, p.Y)
            // Fade based on p.Life (0-1)
        }
    }
}
```

### Example 4: Custom Explosion Types

Create and use different explosion patterns:

```go
package main

import (
    "math"
    "math/rand"
    "go-fireworks/pkg/particles"
)

func main() {
    ps := particles.NewParticleSystem(800, 600)

    // Radial explosion (symmetrical burst)
    radial := particles.ExplosionConfig{
        NumParticles: 30,
        Speed:        15.0,
        Type:         particles.ExplosionRadial,
        Color:        0xFF0000,
        Char:         '⠁',
    }
    ps.AddParticles(particles.CreateExplosion(200, 300, radial))

    // Directional explosion (cone-shaped)
    directional := particles.ExplosionConfig{
        NumParticles: 25,
        Speed:        20.0,
        Type:         particles.ExplosionDirectional,
        Direction:    math.Pi / 4, // 45 degrees
        Color:        0x00FF00,
        Char:         '⠁',
    }
    ps.AddParticles(particles.CreateExplosion(400, 300, directional))

    // Sideways explosion (horizontal split)
    sideways := particles.ExplosionConfig{
        NumParticles: 20,
        Speed:        25.0,
        Type:         particles.ExplosionSideways,
        Color:        0x0000FF,
        Char:         '⠁',
    }
    ps.AddParticles(particles.CreateExplosion(600, 300, sideways))

    // Spiral explosion (galaxy pattern)
    spiral := particles.ExplosionConfig{
        NumParticles: 40,
        Speed:        18.0,
        Type:         particles.ExplosionSpiral,
        Color:        0xFF00FF,
        Char:         '⠁',
    }
    ps.AddParticles(particles.CreateExplosion(400, 150, spiral))

    // Random explosion type
    random := particles.DefaultExplosionConfig()
    random.NumParticles = 35
    ps.AddParticles(particles.CreateRandomExplosion(400, 450, random))
}
```

### Example 5: Custom Physics Configuration

Fine-tune the physics simulation:

```go
package main

import "go-fireworks/pkg/particles"

func main() {
    // Create custom physics config
    config := particles.PhysicsConfig{
        Gravity:          3.0,   // Stronger gravity for faster fall
        Decay:            0.99,  // More air resistance
        CollisionRadius:  2.0,   // Larger collision detection
        Restitution:      0.9,   // Bouncier collisions
        LifeDecayRate:    0.02,  // Faster fade out
        GroundFadeHeight: 0.85,  // Start fading earlier
    }

    ps := particles.NewParticleSystemWithConfig(800, 600, config)

    // Use the particle system...
}
```

### Example 6: Integrating with tcell (Terminal UI)

Here's how the main application integrates with tcell:

```go
package main

import (
    "github.com/gdamore/tcell/v2"
    "go-fireworks/pkg/audio"
    "go-fireworks/pkg/fireworks"
    "go-fireworks/pkg/particles"
)

func main() {
    // Initialize tcell screen
    screen, _ := tcell.NewScreen()
    screen.Init()
    defer screen.Fini()

    width, height := screen.Size()

    // Initialize audio
    analyzer := audio.NewAnalyzer()
    analyzer.Start()
    defer analyzer.Stop()

    // Create fireworks show
    show := fireworks.NewShow(width, height)

    // Map tcell colors to particle colors
    tcellColors := []tcell.Color{
        tcell.ColorRed,
        tcell.ColorGreen,
        tcell.ColorBlue,
        tcell.ColorYellow,
    }
    particleColors := make([]particles.Color, len(tcellColors))
    for i, c := range tcellColors {
        particleColors[i] = particles.Color(c)
    }
    show.Colors = particleColors

    // Render loop
    for {
        screen.Clear()

        // Draw particles
        for _, p := range show.GetAllParticles() {
            if p.Life > 0 {
                x, y := int(p.X), int(p.Y)
                if x >= 0 && x < width && y >= 0 && y < height {
                    style := tcell.StyleDefault.
                        Foreground(tcell.Color(p.Color)).
                        Background(tcell.ColorBlack)

                    // Fade effect
                    if p.Life < 0.3 {
                        style = style.Dim(true)
                    }

                    screen.SetContent(x, y, p.Char, nil, style)
                }
            }
        }

        screen.Show()
    }
}
```

## Audio Feature Reference

The `audio.Data` struct provides these features:

| Feature | Range | Description | Visualization Use |
|---------|-------|-------------|-------------------|
| `Bass` | 0-1 | Low frequencies (20-250 Hz) | Size, scale |
| `Mid` | 0-1 | Mid frequencies (250-2000 Hz) | Motion speed |
| `High` | 0-1 | High frequencies (2000-20000 Hz) | Sparkle effects |
| `Volume` | 0-1 | Overall loudness (adaptive) | Brightness |
| `SpectralCentroid` | 0-1 | Brightness of sound | Color temperature |
| `SpectralFlux` | 0-1 | Rate of spectral change | Explosion speed |
| `SpectralRolloff` | 0-1 | High frequency content | Particle count |
| `OnsetStrength` | 0-1 | Sudden events | Trigger effects |
| `BeatStrength` | 0-1 | Beat intensity | Launch velocity |
| `Energy` | 0-1 | Overall energy | Spawn rate |
| `IsBeat` | bool | Beat detected | Sync events |
| `BeatConfidence` | 0-1 | Beat detection confidence | Effect intensity |
| `EstimatedBPM` | 60-200 | Tempo estimate | Animation timing |

## Requirements

- **Audio**: PulseAudio or PipeWire with PulseAudio compatibility
- **Go**: 1.21 or later
- **For terminal display**: A terminal supporting Unicode braille characters

## License

MIT License
