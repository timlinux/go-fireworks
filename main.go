package main

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"time"

	"github.com/gdamore/tcell/v2"
)

// ExplosionType defines different explosion patterns
type ExplosionType int

const (
	ExplosionRadial ExplosionType = iota
	ExplosionDirectional
	ExplosionSideways
	ExplosionSpiral
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
	particles     []Particle
	active        bool
	color         tcell.Color
	explosionType ExplosionType
	direction     float64 // For directional explosions (radians)

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
	showAudioViz bool
	// Explosion type counters
	explosionCountRadial      int
	explosionCountDirectional int
	explosionCountSideways    int
	explosionCountSpiral      int
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

	audio := NewAudioAnalyzer()
	audio.Start()

	return &FireworkShow{
		screen:       screen,
		fireworks:    make([]Firework, 0),
		width:        width,
		height:       height,
		running:      true,
		audio:        audio,
		showAudioViz: false,
	}, nil
}

func (fs *FireworkShow) createFirework(audioData AudioData) {
	// Launch from bottom of screen - position depends on launch angle
	startY := float64(fs.height - 1)

	// Randomized target height - fuse time varies so explosions happen at different altitudes
	// Range from 20% to 80% of screen height for variety
	minHeight := fs.height / 5         // 20% from top
	maxHeight := fs.height * 4 / 5     // 80% from top
	targetY := float64(rand.Intn(maxHeight-minHeight) + minHeight)

	// Color selection influenced by SPECTRAL CENTROID (brightness of sound)
	// Low centroid = warm colors (red, orange), High centroid = cool colors (blue, cyan)
	colorIndex := 0
	if fs.audio.IsEnabled() && audioData.SpectralCentroid > 0.05 {
		// Map centroid to color palette
		// 0.0-0.3 = warm (red/orange) indices 0-6
		// 0.3-0.7 = mid (green/yellow) indices 7-14
		// 0.7-1.0 = cool (blue/purple) indices 15-21
		colorIndex = int(audioData.SpectralCentroid * float64(len(colors)))
		if colorIndex >= len(colors) {
			colorIndex = len(colors) - 1
		}
	} else {
		colorIndex = rand.Intn(len(colors))
	}
	color := colors[colorIndex]

	// Launch velocity magnitude: influenced by BEAT STRENGTH and VOLUME
	var launchSpeed float64
	if fs.audio.IsEnabled() && (audioData.Energy > 0.05 || audioData.Volume > 0.05) {
		// Beat strength gives explosive launches
		// Volume provides base speed
		baseLaunch := 8.0
		volumeBoost := audioData.Volume * 8.0      // 0-8 from loudness
		beatBoost := audioData.BeatStrength * 6.0  // 0-6 from beat hits
		launchSpeed = baseLaunch + volumeBoost + beatBoost
	} else {
		// No audio or very low activity: randomize launch speed
		launchSpeed = 6.0 + rand.Float64()*8.0 // Random between 6 and 14
	}

	// Launch angle: influenced by BASS ratio
	// More bass = wider angles (spread out), less bass = narrower (vertical)
	bassRatio := audioData.Bass
	if bassRatio < 0.1 {
		bassRatio = 0.5 // Default to middle
	}
	// Map bass: high bass = wide angles (45-135°), low bass = narrow (75-105°)
	minAngle := 75.0 - (bassRatio * 30.0)  // 45-75 degrees
	maxAngle := 105.0 + (bassRatio * 30.0) // 105-135 degrees
	angleDegrees := minAngle + rand.Float64()*(maxAngle-minAngle)
	angleRadians := angleDegrees * math.Pi / 180.0

	// Convert angle and speed to velocity components
	launchVX := math.Cos(angleRadians) * launchSpeed
	launchVY := -math.Sin(angleRadians) * launchSpeed // Negative because up is negative Y

	// Starting X position: if angled left, start from right side and vice versa
	// This ensures fireworks arc across the screen
	var startX float64
	if angleDegrees < 90 {
		// Angled left - start from right side
		startX = float64(fs.width*3/4 + rand.Intn(fs.width/4))
	} else if angleDegrees > 90 {
		// Angled right - start from left side
		startX = float64(rand.Intn(fs.width / 4))
	} else {
		// Straight up - start from middle area
		startX = float64(fs.width/4 + rand.Intn(fs.width/2))
	}

	// Randomly select explosion type
	explosionType := ExplosionType(rand.Intn(4))

	// For directional explosions, choose one of 8 cardinal directions
	var direction float64
	if explosionType == ExplosionDirectional {
		// 0°, 45°, 90°, 135°, 180°, 225°, 270°, 315°
		direction = float64(rand.Intn(8)) * math.Pi / 4
	}

	fs.fireworks = append(fs.fireworks, Firework{
		rocketX:       startX,
		rocketY:       startY,
		rocketVX:      launchVX,
		rocketVY:      launchVY,
		launching:     true,
		targetY:       targetY,
		active:        true,
		color:         color,
		particles:     nil,
		audioData:     audioData,
		explosionType: explosionType,
		direction:     direction,
	})
}

func (fs *FireworkShow) update(dt float64) {
	gravity := 2.0
	decay := 0.995 // Reduced air resistance so particles travel further

	for i := range fs.fireworks {
		if !fs.fireworks[i].active {
			continue
		}

		fw := &fs.fireworks[i]

		// Launch phase: rocket ascending in parabolic arc
		if fw.launching {
			// Update rocket position (both horizontal and vertical for parabolic arc)
			fw.rocketX += fw.rocketVX * dt
			fw.rocketY += fw.rocketVY * dt
			fw.rocketVY += gravity * dt // Gravity affects vertical velocity

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

			// Very slow decay - particles should nearly reach the ground before fading
			// Life decreases based on time and vertical position
			lifeDecay := dt * 0.015 // Much slower base decay

			// Only fade faster when very close to ground (bottom 10% of screen)
			groundThreshold := float64(fs.height) * 0.9
			if p.y > groundThreshold {
				// Near ground - start fading faster
				proximityFactor := (p.y - groundThreshold) / (float64(fs.height) - groundThreshold)
				lifeDecay += dt * 0.2 * proximityFactor
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

	// Apply catherine wheel scattering effect to clustered particles
	fs.scatterClusteredParticles()

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

// scatterClusteredParticles detects tight clusters and re-explodes them to separate into single pixels
func (fs *FireworkShow) scatterClusteredParticles() {
	clusterRadius := 2.0      // Particles within this radius are considered clustered (tighter detection)
	reExplodeForce := 8.0     // Strong force to re-explode clusters into single pixels
	minClusterSize := 3       // Minimum number of particles to trigger re-explosion

	for fwIdx := range fs.fireworks {
		fw := &fs.fireworks[fwIdx]
		if !fw.active || fw.launching {
			continue
		}

		// Calculate center of mass for this firework's particles
		centerX := 0.0
		centerY := 0.0
		activeCount := 0

		for _, p := range fw.particles {
			if p.life > 0 {
				centerX += p.x
				centerY += p.y
				activeCount++
			}
		}

		if activeCount < minClusterSize {
			continue
		}

		centerX /= float64(activeCount)
		centerY /= float64(activeCount)

		// Count how many particles are clustered near the center
		clusteredCount := 0
		for _, p := range fw.particles {
			if p.life > 0 {
				dx := p.x - centerX
				dy := p.y - centerY
				dist := math.Sqrt(dx*dx + dy*dy)
				if dist < clusterRadius {
					clusteredCount++
				}
			}
		}

		// If enough particles are clustered, RE-EXPLODE them outward strongly
		if clusteredCount >= minClusterSize {
			for i := range fw.particles {
				p := &fw.particles[i]
				if p.life <= 0 {
					continue
				}

				// Calculate direction from center to particle
				dx := p.x - centerX
				dy := p.y - centerY
				dist := math.Sqrt(dx*dx + dy*dy)

				// Only apply force to particles in the cluster
				if dist < clusterRadius {
					// If particles are very close (nearly on top of each other), give them a random direction
					if dist < 0.1 {
						angle := rand.Float64() * 2 * math.Pi
						dx = math.Cos(angle)
						dy = math.Sin(angle)
						dist = 1.0
					}

					// Normalize direction
					nx := dx / dist
					ny := dy / dist

					// Apply STRONG radial outward force - like a secondary explosion
					// Much stronger for particles closer to center (inverse relationship)
					forceMagnitude := reExplodeForce * (1.0 + (clusterRadius-dist)/clusterRadius)

					// Replace velocity with explosion velocity (not just add to it)
					// This ensures particles actually separate
					p.vx = nx * forceMagnitude
					p.vy = ny * forceMagnitude

					// Refresh life slightly so re-exploded particles are visible
					p.life = math.Min(p.life+0.2, 1.0)
				}
			}
		}
	}
}

func (fs *FireworkShow) explodeRocket(index int) {
	fw := &fs.fireworks[index]

	// Size (number of particles) influenced by SPECTRAL ROLLOFF
	// High rolloff (lots of high freq) = more particles
	baseParticles := 8
	rolloffBoost := int(fw.audioData.SpectralRolloff * 25) // 0-25 particles from rolloff
	numParticles := baseParticles + rolloffBoost

	// Cap particles for performance
	if numParticles > 40 {
		numParticles = 40
	}
	if numParticles < 10 {
		numParticles = 10
	}

	particles := make([]Particle, numParticles)

	// Explosion radial speed influenced by SPECTRAL FLUX (rate of change)
	// High flux = fast/dynamic = faster explosions
	// Use the rocket's upward velocity magnitude as base
	rocketSpeed := math.Sqrt(fw.rocketVX*fw.rocketVX + fw.rocketVY*fw.rocketVY)
	baseExplosion := math.Max(rocketSpeed*2.5, 18.0)

	// Spectral flux dramatically affects explosion speed
	fluxBoost := fw.audioData.SpectralFlux * 15.0 // 0-15 additional speed

	// Add randomization to the explosion force (±30% variation)
	randomMultiplier := 0.7 + rand.Float64()*0.6 // Random between 0.7 and 1.3
	explosionSpeed := (baseExplosion + fluxBoost) * randomMultiplier

	// Generate particles based on explosion type
	switch fw.explosionType {
	case ExplosionRadial:
		particles = fs.createRadialExplosion(numParticles, explosionSpeed, fw)
		fs.explosionCountRadial++
	case ExplosionDirectional:
		particles = fs.createDirectionalExplosion(numParticles, explosionSpeed, fw)
		fs.explosionCountDirectional++
	case ExplosionSideways:
		particles = fs.createSidewaysExplosion(numParticles, explosionSpeed, fw)
		fs.explosionCountSideways++
	case ExplosionSpiral:
		particles = fs.createSpiralExplosion(numParticles, explosionSpeed, fw)
		fs.explosionCountSpiral++
	}

	fw.particles = particles
}

// createRadialExplosion creates a symmetrical radial burst
func (fs *FireworkShow) createRadialExplosion(numParticles int, explosionSpeed float64, fw *Firework) []Particle {
	particles := make([]Particle, numParticles)
	angleIncrement := (2 * math.Pi) / float64(numParticles)
	initialAngle := rand.Float64() * 2 * math.Pi

	for i := 0; i < numParticles; i++ {
		angle := initialAngle + float64(i)*angleIncrement
		speed := explosionSpeed * (0.5 + rand.Float64()*1.0)

		radialVX := math.Cos(angle) * speed
		radialVY := math.Sin(angle) * speed

		particles[i] = Particle{
			x:     fw.rocketX,
			y:     fw.rocketY,
			vx:    fw.rocketVX + radialVX,
			vy:    fw.rocketVY + radialVY,
			life:  1.0,
			color: fw.color,
			char:  '⠁',
		}
	}
	return particles
}

// createDirectionalExplosion creates a cone-shaped blast in one of 8 directions
func (fs *FireworkShow) createDirectionalExplosion(numParticles int, explosionSpeed float64, fw *Firework) []Particle {
	particles := make([]Particle, numParticles)

	// Cone angle: 60 degrees (±30° from center direction)
	coneAngle := math.Pi / 3 // 60 degrees

	for i := 0; i < numParticles; i++ {
		// Distribute particles within cone
		angleOffset := (rand.Float64() - 0.5) * coneAngle
		angle := fw.direction + angleOffset

		// Vary speed for depth effect
		speed := explosionSpeed * (0.6 + rand.Float64()*0.8)

		radialVX := math.Cos(angle) * speed
		radialVY := math.Sin(angle) * speed

		particles[i] = Particle{
			x:     fw.rocketX,
			y:     fw.rocketY,
			vx:    fw.rocketVX + radialVX,
			vy:    fw.rocketVY + radialVY,
			life:  1.0,
			color: fw.color,
			char:  '⠁',
		}
	}
	return particles
}

// createSidewaysExplosion creates a horizontal line blast
func (fs *FireworkShow) createSidewaysExplosion(numParticles int, explosionSpeed float64, fw *Firework) []Particle {
	particles := make([]Particle, numParticles)

	for i := 0; i < numParticles; i++ {
		// Split particles between left and right
		direction := 1.0
		if i%2 == 0 {
			direction = -1.0 // Left
		}

		// Horizontal velocity with slight randomness
		horizontalSpeed := explosionSpeed * (0.7 + rand.Float64()*0.6) * direction
		// Small vertical component for some spread
		verticalSpeed := (rand.Float64() - 0.5) * explosionSpeed * 0.3

		particles[i] = Particle{
			x:     fw.rocketX,
			y:     fw.rocketY,
			vx:    fw.rocketVX + horizontalSpeed,
			vy:    fw.rocketVY + verticalSpeed,
			life:  1.0,
			color: fw.color,
			char:  '⠁',
		}
	}
	return particles
}

// createSpiralExplosion creates a spiral galaxy pattern
func (fs *FireworkShow) createSpiralExplosion(numParticles int, explosionSpeed float64, fw *Firework) []Particle {
	particles := make([]Particle, numParticles)

	// Number of spiral arms
	numArms := 3
	initialRotation := rand.Float64() * 2 * math.Pi

	for i := 0; i < numParticles; i++ {
		// Which spiral arm
		armIndex := i % numArms
		// Position along the arm (0 to 1)
		t := float64(i) / float64(numParticles)

		// Spiral equation: radius increases with angle
		radius := t * explosionSpeed * 0.5
		angle := initialRotation + float64(armIndex)*2*math.Pi/float64(numArms) + t*4*math.Pi

		// Add some randomness
		radius *= 0.8 + rand.Float64()*0.4

		// Convert to velocity
		speed := explosionSpeed * (0.4 + t*0.6) // Speed increases with distance
		radialVX := math.Cos(angle) * speed
		radialVY := math.Sin(angle) * speed

		particles[i] = Particle{
			x:     fw.rocketX,
			y:     fw.rocketY,
			vx:    fw.rocketVX + radialVX,
			vy:    fw.rocketVY + radialVY,
			life:  1.0,
			color: fw.color,
			char:  '⠁',
		}
	}
	return particles
}

// countActiveParticles returns the total number of active particles across all fireworks
func (fs *FireworkShow) countActiveParticles() int {
	count := 0
	for _, fw := range fs.fireworks {
		if !fw.active || fw.launching {
			continue
		}
		for _, p := range fw.particles {
			if p.life > 0 {
				count++
			}
		}
	}
	return count
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

	// Draw all fireworks
	for _, fw := range fs.fireworks {
		if !fw.active {
			continue
		}

		// Draw launching firework ball
		if fw.launching {
			x := int(fw.rocketX)
			y := int(fw.rocketY)

			// Check bounds
			if x >= 0 && x < fs.width && y >= 6 && y < fs.height {
				style := tcell.StyleDefault.Foreground(fw.color).Background(tcell.ColorBlack)
				// Draw firework as a ball shape
				fs.screen.SetContent(x, y, '●', nil, style)
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

	// Explosion type statistics
	y += 2
	totalExplosions := fs.explosionCountRadial + fs.explosionCountDirectional +
	                   fs.explosionCountSideways + fs.explosionCountSpiral
	fs.drawText(startX, y, "EXPLOSION TYPES:", tcell.ColorFuchsia)
	y++
	fs.drawText(startX, y, "  Total:       "+formatInt(totalExplosions), tcell.ColorWhite)
	y++
	fs.drawText(startX, y, "  Radial:      "+formatInt(fs.explosionCountRadial), tcell.ColorWhite)
	y++
	fs.drawText(startX, y, "  Directional: "+formatInt(fs.explosionCountDirectional), tcell.ColorWhite)
	y++
	fs.drawText(startX, y, "  Sideways:    "+formatInt(fs.explosionCountSideways), tcell.ColorWhite)
	y++
	fs.drawText(startX, y, "  Spiral:      "+formatInt(fs.explosionCountSpiral), tcell.ColorWhite)

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
			if ev.Rune() == 'v' || ev.Rune() == 'V' {
				fs.showAudioViz = !fs.showAudioViz
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

	for fs.running {
		select {
		case <-ticker.C:
			now := time.Now()
			dt := now.Sub(lastUpdate).Seconds()
			lastUpdate = now

			// Get audio data
			audioData := fs.audio.GetData()

			// Spawn new fireworks synchronized to detected beats
			shouldSpawnAudio := false
			if fs.audio.IsEnabled() {
				// PRIMARY: Actual detected beats - synchronized explosions!
				if audioData.IsBeat {
					// Randomize number of fireworks based on beat confidence
					// Low confidence (0.3-0.5): 1 firework
					// Medium confidence (0.5-0.7): 1-2 fireworks
					// High confidence (0.7-1.0): 2-4 fireworks
					var numFireworks int
					if audioData.BeatConfidence > 0.7 {
						numFireworks = 2 + rand.Intn(3) // 2-4 fireworks
					} else if audioData.BeatConfidence > 0.5 {
						numFireworks = 1 + rand.Intn(2) // 1-2 fireworks
					} else {
						numFireworks = 1 // Single firework
					}

					// Spawn the fireworks
					for i := 0; i < numFireworks; i++ {
						fs.createFirework(audioData)
					}
				}

				// SECONDARY: Strong onsets ONLY if not on a beat (cymbal crashes, snare hits, etc.)
				// This prevents double-spawning on beats
				if !audioData.IsBeat && audioData.OnsetStrength > 0.6 {
					shouldSpawnAudio = true
				}

				// Update bass peak for other uses
				fs.lastBassPeak = audioData.Bass
			}

			// Dynamic particle threshold based on music intensity
			// During intense music, allow many more particles on screen
			particleCount := fs.countActiveParticles()

			// Calculate music intensity (0-1)
			musicIntensity := 0.0
			if fs.audio.IsEnabled() {
				// Combine onset, beat, and energy for intensity metric
				musicIntensity = (audioData.OnsetStrength*0.4 +
				                  audioData.BeatStrength*0.3 +
				                  audioData.Energy*0.3)
			}

			// Dynamic threshold: 30 (quiet) to 80 (intense)
			baseThreshold := 30.0
			maxThreshold := 80.0
			particleThreshold := int(baseThreshold + musicIntensity*(maxThreshold-baseThreshold))

			// Allow spawning if below threshold OR if music is very intense (override)
			canSpawn := particleCount < particleThreshold

			// Override: if beat detected with high confidence, always allow spawning
			if audioData.IsBeat && audioData.BeatConfidence > 0.6 {
				canSpawn = true
			}

			if canSpawn {
				if shouldSpawnAudio {
					fs.createFirework(audioData)
				} else if now.Sub(fireworkTimer).Seconds() > fireworkInterval {
					// Fallback: spawn random fireworks if no audio or low activity
					fs.createFirework(audioData)
					fireworkTimer = now
					fireworkInterval = 0.5 + rand.Float64()*2.0
				}
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
