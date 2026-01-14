package particles

import (
	"math"
	"math/rand"
)

// ExplosionType defines different explosion patterns
type ExplosionType int

const (
	ExplosionRadial      ExplosionType = iota // Symmetrical burst in all directions
	ExplosionDirectional                      // Cone-shaped blast in one direction
	ExplosionSideways                         // Horizontal left-right split
	ExplosionSpiral                           // Spiral galaxy pattern
)

// ExplosionConfig controls explosion behavior
type ExplosionConfig struct {
	NumParticles   int           // Number of particles to create
	Speed          float64       // Base explosion speed
	Type           ExplosionType // Explosion pattern
	Direction      float64       // Direction in radians (for directional explosions)
	Color          Color         // Particle color
	Char           rune          // Display character
	InheritVelocity bool         // Whether to inherit source velocity
	SourceVX       float64       // Source X velocity to inherit
	SourceVY       float64       // Source Y velocity to inherit
}

// DefaultExplosionConfig returns sensible defaults
func DefaultExplosionConfig() ExplosionConfig {
	return ExplosionConfig{
		NumParticles:    20,
		Speed:           18.0,
		Type:            ExplosionRadial,
		Direction:       0,
		Color:           0,
		Char:            '⠁',
		InheritVelocity: true,
		SourceVX:        0,
		SourceVY:        0,
	}
}

// CreateExplosion generates particles for an explosion at the given position
func CreateExplosion(x, y float64, config ExplosionConfig) []Particle {
	switch config.Type {
	case ExplosionDirectional:
		return createDirectionalExplosion(x, y, config)
	case ExplosionSideways:
		return createSidewaysExplosion(x, y, config)
	case ExplosionSpiral:
		return createSpiralExplosion(x, y, config)
	default:
		return createRadialExplosion(x, y, config)
	}
}

// CreateRandomExplosion creates an explosion with a random type
func CreateRandomExplosion(x, y float64, config ExplosionConfig) []Particle {
	config.Type = ExplosionType(rand.Intn(4))
	if config.Type == ExplosionDirectional {
		config.Direction = float64(rand.Intn(8)) * math.Pi / 4
	}
	return CreateExplosion(x, y, config)
}

func createRadialExplosion(x, y float64, config ExplosionConfig) []Particle {
	particles := make([]Particle, config.NumParticles)
	angleIncrement := (2 * math.Pi) / float64(config.NumParticles)
	initialAngle := rand.Float64() * 2 * math.Pi

	for i := 0; i < config.NumParticles; i++ {
		angle := initialAngle + float64(i)*angleIncrement
		speed := config.Speed * (0.5 + rand.Float64()*1.0)

		vx := math.Cos(angle) * speed
		vy := math.Sin(angle) * speed

		if config.InheritVelocity {
			vx += config.SourceVX
			vy += config.SourceVY
		}

		particles[i] = Particle{
			X:     x,
			Y:     y,
			VX:    vx,
			VY:    vy,
			Life:  1.0,
			Color: config.Color,
			Char:  config.Char,
		}
	}
	return particles
}

func createDirectionalExplosion(x, y float64, config ExplosionConfig) []Particle {
	particles := make([]Particle, config.NumParticles)
	coneAngle := math.Pi / 4 // 45 degree cone

	for i := 0; i < config.NumParticles; i++ {
		angleOffset := (rand.Float64() - 0.5) * coneAngle
		angle := config.Direction + angleOffset
		speed := config.Speed * (1.2 + rand.Float64()*0.6)

		vx := math.Cos(angle) * speed
		vy := math.Sin(angle) * speed

		if config.InheritVelocity {
			vx += config.SourceVX * 0.2
			vy += config.SourceVY * 0.2
		}

		particles[i] = Particle{
			X:     x,
			Y:     y,
			VX:    vx,
			VY:    vy,
			Life:  1.0,
			Color: config.Color,
			Char:  config.Char,
		}
	}
	return particles
}

func createSidewaysExplosion(x, y float64, config ExplosionConfig) []Particle {
	particles := make([]Particle, config.NumParticles)

	for i := 0; i < config.NumParticles; i++ {
		direction := 1.0
		if i%2 == 0 {
			direction = -1.0
		}

		horizontalSpeed := config.Speed * (1.3 + rand.Float64()*0.5) * direction
		verticalSpeed := (rand.Float64() - 0.5) * config.Speed * 0.15

		particles[i] = Particle{
			X:     x,
			Y:     y,
			VX:    horizontalSpeed,
			VY:    verticalSpeed,
			Life:  1.0,
			Color: config.Color,
			Char:  config.Char,
		}
	}
	return particles
}

func createSpiralExplosion(x, y float64, config ExplosionConfig) []Particle {
	particles := make([]Particle, config.NumParticles)
	numArms := 4
	initialRotation := rand.Float64() * 2 * math.Pi

	for i := 0; i < config.NumParticles; i++ {
		armIndex := i % numArms
		t := float64(i) / float64(config.NumParticles)

		angle := initialRotation + float64(armIndex)*2*math.Pi/float64(numArms) + t*6*math.Pi
		angleNoise := (rand.Float64() - 0.5) * 0.3

		speed := config.Speed * (0.6 + t*1.0)
		vx := math.Cos(angle+angleNoise) * speed
		vy := math.Sin(angle+angleNoise) * speed

		particles[i] = Particle{
			X:     x,
			Y:     y,
			VX:    vx,
			VY:    vy,
			Life:  1.0,
			Color: config.Color,
			Char:  config.Char,
		}
	}
	return particles
}
