package display

import (
	"go-fireworks/pkg/audio"
	"go-fireworks/pkg/particles"
)

// Waveform renders audio waveform data using the particle renderer.
// It places particles along the waveform shape so that sub-cell
// rendering (braille/quarter-block) produces a high-resolution display.
type Waveform struct {
	width      int
	height     int
	renderMode particles.RenderMode
	colors     []particles.Color
}

// NewWaveform creates a waveform display
func NewWaveform(width, height int, mode particles.RenderMode, colors []particles.Color) *Waveform {
	return &Waveform{
		width:      width,
		height:     height,
		renderMode: mode,
		colors:     colors,
	}
}

// SetBounds updates display dimensions
func (w *Waveform) SetBounds(width, height int) {
	w.width = width
	w.height = height
}

// SetRenderMode changes the render mode
func (w *Waveform) SetRenderMode(mode particles.RenderMode) {
	w.renderMode = mode
}

// RenderWaveform converts waveform samples into particles positioned along the wave.
// The waveform spans the full width; amplitude maps to vertical position.
// Color shifts based on audio features.
func (w *Waveform) RenderWaveform(analyzer *audio.Analyzer) []particles.Particle {
	samples := analyzer.GetWaveform()
	if len(samples) == 0 {
		return nil
	}

	audioData := analyzer.GetData()
	subW, subH := particles.TermToSubCell(w.width, w.height, w.renderMode)

	// Reserve top 2 rows for title
	var topMargin int
	if w.renderMode == particles.RenderBraille {
		topMargin = 8 // 2 terminal rows * 4
	} else {
		topMargin = 4 // 2 terminal rows * 2
	}

	drawHeight := subH - topMargin
	centerY := float64(topMargin) + float64(drawHeight)/2.0

	// Pick color based on spectral centroid
	colorIdx := 0
	if len(w.colors) > 0 {
		if audioData.SpectralCentroid > 0.05 {
			colorIdx = int(audioData.SpectralCentroid * float64(len(w.colors)))
			if colorIdx >= len(w.colors) {
				colorIdx = len(w.colors) - 1
			}
		}
	}
	color := w.colors[colorIdx%len(w.colors)]

	// Secondary color for fills
	color2Idx := (colorIdx + len(w.colors)/3) % len(w.colors)
	color2 := w.colors[color2Idx]

	// Map samples across the sub-cell width
	result := make([]particles.Particle, 0, subW*2)

	for x := 0; x < subW; x++ {
		// Map x position to sample index
		sampleIdx := x * len(samples) / subW
		if sampleIdx >= len(samples) {
			sampleIdx = len(samples) - 1
		}

		sample := samples[sampleIdx]

		// Map sample amplitude (-1..1) to vertical position
		amplitude := float64(drawHeight) / 2.0 * 0.85
		y := centerY - sample*amplitude

		// Clamp
		if y < float64(topMargin) {
			y = float64(topMargin)
		}
		if y >= float64(subH-1) {
			y = float64(subH - 1)
		}

		// Main waveform particle
		result = append(result, particles.Particle{
			X:    float64(x),
			Y:    y,
			Life: 0.8 + audioData.Volume*0.2,
			Color: color,
			Char: '⠿',
		})

		// Fill between center line and waveform for a solid look
		step := 1.0
		if y < centerY {
			for fy := y + step; fy < centerY; fy += step * 2 {
				result = append(result, particles.Particle{
					X:    float64(x),
					Y:    fy,
					Life: 0.4,
					Color: color2,
					Char: '⠁',
				})
			}
		} else if y > centerY {
			for fy := centerY + step; fy < y; fy += step * 2 {
				result = append(result, particles.Particle{
					X:    float64(x),
					Y:    fy,
					Life: 0.4,
					Color: color2,
					Char: '⠁',
				})
			}
		}
	}

	return result
}
