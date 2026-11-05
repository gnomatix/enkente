package cmd

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/gnomatix/enkente/pkg/api"
	"github.com/gnomatix/enkente/pkg/parser"
	"github.com/gnomatix/enkente/pkg/storage"
	"github.com/gnomatix/enkente/pkg/theme"
	"github.com/spf13/cobra"
)

var (
	servePort     int
	serveDB       string
	serveHeadless bool
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the ingestion API server with a live TUI dashboard",
	Long: `Starts an HTTP server that listens for POST /ingest requests and displays
incoming messages in a real-time BubbleTea TUI with the worker swarm.

Accepted messages are persisted to the embedded BoltDB datastore given by
--db (pass an empty string to disable persistence) and can be read back from
GET /sessions, GET /sessions/{id}/messages and GET /sessions/{id}/messages/{n}.

With --headless no TUI is started: messages are logged to stdout and the
process runs until SIGINT or SIGTERM, which shuts the server down cleanly.
Use this under CI, containers or a service manager where no TTY exists.

Send messages with:
  curl -X POST http://localhost:8080/ingest -d '{"type":"user","message":"Hello!"}'`,
	RunE: func(cmd *cobra.Command, args []string) error {
		var opts []api.Option
		if serveDB != "" {
			store, err := storage.NewBoltStorage(serveDB)
			if err != nil {
				return fmt.Errorf("open database %s: %w", serveDB, err)
			}
			defer store.Close()
			opts = append(opts, api.WithStore(store))
		}

		if serveHeadless {
			return runHeadless(cmd, opts)
		}

		p := tea.NewProgram(
			initialServeModel(servePort, serveDB),
			tea.WithAltScreen(),
			tea.WithMouseCellMotion(),
		)

		handler := func(workerID int, msg parser.AntigravityMessage) {
			p.Send(serveMsg{workerID: workerID, msg: msg})
		}

		server := api.NewServer(servePort, 4, handler, opts...)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		serverErr := make(chan error, 1)
		go func() { serverErr <- server.Serve(ctx, nil) }()

		_, err := p.Run()
		cancel() // stop accepting requests once the TUI exits
		if err != nil {
			return fmt.Errorf("TUI error: %w", err)
		}
		if err := <-serverErr; err != nil {
			return fmt.Errorf("server error: %w", err)
		}
		return nil
	},
}

// runHeadless serves without a TUI, logging each message, until a
// termination signal arrives.
func runHeadless(cmd *cobra.Command, opts []api.Option) error {
	logger := log.New(cmd.OutOrStdout(), "", log.LstdFlags|log.LUTC)

	handler := func(workerID int, msg parser.AntigravityMessage) {
		sender := msg.Type
		if msg.User != "" {
			sender = msg.User
		}
		logger.Printf("[W%d] %s/%d %s: %s", workerID, msg.SessionID, msg.MessageID, sender, msg.Message)
	}

	server := api.NewServer(servePort, 4, handler, opts...)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Bind before announcing, so a taken port fails without a misleading
	// "listening" line.
	ln, err := net.Listen("tcp", server.Addr())
	if err != nil {
		return fmt.Errorf("server error: %w", err)
	}

	persistence := "persistence disabled"
	if serveDB != "" {
		persistence = "db " + serveDB
	}
	logger.Printf("enkente listening on %s (%s); POST /ingest to submit messages, GET / for the viewer", ln.Addr(), persistence)

	if err := server.Serve(ctx, ln); err != nil {
		return fmt.Errorf("server error: %w", err)
	}
	logger.Printf("enkente shut down")
	return nil
}

func init() {
	rootCmd.AddCommand(serveCmd)
	serveCmd.Flags().IntVarP(&servePort, "port", "p", 8080, "Port to listen on")
	serveCmd.Flags().StringVar(&serveDB, "db", "enkente.db", "Path to the BoltDB datastore (empty string disables persistence)")
	serveCmd.Flags().BoolVar(&serveHeadless, "headless", false, "Run without the TUI; log to stdout and stop on SIGINT/SIGTERM")
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
	dbPath   string
	msgCount int
}

func initialServeModel(port int, dbPath string) serveModel {
	return serveModel{port: port, dbPath: dbPath}
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

		// A terminal shorter than the header and footer (or one that reports
		// no size at all) must not produce a negative viewport height: the
		// viewport panics on GotoBottom when Height < 0.
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

	case serveMsg:
		m.msgCount++

		colorWorker := lipgloss.Color("6") // Cyan
		colorTime := lipgloss.Color("240") // Dark Gray
		colorCount := lipgloss.Color("5")  // Magenta

		// Wes Anderson palette for user colors
		userPalette := theme.AllUserColors()

		var senderColor lipgloss.Color
		if msg.msg.Type == "system" {
			senderColor = theme.SystemColor
		} else {
			// Hash the user identity to a stable palette index
			identity := msg.msg.User
			if identity == "" {
				identity = msg.msg.Type
			}
			hash := uint(0)
			for _, c := range identity {
				hash = hash*31 + uint(c)
			}
			senderColor = userPalette[hash%uint(len(userPalette))]
		}

		timeStr := lipgloss.NewStyle().Foreground(colorTime).Render("[" + msg.msg.Timestamp.Format(time.TimeOnly) + "]")
		workerStr := lipgloss.NewStyle().Foreground(colorWorker).Bold(true).Render(fmt.Sprintf("[W%d]", msg.workerID))
		countStr := lipgloss.NewStyle().Foreground(colorCount).Render(fmt.Sprintf("#%d", m.msgCount))

		sender := msg.msg.Type
		if msg.msg.User != "" {
			sender = msg.msg.User
		}
		typeStr := lipgloss.NewStyle().Foreground(senderColor).Bold(true).Render(sender)
		msgStr := lipgloss.NewStyle().Foreground(senderColor).Render(msg.msg.Message)

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
	persistence := "no persistence"
	if m.dbPath != "" {
		persistence = "db: " + m.dbPath
	}
	status := lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Render(fmt.Sprintf(" ● LIVE  %d msgs  %s", m.msgCount, persistence))
	line := strings.Repeat("─", max(0, m.viewport.Width-lipgloss.Width(title)-lipgloss.Width(status)))
	return lipgloss.JoinHorizontal(lipgloss.Center, title, line, status)
}

func (m serveModel) footerView() string {
	info := infoStyle.Render(fmt.Sprintf("%3.f%%", m.viewport.ScrollPercent()*100))
	hint := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(" q/esc to quit • scroll with mouse/arrows ")
	line := strings.Repeat("─", max(0, m.viewport.Width-lipgloss.Width(info)-lipgloss.Width(hint)))
	return lipgloss.JoinHorizontal(lipgloss.Center, hint, line, info)
}
