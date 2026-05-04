package particles

// RenderMode determines which character set is used for sub-cell rendering
type RenderMode int

const (
	// RenderBraille uses braille characters (2x4 dots per terminal cell)
	// Effective resolution: width*2 x height*4
	RenderBraille RenderMode = iota

	// RenderQuarterBlock uses quarter block characters (2x2 quadrants per terminal cell)
	// Effective resolution: width*2 x height*2
	RenderQuarterBlock
)

// RenderedCell represents a single terminal cell with its character and color
type RenderedCell struct {
	Char  rune
	Color Color
	Life  float64 // Brightest particle's life in this cell (for styling)
}

// Renderer composites particles into terminal cells using sub-cell resolution
type Renderer struct {
	Mode   RenderMode
	Width  int // Terminal width in cells
	Height int // Terminal height in cells
}

// NewRenderer creates a renderer with the specified mode and dimensions
func NewRenderer(mode RenderMode, width, height int) *Renderer {
	return &Renderer{
		Mode:   mode,
		Width:  width,
		Height: height,
	}
}

// SubCellWidth returns the effective horizontal resolution
func (r *Renderer) SubCellWidth() int {
	return r.Width * 2
}

// SubCellHeight returns the effective vertical resolution
func (r *Renderer) SubCellHeight() int {
	switch r.Mode {
	case RenderBraille:
		return r.Height * 4
	case RenderQuarterBlock:
		return r.Height * 2
	default:
		return r.Height * 4
	}
}

// RenderParticles composites all active particles into a grid of terminal cells.
// The particles' X,Y coordinates are treated as sub-cell coordinates.
// Returns a map of (cellX, cellY) -> RenderedCell for non-empty cells.
func (r *Renderer) RenderParticles(particles []Particle) map[[2]int]RenderedCell {
	cells := make(map[[2]int]RenderedCell)

	// Track which sub-dots are set per cell, and the dominant color
	type cellData struct {
		dots  uint8   // bitmask of active sub-dots
		color Color   // color of brightest particle
		life  float64 // brightest life value
	}
	grid := make(map[[2]int]*cellData)

	for _, p := range particles {
		if p.Life <= 0 {
			continue
		}

		// Convert particle position to cell and sub-position
		cellX, cellY, dotIndex := r.particleToDot(p.X, p.Y)

		// Bounds check
		if cellX < 0 || cellX >= r.Width || cellY < 0 || cellY >= r.Height {
			continue
		}

		key := [2]int{cellX, cellY}
		data, exists := grid[key]
		if !exists {
			data = &cellData{}
			grid[key] = data
		}

		// Set the sub-dot
		data.dots |= 1 << dotIndex

		// Keep the brightest particle's color
		if p.Life > data.life {
			data.life = p.Life
			data.color = p.Color
		}
	}

	// Convert grid data to rendered cells
	for key, data := range grid {
		var ch rune
		switch r.Mode {
		case RenderBraille:
			ch = dotsTobraille(data.dots)
		case RenderQuarterBlock:
			ch = dotsToQuarterBlock(data.dots)
		}

		cells[key] = RenderedCell{
			Char:  ch,
			Color: data.color,
			Life:  data.life,
		}
	}

	return cells
}

// particleToDot converts a particle's float position to a cell coordinate and dot index.
// For braille (2x4): dot indices are 0-7, laid out as:
//
//	col0 col1
//	 0    1     row 0
//	 2    3     row 1
//	 4    5     row 2
//	 6    7     row 3
//
// For quarter block (2x2): dot indices are 0-3, laid out as:
//
//	col0 col1
//	 0    1     row 0
//	 2    3     row 1
func (r *Renderer) particleToDot(x, y float64) (cellX, cellY int, dotIndex uint8) {
	switch r.Mode {
	case RenderBraille:
		// Sub-cell coordinate
		subX := int(x)
		subY := int(y)

		cellX = subX / 2
		cellY = subY / 4

		localCol := subX % 2
		localRow := subY % 4

		if localCol < 0 {
			localCol += 2
			cellX--
		}
		if localRow < 0 {
			localRow += 4
			cellY--
		}

		dotIndex = uint8(localRow*2 + localCol)

	case RenderQuarterBlock:
		subX := int(x)
		subY := int(y)

		cellX = subX / 2
		cellY = subY / 2

		localCol := subX % 2
		localRow := subY % 2

		if localCol < 0 {
			localCol += 2
			cellX--
		}
		if localRow < 0 {
			localRow += 2
			cellY--
		}

		dotIndex = uint8(localRow*2 + localCol)
	}

	return
}

// dotsTobraille converts a bitmask of 8 dots to a braille Unicode character.
// Braille Unicode encoding (U+2800 base):
//
//	Dot positions:    Bit values:
//	 1  4              0  3
//	 2  5              1  4
//	 3  6              2  5
//	 7  8              6  7
//
// Our layout (row-major, col0 first):
//
//	col0 col1
//	 0    1     row 0
//	 2    3     row 1
//	 4    5     row 2
//	 6    7     row 3
func dotsTobraille(dots uint8) rune {
	// Map our dot indices to braille bit positions
	// Our index -> braille bit
	// 0 (r0,c0) -> bit 0 (dot 1)
	// 1 (r0,c1) -> bit 3 (dot 4)
	// 2 (r1,c0) -> bit 1 (dot 2)
	// 3 (r1,c1) -> bit 4 (dot 5)
	// 4 (r2,c0) -> bit 2 (dot 3)
	// 5 (r2,c1) -> bit 5 (dot 6)
	// 6 (r3,c0) -> bit 6 (dot 7)
	// 7 (r3,c1) -> bit 7 (dot 8)
	var brailleBits uint8
	if dots&(1<<0) != 0 {
		brailleBits |= 1 << 0
	}
	if dots&(1<<1) != 0 {
		brailleBits |= 1 << 3
	}
	if dots&(1<<2) != 0 {
		brailleBits |= 1 << 1
	}
	if dots&(1<<3) != 0 {
		brailleBits |= 1 << 4
	}
	if dots&(1<<4) != 0 {
		brailleBits |= 1 << 2
	}
	if dots&(1<<5) != 0 {
		brailleBits |= 1 << 5
	}
	if dots&(1<<6) != 0 {
		brailleBits |= 1 << 6
	}
	if dots&(1<<7) != 0 {
		brailleBits |= 1 << 7
	}
	return rune(0x2800 + int(brailleBits))
}

// dotsToQuarterBlock converts a bitmask of 4 dots to a quarter block Unicode character.
// Layout:
//
//	col0 col1
//	 0    1     row 0 (top)
//	 2    3     row 1 (bottom)
//
// Characters:
//
//	0000 = ' '  (space)
//	0001 = '▘'  top-left
//	0010 = '▝'  top-right
//	0011 = '▀'  top
//	0100 = '▖'  bottom-left
//	0101 = '▌'  left
//	0110 = '▞'  diagonal (bottom-left + top-right)
//	0111 = '▛'  top + bottom-left
//	1000 = '▗'  bottom-right
//	1001 = '▚'  diagonal (top-left + bottom-right)
//	1010 = '▐'  right
//	1011 = '▜'  top + bottom-right
//	1100 = '▄'  bottom
//	1101 = '▙'  bottom + top-left
//	1110 = '▟'  bottom + top-right
//	1111 = '█'  full
func dotsToQuarterBlock(dots uint8) rune {
	// Use only lower 4 bits
	index := dots & 0x0F
	quarterBlocks := [16]rune{
		' ', '▘', '▝', '▀',
		'▖', '▌', '▞', '▛',
		'▗', '▚', '▐', '▜',
		'▄', '▙', '▟', '█',
	}
	return quarterBlocks[index]
}
