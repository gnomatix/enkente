package cmd

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/gnomatix/enkente/pkg/parser"
	"github.com/spf13/cobra"
)

var logFile string

var tailCmd = &cobra.Command{
	Use:   "tail",
	Short: "Live tail an Antigravity chat log to see the worker swarm",
	Run: func(cmd *cobra.Command, args []string) {
		if logFile == "" {
			log.Fatal("Please provide a path to the live logs.json using --log")
		}

		p := tea.NewProgram(
			initialModel(),
			tea.WithAltScreen(),
			tea.WithMouseCellMotion(),
		)

		done := make(chan struct{})

		handler := func(workerID int, msg parser.AntigravityMessage) {
			p.Send(tailMsg{workerID: workerID, msg: msg})
		}

		err := parser.TailChatLog(logFile, 500*time.Millisecond, 4, handler, done)
		if err != nil {
			log.Fatalf("Failed to start tailer: %v", err)
		}

		if _, err := p.Run(); err != nil {
			log.Fatalf("Alas, there's been an error: %v", err)
		}
	},
}

func init() {
	rootCmd.AddCommand(tailCmd)
	tailCmd.Flags().StringVarP(&logFile, "log", "l", "", "Path to the live logs.json to tail")
	tailCmd.MarkFlagRequired("log")
}

// Bubble Tea definitions below

var (
	titleStyle = func() lipgloss.Style {
		b := lipgloss.RoundedBorder()
		b.Right = "├"
		return lipgloss.NewStyle().BorderStyle(b).Padding(0, 1)
	}()

	infoStyle = func() lipgloss.Style {
		b := lipgloss.RoundedBorder()
		b.Left = "┤"
		return titleStyle.BorderStyle(b)
	}()
)

type tailMsg struct {
	workerID int
	msg      parser.AntigravityMessage
}

type model struct {
	content  string
	ready    bool
	viewport viewport.Model
}

func initialModel() model {
	return model{}
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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

	case tailMsg:
		colorWorker := lipgloss.Color("36") // Cyan
		colorSystem := lipgloss.Color("34") // Blue
		colorUser := lipgloss.Color("32")   // Green
		colorTime := lipgloss.Color("240")  // Dark Gray

		typeColor := colorSystem
		if msg.msg.Type == "user" {
			typeColor = colorUser
		}

		timeStr := lipgloss.NewStyle().Foreground(colorTime).Render("[" + msg.msg.Timestamp.Format("15:04:05") + "]")
		workerStr := lipgloss.NewStyle().Foreground(colorWorker).Render(fmt.Sprintf("[Worker-%d]", msg.workerID))
		msgStr := lipgloss.NewStyle().Foreground(typeColor).Render(fmt.Sprintf("%s: %s", msg.msg.Type, msg.msg.Message))

		newLine := fmt.Sprintf("%s %s %s\n", timeStr, workerStr, msgStr)
		m.content += newLine
		m.viewport.SetContent(m.content)
		m.viewport.GotoBottom()
	}

	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m model) View() string {
	if !m.ready {
		return "\n  Initializing..."
	}
	return fmt.Sprintf("%s\n%s\n%s", m.headerView(), m.viewport.View(), m.footerView())
}

func (m model) headerView() string {
	title := titleStyle.Render("enkente Live Worker Swarm")
	line := strings.Repeat("─", max(0, m.viewport.Width-lipgloss.Width(title)))
	return lipgloss.JoinHorizontal(lipgloss.Center, title, line)
}

func (m model) footerView() string {
	info := infoStyle.Render(fmt.Sprintf("%3.f%%", m.viewport.ScrollPercent()*100))
	line := strings.Repeat("─", max(0, m.viewport.Width-lipgloss.Width(info)))
	return lipgloss.JoinHorizontal(lipgloss.Center, line, info)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
