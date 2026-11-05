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
	"github.com/gnomatix/enkente/pkg/storage"
	"github.com/spf13/cobra"
)

var (
	logFile string
	tailDB  string
)

var tailCmd = &cobra.Command{
	Use:   "tail",
	Short: "Live tail an Antigravity chat log to see the worker swarm",
	Long: `Watches an Antigravity JSON chat log and streams new messages through the
worker swarm into a live TUI.

With --db, every message is also persisted to the embedded BoltDB datastore
under its own sessionId and messageId. Re-tailing the same log is idempotent:
a message already stored with the same identity is overwritten, not duplicated.`,
	Run: func(cmd *cobra.Command, args []string) {
		if logFile == "" {
			log.Fatal("Please provide a path to the live logs.json using --log")
		}

		var store *storage.BoltStorage
		if tailDB != "" {
			var err error
			store, err = storage.NewBoltStorage(tailDB)
			if err != nil {
				log.Fatalf("Failed to open database %s: %v", tailDB, err)
			}
			defer store.Close()
		}

		p := tea.NewProgram(
			initialModel(),
			tea.WithAltScreen(),
			tea.WithMouseCellMotion(),
		)

		done := make(chan struct{})

		handler := func(workerID int, msg parser.AntigravityMessage) {
			var err error
			if store != nil {
				err = store.PutMessage(msg)
			}
			p.Send(tailMsg{workerID: workerID, msg: msg, err: err})
		}

		opts := parser.TailOptions{OnError: func(err error) {
			p.Send(tailErrMsg{err: err})
		}}
		err := parser.TailChatLogWithOptions(logFile, 500*time.Millisecond, 4, handler, done, opts)
		if err != nil {
			log.Fatalf("Failed to start tailer: %v", err)
		}

		_, err = p.Run()
		close(done) // stop the poller and let the worker channel drain
		if err != nil {
			log.Fatalf("Alas, there's been an error: %v", err)
		}
	},
}

func init() {
	rootCmd.AddCommand(tailCmd)
	tailCmd.Flags().StringVarP(&logFile, "log", "l", "", "Path to the live logs.json to tail")
	tailCmd.Flags().StringVar(&tailDB, "db", "", "Path to a BoltDB datastore to persist tailed messages into (default: none)")
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
	err      error // non-nil when persisting the message failed
}

// tailErrMsg reports that the log could not be read or parsed on a poll.
type tailErrMsg struct {
	err error
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

		// Clamp so a tiny or unsized terminal cannot yield a negative viewport
		// height, which panics inside viewport.GotoBottom.
		bodyHeight := max(0, msg.Height-verticalMarginHeight)

		if !m.ready {
			m.viewport = viewport.New(msg.Width, bodyHeight)
			m.viewport.YPosition = headerHeight
			m.viewport.HighPerformanceRendering = false
			m.ready = true
		} else {
			m.viewport.Width = msg.Width
			m.viewport.Height = bodyHeight
		}

	case tailErrMsg:
		timeStr := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("[" + time.Now().Format("15:04:05") + "]")
		errStr := lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render(fmt.Sprintf("! log unreadable, retrying: %v", msg.err))
		m.content += fmt.Sprintf("%s %s\n", timeStr, errStr)
		m.viewport.SetContent(m.content)
		m.viewport.GotoBottom()

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
		if msg.err != nil {
			errStr := lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render(fmt.Sprintf("  ! persist failed: %v", msg.err))
			newLine += errStr + "\n"
		}
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
