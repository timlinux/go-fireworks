// Package fireworks provides an audio-reactive fireworks display system.
// It combines particle physics with audio analysis to create synchronized
// visual effects that respond to music.
package fireworks

import (
	"math"
	"math/rand"

	"tim-particles/pkg/audio"
	"tim-particles/pkg/particles"
)

// Firework represents a single firework with launch and explosion phases
type Firework struct {
	// Launch phase
	RocketX, RocketY   float64
	RocketVX, RocketVY float64
	Launching          bool
	TargetY            float64

	// Explosion phase
	Particles     *particles.ParticleSystem
	Active        bool
	Color         particles.Color
	ExplosionType particles.ExplosionType
	Direction     float64

	// Audio data captured at launch
	AudioData audio.Data
}

// Config controls firework behavior
type Config struct {
	// Physics
	Gravity float64 // Default: 2.0
	Decay   float64 // Air resistance, default: 0.995

	// Launch parameters
	BaseLaunchSpeed  float64 // Default: 8.0
	MaxLaunchSpeed   float64 // Default: 22.0
	MinAngle         float64 // Degrees, default: 45.0
	MaxAngle         float64 // Degrees, default: 135.0

	// Explosion parameters
	BaseParticles    int     // Default: 8
	MaxParticles     int     // Default: 40
	BaseExplosion    float64 // Default: 18.0

	// Audio reactivity weights
	VolumeWeight     float64 // How much volume affects launch speed
	BeatWeight       float64 // How much beat affects launch speed
	BassWeight       float64 // How much bass affects angle spread
	FluxWeight       float64 // How much flux affects explosion speed
	RolloffWeight    float64 // How much rolloff affects particle count
	CentroidWeight   float64 // How much centroid affects color selection
}

// DefaultConfig returns sensible default configuration
func DefaultConfig() Config {
	return Config{
		Gravity:         2.0,
		Decay:           0.995,
		BaseLaunchSpeed: 8.0,
		MaxLaunchSpeed:  22.0,
		MinAngle:        45.0,
		MaxAngle:        135.0,
		BaseParticles:   8,
		MaxParticles:    40,
		BaseExplosion:   18.0,
		VolumeWeight:    8.0,
		BeatWeight:      6.0,
		BassWeight:      30.0,
		FluxWeight:      15.0,
		RolloffWeight:   25.0,
		CentroidWeight:  1.0,
	}
}

// Show manages the entire fireworks display
type Show struct {
	Fireworks []Firework
	Width     int
	Height    int
	Config    Config
	Colors    []particles.Color

	// Statistics
	ExplosionCountRadial      int
	ExplosionCountDirectional int
	ExplosionCountSideways    int
	ExplosionCountSpiral      int

	lastFireworkColor particles.Color
}

// NewShow creates a new fireworks show with default configuration
func NewShow(width, height int) *Show {
	return NewShowWithConfig(width, height, DefaultConfig())
}

// NewShowWithConfig creates a new fireworks show with custom configuration
func NewShowWithConfig(width, height int, config Config) *Show {
	return &Show{
		Fireworks: make([]Firework, 0),
		Width:     width,
		Height:    height,
		Config:    config,
		Colors:    DefaultColors(),
	}
}

// DefaultColors returns the default color palette
func DefaultColors() []particles.Color {
	// These values correspond to tcell colors
	return []particles.Color{
		0x800000, // Red
		0xFF4500, // OrangeRed
		0xFF8C00, // DarkOrange
		0xFFA500, // Orange
		0xFFD700, // Gold
		0xFFFF00, // Yellow
		0xADFF2F, // GreenYellow
		0x00FF00, // Lime
		0x00FF7F, // SpringGreen
		0x008000, // Green
		0x00FFFF, // Aqua
		0x00BFFF, // DeepSkyBlue
		0x1E90FF, // DodgerBlue
		0x0000FF, // Blue
		0x8A2BE2, // BlueViolet
		0x800080, // Purple
		0x9400D3, // DarkViolet
		0xFF00FF, // Fuchsia
		0xFF1493, // DeepPink
		0xFF69B4, // HotPink
		0xFFC0CB, // Pink
		0xDC143C, // Crimson
	}
}

// CreateFirework spawns a new firework based on audio data
func (s *Show) CreateFirework(audioData audio.Data) {
	startY := float64(s.Height - 1)

	// Random target height (20% to 80% of screen)
	minHeight := s.Height / 5
	maxHeight := s.Height * 4 / 5
	targetY := float64(rand.Intn(maxHeight-minHeight) + minHeight)

	// Color selection based on spectral centroid
	colorIndex := s.selectColor(audioData)
	color := s.Colors[colorIndex]

	// Ensure different color from last firework
	attempts := 0
	for color == s.lastFireworkColor && attempts < 5 {
		colorIndex = rand.Intn(len(s.Colors))
		color = s.Colors[colorIndex]
		attempts++
	}
	s.lastFireworkColor = color

	// Launch speed based on audio
	launchSpeed := s.calculateLaunchSpeed(audioData)

	// Launch angle based on bass
	launchVX, launchVY := s.calculateLaunchVelocity(audioData, launchSpeed)

	// Starting position based on angle
	angleDegrees := math.Atan2(-launchVY, launchVX) * 180 / math.Pi
	var startX float64
	if angleDegrees < 90 {
		startX = float64(s.Width*3/4 + rand.Intn(s.Width/4))
	} else if angleDegrees > 90 {
		startX = float64(rand.Intn(s.Width / 4))
	} else {
		startX = float64(s.Width/4 + rand.Intn(s.Width/2))
	}

	// Random explosion type
	explosionType := particles.ExplosionType(rand.Intn(4))
	var direction float64
	if explosionType == particles.ExplosionDirectional {
		direction = float64(rand.Intn(8)) * math.Pi / 4
	}

	s.Fireworks = append(s.Fireworks, Firework{
		RocketX:       startX,
		RocketY:       startY,
		RocketVX:      launchVX,
		RocketVY:      launchVY,
		Launching:     true,
		TargetY:       targetY,
		Active:        true,
		Color:         color,
		Particles:     nil,
		AudioData:     audioData,
		ExplosionType: explosionType,
		Direction:     direction,
	})
}

func (s *Show) selectColor(audioData audio.Data) int {
	if audioData.SpectralCentroid > 0.05 {
		colorIndex := int(audioData.SpectralCentroid * float64(len(s.Colors)) * s.Config.CentroidWeight)
		if colorIndex >= len(s.Colors) {
			colorIndex = len(s.Colors) - 1
		}
		return colorIndex
	}
	return rand.Intn(len(s.Colors))
}

func (s *Show) calculateLaunchSpeed(audioData audio.Data) float64 {
	if audioData.Energy > 0.05 || audioData.Volume > 0.05 {
		volumeBoost := audioData.Volume * s.Config.VolumeWeight
		beatBoost := audioData.BeatStrength * s.Config.BeatWeight
		speed := s.Config.BaseLaunchSpeed + volumeBoost + beatBoost
		if speed > s.Config.MaxLaunchSpeed {
			speed = s.Config.MaxLaunchSpeed
		}
		return speed
	}
	return s.Config.BaseLaunchSpeed + rand.Float64()*(s.Config.MaxLaunchSpeed-s.Config.BaseLaunchSpeed)/2
}

func (s *Show) calculateLaunchVelocity(audioData audio.Data, speed float64) (vx, vy float64) {
	bassRatio := audioData.Bass
	if bassRatio < 0.1 {
		bassRatio = 0.5
	}

	minAngle := 75.0 - (bassRatio * s.Config.BassWeight)
	maxAngle := 105.0 + (bassRatio * s.Config.BassWeight)
	angleDegrees := minAngle + rand.Float64()*(maxAngle-minAngle)
	angleRadians := angleDegrees * math.Pi / 180.0

	vx = math.Cos(angleRadians) * speed
	vy = -math.Sin(angleRadians) * speed
	return
}

// Update advances the simulation by dt seconds
func (s *Show) Update(dt float64) {
	for i := range s.Fireworks {
		if !s.Fireworks[i].Active {
			continue
		}

		fw := &s.Fireworks[i]

		if fw.Launching {
			// Update rocket position
			fw.RocketX += fw.RocketVX * dt
			fw.RocketY += fw.RocketVY * dt
			fw.RocketVY += s.Config.Gravity * dt

			// Check for explosion
			if fw.RocketY <= fw.TargetY || fw.RocketVY >= 0 {
				s.explodeRocket(i)
				fw.Launching = false
			}
			continue
		}

		// Update particles
		if fw.Particles != nil {
			fw.Particles.Update(dt)

			if fw.Particles.ActiveCount() == 0 {
				fw.Active = false
			}
		}
	}

	// Remove inactive fireworks
	active := make([]Firework, 0, len(s.Fireworks))
	for _, fw := range s.Fireworks {
		if fw.Active {
			active = append(active, fw)
		}
	}
	s.Fireworks = active
}

func (s *Show) explodeRocket(index int) {
	fw := &s.Fireworks[index]

	// Particle count based on spectral rolloff
	rolloffBoost := int(fw.AudioData.SpectralRolloff * float64(s.Config.RolloffWeight))
	numParticles := s.Config.BaseParticles + rolloffBoost
	if numParticles > s.Config.MaxParticles {
		numParticles = s.Config.MaxParticles
	}
	if numParticles < 10 {
		numParticles = 10
	}

	// Explosion speed based on rocket speed and spectral flux
	rocketSpeed := math.Sqrt(fw.RocketVX*fw.RocketVX + fw.RocketVY*fw.RocketVY)
	baseExplosion := math.Max(rocketSpeed*2.5, s.Config.BaseExplosion)
	fluxBoost := fw.AudioData.SpectralFlux * s.Config.FluxWeight
	randomMultiplier := 0.7 + rand.Float64()*0.6
	explosionSpeed := (baseExplosion + fluxBoost) * randomMultiplier

	// Create explosion
	config := particles.ExplosionConfig{
		NumParticles:    numParticles,
		Speed:           explosionSpeed,
		Type:            fw.ExplosionType,
		Direction:       fw.Direction,
		Color:           fw.Color,
		Char:            '⠁',
		InheritVelocity: true,
		SourceVX:        fw.RocketVX,
		SourceVY:        fw.RocketVY,
	}

	explosionParticles := particles.CreateExplosion(fw.RocketX, fw.RocketY, config)

	// Create particle system for this firework
	fw.Particles = particles.NewParticleSystem(s.Width, s.Height)
	fw.Particles.AddParticles(explosionParticles)

	// Update statistics
	switch fw.ExplosionType {
	case particles.ExplosionRadial:
		s.ExplosionCountRadial++
	case particles.ExplosionDirectional:
		s.ExplosionCountDirectional++
	case particles.ExplosionSideways:
		s.ExplosionCountSideways++
	case particles.ExplosionSpiral:
		s.ExplosionCountSpiral++
	}
}

// ActiveParticleCount returns total active particles across all fireworks
func (s *Show) ActiveParticleCount() int {
	count := 0
	for _, fw := range s.Fireworks {
		if !fw.Active || fw.Launching {
			continue
		}
		if fw.Particles != nil {
			count += fw.Particles.ActiveCount()
		}
	}
	return count
}

// SetBounds updates the display bounds
func (s *Show) SetBounds(width, height int) {
	s.Width = width
	s.Height = height
}

// GetAllParticles returns all active particles for rendering
func (s *Show) GetAllParticles() []particles.Particle {
	result := make([]particles.Particle, 0)
	for _, fw := range s.Fireworks {
		if !fw.Active || fw.Launching || fw.Particles == nil {
			continue
		}
		for _, p := range fw.Particles.Particles {
			if p.Life > 0 {
				result = append(result, p)
			}
		}
	}
	return result
}

// GetLaunchingRockets returns rockets that are still ascending
func (s *Show) GetLaunchingRockets() []Firework {
	result := make([]Firework, 0)
	for _, fw := range s.Fireworks {
		if fw.Active && fw.Launching {
			result = append(result, fw)
		}
	}
	return result
}

// ShouldSpawn determines if a new firework should be spawned based on audio
func (s *Show) ShouldSpawn(audioData audio.Data, hasAudio bool) (spawn bool, count int) {
	if !hasAudio {
		return true, 1
	}

	// Beat detection - primary trigger
	if audioData.IsBeat {
		if audioData.BeatConfidence > 0.7 {
			return true, 2 + rand.Intn(3) // 2-4 fireworks
		} else if audioData.BeatConfidence > 0.5 {
			return true, 1 + rand.Intn(2) // 1-2 fireworks
		}
		return true, 1
	}

	// Strong onset - secondary trigger
	if audioData.OnsetStrength > 0.6 {
		return true, 1
	}

	return false, 0
}

// DynamicParticleThreshold returns max particles based on music intensity
func (s *Show) DynamicParticleThreshold(audioData audio.Data, hasAudio bool) int {
	if !hasAudio {
		return 30
	}

	musicIntensity := audioData.OnsetStrength*0.4 + audioData.BeatStrength*0.3 + audioData.Energy*0.3
	baseThreshold := 30.0
	maxThreshold := 80.0

	return int(baseThreshold + musicIntensity*(maxThreshold-baseThreshold))
}

// TotalExplosions returns the total number of explosions that have occurred
func (s *Show) TotalExplosions() int {
	return s.ExplosionCountRadial + s.ExplosionCountDirectional +
		s.ExplosionCountSideways + s.ExplosionCountSpiral
}
