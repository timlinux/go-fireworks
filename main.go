package main

import (
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/gdamore/tcell/v2"

	"tim-particles/pkg/audio"
	"tim-particles/pkg/fireworks"
	"tim-particles/pkg/particles"
)

// FireworkShow manages the terminal-based fireworks display
type FireworkShow struct {
	screen       tcell.Screen
	show         *fireworks.Show
	width        int
	height       int
	running      bool
	audio        *audio.Analyzer
	lastBassPeak float64
	debugMode    bool
	showAudioViz bool
}

var colors = []tcell.Color{
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

var chars = []rune{
	'⠁', '⠂', '⠄', '⠈', '⠐', '⠠', // sparse patterns
	'⠃', '⠅', '⠉', '⠑', '⠡', '⠢', // light patterns
	'⠆', '⠊', '⠒', '⠔', '⠤', '⠨', // medium patterns
	'⠇', '⠋', '⠓', '⠕', '⠥', '⠩', // medium-dense patterns
	'⠿', '⡿', '⣿', '⢿', '⣾', '⣷', // dense patterns
	'⣯', '⣟', '⣻', '⣽', '⣺', '⣳', // varied dense patterns
}

func NewFireworkShow() (*FireworkShow, error) {
	screen, err := tcell.NewScreen()
	if err != nil {
		return nil, err
	}

	if err := screen.Init(); err != nil {
		return nil, err
	}

	width, height := screen.Size()
	screen.SetStyle(tcell.StyleDefault.Background(tcell.ColorBlack))
	screen.Clear()

	audioAnalyzer := audio.NewAnalyzer()
	audioAnalyzer.Start()

	// Create fireworks show with tcell-compatible colors
	show := fireworks.NewShow(width, height)
	tcellColors := make([]particles.Color, len(colors))
	for i, c := range colors {
		tcellColors[i] = particles.Color(c)
	}
	show.Colors = tcellColors

	return &FireworkShow{
		screen:       screen,
		show:         show,
		width:        width,
		height:       height,
		running:      true,
		audio:        audioAnalyzer,
		showAudioViz: false,
	}, nil
}

// drawAudioTable draws a bordered table with audio visualization
func (fs *FireworkShow) drawAudioTable() {
	audioData := fs.audio.GetData()

	// Table configuration
	labelWidth := 16
	barWidth := 22
	tableWidth := labelWidth + barWidth + 3 // +3 for borders and separator
	startX := (fs.width - tableWidth) / 2
	startY := 2 // One line of whitespace after title (title is at y=0)

	// Box drawing characters
	topLeft := '┌'
	topRight := '┐'
	bottomLeft := '└'
	bottomRight := '┘'
	horizontal := '─'
	vertical := '│'
	cross := '┼'
	tLeft := '├'
	tRight := '┤'

	borderStyle := tcell.StyleDefault.Foreground(tcell.ColorWhite).Background(tcell.ColorBlack)

	// Table data: label, value, color
	rows := []struct {
		label string
		value float64
		color tcell.Color
	}{
		{"CENTROID:COLOR ", audioData.SpectralCentroid, tcell.ColorYellow},
		{"FLUX:EXPLOSION ", audioData.SpectralFlux, tcell.ColorAqua},
		{"ROLLOFF:SIZE   ", audioData.SpectralRolloff, tcell.ColorFuchsia},
		{"ONSET:TRIGGER  ", audioData.OnsetStrength, tcell.ColorGreen},
		{"BEAT:LAUNCH    ", audioData.BeatStrength, tcell.ColorRed},
	}

	// Draw top border
	fs.screen.SetContent(startX, startY, topLeft, nil, borderStyle)
	for i := 0; i < labelWidth; i++ {
		fs.screen.SetContent(startX+1+i, startY, horizontal, nil, borderStyle)
	}
	fs.screen.SetContent(startX+1+labelWidth, startY, cross, nil, borderStyle)
	for i := 0; i < barWidth; i++ {
		fs.screen.SetContent(startX+2+labelWidth+i, startY, horizontal, nil, borderStyle)
	}
	fs.screen.SetContent(startX+tableWidth-1, startY, topRight, nil, borderStyle)

	// Draw rows
	currentY := startY + 1
	for idx, row := range rows {
		// Draw left border
		fs.screen.SetContent(startX, currentY, vertical, nil, borderStyle)

		// Draw label (left-padded)
		labelStyle := tcell.StyleDefault.Foreground(row.color).Background(tcell.ColorBlack)
		for i := 0; i < labelWidth; i++ {
			if i < len(row.label) {
				fs.screen.SetContent(startX+1+i, currentY, rune(row.label[i]), nil, labelStyle)
			} else {
				fs.screen.SetContent(startX+1+i, currentY, ' ', nil, borderStyle)
			}
		}

		// Draw separator
		fs.screen.SetContent(startX+1+labelWidth, currentY, vertical, nil, borderStyle)

		// Draw bar
		barStyle := tcell.StyleDefault.Foreground(row.color).Background(tcell.ColorBlack)
		filled := int(row.value * float64(barWidth))
		if filled > barWidth {
			filled = barWidth
		}
		for i := 0; i < barWidth; i++ {
			var ch rune
			if i < filled {
				ch = '█'
			} else {
				ch = '░'
			}
			fs.screen.SetContent(startX+2+labelWidth+i, currentY, ch, nil, barStyle)
		}

		// Draw right border
		fs.screen.SetContent(startX+tableWidth-1, currentY, vertical, nil, borderStyle)

		currentY++

		// Draw separator line between rows (except after last row)
		if idx < len(rows)-1 {
			fs.screen.SetContent(startX, currentY, tLeft, nil, borderStyle)
			for i := 0; i < labelWidth; i++ {
				fs.screen.SetContent(startX+1+i, currentY, horizontal, nil, borderStyle)
			}
			fs.screen.SetContent(startX+1+labelWidth, currentY, cross, nil, borderStyle)
			for i := 0; i < barWidth; i++ {
				fs.screen.SetContent(startX+2+labelWidth+i, currentY, horizontal, nil, borderStyle)
			}
			fs.screen.SetContent(startX+tableWidth-1, currentY, tRight, nil, borderStyle)
			currentY++
		}
	}

	// Draw bottom border
	fs.screen.SetContent(startX, currentY, bottomLeft, nil, borderStyle)
	for i := 0; i < labelWidth; i++ {
		fs.screen.SetContent(startX+1+i, currentY, horizontal, nil, borderStyle)
	}
	fs.screen.SetContent(startX+1+labelWidth, currentY, cross, nil, borderStyle)
	for i := 0; i < barWidth; i++ {
		fs.screen.SetContent(startX+2+labelWidth+i, currentY, horizontal, nil, borderStyle)
	}
	fs.screen.SetContent(startX+tableWidth-1, currentY, bottomRight, nil, borderStyle)
}

func (fs *FireworkShow) render() {
	fs.screen.Clear()

	// Draw title
	title := "✨ FIREWORKS SHOW ✨ (ESC/Q: exit, D: debug, V: audio viz)"
	style := tcell.StyleDefault.Foreground(tcell.ColorWhite).Background(tcell.ColorBlack)
	startX := (fs.width - len(title)) / 2
	for i, ch := range title {
		fs.screen.SetContent(startX+i, 0, ch, nil, style)
	}

	// Draw music note indicator when audio syncing
	if fs.audio.IsEnabled() {
		audioData := fs.audio.GetData()
		// Show music note if there's any significant audio activity
		if audioData.Volume > 0.05 || audioData.Energy > 0.05 {
			noteStyle := tcell.StyleDefault.Foreground(tcell.ColorGreen).Background(tcell.ColorBlack)
			// Place music note at the end of the title
			noteX := startX + len(title) + 2
			fs.screen.SetContent(noteX, 0, '♪', nil, noteStyle)
		}
	}

	// Draw audio visualization table if enabled
	if fs.showAudioViz && fs.audio.IsEnabled() {
		fs.drawAudioTable()
	} else if !fs.audio.IsEnabled() {
		// Center the audio disabled message below the title with whitespace
		msg := "Audio: Disabled (press D for details)"
		msgX := (fs.width - len(msg)) / 2
		fs.drawText(msgX, 2, msg, tcell.ColorRed)
	}

	// Draw launching rockets
	for _, fw := range fs.show.GetLaunchingRockets() {
		x := int(fw.RocketX)
		y := int(fw.RocketY)

		if x >= 0 && x < fs.width && y >= 6 && y < fs.height {
			style := tcell.StyleDefault.Foreground(tcell.Color(fw.Color)).Background(tcell.ColorBlack)
			fs.screen.SetContent(x, y, '●', nil, style)
		}
	}

	// Draw all particles
	for _, p := range fs.show.GetAllParticles() {
		x := int(p.X)
		y := int(p.Y)

		if x < 0 || x >= fs.width || y < 6 || y >= fs.height {
			continue
		}

		style := tcell.StyleDefault.Foreground(tcell.Color(p.Color)).Background(tcell.ColorBlack)

		if p.Life < 0.2 {
			style = tcell.StyleDefault.Foreground(tcell.ColorGray).Background(tcell.ColorBlack)
		} else if p.Life < 0.5 {
			style = tcell.StyleDefault.Foreground(tcell.Color(p.Color)).Background(tcell.ColorBlack).Dim(true)
		}

		fs.screen.SetContent(x, y, p.Char, nil, style)
	}

	// Draw debug info
	if fs.debugMode {
		fs.renderDebugInfo()
	}

	fs.screen.Show()
}

func (fs *FireworkShow) drawText(x, y int, text string, color tcell.Color) {
	style := tcell.StyleDefault.Foreground(color).Background(tcell.ColorBlack)
	for i, ch := range text {
		fs.screen.SetContent(x+i, y, ch, nil, style)
	}
}

func (fs *FireworkShow) renderDebugInfo() {
	// Position debug info on the right side of screen
	startX := fs.width - 45
	if startX < 0 {
		startX = 0
	}
	startY := 1

	// Header
	fs.drawText(startX, startY, "=== DEBUG MODE (Press D to hide) ===", tcell.ColorWhite)

	y := startY + 2

	// Audio status
	pacatPath, device := fs.audio.GetDebugInfo()
	if fs.audio.IsEnabled() {
		fs.drawText(startX, y, "AUDIO: Enabled", tcell.ColorGreen)
		y++
		if pacatPath != "" {
			fs.drawText(startX, y, "  pacat: "+pacatPath, tcell.ColorGray)
			y++
		}
		if device != "" {
			fs.drawText(startX, y, "  device: "+device, tcell.ColorGray)
		}
	} else {
		fs.drawText(startX, y, "AUDIO: Disabled", tcell.ColorRed)
		y++
		if pacatPath != "" {
			fs.drawText(startX, y, "  pacat: "+pacatPath, tcell.ColorGray)
			y++
		}
		errorMsg := fs.audio.GetErrorMsg()
		if errorMsg != "" {
			// Word wrap the error message
			fs.drawText(startX, y, "  Error:", tcell.ColorRed)
			y++
			maxWidth := 40
			for len(errorMsg) > 0 {
				chunk := errorMsg
				if len(chunk) > maxWidth {
					chunk = chunk[:maxWidth]
				}
				fs.drawText(startX+4, y, chunk, tcell.ColorRed)
				y++
				if len(errorMsg) > maxWidth {
					errorMsg = errorMsg[maxWidth:]
				} else {
					break
				}
			}
			y-- // Adjust back one line
		}
	}

	audioData := fs.audio.GetData()

	// Audio data with precise values
	y += 2
	fs.drawText(startX, y, "AUDIO DATA:", tcell.ColorYellow)
	y++
	fs.drawText(startX, y, "  Volume:    "+formatFloat(audioData.Volume), tcell.ColorWhite)
	y++
	fs.drawText(startX, y, "  Centroid:  "+formatFloat(audioData.SpectralCentroid), tcell.ColorWhite)
	y++
	fs.drawText(startX, y, "  Flux:      "+formatFloat(audioData.SpectralFlux), tcell.ColorWhite)
	y++
	fs.drawText(startX, y, "  Rolloff:   "+formatFloat(audioData.SpectralRolloff), tcell.ColorWhite)
	y++
	fs.drawText(startX, y, "  Onset:     "+formatFloat(audioData.OnsetStrength), tcell.ColorWhite)
	y++
	fs.drawText(startX, y, "  Beat:      "+formatFloat(audioData.BeatStrength), tcell.ColorWhite)
	y++
	fs.drawText(startX, y, "  Energy:    "+formatFloat(audioData.Energy), tcell.ColorWhite)
	y++

	// Beat synchronization info
	beatIndicator := "NO"
	beatColor := tcell.ColorRed
	if audioData.IsBeat {
		beatIndicator = "YES"
		beatColor = tcell.ColorGreen
	}
	fs.drawText(startX, y, "  IsBeat:    ", tcell.ColorWhite)
	fs.drawText(startX+13, y, beatIndicator, beatColor)
	y++
	fs.drawText(startX, y, "  BeatConf:  "+formatFloat(audioData.BeatConfidence), tcell.ColorWhite)
	y++
	fs.drawText(startX, y, "  BPM:       "+fmt.Sprintf("%.1f", audioData.EstimatedBPM), tcell.ColorAqua)

	// Firework statistics
	y += 2
	fs.drawText(startX, y, "FIREWORK STATS:", tcell.ColorGreen)
	y++

	totalFireworks := len(fs.show.Fireworks)
	launching := len(fs.show.GetLaunchingRockets())
	exploded := totalFireworks - launching
	totalParticles := fs.show.ActiveParticleCount()

	fs.drawText(startX, y, "  Total:     "+formatInt(totalFireworks), tcell.ColorWhite)
	y++
	fs.drawText(startX, y, "  Launching: "+formatInt(launching), tcell.ColorWhite)
	y++
	fs.drawText(startX, y, "  Exploded:  "+formatInt(exploded), tcell.ColorWhite)
	y++
	fs.drawText(startX, y, "  Particles: "+formatInt(totalParticles), tcell.ColorWhite)

	// Explosion type statistics
	y += 2
	totalExplosions := fs.show.TotalExplosions()
	fs.drawText(startX, y, "EXPLOSION TYPES:", tcell.ColorFuchsia)
	y++
	fs.drawText(startX, y, "  Total:       "+formatInt(totalExplosions), tcell.ColorWhite)
	y++
	fs.drawText(startX, y, "  Radial:      "+formatInt(fs.show.ExplosionCountRadial), tcell.ColorWhite)
	y++
	fs.drawText(startX, y, "  Directional: "+formatInt(fs.show.ExplosionCountDirectional), tcell.ColorWhite)
	y++
	fs.drawText(startX, y, "  Sideways:    "+formatInt(fs.show.ExplosionCountSideways), tcell.ColorWhite)
	y++
	fs.drawText(startX, y, "  Spiral:      "+formatInt(fs.show.ExplosionCountSpiral), tcell.ColorWhite)

	// Effects mapping
	y += 2
	fs.drawText(startX, y, "EFFECTS MAPPING:", tcell.ColorAqua)
	y++
	fs.drawText(startX, y, "  Volume    → Color", tcell.ColorWhite)
	y++
	fs.drawText(startX, y, "  Pitch     → Speed", tcell.ColorWhite)
	y++
	fs.drawText(startX, y, "  RollingAvg→ Size", tcell.ColorWhite)
	y++
	fs.drawText(startX, y, "  Energy    → Density", tcell.ColorWhite)
}

func formatFloat(val float64) string {
	return fmt.Sprintf("%.4f", val)
}

func formatInt(val int) string {
	return fmt.Sprintf("%d", val)
}

func (fs *FireworkShow) handleInput() {
	for {
		ev := fs.screen.PollEvent()
		switch ev := ev.(type) {
		case *tcell.EventKey:
			if ev.Key() == tcell.KeyEscape || ev.Rune() == 'q' || ev.Rune() == 'Q' {
				fs.running = false
				return
			}
			if ev.Rune() == 'd' || ev.Rune() == 'D' {
				fs.debugMode = !fs.debugMode
			}
			if ev.Rune() == 'v' || ev.Rune() == 'V' {
				fs.showAudioViz = !fs.showAudioViz
			}
		case *tcell.EventResize:
			fs.width, fs.height = fs.screen.Size()
			fs.show.SetBounds(fs.width, fs.height)
			fs.screen.Sync()
		}
	}
}

func (fs *FireworkShow) Run() {
	defer fs.screen.Fini()
	defer fs.audio.Stop()

	// Handle input in separate goroutine
	go fs.handleInput()

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	fireworkTimer := time.Now()
	fireworkInterval := 1.0 + rand.Float64()*2.0

	lastUpdate := time.Now()

	for fs.running {
		select {
		case <-ticker.C:
			now := time.Now()
			dt := now.Sub(lastUpdate).Seconds()
			lastUpdate = now

			// Get audio data
			audioData := fs.audio.GetData()
			hasAudio := fs.audio.IsEnabled()

			// Determine if we should spawn fireworks
			shouldSpawn, spawnCount := fs.show.ShouldSpawn(audioData, hasAudio)

			// Check particle threshold
			particleCount := fs.show.ActiveParticleCount()
			threshold := fs.show.DynamicParticleThreshold(audioData, hasAudio)

			canSpawn := particleCount < threshold

			// Override for high confidence beats
			if audioData.IsBeat && audioData.BeatConfidence > 0.6 {
				canSpawn = true
			}

			if canSpawn {
				if shouldSpawn && hasAudio {
					for i := 0; i < spawnCount; i++ {
						fs.show.CreateFirework(audioData)
					}
				} else if now.Sub(fireworkTimer).Seconds() > fireworkInterval {
					// Fallback random spawning
					fs.show.CreateFirework(audioData)
					fireworkTimer = now
					fireworkInterval = 0.5 + rand.Float64()*2.0
				}
			}

			fs.lastBassPeak = audioData.Bass

			// Update physics
			fs.show.Update(dt)

			// Render
			fs.render()
		}
	}
}

func main() {
	rand.Seed(time.Now().UnixNano())

	show, err := NewFireworkShow()
	if err != nil {
		panic(err)
	}

	show.Run()
	os.Exit(0)
}
