package monitor

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"bandgo/utils"

	tea "github.com/charmbracelet/bubbletea"
)

const progressBarWidth = 20

type WorkerSnapshot struct {
	ID            int
	Downloaded    uint64
	ContentLength int64
	SpeedBps      float64
}

type Aggregator struct {
	mu      sync.RWMutex
	total   map[int]uint64
	window  map[int]uint64
	length  map[int]int64
	speed   map[int]float64
	started time.Time
}

func NewAggregator() *Aggregator {
	return &Aggregator{
		total:   make(map[int]uint64),
		window:  make(map[int]uint64),
		length:  make(map[int]int64),
		speed:   make(map[int]float64),
		started: time.Now(),
	}
}

func (a *Aggregator) RegisterWorker(workerID int) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if _, ok := a.total[workerID]; !ok {
		a.total[workerID] = 0
		a.window[workerID] = 0
		a.length[workerID] = -1
		a.speed[workerID] = 0
	}
}

func (a *Aggregator) SetContentLength(workerID int, contentLength int64) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if _, ok := a.total[workerID]; !ok {
		a.total[workerID] = 0
		a.window[workerID] = 0
		a.speed[workerID] = 0
	}
	a.length[workerID] = contentLength
}

func (a *Aggregator) AddDownloaded(workerID int, n int) {
	if n <= 0 {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()

	if _, ok := a.total[workerID]; !ok {
		a.total[workerID] = 0
		a.window[workerID] = 0
		a.length[workerID] = -1
		a.speed[workerID] = 0
	}
	a.total[workerID] += uint64(n)
	a.window[workerID] += uint64(n)
}

func (a *Aggregator) Tick(interval time.Duration) {
	seconds := interval.Seconds()
	if seconds <= 0 {
		return
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	for id := range a.window {
		a.speed[id] = float64(a.window[id]) / seconds
		a.window[id] = 0
	}
}

func (a *Aggregator) Elapsed() time.Duration {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return time.Since(a.started)
}

func (a *Aggregator) Snapshot() []WorkerSnapshot {
	a.mu.RLock()
	defer a.mu.RUnlock()

	workers := make([]WorkerSnapshot, 0, len(a.total))
	for id := range a.total {
		workers = append(workers, WorkerSnapshot{
			ID:            id,
			Downloaded:    a.total[id],
			ContentLength: a.length[id],
			SpeedBps:      a.speed[id],
		})
	}

	sort.Slice(workers, func(i, j int) bool {
		return workers[i].ID < workers[j].ID
	})

	return workers
}

func (a *Aggregator) Totals() (downloaded uint64, speedBps float64) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	for id := range a.total {
		downloaded += a.total[id]
		speedBps += a.speed[id]
	}

	return downloaded, speedBps
}

type tickMsg time.Time

func scheduleTick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

type tuiModel struct {
	target     string
	agg        *Aggregator
	concurrent int
}

func NewTUIModel(target string, concurrent int, agg *Aggregator) tea.Model {
	return tuiModel{
		target:     target,
		agg:        agg,
		concurrent: concurrent,
	}
}

func (m tuiModel) Init() tea.Cmd {
	return scheduleTick()
}

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "q" || msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
	case tickMsg:
		m.agg.Tick(time.Second)
		return m, scheduleTick()
	}

	return m, nil
}

func (m tuiModel) View() string {
	var b strings.Builder
	totalDownloaded, totalSpeed := m.agg.Totals()

	b.WriteString("BandGo Worker Monitor\n")
	b.WriteString(fmt.Sprintf("Target: %s\n", m.target))
	b.WriteString(fmt.Sprintf("Elapsed: %s\n", m.agg.Elapsed().Round(time.Second)))
	b.WriteString(fmt.Sprintf("Configured Workers: %d\n\n", m.concurrent))
	b.WriteString(fmt.Sprintf("Total Speed: %s/s\n", utils.ReadableBytes(totalSpeed)))
	b.WriteString(fmt.Sprintf("Total Downloaded: %s\n\n", utils.ReadableBytes(float64(totalDownloaded))))

	workers := m.agg.Snapshot()
	if len(workers) == 0 {
		b.WriteString("Waiting for workers to register...\n")
		return b.String()
	}

	for _, w := range workers {
		bar := progressBar(w.Downloaded, w.ContentLength, progressBarWidth)
		pct := "--"
		if w.ContentLength > 0 {
			p := (float64(w.Downloaded) / float64(w.ContentLength)) * 100
			if p > 100 {
				p = 100
			}
			pct = fmt.Sprintf("%5.1f%%", p)
		}

		b.WriteString(fmt.Sprintf("%d: %s %s %s/s", w.ID, bar, pct, utils.ReadableBytes(w.SpeedBps)))
		if w.ContentLength > 0 {
			b.WriteString(fmt.Sprintf(" (%s/%s)", utils.ReadableBytes(float64(w.Downloaded)), utils.ReadableBytes(float64(w.ContentLength))))
		}
		b.WriteString("\n")
	}

	b.WriteString("\nPress q / Ctrl+C to quit.\n")
	return b.String()
}

func progressBar(downloaded uint64, total int64, width int) string {
	if width <= 0 {
		return "[]"
	}

	filled := 0
	if total > 0 {
		ratio := float64(downloaded) / float64(total)
		if ratio > 1 {
			ratio = 1
		}
		filled = int(ratio * float64(width))
	}

	if filled < 0 {
		filled = 0
	}
	if filled > width {
		filled = width
	}

	return "[" + strings.Repeat("=", filled) + strings.Repeat(" ", width-filled) + "]"
}

func StartTUI(target string, concurrent int, agg *Aggregator) error {
	p := tea.NewProgram(NewTUIModel(target, concurrent, agg), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
