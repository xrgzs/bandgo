package monitor

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"bandgo/utils"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const progressBarWidth = 24

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
	width      int
	height     int
	offset     int
	progress   progress.Model
	spinner    spinner.Model
}

func NewTUIModel(target string, concurrent int, agg *Aggregator) tea.Model {
	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("69"))

	return tuiModel{
		target:     target,
		agg:        agg,
		concurrent: concurrent,
		progress:   progress.New(progress.WithDefaultGradient(), progress.WithWidth(progressBarWidth)),
		spinner:    sp,
	}
}

func (m tuiModel) Init() tea.Cmd {
	return tea.Batch(scheduleTick(), m.spinner.Tick)
}

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "q" || msg.String() == "ctrl+c" {
			return m, tea.Quit
		}

		workersLen := len(m.agg.Snapshot())
		pageSize := m.workerViewportHeight()
		switch msg.String() {
		case "up", "k":
			m.offset--
		case "down", "j":
			m.offset++
		case "pgup", "b":
			m.offset -= pageSize
		case "pgdown", "f", " ":
			m.offset += pageSize
		case "home", "g":
			m.offset = 0
		case "end", "G":
			m.offset = workersLen - pageSize
		}
		m.clampOffset(workersLen, pageSize)
	case tickMsg:
		m.agg.Tick(time.Second)
		m.clampOffset(len(m.agg.Snapshot()), m.workerViewportHeight())
		return m, scheduleTick()
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		barWidth := progressBarWidth
		if msg.Width > 0 {
			candidate := msg.Width / 4
			if candidate < 16 {
				candidate = 16
			}
			if candidate > 40 {
				candidate = 40
			}
			barWidth = candidate
		}
		m.progress.Width = barWidth
		m.clampOffset(len(m.agg.Snapshot()), m.workerViewportHeight())
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m *tuiModel) workerViewportHeight() int {
	if m.height <= 0 {
		return 18
	}

	// Header + summaries + hint lines take around 11 rows.
	visible := m.height - 11
	if visible < 5 {
		visible = 5
	}
	return visible
}

func (m *tuiModel) clampOffset(total, pageSize int) {
	if total <= pageSize {
		m.offset = 0
		return
	}
	if m.offset < 0 {
		m.offset = 0
	}
	maxOffset := total - pageSize
	if m.offset > maxOffset {
		m.offset = maxOffset
	}
}

func (m tuiModel) View() string {
	var b strings.Builder
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	metaStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("246"))
	summaryStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("86"))
	workerIDStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("110"))

	b.WriteString(titleStyle.Render("BandGo Worker Monitor"))
	b.WriteString("\n")
	b.WriteString(metaStyle.Render(fmt.Sprintf("Target: %s", m.target)))
	b.WriteString("\n")
	b.WriteString(metaStyle.Render(fmt.Sprintf("Elapsed: %s", m.agg.Elapsed().Round(time.Second))))
	b.WriteString("\n")
	b.WriteString(metaStyle.Render(fmt.Sprintf("Configured Workers: %d", m.concurrent)))
	b.WriteString("\n\n")

	workers := m.agg.Snapshot()
	if len(workers) == 0 {
		b.WriteString("Waiting for workers to register...\n")
		return b.String()
	}

	pageSize := m.workerViewportHeight()
	m.clampOffset(len(workers), pageSize)
	start := m.offset
	end := start + pageSize
	if end > len(workers) {
		end = len(workers)
	}

	b.WriteString(metaStyle.Render(fmt.Sprintf("Workers %d-%d/%d (j/k or up/down scroll, PgUp/PgDn page)", start+1, end, len(workers))))
	b.WriteString("\n")

	for _, w := range workers[start:end] {
		idLabel := workerIDStyle.Render(fmt.Sprintf("%2d", w.ID))
		speed := fmt.Sprintf("%s/s", utils.ReadableBytes(w.SpeedBps))

		if w.ContentLength > 0 {
			fraction := float64(w.Downloaded) / float64(w.ContentLength)
			if fraction > 1 {
				fraction = 1
			}
			pct := fmt.Sprintf("%5.1f%%", fraction*100)
			bar := m.progress.ViewAs(fraction)
			b.WriteString(fmt.Sprintf("%s | %s %s %s (%s/%s)\n",
				idLabel,
				bar,
				pct,
				speed,
				utils.ReadableBytes(float64(w.Downloaded)),
				utils.ReadableBytes(float64(w.ContentLength)),
			))
			continue
		}

		// Unknown content-length: use spinner to show this worker is actively downloading.
		b.WriteString(fmt.Sprintf("%s | %s %s %s downloaded\n",
			idLabel,
			m.spinner.View(),
			speed,
			utils.ReadableBytes(float64(w.Downloaded)),
		))
	}

	b.WriteString("\n")

	totalDownloaded, totalSpeed := m.agg.Totals()
	b.WriteString(summaryStyle.Render(fmt.Sprintf("Total Speed: %s/s", utils.ReadableBytes(totalSpeed))))
	b.WriteString("\n")
	b.WriteString(summaryStyle.Render(fmt.Sprintf("Total Downloaded: %s", utils.ReadableBytes(float64(totalDownloaded)))))

	b.WriteString("\n\n")
	b.WriteString(metaStyle.Render("Press q / Ctrl+C to quit."))
	b.WriteString("\n")
	return b.String()
}

func StartTUI(target string, concurrent int, agg *Aggregator) error {
	p := tea.NewProgram(NewTUIModel(target, concurrent, agg), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
