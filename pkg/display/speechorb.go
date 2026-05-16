package display

import (
	"math"
	"math/rand"

	"go-fireworks/pkg/audio"
	"go-fireworks/pkg/particles"
)

// SpeechOrb manages the speech-reactive fireworks emitter mode.
// Fireworks explode from the center of the screen in sequentially
// rotating radial directions, triggered by microphone speech activity.
type SpeechOrb struct {
	systems    []*particles.ParticleSystem
	width      int // terminal cells
	height     int // terminal cells
	renderMode particles.RenderMode
	angle      float64 // current emission angle (rotates sequentially)
	colors     []particles.Color
	colorIndex int
	cooldown   float64 // time until next firework can spawn
}

// NewSpeechOrb creates a new speech orb display
func NewSpeechOrb(width, height int, mode particles.RenderMode, colors []particles.Color) *SpeechOrb {
	return &SpeechOrb{
		systems:    make([]*particles.ParticleSystem, 0),
		width:      width,
		height:     height,
		renderMode: mode,
		angle:      0,
		colors:     colors,
		colorIndex: 0,
	}
}

// SetBounds updates display dimensions
func (so *SpeechOrb) SetBounds(width, height int) {
	so.width = width
	so.height = height
}

// SetRenderMode changes the render mode
func (so *SpeechOrb) SetRenderMode(mode particles.RenderMode) {
	so.renderMode = mode
	so.systems = so.systems[:0]
}

// Update advances the simulation and spawns firework explosions based on mic audio
func (so *SpeechOrb) Update(dt float64, audioData audio.Data, hasAudio bool) {
	// Decrease cooldown
	if so.cooldown > 0 {
		so.cooldown -= dt
	}

	// Determine speech activity level
	volume := audioData.Volume
	energy := audioData.Energy
	speechLevel := math.Max(volume, energy)

	// Spawn firework explosions from center when speech detected
	if speechLevel > 0.08 && so.cooldown <= 0 {
		// How many fireworks to spawn based on intensity
		count := 1
		if audioData.IsBeat || speechLevel > 0.6 {
			count = 2 + rand.Intn(2)
		} else if speechLevel > 0.4 {
			count = 1 + rand.Intn(2)
		}

		for i := 0; i < count; i++ {
			so.spawnFirework(speechLevel, audioData)
		}

		// Cooldown: low volume = long pause, high volume = rapid fire
		// Range: ~1.5s at low volume down to ~0.08s at max volume
		so.cooldown = 0.08 + (1.0-speechLevel)*(1.0-speechLevel)*1.5
	}

	// Update all particle systems
	alive := make([]*particles.ParticleSystem, 0, len(so.systems))
	for _, ps := range so.systems {
		ps.Update(dt)
		if ps.ActiveCount() > 0 {
			alive = append(alive, ps)
		}
	}
	so.systems = alive
}

func (so *SpeechOrb) spawnFirework(intensity float64, audioData audio.Data) {
	subW, subH := particles.TermToSubCell(so.width, so.height, so.renderMode)
	centerX := float64(subW) / 2.0
	centerY := float64(subH) / 2.0

	// Pick color based on spectral centroid
	if audioData.SpectralCentroid > 0.05 && len(so.colors) > 0 {
		so.colorIndex = int(audioData.SpectralCentroid * float64(len(so.colors)))
		if so.colorIndex >= len(so.colors) {
			so.colorIndex = len(so.colors) - 1
		}
	} else {
		so.colorIndex = rand.Intn(len(so.colors))
	}
	color := so.colors[so.colorIndex%len(so.colors)]

	// Particle count based on intensity and rolloff
	numParticles := 10 + int(intensity*25)
	if audioData.SpectralRolloff > 0.3 {
		numParticles += int(audioData.SpectralRolloff * 15)
	}
	if numParticles > 45 {
		numParticles = 45
	}

	// Explosion speed based on intensity and flux
	speed := 12.0 + intensity*20.0 + audioData.SpectralFlux*10.0

	// Pick explosion type - use the sequential angle as the direction
	explosionType := particles.ExplosionType(rand.Intn(4))

	config := particles.ExplosionConfig{
		NumParticles:    numParticles,
		Speed:           speed,
		Type:            explosionType,
		Direction:       so.angle,
		Color:           color,
		Char:            '⠁',
		InheritVelocity: false,
		SourceVX:        0,
		SourceVY:        0,
	}

	explosionParticles := particles.CreateExplosion(centerX, centerY, config)

	// Create particle system for this explosion
	ps := particles.NewParticleSystem(subW, subH)
	ps.RenderMode = so.renderMode
	ps.Config.Gravity = 1.5
	ps.Config.LifeDecayRate = 0.018
	ps.AddParticles(explosionParticles)

	so.systems = append(so.systems, ps)

	// Advance emission angle sequentially
	so.angle += 0.4 + intensity*0.3
	if so.angle > 2*math.Pi {
		so.angle -= 2 * math.Pi
	}
}

// GetParticles returns all active particles across all systems for rendering
func (so *SpeechOrb) GetParticles() []particles.Particle {
	result := make([]particles.Particle, 0)
	for _, ps := range so.systems {
		for _, p := range ps.Particles {
			if p.Life > 0 {
				result = append(result, p)
			}
		}
	}
	return result
}

// ActiveParticleCount returns the total number of live particles
func (so *SpeechOrb) ActiveParticleCount() int {
	count := 0
	for _, ps := range so.systems {
		count += ps.ActiveCount()
	}
	return count
}

// OrbRadius returns the radius of the center orb in terminal cells
func (so *SpeechOrb) OrbRadius(audioData audio.Data) int {
	base := 3
	pulse := int(audioData.Volume * 2)
	return base + pulse
}
