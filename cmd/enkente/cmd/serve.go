package cmd

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/gnomatix/enkente/pkg/api"
	"github.com/gnomatix/enkente/pkg/parser"
	"github.com/spf13/cobra"
)

var servePort int

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the ingestion API server with a live TUI dashboard",
	Long: `Starts an HTTP server that listens for POST /ingest requests and displays
incoming messages in a real-time BubbleTea TUI with the worker swarm.

Send messages with:
  curl -X POST http://localhost:8080/ingest -d '{"type":"user","message":"Hello!"}'`,
	Run: func(cmd *cobra.Command, args []string) {
		p := tea.NewProgram(
			initialServeModel(servePort),
			tea.WithAltScreen(),
			tea.WithMouseCellMotion(),
		)

		handler := func(workerID int, msg parser.AntigravityMessage) {
			p.Send(serveMsg{workerID: workerID, msg: msg})
		}

		server := api.NewServer(servePort, 4, handler)

		go func() {
			if err := server.Start(); err != nil {
				log.Fatalf("Server error: %v", err)
			}
		}()

		if _, err := p.Run(); err != nil {
			log.Fatalf("TUI error: %v", err)
		}
	},
}

func init() {
	rootCmd.AddCommand(serveCmd)
	serveCmd.Flags().IntVarP(&servePort, "port", "p", 8080, "Port to listen on")
}

// Bubble Tea model for the serve command

type serveMsg struct {
	workerID int
	msg      parser.AntigravityMessage
}

type serveModel struct {
	content  string
	ready    bool
	viewport viewport.Model
	port     int
	msgCount int
}

func initialServeModel(port int) serveModel {
	return serveModel{port: port}
}

func (m serveModel) Init() tea.Cmd {
	return nil
}

func (m serveModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	switch msg := msg.(type) {
	case tea.KeyMsg:
		if k := msg.String(); k == "ctrl+c" || k == "q" || k == "esc" {
			return m, tea.Quit
		}
	case tea.WindowSizeMsg:
		headerHeight := lipgloss.Height(m.headerView())
		footerHeight := lipgloss.Height(m.footerView())
		verticalMarginHeight := headerHeight + footerHeight

		if !m.ready {
			m.viewport = viewport.New(msg.Width, msg.Height-verticalMarginHeight)
			m.viewport.YPosition = headerHeight
			m.viewport.HighPerformanceRendering = false
			m.ready = true
		} else {
			m.viewport.Width = msg.Width
			m.viewport.Height = msg.Height - verticalMarginHeight
		}

	case serveMsg:
		m.msgCount++

		colorWorker := lipgloss.Color("6") // Cyan
		colorSystem := lipgloss.Color("4") // Blue
		colorUser := lipgloss.Color("2")   // Green
		colorTime := lipgloss.Color("240") // Dark Gray
		colorCount := lipgloss.Color("5")  // Magenta

		typeColor := colorSystem
		if msg.msg.Type == "user" {
			typeColor = colorUser
		}

		timeStr := lipgloss.NewStyle().Foreground(colorTime).Render("[" + msg.msg.Timestamp.Format(time.TimeOnly) + "]")
		workerStr := lipgloss.NewStyle().Foreground(colorWorker).Bold(true).Render(fmt.Sprintf("[W%d]", msg.workerID))
		countStr := lipgloss.NewStyle().Foreground(colorCount).Render(fmt.Sprintf("#%d", m.msgCount))
		typeStr := lipgloss.NewStyle().Foreground(typeColor).Bold(true).Render(msg.msg.Type)
		msgStr := lipgloss.NewStyle().Foreground(typeColor).Render(msg.msg.Message)

		newLine := fmt.Sprintf("%s %s %s %s: %s\n", timeStr, workerStr, countStr, typeStr, msgStr)
		m.content += newLine
		m.viewport.SetContent(m.content)
		m.viewport.GotoBottom()
	}

	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m serveModel) View() string {
	if !m.ready {
		return fmt.Sprintf("\n  Starting enkente on port %d...\n  POST to http://localhost:%d/ingest\n", m.port, m.port)
	}
	return fmt.Sprintf("%s\n%s\n%s", m.headerView(), m.viewport.View(), m.footerView())
}

func (m serveModel) headerView() string {
	title := titleStyle.Render(fmt.Sprintf("enkente :%d", m.port))
	status := lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Render(fmt.Sprintf(" ● LIVE  %d msgs", m.msgCount))
	line := strings.Repeat("─", max(0, m.viewport.Width-lipgloss.Width(title)-lipgloss.Width(status)))
	return lipgloss.JoinHorizontal(lipgloss.Center, title, line, status)
}

func (m serveModel) footerView() string {
	info := infoStyle.Render(fmt.Sprintf("%3.f%%", m.viewport.ScrollPercent()*100))
	hint := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(" q/esc to quit • scroll with mouse/arrows ")
	line := strings.Repeat("─", max(0, m.viewport.Width-lipgloss.Width(info)-lipgloss.Width(hint)))
	return lipgloss.JoinHorizontal(lipgloss.Center, hint, line, info)
}
