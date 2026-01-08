package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"math/cmplx"
	"os/exec"
	"path/filepath"
	"sync"
)

// AudioAnalyzer captures and analyzes system audio
type AudioAnalyzer struct {
	bass           float64
	mid            float64
	high           float64
	volume         float64
	pitch          float64
	rollingAverage float64
	energy         float64
	volumeHistory  []float64
	cmd            *exec.Cmd
	running        bool
	mu             sync.RWMutex
	enabled        bool
	errorMsg       string
	pacatPath      string
	deviceUsed     string
}

// AudioData contains the analyzed audio features
type AudioData struct {
	Bass           float64 // 0-1, low frequencies (20-250 Hz)
	Mid            float64 // 0-1, mid frequencies (250-2000 Hz)
	High           float64 // 0-1, high frequencies (2000-20000 Hz)
	Volume         float64 // 0-1, overall volume (instant)
	Pitch          float64 // 0-1, normalized dominant frequency
	RollingAverage float64 // 0-1, smoothed volume over time
	Energy         float64 // 0-1, combined energy across all frequencies
}

// NewAudioAnalyzer creates a new audio analyzer
func NewAudioAnalyzer() *AudioAnalyzer {
	return &AudioAnalyzer{
		enabled:       true,
		volumeHistory: make([]float64, 0, 50), // Keep 50 samples for rolling average
	}
}

// Start begins capturing and analyzing audio
func (a *AudioAnalyzer) Start() error {
	// Look up pacat in PATH (which is set by the Nix wrapper)
	pacatPath, err := exec.LookPath("pacat")
	if err != nil {
		a.enabled = false
		a.errorMsg = fmt.Sprintf("pacat not found in PATH: %v", err)
		return nil
	}

	// Verify it works
	testCmd := exec.Command(pacatPath, "--version")
	if err := testCmd.Run(); err != nil {
		a.enabled = false
		a.errorMsg = fmt.Sprintf("pacat not working: %v", err)
		return nil
	}

	a.pacatPath = pacatPath
	a.running = true

	// Try to auto-detect running monitor sources
	devices := a.detectMonitorDevices(pacatPath)

	// Fallback to common names if detection fails
	if len(devices) == 0 {
		devices = []string{
			"@DEFAULT_MONITOR@",
			"@DEFAULT_SINK@.monitor",
		}
		a.errorMsg = "Auto-detection failed, trying defaults"
	} else {
		a.errorMsg = fmt.Sprintf("Found %d monitors", len(devices))
	}

	var stdout io.ReadCloser
	var lastErr error
	deviceTried := ""

	for _, device := range devices {
		deviceTried = device
		// Start audio capture using pacat
		a.cmd = exec.Command(pacatPath, "--record", "--rate=44100", "--channels=1", "--format=s16le", "-d", device)

		var err error
		stdout, err = a.cmd.StdoutPipe()
		if err != nil {
			lastErr = err
			continue
		}

		if err := a.cmd.Start(); err != nil {
			lastErr = err
			stdout = nil
			continue
		}

		// Success! Break out of loop
		lastErr = nil
		a.deviceUsed = device
		break
	}

	if stdout == nil {
		a.enabled = false
		if lastErr != nil {
			a.errorMsg = fmt.Sprintf("All %d devices failed. Last: %s - %v", len(devices), deviceTried, lastErr)
		} else {
			a.errorMsg = fmt.Sprintf("No audio devices found (tried %d)", len(devices))
		}
		return nil
	}

	// Successfully started!
	a.enabled = true
	a.errorMsg = "" // Clear any previous errors

	go func() {
		const (
			sampleRate = 44100
			bufferSize = 2048
			bytesPerSample = 2 // 16-bit samples
		)

		buffer := make([]byte, bufferSize*bytesPerSample)

		for a.running {
			n, err := stdout.Read(buffer)
			if err != nil || n == 0 {
				continue
			}

			// Convert bytes to samples
			samples := make([]float64, n/bytesPerSample)
			buf := bytes.NewReader(buffer[:n])

			for i := range samples {
				var sample int16
				if err := binary.Read(buf, binary.LittleEndian, &sample); err != nil {
					break
				}
				samples[i] = float64(sample) / 32768.0
			}

			// Perform FFT and analyze
			a.analyze(samples, sampleRate)
		}
	}()

	return nil
}

// detectMonitorDevices tries to auto-detect available monitor sources
func (a *AudioAnalyzer) detectMonitorDevices(pacatPath string) []string {
	// Derive pactl path from pacat path (they're in the same directory)
	pactlPath := filepath.Join(filepath.Dir(pacatPath), "pactl")

	// Try to list monitor sources
	cmd := exec.Command(pactlPath, "list", "short", "sources")
	output, err := cmd.Output()
	if err != nil {
		return nil
	}

	var monitors []string
	lines := bytes.Split(output, []byte("\n"))
	for _, line := range lines {
		// Look for monitor sources
		if bytes.Contains(line, []byte("monitor")) {
			fields := bytes.Fields(line)
			if len(fields) >= 2 {
				deviceName := string(fields[1])
				// Prefer RUNNING monitors, but include all
				if bytes.Contains(line, []byte("RUNNING")) {
					// Insert at beginning (prioritize running)
					monitors = append([]string{deviceName}, monitors...)
				} else {
					monitors = append(monitors, deviceName)
				}
			}
		}
	}

	return monitors
}

// analyze performs FFT and extracts frequency band energies
func (a *AudioAnalyzer) analyze(samples []float64, sampleRate int) {
	if len(samples) == 0 {
		return
	}

	// Calculate RMS for volume
	rms := 0.0
	for _, s := range samples {
		rms += s * s
	}
	rms = math.Sqrt(rms / float64(len(samples)))

	// Perform FFT
	n := len(samples)
	if n == 0 {
		return
	}

	// Pad to power of 2 for FFT
	fftSize := 1
	for fftSize < n {
		fftSize *= 2
	}

	// Convert to complex
	complexSamples := make([]complex128, fftSize)
	for i := 0; i < n && i < fftSize; i++ {
		complexSamples[i] = complex(samples[i], 0)
	}

	// Apply Hann window
	for i := range complexSamples[:n] {
		window := 0.5 * (1 - math.Cos(2*math.Pi*float64(i)/float64(n)))
		complexSamples[i] *= complex(window, 0)
	}

	// Perform FFT
	fft := naiveFFT(complexSamples)

	// Calculate frequency band energies and find dominant frequency
	bass := 0.0
	mid := 0.0
	high := 0.0
	maxMagnitude := 0.0
	dominantFreq := 0.0

	// Frequency resolution
	freqRes := float64(sampleRate) / float64(fftSize)

	for i := 0; i < fftSize/2; i++ {
		freq := float64(i) * freqRes
		magnitude := cmplx.Abs(fft[i])

		// Track dominant frequency (skip DC component at i=0)
		if i > 0 && magnitude > maxMagnitude && freq < 10000 {
			maxMagnitude = magnitude
			dominantFreq = freq
		}

		if freq < 250 {
			bass += magnitude
		} else if freq < 2000 {
			mid += magnitude
		} else if freq < 20000 {
			high += magnitude
		}
	}

	// Normalize frequency bands
	maxVal := 10000.0
	bass = math.Min(bass/maxVal, 1.0)
	mid = math.Min(mid/maxVal, 1.0)
	high = math.Min(high/maxVal, 1.0)
	vol := math.Min(rms*10, 1.0)

	// Normalize pitch to 0-1 (map 20Hz-2000Hz to 0-1)
	pitch := 0.0
	if dominantFreq > 20 {
		pitch = math.Min((dominantFreq-20.0)/1980.0, 1.0)
	}

	// Calculate overall energy (weighted combination of all frequencies)
	energy := (bass*0.3 + mid*0.4 + high*0.3)
	energy = math.Min(energy, 1.0)

	a.mu.Lock()
	// Smooth the values with exponential moving average
	alpha := 0.3
	a.bass = alpha*bass + (1-alpha)*a.bass
	a.mid = alpha*mid + (1-alpha)*a.mid
	a.high = alpha*high + (1-alpha)*a.high
	a.volume = alpha*vol + (1-alpha)*a.volume
	a.pitch = alpha*pitch + (1-alpha)*a.pitch
	a.energy = alpha*energy + (1-alpha)*a.energy

	// Update rolling average
	a.volumeHistory = append(a.volumeHistory, vol)
	if len(a.volumeHistory) > 50 {
		a.volumeHistory = a.volumeHistory[1:]
	}

	// Calculate rolling average
	sum := 0.0
	for _, v := range a.volumeHistory {
		sum += v
	}
	a.rollingAverage = sum / float64(len(a.volumeHistory))

	a.mu.Unlock()
}

// naiveFFT performs a simple FFT (Cooley-Tukey algorithm)
func naiveFFT(x []complex128) []complex128 {
	n := len(x)
	if n <= 1 {
		return x
	}

	// Divide
	even := make([]complex128, n/2)
	odd := make([]complex128, n/2)
	for i := 0; i < n/2; i++ {
		even[i] = x[i*2]
		odd[i] = x[i*2+1]
	}

	// Conquer
	fftEven := naiveFFT(even)
	fftOdd := naiveFFT(odd)

	// Combine
	result := make([]complex128, n)
	for k := 0; k < n/2; k++ {
		t := cmplx.Exp(complex(0, -2*math.Pi*float64(k)/float64(n))) * fftOdd[k]
		result[k] = fftEven[k] + t
		result[k+n/2] = fftEven[k] - t
	}

	return result
}

// GetData returns the current audio analysis data
func (a *AudioAnalyzer) GetData() AudioData {
	if !a.enabled {
		return AudioData{}
	}

	a.mu.RLock()
	defer a.mu.RUnlock()

	return AudioData{
		Bass:           a.bass,
		Mid:            a.mid,
		High:           a.high,
		Volume:         a.volume,
		Pitch:          a.pitch,
		RollingAverage: a.rollingAverage,
		Energy:         a.energy,
	}
}

// Stop stops the audio analyzer
func (a *AudioAnalyzer) Stop() {
	a.running = false
	if a.cmd != nil && a.cmd.Process != nil {
		a.cmd.Process.Kill()
	}
}

// IsEnabled returns whether audio analysis is enabled
func (a *AudioAnalyzer) IsEnabled() bool {
	return a.enabled
}

// GetErrorMsg returns the error message if audio failed to initialize
func (a *AudioAnalyzer) GetErrorMsg() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.errorMsg
}

// GetDebugInfo returns debug information about audio setup
func (a *AudioAnalyzer) GetDebugInfo() (pacatPath, device string) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.pacatPath, a.deviceUsed
}
