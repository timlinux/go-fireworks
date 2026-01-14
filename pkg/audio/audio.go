// Package audio provides real-time audio analysis with FFT-based spectral features,
// beat detection, and tempo estimation. It captures system audio via PulseAudio.
package audio

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

// Data contains analyzed audio features suitable for driving visualizations
type Data struct {
	// Basic frequency bands (0-1 normalized)
	Bass float64 // Low frequencies (20-250 Hz)
	Mid  float64 // Mid frequencies (250-2000 Hz)
	High float64 // High frequencies (2000-20000 Hz)

	// Overall volume (0-1, adaptive normalized)
	Volume float64

	// Advanced spectral features (0-1 normalized)
	SpectralCentroid float64 // Brightness (low=dark/warm, high=bright/cool)
	SpectralFlux     float64 // Rate of change (high=dynamic)
	SpectralRolloff  float64 // Harshness (high=lots of high freq)

	// Temporal features (0-1 normalized)
	OnsetStrength float64 // Sudden event detection
	BeatStrength  float64 // Beat/pulse strength

	// Derived features
	Energy         float64 // Overall energy (0-1)
	RollingAverage float64 // Smoothed volume (0-1)

	// Beat synchronization
	IsBeat         bool    // True if this frame contains a beat
	BeatConfidence float64 // Confidence of beat detection (0-1)
	EstimatedBPM   float64 // Current estimated tempo
}

// Analyzer captures and analyzes system audio in real-time
type Analyzer struct {
	// Basic frequency bands
	bass float64
	mid  float64
	high float64

	// Advanced spectral features
	spectralCentroid float64
	spectralFlux     float64
	spectralRolloff  float64

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
	spectrumHistory []float64
	onsetHistory    []float64
	onsetEnvelope   []float64

	// Beat tracking
	estimatedTempo float64
	beatPhase      float64
	lastBeatTime   float64
	frameCount     int
	energyBuffer   []float64
	beatTimes      []float64
	isBeat         bool
	beatConfidence float64
	adaptiveThresh float64

	cmd         *exec.Cmd
	running     bool
	mu          sync.RWMutex
	enabled     bool
	errorMsg    string
	pacatPath   string
	deviceUsed  string
	sampleCount int
}

// NewAnalyzer creates a new audio analyzer
func NewAnalyzer() *Analyzer {
	return &Analyzer{
		enabled:         true,
		volumeHistory:   make([]float64, 0, 50),
		spectrumHistory: make([]float64, 0),
		onsetHistory:    make([]float64, 0, 10),
		onsetEnvelope:   make([]float64, 0, 200),
		energyBuffer:    make([]float64, 0, 500),
		beatTimes:       make([]float64, 0, 100),
		volumeMin:       math.MaxFloat64,
		volumeMax:       0.0,
		centroidMin:     math.MaxFloat64,
		centroidMax:     0.0,
		estimatedTempo:  120.0,
		beatPhase:       0.0,
		lastBeatTime:    0.0,
		frameCount:      0,
		isBeat:          false,
		beatConfidence:  0.0,
		adaptiveThresh:  0.5,
	}
}

// Start begins capturing and analyzing audio
// Returns nil even if audio is unavailable (graceful degradation)
func (a *Analyzer) Start() error {
	pacatPath, err := exec.LookPath("pacat")
	if err != nil {
		a.enabled = false
		a.errorMsg = fmt.Sprintf("pacat not found in PATH: %v", err)
		return nil
	}

	testCmd := exec.Command(pacatPath, "--version")
	if err := testCmd.Run(); err != nil {
		a.enabled = false
		a.errorMsg = fmt.Sprintf("pacat not working: %v", err)
		return nil
	}

	a.pacatPath = pacatPath
	a.running = true

	devices := a.detectMonitorDevices(pacatPath)

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

	a.enabled = true
	a.errorMsg = ""

	go func() {
		const (
			sampleRate     = 44100
			bufferSize     = 2048
			bytesPerSample = 2
		)

		buffer := make([]byte, bufferSize*bytesPerSample)

		for a.running {
			n, err := stdout.Read(buffer)
			if err != nil || n == 0 {
				continue
			}

			samples := make([]float64, n/bytesPerSample)
			buf := bytes.NewReader(buffer[:n])

			for i := range samples {
				var sample int16
				if err := binary.Read(buf, binary.LittleEndian, &sample); err != nil {
					break
				}
				samples[i] = float64(sample) / 32768.0
			}

			a.analyze(samples, sampleRate)
		}
	}()

	return nil
}

func (a *Analyzer) detectMonitorDevices(pacatPath string) []string {
	pactlPath := filepath.Join(filepath.Dir(pacatPath), "pactl")

	cmd := exec.Command(pactlPath, "list", "short", "sources")
	output, err := cmd.Output()
	if err != nil {
		return nil
	}

	var monitors []string
	lines := bytes.Split(output, []byte("\n"))
	for _, line := range lines {
		if bytes.Contains(line, []byte("monitor")) {
			fields := bytes.Fields(line)
			if len(fields) >= 2 {
				deviceName := string(fields[1])
				if bytes.Contains(line, []byte("RUNNING")) {
					monitors = append([]string{deviceName}, monitors...)
				} else {
					monitors = append(monitors, deviceName)
				}
			}
		}
	}

	return monitors
}

func (a *Analyzer) analyze(samples []float64, sampleRate int) {
	if len(samples) == 0 {
		return
	}

	// Calculate RMS
	rms := 0.0
	for _, s := range samples {
		rms += s * s
	}
	rms = math.Sqrt(rms / float64(len(samples)))

	// FFT setup
	n := len(samples)
	if n == 0 {
		return
	}

	fftSize := 1
	for fftSize < n {
		fftSize *= 2
	}

	complexSamples := make([]complex128, fftSize)
	for i := 0; i < n && i < fftSize; i++ {
		complexSamples[i] = complex(samples[i], 0)
	}

	// Hann window
	for i := range complexSamples[:n] {
		window := 0.5 * (1 - math.Cos(2*math.Pi*float64(i)/float64(n)))
		complexSamples[i] *= complex(window, 0)
	}

	fft := naiveFFT(complexSamples)

	spectrum := make([]float64, fftSize/2)
	freqRes := float64(sampleRate) / float64(fftSize)

	bass := 0.0
	mid := 0.0
	high := 0.0
	totalEnergy := 0.0
	weightedFreq := 0.0

	for i := 0; i < fftSize/2; i++ {
		freq := float64(i) * freqRes
		magnitude := cmplx.Abs(fft[i])
		spectrum[i] = magnitude
		totalEnergy += magnitude
		weightedFreq += magnitude * freq

		if freq < 250 {
			bass += magnitude
		} else if freq < 2000 {
			mid += magnitude
		} else if freq < 20000 {
			high += magnitude
		}
	}

	// Spectral centroid
	centroid := 0.0
	if totalEnergy > 0 {
		centroid = weightedFreq / totalEnergy
	}

	// Spectral rolloff
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

	// Spectral flux
	flux := 0.0
	if len(a.spectrumHistory) == len(spectrum) {
		for i := 0; i < len(spectrum); i++ {
			diff := spectrum[i] - a.spectrumHistory[i]
			if diff > 0 {
				flux += diff
			}
		}
	}
	a.spectrumHistory = make([]float64, len(spectrum))
	copy(a.spectrumHistory, spectrum)

	// Onset strength
	currentEnergy := totalEnergy
	onsetStrength := 0.0
	if len(a.onsetHistory) > 0 {
		avgPastEnergy := 0.0
		for _, e := range a.onsetHistory {
			avgPastEnergy += e
		}
		avgPastEnergy /= float64(len(a.onsetHistory))

		if currentEnergy > avgPastEnergy*1.5 {
			onsetStrength = (currentEnergy - avgPastEnergy) / (avgPastEnergy + 0.001)
		}
	}
	a.onsetHistory = append(a.onsetHistory, currentEnergy)
	if len(a.onsetHistory) > 10 {
		a.onsetHistory = a.onsetHistory[1:]
	}

	// Beat tracking
	normalizedOnset := math.Min(onsetStrength, 1.0)
	a.onsetEnvelope = append(a.onsetEnvelope, normalizedOnset)
	if len(a.onsetEnvelope) > 200 {
		a.onsetEnvelope = a.onsetEnvelope[1:]
	}

	a.energyBuffer = append(a.energyBuffer, currentEnergy)
	if len(a.energyBuffer) > 500 {
		a.energyBuffer = a.energyBuffer[1:]
	}

	a.frameCount++
	frameRate := 50.0
	currentTime := float64(a.frameCount) / frameRate

	if a.frameCount%25 == 0 && len(a.energyBuffer) > 100 {
		a.estimatedTempo = a.estimateTempoFromBeats()
	}

	isBeat, confidence := a.detectBeat(currentEnergy, normalizedOnset, currentTime)

	if isBeat {
		a.beatTimes = append(a.beatTimes, currentTime)
		if len(a.beatTimes) > 100 {
			a.beatTimes = a.beatTimes[1:]
		}
		a.lastBeatTime = currentTime
	}

	beatStrength := 0.0
	if isBeat {
		beatStrength = confidence
	}

	// Normalize features
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
		vol = math.Min(rms*10, 1.0)
	}

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
		normalizedCentroid = math.Min(centroid/10000.0, 1.0)
	}

	maxBandVal := 15000.0
	bass = math.Min(bass/maxBandVal, 1.0)
	mid = math.Min(mid/maxBandVal, 1.0)
	high = math.Min(high/maxBandVal, 1.0)

	normalizedRolloff := math.Min(rolloffFreq/float64(sampleRate/2), 1.0)
	normalizedFlux := math.Min(flux/5000.0, 1.0)

	energy := (bass*0.3 + mid*0.4 + high*0.3)
	energy = math.Min(energy, 1.0)

	a.mu.Lock()
	alphaFast := 0.6
	alphaMedium := 0.4
	alphaSlow := 0.2

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

	a.volumeHistory = append(a.volumeHistory, vol)
	if len(a.volumeHistory) > 50 {
		a.volumeHistory = a.volumeHistory[1:]
	}

	sum := 0.0
	for _, v := range a.volumeHistory {
		sum += v
	}
	a.rollingAverage = sum / float64(len(a.volumeHistory))

	a.mu.Unlock()
}

func (a *Analyzer) detectBeat(currentEnergy, onsetStrength, currentTime float64) (bool, float64) {
	if len(a.energyBuffer) < 20 {
		return false, 0.0
	}

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

	C := 1.5
	if stddev > mean*0.5 {
		C = 1.2
	} else if stddev < mean*0.2 {
		C = 2.0
	}

	threshold := mean + C*stddev
	a.adaptiveThresh = threshold

	minBeatInterval := 0.2

	isEnergyHigh := currentEnergy > threshold
	isLocalPeak := true
	if len(a.energyBuffer) > 3 {
		for i := len(a.energyBuffer) - 3; i < len(a.energyBuffer)-1; i++ {
			if currentEnergy <= a.energyBuffer[i] {
				isLocalPeak = false
				break
			}
		}
	}

	timeSinceLastBeat := currentTime - a.lastBeatTime
	notTooSoon := timeSinceLastBeat > minBeatInterval

	isBeat := isEnergyHigh && isLocalPeak && notTooSoon

	confidence := 0.0
	if isBeat && threshold > 0 {
		confidence = math.Min((currentEnergy-threshold)/threshold, 1.0)
		confidence = math.Min(confidence+onsetStrength*0.3, 1.0)
	}

	a.isBeat = isBeat
	a.beatConfidence = confidence

	return isBeat, confidence
}

func (a *Analyzer) estimateTempoFromBeats() float64 {
	if len(a.beatTimes) < 4 {
		return a.estimatedTempo
	}

	intervals := make([]float64, 0, len(a.beatTimes)-1)
	for i := 1; i < len(a.beatTimes); i++ {
		interval := a.beatTimes[i] - a.beatTimes[i-1]
		if interval > 0.3 && interval < 1.0 {
			intervals = append(intervals, interval)
		}
	}

	if len(intervals) == 0 {
		return a.estimatedTempo
	}

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
	bpm := 60.0 / medianInterval

	if bpm < 60.0 {
		bpm = 60.0
	}
	if bpm > 200.0 {
		bpm = 200.0
	}

	smoothing := 0.7
	smoothedBPM := smoothing*a.estimatedTempo + (1.0-smoothing)*bpm

	return smoothedBPM
}

// naiveFFT performs FFT using Cooley-Tukey algorithm
func naiveFFT(x []complex128) []complex128 {
	n := len(x)
	if n <= 1 {
		return x
	}

	even := make([]complex128, n/2)
	odd := make([]complex128, n/2)
	for i := 0; i < n/2; i++ {
		even[i] = x[i*2]
		odd[i] = x[i*2+1]
	}

	fftEven := naiveFFT(even)
	fftOdd := naiveFFT(odd)

	result := make([]complex128, n)
	for k := 0; k < n/2; k++ {
		t := cmplx.Exp(complex(0, -2*math.Pi*float64(k)/float64(n))) * fftOdd[k]
		result[k] = fftEven[k] + t
		result[k+n/2] = fftEven[k] - t
	}

	return result
}

// GetData returns the current audio analysis data
func (a *Analyzer) GetData() Data {
	if !a.enabled {
		return Data{}
	}

	a.mu.RLock()
	defer a.mu.RUnlock()

	return Data{
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
func (a *Analyzer) Stop() {
	a.running = false
	if a.cmd != nil && a.cmd.Process != nil {
		a.cmd.Process.Kill()
	}
}

// IsEnabled returns whether audio analysis is active
func (a *Analyzer) IsEnabled() bool {
	return a.enabled
}

// GetErrorMsg returns any error message from initialization
func (a *Analyzer) GetErrorMsg() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.errorMsg
}

// GetDebugInfo returns debug information about audio setup
func (a *Analyzer) GetDebugInfo() (pacatPath, device string) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.pacatPath, a.deviceUsed
}
