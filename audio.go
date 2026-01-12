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
	// Basic frequency bands
	bass float64
	mid  float64
	high float64

	// Advanced spectral features
	spectralCentroid float64 // Brightness of sound
	spectralFlux     float64 // Rate of change in spectrum
	spectralRolloff  float64 // Frequency containing 85% of energy

	// Temporal features
	volume         float64
	rms            float64
	onsetStrength  float64
	beatStrength   float64
	energy         float64
	rollingAverage float64

	// Adaptive normalization
	volumeMin   float64
	volumeMax   float64
	centroidMin float64
	centroidMax float64

	// History for analysis
	volumeHistory   []float64
	spectrumHistory []float64 // Previous spectrum for flux calculation
	onsetHistory    []float64
	onsetEnvelope   []float64 // Onset strength over time for beat tracking

	// Beat tracking state (improved)
	estimatedTempo   float64   // Estimated BPM
	beatPhase        float64   // Current position in beat cycle (0-1)
	lastBeatTime     float64   // Time of last detected beat
	frameCount       int       // Total frames processed
	energyBuffer     []float64 // 10s buffer of energy for beat detection
	beatTimes        []float64 // Timestamps of recent beats for tempo tracking
	isBeat           bool      // True if current frame is a beat
	beatConfidence   float64   // Confidence of beat detection (0-1)
	adaptiveThresh   float64   // Adaptive threshold for beat detection

	cmd         *exec.Cmd
	running     bool
	mu          sync.RWMutex
	enabled     bool
	errorMsg    string
	pacatPath   string
	deviceUsed  string
	sampleCount int // For adaptive normalization
}

// AudioData contains the analyzed audio features
type AudioData struct {
	// Basic features
	Bass   float64 // 0-1, low frequencies (20-250 Hz)
	Mid    float64 // 0-1, mid frequencies (250-2000 Hz)
	High   float64 // 0-1, high frequencies (2000-20000 Hz)
	Volume float64 // 0-1, overall volume (adaptive normalized)

	// Advanced spectral features
	SpectralCentroid float64 // 0-1, brightness (low=dark/warm, high=bright/cool)
	SpectralFlux     float64 // 0-1, rate of change (high=dynamic)
	SpectralRolloff  float64 // 0-1, harshness (high=lots of high freq)

	// Temporal features
	OnsetStrength float64 // 0-1, sudden event detection
	BeatStrength  float64 // 0-1, beat/pulse strength

	// Derived features
	Energy         float64 // 0-1, overall energy
	RollingAverage float64 // 0-1, smoothed volume

	// Beat synchronization
	IsBeat         bool    // True if this frame contains a beat
	BeatConfidence float64 // Confidence of beat detection (0-1)
	EstimatedBPM   float64 // Current estimated tempo
}

// NewAudioAnalyzer creates a new audio analyzer
func NewAudioAnalyzer() *AudioAnalyzer {
	return &AudioAnalyzer{
		enabled:         true,
		volumeHistory:   make([]float64, 0, 50),
		spectrumHistory: make([]float64, 0),
		onsetHistory:    make([]float64, 0, 10),
		onsetEnvelope:   make([]float64, 0, 200), // ~4 seconds at 50Hz
		energyBuffer:    make([]float64, 0, 500), // ~10 seconds at 50Hz
		beatTimes:       make([]float64, 0, 100), // Track last 100 beats
		volumeMin:       math.MaxFloat64,
		volumeMax:       0.0,
		centroidMin:     math.MaxFloat64,
		centroidMax:     0.0,
		estimatedTempo:  120.0, // Default 120 BPM
		beatPhase:       0.0,
		lastBeatTime:    0.0,
		frameCount:      0,
		isBeat:          false,
		beatConfidence:  0.0,
		adaptiveThresh:  0.5,
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
			sampleRate     = 44100
			bufferSize     = 2048
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

	// Build magnitude spectrum
	spectrum := make([]float64, fftSize/2)
	freqRes := float64(sampleRate) / float64(fftSize)

	// Calculate frequency band energies and advanced features
	bass := 0.0
	mid := 0.0
	high := 0.0
	totalEnergy := 0.0
	weightedFreq := 0.0 // For spectral centroid

	for i := 0; i < fftSize/2; i++ {
		freq := float64(i) * freqRes
		magnitude := cmplx.Abs(fft[i])
		spectrum[i] = magnitude
		totalEnergy += magnitude

		// Weighted frequency for centroid
		weightedFreq += magnitude * freq

		// Frequency bands
		if freq < 250 {
			bass += magnitude
		} else if freq < 2000 {
			mid += magnitude
		} else if freq < 20000 {
			high += magnitude
		}
	}

	// 1. SPECTRAL CENTROID - Brightness of sound
	centroid := 0.0
	if totalEnergy > 0 {
		centroid = weightedFreq / totalEnergy
	}

	// 2. SPECTRAL ROLLOFF - Frequency containing 85% of energy
	rolloffThreshold := totalEnergy * 0.85
	cumulativeEnergy := 0.0
	rolloffFreq := 0.0
	for i := 0; i < fftSize/2; i++ {
		cumulativeEnergy += spectrum[i]
		if cumulativeEnergy >= rolloffThreshold {
			rolloffFreq = float64(i) * freqRes
			break
		}
	}

	// 3. SPECTRAL FLUX - Change in spectrum over time
	flux := 0.0
	if len(a.spectrumHistory) == len(spectrum) {
		for i := 0; i < len(spectrum); i++ {
			diff := spectrum[i] - a.spectrumHistory[i]
			if diff > 0 { // Only positive changes
				flux += diff
			}
		}
	}
	// Update spectrum history
	a.spectrumHistory = make([]float64, len(spectrum))
	copy(a.spectrumHistory, spectrum)

	// 4. ONSET STRENGTH - Detect sudden increases in energy
	currentEnergy := totalEnergy
	onsetStrength := 0.0
	if len(a.onsetHistory) > 0 {
		// Compare to recent average
		avgPastEnergy := 0.0
		for _, e := range a.onsetHistory {
			avgPastEnergy += e
		}
		avgPastEnergy /= float64(len(a.onsetHistory))

		// Onset if current energy significantly exceeds average
		if currentEnergy > avgPastEnergy*1.5 {
			onsetStrength = (currentEnergy - avgPastEnergy) / (avgPastEnergy + 0.001)
		}
	}
	// Update onset history
	a.onsetHistory = append(a.onsetHistory, currentEnergy)
	if len(a.onsetHistory) > 10 {
		a.onsetHistory = a.onsetHistory[1:]
	}

	// 5. BEAT TRACKING - Improved with 10s buffer and adaptive threshold
	// Add current onset strength to envelope history
	normalizedOnset := math.Min(onsetStrength, 1.0)
	a.onsetEnvelope = append(a.onsetEnvelope, normalizedOnset)
	if len(a.onsetEnvelope) > 200 { // Keep ~4 seconds at 50Hz
		a.onsetEnvelope = a.onsetEnvelope[1:]
	}

	// Add current energy to 10s buffer for beat detection
	a.energyBuffer = append(a.energyBuffer, currentEnergy)
	if len(a.energyBuffer) > 500 { // Keep ~10 seconds at 50Hz
		a.energyBuffer = a.energyBuffer[1:]
	}

	// Increment frame counter
	a.frameCount++
	frameRate := 50.0 // Hz
	currentTime := float64(a.frameCount) / frameRate

	// Estimate tempo every 25 frames (~0.5 second) for better adaptation
	if a.frameCount%25 == 0 && len(a.energyBuffer) > 100 {
		a.estimatedTempo = a.estimateTempoFromBeats()
	}

	// DETECT ACTUAL BEATS using adaptive threshold
	isBeat, confidence := a.detectBeat(currentEnergy, normalizedOnset, currentTime)

	// If beat detected, add to beat times
	if isBeat {
		a.beatTimes = append(a.beatTimes, currentTime)
		// Keep only last 100 beats (or ~2 minutes at 120 BPM)
		if len(a.beatTimes) > 100 {
			a.beatTimes = a.beatTimes[1:]
		}
		a.lastBeatTime = currentTime
	}

	// Calculate beat strength for visual feedback
	beatStrength := 0.0
	if isBeat {
		beatStrength = confidence
	}

	// Normalize features
	// Adaptive normalization for volume
	a.sampleCount++
	if rms < a.volumeMin {
		a.volumeMin = rms
	}
	if rms > a.volumeMax {
		a.volumeMax = rms
	}
	vol := 0.0
	if a.volumeMax > a.volumeMin && a.sampleCount > 10 {
		vol = (rms - a.volumeMin) / (a.volumeMax - a.volumeMin)
		vol = math.Max(0, math.Min(vol, 1.0))
	} else {
		vol = math.Min(rms*10, 1.0) // Fallback
	}

	// Adaptive normalization for centroid
	if centroid < a.centroidMin {
		a.centroidMin = centroid
	}
	if centroid > a.centroidMax {
		a.centroidMax = centroid
	}
	normalizedCentroid := 0.0
	if a.centroidMax > a.centroidMin && a.sampleCount > 10 {
		normalizedCentroid = (centroid - a.centroidMin) / (a.centroidMax - a.centroidMin)
		normalizedCentroid = math.Max(0, math.Min(normalizedCentroid, 1.0))
	} else {
		normalizedCentroid = math.Min(centroid/10000.0, 1.0) // Fallback
	}

	// Normalize other features
	maxBandVal := 15000.0
	bass = math.Min(bass/maxBandVal, 1.0)
	mid = math.Min(mid/maxBandVal, 1.0)
	high = math.Min(high/maxBandVal, 1.0)

	normalizedRolloff := math.Min(rolloffFreq/float64(sampleRate/2), 1.0)
	normalizedFlux := math.Min(flux/5000.0, 1.0)

	// normalizedOnset := math.Min(onsetStrength, 1.0)

	energy := (bass*0.3 + mid*0.4 + high*0.3)
	energy = math.Min(energy, 1.0)

	a.mu.Lock()
	// Less aggressive smoothing for more responsiveness
	alphaFast := 0.6   // For fast-changing features (onset, beat, flux)
	alphaMedium := 0.4 // For medium features (volume, bands)
	alphaSlow := 0.2   // For slow features (centroid, rolloff)

	a.bass = alphaMedium*bass + (1-alphaMedium)*a.bass
	a.mid = alphaMedium*mid + (1-alphaMedium)*a.mid
	a.high = alphaMedium*high + (1-alphaMedium)*a.high
	a.volume = alphaMedium*vol + (1-alphaMedium)*a.volume
	a.energy = alphaMedium*energy + (1-alphaMedium)*a.energy

	a.spectralCentroid = alphaSlow*normalizedCentroid + (1-alphaSlow)*a.spectralCentroid
	a.spectralRolloff = alphaSlow*normalizedRolloff + (1-alphaSlow)*a.spectralRolloff

	a.spectralFlux = alphaFast*normalizedFlux + (1-alphaFast)*a.spectralFlux
	a.onsetStrength = alphaFast*normalizedOnset + (1-alphaFast)*a.onsetStrength
	a.beatStrength = alphaFast*beatStrength + (1-alphaFast)*a.beatStrength

	a.rms = rms

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

// detectBeat detects if current frame contains a beat using adaptive threshold
func (a *AudioAnalyzer) detectBeat(currentEnergy, onsetStrength, currentTime float64) (bool, float64) {
	// Need sufficient history
	if len(a.energyBuffer) < 20 {
		return false, 0.0
	}

	// Calculate mean and variance of energy over buffer
	mean := 0.0
	for _, e := range a.energyBuffer {
		mean += e
	}
	mean /= float64(len(a.energyBuffer))

	variance := 0.0
	for _, e := range a.energyBuffer {
		diff := e - mean
		variance += diff * diff
	}
	variance /= float64(len(a.energyBuffer))
	stddev := math.Sqrt(variance)

	// Adaptive threshold: mean + C * stddev
	// C is dynamic based on recent variation
	C := 1.5 // Base multiplier
	if stddev > mean*0.5 {
		// High variation - lower threshold to catch beats
		C = 1.2
	} else if stddev < mean*0.2 {
		// Low variation - higher threshold to avoid false positives
		C = 2.0
	}

	threshold := mean + C*stddev
	a.adaptiveThresh = threshold // Store for debugging

	// Beat conditions:
	// 1. Energy exceeds adaptive threshold
	// 2. Local peak (higher than recent frames)
	// 3. Minimum time since last beat (refractory period)
	minBeatInterval := 0.2 // 200ms = max 300 BPM

	isEnergyHigh := currentEnergy > threshold
	isLocalPeak := true
	if len(a.energyBuffer) > 3 {
		// Check if current energy is higher than last 3 frames (local peak)
		for i := len(a.energyBuffer) - 3; i < len(a.energyBuffer)-1; i++ {
			if currentEnergy <= a.energyBuffer[i] {
				isLocalPeak = false
				break
			}
		}
	}

	timeSinceLastBeat := currentTime - a.lastBeatTime
	notTooSoon := timeSinceLastBeat > minBeatInterval

	// Combine conditions
	isBeat := isEnergyHigh && isLocalPeak && notTooSoon

	// Calculate confidence based on how much we exceed threshold
	confidence := 0.0
	if isBeat && threshold > 0 {
		confidence = math.Min((currentEnergy-threshold)/threshold, 1.0)
		// Boost confidence with onset strength
		confidence = math.Min(confidence+onsetStrength*0.3, 1.0)
	}

	// Store beat state
	a.isBeat = isBeat
	a.beatConfidence = confidence

	return isBeat, confidence
}

// estimateTempoFromBeats estimates tempo from actual detected beat times
func (a *AudioAnalyzer) estimateTempoFromBeats() float64 {
	// Need at least 4 beats to estimate tempo
	if len(a.beatTimes) < 4 {
		return a.estimatedTempo // Keep previous estimate
	}

	// Calculate intervals between consecutive beats
	intervals := make([]float64, 0, len(a.beatTimes)-1)
	for i := 1; i < len(a.beatTimes); i++ {
		interval := a.beatTimes[i] - a.beatTimes[i-1]
		// Filter out unreasonable intervals (outside 60-200 BPM range)
		if interval > 0.3 && interval < 1.0 { // 60-200 BPM
			intervals = append(intervals, interval)
		}
	}

	if len(intervals) == 0 {
		return a.estimatedTempo
	}

	// Use median interval (more robust than mean)
	// Sort intervals
	sorted := make([]float64, len(intervals))
	copy(sorted, intervals)
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i] > sorted[j] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	medianInterval := sorted[len(sorted)/2]

	// Convert to BPM
	bpm := 60.0 / medianInterval

	// Clamp to reasonable range
	if bpm < 60.0 {
		bpm = 60.0
	}
	if bpm > 200.0 {
		bpm = 200.0
	}

	// Smooth with previous estimate (prevent jumps)
	smoothing := 0.7
	smoothedBPM := smoothing*a.estimatedTempo + (1.0-smoothing)*bpm

	return smoothedBPM
}

// estimateTempoAutocorrelation estimates tempo using autocorrelation on onset envelope
func (a *AudioAnalyzer) estimateTempoAutocorrelation() float64 {
	if len(a.onsetEnvelope) < 50 {
		return 120.0 // Default
	}

	// Autocorrelation: find periodicity in onset envelope
	// Search for tempo between 60-180 BPM
	frameRate := 50.0 // Hz (our update rate)
	minBPM := 60.0
	maxBPM := 180.0

	// Convert BPM range to frame lags
	maxLag := int((60.0 / minBPM) * frameRate) // Frames for slowest tempo
	minLag := int((60.0 / maxBPM) * frameRate) // Frames for fastest tempo

	if maxLag >= len(a.onsetEnvelope) {
		maxLag = len(a.onsetEnvelope) - 1
	}
	if minLag < 2 {
		minLag = 2
	}

	// Calculate autocorrelation for each lag
	maxCorrelation := 0.0
	bestLag := minLag

	for lag := minLag; lag <= maxLag; lag++ {
		correlation := 0.0
		count := 0

		// Calculate correlation at this lag
		for i := 0; i < len(a.onsetEnvelope)-lag; i++ {
			correlation += a.onsetEnvelope[i] * a.onsetEnvelope[i+lag]
			count++
		}

		if count > 0 {
			correlation /= float64(count)
		}

		// Find maximum correlation
		if correlation > maxCorrelation {
			maxCorrelation = correlation
			bestLag = lag
		}
	}

	// Convert best lag to BPM
	if bestLag > 0 {
		beatInterval := float64(bestLag) / frameRate // seconds per beat
		estimatedBPM := 60.0 / beatInterval

		// Clamp to reasonable range
		if estimatedBPM < minBPM {
			estimatedBPM = minBPM
		}
		if estimatedBPM > maxBPM {
			estimatedBPM = maxBPM
		}

		return estimatedBPM
	}

	return 120.0 // Fallback
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
		Bass:             a.bass,
		Mid:              a.mid,
		High:             a.high,
		Volume:           a.volume,
		SpectralCentroid: a.spectralCentroid,
		SpectralFlux:     a.spectralFlux,
		SpectralRolloff:  a.spectralRolloff,
		OnsetStrength:    a.onsetStrength,
		BeatStrength:     a.beatStrength,
		Energy:           a.energy,
		RollingAverage:   a.rollingAverage,
		IsBeat:           a.isBeat,
		BeatConfidence:   a.beatConfidence,
		EstimatedBPM:     a.estimatedTempo,
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
