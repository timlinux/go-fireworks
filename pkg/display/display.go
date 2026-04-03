// Package display provides a high-level API for embedding audio-reactive
// fireworks into any Go terminal application using tcell.
//
// Basic usage:
//
//	display, _ := display.New(screen)
//	defer display.Stop()
//	display.Start()
//
// Or for manual control:
//
//	display, _ := display.New(screen)
//	for {
//	    display.Update(dt)
//	    display.Render()
//	}
package display

import (
	"math/rand"
	"time"

	"github.com/gdamore/tcell/v2"

	"go-fireworks/pkg/audio"
	"go-fireworks/pkg/fireworks"
	"go-fireworks/pkg/particles"
)

// Display provides a complete audio-reactive fireworks visualization
// that can be embedded into any tcell application.
type Display struct {
	screen  tcell.Screen
	show    *fireworks.Show
	audio   *audio.Analyzer
	running bool
	config  Config

	// Timing
	lastUpdate     time.Time
	spawnTimer     time.Time
	spawnInterval  float64

	// Rendering bounds
	minX, minY int
	maxX, maxY int
}

// Config controls the display behavior
type Config struct {
	// Bounds (0 = use full screen)
	MinX, MinY int
	MaxX, MaxY int

	// Show title bar
	ShowTitle bool
	Title     string

	// Audio visualization overlay
	ShowAudioViz bool

	// Spawn rate when no audio (seconds between spawns)
	FallbackSpawnRate float64

	// Custom colors (nil = use defaults)
	Colors []tcell.Color

	// Fireworks config (nil = use defaults)
	FireworksConfig *fireworks.Config
}

// DefaultConfig returns sensible defaults
func DefaultConfig() Config {
	return Config{
		ShowTitle:         false,
		Title:             "Fireworks",
		ShowAudioViz:      false,
		FallbackSpawnRate: 1.5,
		Colors:            nil,
		FireworksConfig:   nil,
	}
}

// New creates a new fireworks display attached to the given screen
func New(screen tcell.Screen) (*Display, error) {
	return NewWithConfig(screen, DefaultConfig())
}

// NewWithConfig creates a display with custom configuration
func NewWithConfig(screen tcell.Screen, config Config) (*Display, error) {
	width, height := screen.Size()

	// Set bounds
	minX, minY := config.MinX, config.MinY
	maxX, maxY := config.MaxX, config.MaxY
	if maxX == 0 {
		maxX = width
	}
	if maxY == 0 {
		maxY = height
	}

	// Create fireworks show
	var show *fireworks.Show
	if config.FireworksConfig != nil {
		show = fireworks.NewShowWithConfig(maxX-minX, maxY-minY, *config.FireworksConfig)
	} else {
		show = fireworks.NewShow(maxX-minX, maxY-minY)
	}

	// Set colors
	if config.Colors != nil {
		particleColors := make([]particles.Color, len(config.Colors))
		for i, c := range config.Colors {
			particleColors[i] = particles.Color(c)
		}
		show.Colors = particleColors
	} else {
		show.Colors = defaultColors()
	}

	// Create audio analyzer
	analyzer := audio.NewAnalyzer()
	analyzer.Start()

	return &Display{
		screen:        screen,
		show:          show,
		audio:         analyzer,
		config:        config,
		minX:          minX,
		minY:          minY,
		maxX:          maxX,
		maxY:          maxY,
		lastUpdate:    time.Now(),
		spawnTimer:    time.Now(),
		spawnInterval: config.FallbackSpawnRate,
	}, nil
}

// Start runs the display in a background goroutine at 50fps.
// Call Stop() to terminate.
func (d *Display) Start() {
	d.running = true
	go func() {
		ticker := time.NewTicker(20 * time.Millisecond)
		defer ticker.Stop()

		for d.running {
			<-ticker.C
			d.Update(0) // Auto-calculate dt
			d.screen.Clear() // Clear screen before rendering to prevent particle trails
			d.Render()
			d.screen.Show()
		}
	}()
}

// Stop terminates the background display loop and cleans up audio
func (d *Display) Stop() {
	d.running = false
	d.audio.Stop()
}

// Update advances the simulation. Pass 0 for dt to auto-calculate.
func (d *Display) Update(dt float64) {
	now := time.Now()
	if dt == 0 {
		dt = now.Sub(d.lastUpdate).Seconds()
	}
	d.lastUpdate = now

	// Get audio data
	audioData := d.audio.GetData()
	hasAudio := d.audio.IsEnabled()

	// Spawn logic
	shouldSpawn, count := d.show.ShouldSpawn(audioData, hasAudio)
	particleCount := d.show.ActiveParticleCount()
	threshold := d.show.DynamicParticleThreshold(audioData, hasAudio)

	canSpawn := particleCount < threshold
	if audioData.IsBeat && audioData.BeatConfidence > 0.6 {
		canSpawn = true
	}

	if canSpawn {
		if shouldSpawn && hasAudio {
			for i := 0; i < count; i++ {
				d.show.CreateFirework(audioData)
			}
		} else if now.Sub(d.spawnTimer).Seconds() > d.spawnInterval {
			d.show.CreateFirework(audioData)
			d.spawnTimer = now
			d.spawnInterval = d.config.FallbackSpawnRate * (0.5 + rand.Float64())
		}
	}

	// Update physics
	d.show.Update(dt)
}

// Render draws the current state to the screen (does not call Show)
func (d *Display) Render() {
	d.renderToRegion(d.minX, d.minY, d.maxX, d.maxY)
}

// RenderToRegion draws to a specific screen region
func (d *Display) RenderToRegion(minX, minY, maxX, maxY int) {
	d.renderToRegion(minX, minY, maxX, maxY)
}

func (d *Display) renderToRegion(minX, minY, maxX, maxY int) {
	width := maxX - minX
	titleOffset := 0

	// Draw title if enabled
	if d.config.ShowTitle && d.config.Title != "" {
		titleOffset = 1
		style := tcell.StyleDefault.Foreground(tcell.ColorWhite).Background(tcell.ColorBlack)
		title := d.config.Title
		startX := minX + (width-len(title))/2
		for i, ch := range title {
			if startX+i >= minX && startX+i < maxX {
				d.screen.SetContent(startX+i, minY, ch, nil, style)
			}
		}

		// Music indicator
		if d.audio.IsEnabled() {
			audioData := d.audio.GetData()
			if audioData.Volume > 0.05 || audioData.Energy > 0.05 {
				noteStyle := tcell.StyleDefault.Foreground(tcell.ColorGreen).Background(tcell.ColorBlack)
				noteX := startX + len(title) + 2
				if noteX < maxX {
					d.screen.SetContent(noteX, minY, '♪', nil, noteStyle)
				}
			}
		}
	}

	// Draw launching rockets
	for _, fw := range d.show.GetLaunchingRockets() {
		x := minX + int(fw.RocketX)
		y := minY + titleOffset + int(fw.RocketY)

		if x >= minX && x < maxX && y >= minY+titleOffset && y < maxY {
			style := tcell.StyleDefault.Foreground(tcell.Color(fw.Color)).Background(tcell.ColorBlack)
			d.screen.SetContent(x, y, '●', nil, style)
		}
	}

	// Draw particles
	for _, p := range d.show.GetAllParticles() {
		x := minX + int(p.X)
		y := minY + titleOffset + int(p.Y)

		if x < minX || x >= maxX || y < minY+titleOffset || y >= maxY {
			continue
		}

		style := tcell.StyleDefault.Foreground(tcell.Color(p.Color)).Background(tcell.ColorBlack)
		if p.Life < 0.2 {
			style = tcell.StyleDefault.Foreground(tcell.ColorGray).Background(tcell.ColorBlack)
		} else if p.Life < 0.5 {
			style = style.Dim(true)
		}

		d.screen.SetContent(x, y, p.Char, nil, style)
	}
}

// SetBounds updates the display region
func (d *Display) SetBounds(minX, minY, maxX, maxY int) {
	d.minX, d.minY = minX, minY
	d.maxX, d.maxY = maxX, maxY
	d.show.SetBounds(maxX-minX, maxY-minY)
}

// IsAudioEnabled returns whether audio reactivity is active
func (d *Display) IsAudioEnabled() bool {
	return d.audio.IsEnabled()
}

// GetAudioData returns current audio analysis data
func (d *Display) GetAudioData() audio.Data {
	return d.audio.GetData()
}

// GetShow returns the underlying fireworks show for advanced control
func (d *Display) GetShow() *fireworks.Show {
	return d.show
}

// TriggerFirework manually spawns a firework
func (d *Display) TriggerFirework() {
	d.show.CreateFirework(d.audio.GetData())
}

// TriggerFireworks manually spawns multiple fireworks
func (d *Display) TriggerFireworks(count int) {
	audioData := d.audio.GetData()
	for i := 0; i < count; i++ {
		d.show.CreateFirework(audioData)
	}
}

// Clear removes all active fireworks
func (d *Display) Clear() {
	d.show.Fireworks = nil
}

func defaultColors() []particles.Color {
	tcellColors := []tcell.Color{
		tcell.ColorRed,
		tcell.ColorOrangeRed,
		tcell.ColorDarkOrange,
		tcell.ColorOrange,
		tcell.ColorGold,
		tcell.ColorYellow,
		tcell.ColorGreenYellow,
		tcell.ColorLime,
		tcell.ColorSpringGreen,
		tcell.ColorGreen,
		tcell.ColorAqua,
		tcell.ColorDeepSkyBlue,
		tcell.ColorDodgerBlue,
		tcell.ColorBlue,
		tcell.ColorBlueViolet,
		tcell.ColorPurple,
		tcell.ColorDarkViolet,
		tcell.ColorFuchsia,
		tcell.ColorDeepPink,
		tcell.ColorHotPink,
		tcell.ColorPink,
		tcell.ColorCrimson,
	}
	result := make([]particles.Color, len(tcellColors))
	for i, c := range tcellColors {
		result[i] = particles.Color(c)
	}
	return result
}
