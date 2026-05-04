// Package particles provides particle physics simulation for visual effects.
// It includes particle types, physics calculations, and collision detection.
package particles

import (
	"math"
	"math/rand"
)

// Color represents a terminal color value (compatible with tcell.Color)
// Must be int64 to hold full tcell.Color values which are uint64
type Color int64

// Particle represents a single particle with position, velocity, and lifecycle
type Particle struct {
	X, Y   float64 // Position
	VX, VY float64 // Velocity
	Life   float64 // Remaining life (0-1)
	Color  Color   // Particle color
	Char   rune    // Display character
}

// PhysicsConfig controls particle physics behavior
type PhysicsConfig struct {
	Gravity          float64 // Downward acceleration (default: 2.0)
	Decay            float64 // Air resistance factor (default: 0.995)
	CollisionRadius  float64 // Distance for collision detection (default: 1.5)
	Restitution      float64 // Bounciness 0-1 (default: 0.8)
	LifeDecayRate    float64 // Base life decay per dt (default: 0.015)
	GroundFadeHeight float64 // Height ratio where fading accelerates (default: 0.9)
}

// DefaultPhysicsConfig returns sensible default physics parameters
func DefaultPhysicsConfig() PhysicsConfig {
	return PhysicsConfig{
		Gravity:          2.0,
		Decay:            0.995,
		CollisionRadius:  1.5,
		Restitution:      0.8,
		LifeDecayRate:    0.015,
		GroundFadeHeight: 0.9,
	}
}

// ParticleSystem manages a collection of particles with physics simulation
type ParticleSystem struct {
	Particles  []Particle
	Config     PhysicsConfig
	Width      int        // Bounds width (in sub-cell coordinates)
	Height     int        // Bounds height (in sub-cell coordinates)
	RenderMode RenderMode // Rendering mode for this system
}

// NewParticleSystem creates a new particle system with default config.
// Width and height are in sub-cell coordinates for the given render mode.
func NewParticleSystem(width, height int) *ParticleSystem {
	return &ParticleSystem{
		Particles:  make([]Particle, 0),
		Config:     DefaultPhysicsConfig(),
		Width:      width,
		Height:     height,
		RenderMode: RenderBraille,
	}
}

// NewParticleSystemWithConfig creates a particle system with custom physics
func NewParticleSystemWithConfig(width, height int, config PhysicsConfig) *ParticleSystem {
	return &ParticleSystem{
		Particles:  make([]Particle, 0),
		Config:     config,
		Width:      width,
		Height:     height,
		RenderMode: RenderBraille,
	}
}

// NewParticleSystemWithMode creates a particle system with a specific render mode.
// Width and height are in terminal cell coordinates; they are automatically
// scaled to sub-cell coordinates based on the render mode.
func NewParticleSystemWithMode(termWidth, termHeight int, mode RenderMode) *ParticleSystem {
	w, h := TermToSubCell(termWidth, termHeight, mode)
	return &ParticleSystem{
		Particles:  make([]Particle, 0),
		Config:     DefaultPhysicsConfig(),
		Width:      w,
		Height:     h,
		RenderMode: mode,
	}
}

// TermToSubCell converts terminal cell dimensions to sub-cell dimensions
func TermToSubCell(termWidth, termHeight int, mode RenderMode) (int, int) {
	switch mode {
	case RenderBraille:
		return termWidth * 2, termHeight * 4
	case RenderQuarterBlock:
		return termWidth * 2, termHeight * 2
	default:
		return termWidth * 2, termHeight * 4
	}
}

// AddParticle adds a particle to the system
func (ps *ParticleSystem) AddParticle(p Particle) {
	ps.Particles = append(ps.Particles, p)
}

// AddParticles adds multiple particles to the system
func (ps *ParticleSystem) AddParticles(particles []Particle) {
	ps.Particles = append(ps.Particles, particles...)
}

// Update advances the particle simulation by dt seconds
func (ps *ParticleSystem) Update(dt float64) {
	for i := range ps.Particles {
		p := &ps.Particles[i]

		if p.Life <= 0 {
			continue
		}

		// Update position
		p.X += p.VX * dt
		p.Y += p.VY * dt

		// Apply gravity
		p.VY += ps.Config.Gravity * dt

		// Apply air resistance
		p.VX *= ps.Config.Decay
		p.VY *= ps.Config.Decay

		// Life decay
		lifeDecay := dt * ps.Config.LifeDecayRate

		// Faster fade near ground
		groundThreshold := float64(ps.Height) * ps.Config.GroundFadeHeight
		if p.Y > groundThreshold {
			proximityFactor := (p.Y - groundThreshold) / (float64(ps.Height) - groundThreshold)
			lifeDecay += dt * 0.2 * proximityFactor
		}

		p.Life -= lifeDecay
	}

	// Handle collisions
	ps.handleCollisions()

	// Scatter clustered particles
	ps.scatterClusters()
}

// handleCollisions detects and resolves particle collisions
func (ps *ParticleSystem) handleCollisions() {
	for i := 0; i < len(ps.Particles); i++ {
		p1 := &ps.Particles[i]
		if p1.Life <= 0 {
			continue
		}

		for j := i + 1; j < len(ps.Particles); j++ {
			p2 := &ps.Particles[j]
			if p2.Life <= 0 {
				continue
			}

			dx := p2.X - p1.X
			dy := p2.Y - p1.Y
			distSq := dx*dx + dy*dy
			dist := math.Sqrt(distSq)

			if dist < ps.Config.CollisionRadius && dist > 0.01 {
				// Normalize direction
				nx := dx / dist
				ny := dy / dist

				// Relative velocity
				dvx := p2.VX - p1.VX
				dvy := p2.VY - p1.VY
				dvn := dvx*nx + dvy*ny

				if dvn < 0 {
					// Elastic collision
					impulse := -(1 + ps.Config.Restitution) * dvn / 2

					p1.VX -= impulse * nx
					p1.VY -= impulse * ny
					p2.VX += impulse * nx
					p2.VY += impulse * ny

					// Separate particles
					overlap := ps.Config.CollisionRadius - dist
					separationX := nx * overlap * 0.5
					separationY := ny * overlap * 0.5
					p1.X -= separationX
					p1.Y -= separationY
					p2.X += separationX
					p2.Y += separationY

					// Brighten on collision
					p1.Life = math.Min(p1.Life+0.1, 1.0)
					p2.Life = math.Min(p2.Life+0.1, 1.0)

					// Random perturbation
					perturbStrength := 0.5
					p1.VX += (rand.Float64()*2 - 1) * perturbStrength
					p1.VY += (rand.Float64()*2 - 1) * perturbStrength
					p2.VX += (rand.Float64()*2 - 1) * perturbStrength
					p2.VY += (rand.Float64()*2 - 1) * perturbStrength
				}
			}
		}
	}
}

// scatterClusters re-explodes tightly clustered particles
func (ps *ParticleSystem) scatterClusters() {
	clusterRadius := 2.0
	reExplodeForce := 8.0
	minClusterSize := 3

	// Calculate center of mass
	centerX := 0.0
	centerY := 0.0
	activeCount := 0

	for _, p := range ps.Particles {
		if p.Life > 0 {
			centerX += p.X
			centerY += p.Y
			activeCount++
		}
	}

	if activeCount < minClusterSize {
		return
	}

	centerX /= float64(activeCount)
	centerY /= float64(activeCount)

	// Count clustered particles
	clusteredCount := 0
	for _, p := range ps.Particles {
		if p.Life > 0 {
			dx := p.X - centerX
			dy := p.Y - centerY
			dist := math.Sqrt(dx*dx + dy*dy)
			if dist < clusterRadius {
				clusteredCount++
			}
		}
	}

	// Re-explode if clustered
	if clusteredCount >= minClusterSize {
		for i := range ps.Particles {
			p := &ps.Particles[i]
			if p.Life <= 0 {
				continue
			}

			dx := p.X - centerX
			dy := p.Y - centerY
			dist := math.Sqrt(dx*dx + dy*dy)

			if dist < clusterRadius {
				if dist < 0.1 {
					angle := rand.Float64() * 2 * math.Pi
					dx = math.Cos(angle)
					dy = math.Sin(angle)
					dist = 1.0
				}

				nx := dx / dist
				ny := dy / dist

				forceMagnitude := reExplodeForce * (1.0 + (clusterRadius-dist)/clusterRadius)
				p.VX = nx * forceMagnitude
				p.VY = ny * forceMagnitude
				p.Life = math.Min(p.Life+0.2, 1.0)
			}
		}
	}
}

// ActiveCount returns the number of particles with life > 0
func (ps *ParticleSystem) ActiveCount() int {
	count := 0
	for _, p := range ps.Particles {
		if p.Life > 0 {
			count++
		}
	}
	return count
}

// RemoveDead removes all particles with life <= 0
func (ps *ParticleSystem) RemoveDead() {
	alive := make([]Particle, 0, len(ps.Particles))
	for _, p := range ps.Particles {
		if p.Life > 0 {
			alive = append(alive, p)
		}
	}
	ps.Particles = alive
}

// Clear removes all particles
func (ps *ParticleSystem) Clear() {
	ps.Particles = ps.Particles[:0]
}

// SetBounds updates the simulation bounds (in sub-cell coordinates)
func (ps *ParticleSystem) SetBounds(width, height int) {
	ps.Width = width
	ps.Height = height
}

// SetBoundsFromTerminal updates bounds from terminal cell dimensions
func (ps *ParticleSystem) SetBoundsFromTerminal(termWidth, termHeight int) {
	ps.Width, ps.Height = TermToSubCell(termWidth, termHeight, ps.RenderMode)
}
