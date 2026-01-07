#!/usr/bin/env bash
package main

import (
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
	particles []Particle
	active    bool
}

// FireworkShow manages the entire fireworks display
type FireworkShow struct {
	screen    tcell.Screen
	fireworks []Firework
	width     int
	height    int
	running   bool
}

var colors = []tcell.Color{
	tcell.ColorRed,
	tcell.ColorGreen,
	tcell.ColorYellow,
	tcell.ColorBlue,
	tcell.ColorMagenta,
	tcell.ColorCyan,
	tcell.ColorOrange,
	tcell.ColorPurple,
	tcell.ColorGold,
	tcell.ColorLightCoral,
	tcell.ColorLightGreen,
	tcell.ColorLightSkyBlue,
}

var chars = []rune{'*', '+', '.', '·', '•', '○', '◦', '∘'}

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

	return &FireworkShow{
		screen:    screen,
		fireworks: make([]Firework, 0),
		width:     width,
		height:    height,
		running:   true,
	}, nil
}

func (fs *FireworkShow) createFirework() {
	// Random position in upper portion of screen
	x := float64(rand.Intn(fs.width))
	y := float64(rand.Intn(fs.height/3) + fs.height/4)

	// Random color for this firework
	color := colors[rand.Intn(len(colors))]

	// Create explosion particles
	numParticles := 50 + rand.Intn(100)
	particles := make([]Particle, numParticles)

	for i := 0; i < numParticles; i++ {
		// Random angle and speed
		angle := rand.Float64() * 2 * math.Pi
		speed := 1.0 + rand.Float64()*3.0

		particles[i] = Particle{
			x:     x,
			y:     y,
			vx:    math.Cos(angle) * speed,
			vy:    math.Sin(angle) * speed,
			life:  1.0,
			color: color,
			char:  chars[rand.Intn(len(chars))],
		}
	}

	fs.fireworks = append(fs.fireworks, Firework{
		particles: particles,
		active:    true,
	})
}

func (fs *FireworkShow) update(dt float64) {
	gravity := 2.0
	decay := 0.98

	for i := range fs.fireworks {
		if !fs.fireworks[i].active {
			continue
		}

		activeParticles := 0
		for j := range fs.fireworks[i].particles {
			p := &fs.fireworks[i].particles[j]

			if p.life <= 0 {
				continue
			}

			// Update position
			p.x += p.vx * dt
			p.y += p.vy * dt

			// Apply gravity
			p.vy += gravity * dt

			// Apply air resistance
			p.vx *= decay
			p.vy *= decay

			// Decrease life
			p.life -= dt * 0.5

			if p.life > 0 {
				activeParticles++
			}
		}

		if activeParticles == 0 {
			fs.fireworks[i].active = false
		}
	}

	// Remove inactive fireworks
	activeFireworks := make([]Firework, 0, len(fs.fireworks))
	for _, fw := range fs.fireworks {
		if fw.active {
			activeFireworks = append(activeFireworks, fw)
		}
	}
	fs.fireworks = activeFireworks
}

func (fs *FireworkShow) render() {
	fs.screen.Clear()

	// Draw title
	title := "✨ FIREWORKS SHOW ✨ (Press ESC or Q to exit)"
	style := tcell.StyleDefault.Foreground(tcell.ColorWhite).Background(tcell.ColorBlack)
	startX := (fs.width - len(title)) / 2
	for i, ch := range title {
		fs.screen.SetContent(startX+i, 0, ch, nil, style)
	}

	// Draw all particles
	for _, fw := range fs.fireworks {
		if !fw.active {
			continue
		}

		for _, p := range fw.particles {
			if p.life <= 0 {
				continue
			}

			x := int(p.x)
			y := int(p.y)

			// Check bounds
			if x < 0 || x >= fs.width || y < 2 || y >= fs.height {
				continue
			}

			// Calculate brightness based on life
			style := tcell.StyleDefault.Foreground(p.color).Background(tcell.ColorBlack)

			// Dim the color as particle fades
			if p.life < 0.3 {
				style = tcell.StyleDefault.Foreground(tcell.ColorGray).Background(tcell.ColorBlack)
			}

			fs.screen.SetContent(x, y, p.char, nil, style)
		}
	}

	fs.screen.Show()
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
		case *tcell.EventResize:
			fs.width, fs.height = fs.screen.Size()
			fs.screen.Sync()
		}
	}
}

func (fs *FireworkShow) Run() {
	defer fs.screen.Fini()

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

			// Spawn new fireworks randomly
			if now.Sub(fireworkTimer).Seconds() > fireworkInterval {
				fs.createFirework()
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
