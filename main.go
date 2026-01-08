package main

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"time"

	"github.com/gdamore/tcell/v2"
)

// Particle represents a single firework particle
type Particle struct {
	x, y   float64
	vx, vy float64
	life   float64
	color  tcell.Color
	char   rune
}

// Firework represents a firework explosion
type Firework struct {
	// Launch phase
	rocketX, rocketY   float64
	rocketVX, rocketVY float64
	launching          bool
	targetY            float64

	// Explosion phase
	particles []Particle
	active    bool
	color     tcell.Color

	// Audio data for explosion
	audioData AudioData
}

// FireworkShow manages the entire fireworks display
type FireworkShow struct {
	screen       tcell.Screen
	fireworks    []Firework
	width        int
	height       int
	running      bool
	audio        *AudioAnalyzer
	lastBassPeak float64
	debugMode    bool
}

var colors = []tcell.Color{
	tcell.ColorRed,
	tcell.ColorGreen,
	tcell.ColorYellow,
	tcell.ColorBlue,
	tcell.ColorFuchsia,
	tcell.ColorAqua,
	tcell.ColorOrange,
	tcell.ColorPurple,
	tcell.ColorLime,
	tcell.ColorTeal,
	tcell.ColorOlive,
	tcell.ColorNavy,
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

	audio := NewAudioAnalyzer()
	audio.Start()

	return &FireworkShow{
		screen:    screen,
		fireworks: make([]Firework, 0),
		width:     width,
		height:    height,
		running:   true,
		audio:     audio,
	}, nil
}

func (fs *FireworkShow) createFirework(audioData AudioData) {
	// Launch from bottom of screen at random X position
	x := float64(rand.Intn(fs.width))
	startY := float64(fs.height - 1)

	// Target height in upper portion of screen
	targetY := float64(rand.Intn(fs.height/3) + fs.height/6)

	// Color selection influenced by VOLUME
	colorIndex := int(audioData.Volume * float64(len(colors)))
	if colorIndex >= len(colors) {
		colorIndex = len(colors) - 1
	}
	if colorIndex < 0 || audioData.Volume < 0.1 {
		colorIndex = rand.Intn(len(colors))
	}
	color := colors[colorIndex]

	// Launch velocity: randomized if no audio, or based on audio activity levels if present
	var launchSpeed float64
	if fs.audio.IsEnabled() && (audioData.Energy > 0.05 || audioData.Volume > 0.05) {
		// Audio is active: use energy and pitch to determine launch speed
		// Higher energy = faster launch (more activity in the music)
		baseLaunch := -8.0
		energyBoost := audioData.Energy * 8.0  // 0-8 additional speed from energy
		pitchBoost := audioData.Pitch * 4.0    // 0-4 additional speed from pitch
		launchSpeed = baseLaunch - energyBoost - pitchBoost
	} else {
		// No audio or very low activity: randomize launch speed
		launchSpeed = -6.0 - rand.Float64()*8.0 // Random between -6 and -14
	}

	fs.fireworks = append(fs.fireworks, Firework{
		rocketX:    x,
		rocketY:    startY,
		rocketVX:   0,
		rocketVY:   launchSpeed,
		launching:  true,
		targetY:    targetY,
		active:     true,
		color:      color,
		particles:  nil,
		audioData:  audioData,
	})
}

func (fs *FireworkShow) update(dt float64) {
	gravity := 2.0
	decay := 0.98

	for i := range fs.fireworks {
		if !fs.fireworks[i].active {
			continue
		}

		fw := &fs.fireworks[i]

		// Launch phase: rocket ascending
		if fw.launching {
			// Update rocket position
			fw.rocketY += fw.rocketVY * dt
			fw.rocketVY += gravity * dt // Gravity slows the rocket

			// Check if rocket should explode
			// Explode when: reached target height, or velocity becomes positive (started falling)
			if fw.rocketY <= fw.targetY || fw.rocketVY >= 0 {
				// Trigger explosion
				fs.explodeRocket(i)
				fw.launching = false
			}
			continue
		}

		// Explosion phase: particles spreading and falling
		activeParticles := 0
		for j := range fw.particles {
			p := &fw.particles[j]

			if p.life <= 0 {
				continue
			}

			// Update position
			p.x += p.vx * dt
			p.y += p.vy * dt

			// Apply gravity (particles fall)
			p.vy += gravity * dt

			// Apply air resistance
			p.vx *= decay
			p.vy *= decay

			// Fade out faster as particles fall and get closer to ground
			// Life decreases based on time and vertical position
			lifeDecay := dt * 0.3

			// Fade faster when falling (positive vy means falling down)
			if p.vy > 0 {
				lifeDecay += dt * 0.4
			}

			p.life -= lifeDecay

			if p.life > 0 {
				activeParticles++
			}
		}

		if activeParticles == 0 {
			fw.active = false
		}
	}

	// Check for particle collisions across all fireworks
	fs.handleParticleCollisions()

	// Remove inactive fireworks
	activeFireworks := make([]Firework, 0, len(fs.fireworks))
	for _, fw := range fs.fireworks {
		if fw.active {
			activeFireworks = append(activeFireworks, fw)
		}
	}
	fs.fireworks = activeFireworks
}

// handleParticleCollisions detects and resolves collisions between particles
func (fs *FireworkShow) handleParticleCollisions() {
	collisionRadius := 1.5 // Particles collide if within this distance
	restitution := 0.8     // Bounciness of collision (0=sticky, 1=perfectly elastic)

	// Collect all active particles with their firework indices
	type ParticleRef struct {
		fwIndex int
		pIndex  int
		p       *Particle
	}
	allParticles := make([]ParticleRef, 0, 100)

	for fwIdx := range fs.fireworks {
		if !fs.fireworks[fwIdx].active || fs.fireworks[fwIdx].launching {
			continue
		}
		for pIdx := range fs.fireworks[fwIdx].particles {
			p := &fs.fireworks[fwIdx].particles[pIdx]
			if p.life > 0 {
				allParticles = append(allParticles, ParticleRef{fwIdx, pIdx, p})
			}
		}
	}

	// Check all pairs for collisions
	for i := 0; i < len(allParticles); i++ {
		for j := i + 1; j < len(allParticles); j++ {
			p1 := allParticles[i].p
			p2 := allParticles[j].p

			// Calculate distance between particles
			dx := p2.x - p1.x
			dy := p2.y - p1.y
			distSq := dx*dx + dy*dy
			dist := math.Sqrt(distSq)

			// Check if particles are colliding
			if dist < collisionRadius && dist > 0.01 { // Avoid division by zero
				// Normalize direction vector
				nx := dx / dist
				ny := dy / dist

				// Calculate relative velocity
				dvx := p2.vx - p1.vx
				dvy := p2.vy - p1.vy
				dvn := dvx*nx + dvy*ny

				// Only resolve if particles are moving towards each other
				if dvn < 0 {
					// Apply elastic collision response
					// For simplicity, assume equal mass particles
					impulse := -(1 + restitution) * dvn / 2

					// Update velocities (repel particles)
					p1.vx -= impulse * nx
					p1.vy -= impulse * ny
					p2.vx += impulse * nx
					p2.vy += impulse * ny

					// Separate particles to prevent overlap
					overlap := collisionRadius - dist
					separationX := nx * overlap * 0.5
					separationY := ny * overlap * 0.5
					p1.x -= separationX
					p1.y -= separationY
					p2.x += separationX
					p2.y += separationY

					// Mini-explosion effect: create visual feedback
					// Brighten the particles briefly by resetting life slightly
					p1.life = math.Min(p1.life+0.1, 1.0)
					p2.life = math.Min(p2.life+0.1, 1.0)

					// Add some random perturbation for more chaotic effect
					perturbStrength := 0.5
					p1.vx += (rand.Float64()*2 - 1) * perturbStrength
					p1.vy += (rand.Float64()*2 - 1) * perturbStrength
					p2.vx += (rand.Float64()*2 - 1) * perturbStrength
					p2.vy += (rand.Float64()*2 - 1) * perturbStrength
				}
			}
		}
	}
}

func (fs *FireworkShow) explodeRocket(index int) {
	fw := &fs.fireworks[index]

	// Size (number of particles) influenced by ROLLING AVERAGE
	baseParticles := 30
	sizeBoost := int(fw.audioData.RollingAverage * 300)
	numParticles := baseParticles + sizeBoost

	// Cap particles for performance
	if numParticles > 400 {
		numParticles = 400
	}
	if numParticles < 20 {
		numParticles = 20
	}

	particles := make([]Particle, numParticles)

	// Explosion radial speed - particles burst outward
	// Use the rocket's upward velocity magnitude as the explosion force
	rocketSpeed := math.Sqrt(fw.rocketVX*fw.rocketVX + fw.rocketVY*fw.rocketVY)
	// Ensure minimum explosion force even if rocket was slow
	explosionSpeed := math.Max(rocketSpeed*0.8, 3.0)

	// Add some variation based on audio pitch
	explosionSpeed += fw.audioData.Pitch * 3.0

	for i := 0; i < numParticles; i++ {
		// Random angle in all directions (full circle)
		angle := rand.Float64() * 2 * math.Pi
		// Random speed variation for more natural explosion
		speed := explosionSpeed * (0.5 + rand.Float64()*0.8)

		// Calculate radial velocity components
		radialVX := math.Cos(angle) * speed
		radialVY := math.Sin(angle) * speed

		// INHERIT rocket's velocity and ADD radial explosion velocity
		// This makes particles arc away while maintaining the rocket's momentum
		particles[i] = Particle{
			x:     fw.rocketX,
			y:     fw.rocketY,
			vx:    fw.rocketVX + radialVX,
			vy:    fw.rocketVY + radialVY,
			life:  1.0,
			color: fw.color,
			char:  chars[rand.Intn(len(chars))],
		}
	}

	fw.particles = particles
}

func (fs *FireworkShow) render() {
	fs.screen.Clear()

	// Draw title
	title := "✨ FIREWORKS SHOW ✨ (Press ESC/Q to exit, D for debug)"
	style := tcell.StyleDefault.Foreground(tcell.ColorWhite).Background(tcell.ColorBlack)
	startX := (fs.width - len(title)) / 2
	for i, ch := range title {
		fs.screen.SetContent(startX+i, 0, ch, nil, style)
	}

	// Draw audio status
	if fs.audio.IsEnabled() {
		audioData := fs.audio.GetData()
		statusY := 1

		// Volume indicator (controls COLOR)
		volBar := createBar("VOL→COLOR", audioData.Volume, tcell.ColorYellow)
		fs.drawText(2, statusY, volBar, tcell.ColorYellow)

		// Pitch indicator (controls SPEED)
		pitchBar := createBar("PITCH→SPEED", audioData.Pitch, tcell.ColorAqua)
		fs.drawText(2, statusY+1, pitchBar, tcell.ColorAqua)

		// Rolling average indicator (controls SIZE)
		avgBar := createBar("AVG→SIZE", audioData.RollingAverage, tcell.ColorFuchsia)
		fs.drawText(2, statusY+2, avgBar, tcell.ColorFuchsia)

		// Energy indicator (controls DENSITY)
		energyBar := createBar("ENERGY→DENSITY", audioData.Energy, tcell.ColorGreen)
		fs.drawText(2, statusY+3, energyBar, tcell.ColorGreen)

		// Bass indicator (for triggering)
		bassBar := createBar("BASS", audioData.Bass, tcell.ColorRed)
		fs.drawText(2, statusY+4, bassBar, tcell.ColorRed)
	} else {
		// Center the audio disabled message below the title
		msg := "Audio: Disabled (press D for details)"
		msgX := (fs.width - len(msg)) / 2
		fs.drawText(msgX, 1, msg, tcell.ColorRed)
	}

	// Draw all fireworks
	for _, fw := range fs.fireworks {
		if !fw.active {
			continue
		}

		// Draw launching rocket
		if fw.launching {
			x := int(fw.rocketX)
			y := int(fw.rocketY)

			// Check bounds
			if x >= 0 && x < fs.width && y >= 6 && y < fs.height {
				style := tcell.StyleDefault.Foreground(fw.color).Background(tcell.ColorBlack)
				// Draw rocket as ascending trail
				fs.screen.SetContent(x, y, '▲', nil, style)
				if y+1 < fs.height {
					fs.screen.SetContent(x, y+1, '|', nil, style)
				}
			}
			continue
		}

		// Draw exploded particles
		for _, p := range fw.particles {
			if p.life <= 0 {
				continue
			}

			x := int(p.x)
			y := int(p.y)

			// Check bounds
			if x < 0 || x >= fs.width || y < 6 || y >= fs.height {
				continue
			}

			// Calculate brightness based on life
			style := tcell.StyleDefault.Foreground(p.color).Background(tcell.ColorBlack)

			// Gradual fading based on life
			if p.life < 0.2 {
				style = tcell.StyleDefault.Foreground(tcell.ColorGray).Background(tcell.ColorBlack)
			} else if p.life < 0.5 {
				// Use a dimmer version of the color for mid-fade
				style = tcell.StyleDefault.Foreground(p.color).Background(tcell.ColorBlack).Dim(true)
			}

			fs.screen.SetContent(x, y, p.char, nil, style)
		}
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
	fs.drawText(startX, y, "  Pitch:     "+formatFloat(audioData.Pitch), tcell.ColorWhite)
	y++
	fs.drawText(startX, y, "  RollingAvg:"+formatFloat(audioData.RollingAverage), tcell.ColorWhite)
	y++
	fs.drawText(startX, y, "  Energy:    "+formatFloat(audioData.Energy), tcell.ColorWhite)
	y++
	fs.drawText(startX, y, "  Bass:      "+formatFloat(audioData.Bass), tcell.ColorWhite)
	y++
	fs.drawText(startX, y, "  LastPeak:  "+formatFloat(fs.lastBassPeak), tcell.ColorWhite)

	// Firework statistics
	y += 2
	fs.drawText(startX, y, "FIREWORK STATS:", tcell.ColorGreen)
	y++

	totalFireworks := len(fs.fireworks)
	launching := 0
	exploded := 0
	totalParticles := 0

	for _, fw := range fs.fireworks {
		if !fw.active {
			continue
		}
		if fw.launching {
			launching++
		} else {
			exploded++
			for _, p := range fw.particles {
				if p.life > 0 {
					totalParticles++
				}
			}
		}
	}

	fs.drawText(startX, y, "  Total:     "+formatInt(totalFireworks), tcell.ColorWhite)
	y++
	fs.drawText(startX, y, "  Launching: "+formatInt(launching), tcell.ColorWhite)
	y++
	fs.drawText(startX, y, "  Exploded:  "+formatInt(exploded), tcell.ColorWhite)
	y++
	fs.drawText(startX, y, "  Particles: "+formatInt(totalParticles), tcell.ColorWhite)

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

func createBar(label string, value float64, color tcell.Color) string {
	barLength := 20
	filled := int(value * float64(barLength))
	if filled > barLength {
		filled = barLength
	}
	bar := ""
	for i := 0; i < filled; i++ {
		bar += "█"
	}
	for i := filled; i < barLength; i++ {
		bar += "░"
	}
	return label + ": " + bar
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
		case *tcell.EventResize:
			fs.width, fs.height = fs.screen.Size()
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
	bassPeakThreshold := 0.4

	for fs.running {
		select {
		case <-ticker.C:
			now := time.Now()
			dt := now.Sub(lastUpdate).Seconds()
			lastUpdate = now

			// Get audio data
			audioData := fs.audio.GetData()

			// Spawn new fireworks based on audio
			shouldSpawnAudio := false
			if fs.audio.IsEnabled() {
				// Detect bass peaks for triggering fireworks
				if audioData.Bass > bassPeakThreshold && audioData.Bass > fs.lastBassPeak*1.3 {
					shouldSpawnAudio = true
					fs.lastBassPeak = audioData.Bass
				} else {
					fs.lastBassPeak = audioData.Bass
				}

				// Also spawn on high volume
				if audioData.Volume > 0.7 && rand.Float64() < 0.3 {
					shouldSpawnAudio = true
				}

				// Energy controls particle density (spawn rate)
				// Higher energy = more frequent spawning = more particles on screen
				energySpawnChance := audioData.Energy * 0.4 // 0-40% chance per tick
				if rand.Float64() < energySpawnChance {
					shouldSpawnAudio = true
				}
			}

			// Spawn fireworks
			if shouldSpawnAudio {
				fs.createFirework(audioData)
			} else if now.Sub(fireworkTimer).Seconds() > fireworkInterval {
				// Fallback: spawn random fireworks if no audio or low activity
				fs.createFirework(audioData)
				fireworkTimer = now
				fireworkInterval = 0.5 + rand.Float64()*2.0
			}

			// Update physics
			fs.update(dt)

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
