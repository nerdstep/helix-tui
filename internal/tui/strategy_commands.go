package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"helix-tui/internal/util"
)

const (
	strategyCommandUsage         = "usage: strategy <run|status|approve <id>|reject <id>|archive <id>|proposal ...|chat ...>"
	strategyProposalCommandUsage = "usage: strategy proposal <status|list|apply <id>|reject <id>>"
	strategyChatCommandUsage     = "usage: strategy chat <status|list|new <title>|use <id>|say <message>>"
)

type strategyChatResultMsg struct {
	threadID uint
	status   string
	isErr    bool
	refresh  bool
}

func (m *Model) handleStrategyCommand(raw string) (bool, tea.Cmd) {
	cmd, handled, parseErr := parseStrategyCommand(raw)
	if !handled {
		return false, nil
	}
	if parseErr != nil {
		m.setStatus(parseErr.status, parseErr.isErr)
		return true, nil
	}
	switch cmd.Type {
	case strategyCommandRun:
		if m.onStrategyRun == nil {
			m.setStatus("strategy runner is not configured", true)
			return true, nil
		}
		if err := m.onStrategyRun(); err != nil {
			m.setStatus(fmt.Sprintf("strategy run request failed: %v", err), true)
			return true, nil
		}
		m.startStrategyLoading()
		return true, tea.Batch(m.refreshCmd(), m.spinner.Tick)
	case strategyCommandStatus:
		return true, m.handleStrategyStatus()
	case strategyCommandApprove:
		return true, m.handleStrategyPlanStatus(cmd.PlanID, "approve", m.onStrategyApprove)
	case strategyCommandReject:
		return true, m.handleStrategyPlanStatus(cmd.PlanID, "reject", m.onStrategyReject)
	case strategyCommandArchive:
		return true, m.handleStrategyPlanStatus(cmd.PlanID, "archive", m.onStrategyArchive)
	case strategyCommandProposalStatus:
		return true, m.handleStrategyProposalStatus()
	case strategyCommandProposalList:
		return true, m.handleStrategyProposalList()
	case strategyCommandProposalApply:
		return true, m.handleStrategyProposalAction(cmd.PlanID, "apply", m.onStrategyProposalApply)
	case strategyCommandProposalReject:
		return true, m.handleStrategyProposalAction(cmd.PlanID, "reject", m.onStrategyProposalReject)
	case strategyCommandChatStatus:
		return true, m.handleStrategyChatStatus()
	case strategyCommandChatList:
		return true, m.handleStrategyChatList()
	case strategyCommandChatNew:
		return true, m.handleStrategyChatNew(cmd.Text)
	case strategyCommandChatUse:
		return true, m.handleStrategyChatUse(cmd.ThreadID)
	case strategyCommandChatSay:
		return true, m.handleStrategyChatSay(cmd.Text)
	default:
		m.setStatus(strategyCommandUsage, true)
		return true, nil
	}
}

func (m *Model) handleStrategyStatus() tea.Cmd {
	if m.strategy.Active == nil {
		if m.strategyLoadError != "" {
			m.setStatus("strategy status error: "+m.strategyLoadError, true)
			return nil
		}
		m.setStatus("strategy status: no active plan", false)
		return nil
	}
	active := m.strategy.Active
	msg := fmt.Sprintf("strategy status: active plan #%d (%s) conf=%.2f model=%s", active.ID, strings.ToLower(strings.TrimSpace(active.Status)), active.Confidence, active.AnalystModel)
	m.setStatus(msg, false)
	return nil
}

func (m *Model) handleStrategyPlanStatus(planID uint, verb string, fn func(uint) error) tea.Cmd {
	if fn == nil {
		m.setStatus("strategy plan controls are not configured", true)
		return nil
	}
	if err := fn(planID); err != nil {
		m.setStatus(fmt.Sprintf("strategy %s failed for #%d: %v", verb, planID, err), true)
		return nil
	}
	m.setStatus(fmt.Sprintf("strategy %s #%d", verb, planID), false)
	return m.refreshCmd()
}

func (m *Model) handleStrategyProposalStatus() tea.Cmd {
	pending := m.pendingStrategyProposals()
	if len(pending) == 0 {
		m.setStatus("strategy proposals: no pending proposals", false)
		return nil
	}
	proposal := pending[0]
	m.setStatus(
		fmt.Sprintf(
			"strategy proposal: #%d kind=%s status=%s %s",
			proposal.ID,
			proposal.Kind,
			proposal.Status,
			summarizeStrategyProposal(proposal),
		),
		false,
	)
	return nil
}

func (m *Model) handleStrategyProposalList() tea.Cmd {
	if len(m.strategy.Proposals) == 0 {
		m.setStatus("strategy proposals: (none)", false)
		return nil
	}
	parts := make([]string, 0, util.MinInt(8, len(m.strategy.Proposals)))
	for _, proposal := range m.strategy.Proposals {
		parts = append(parts, fmt.Sprintf("#%d %s %s", proposal.ID, proposal.Kind, proposal.Status))
		if len(parts) >= 8 {
			break
		}
	}
	m.setStatus("strategy proposals: "+strings.Join(parts, " | "), false)
	return nil
}

func (m *Model) handleStrategyProposalAction(proposalID uint, verb string, fn func(uint) error) tea.Cmd {
	if fn == nil {
		m.setStatus("strategy proposal controls are not configured", true)
		return nil
	}
	if err := fn(proposalID); err != nil {
		m.setStatus(fmt.Sprintf("strategy proposal %s failed for #%d: %v", verb, proposalID, err), true)
		return nil
	}
	m.setStatus(fmt.Sprintf("strategy proposal %s #%d", verb, proposalID), false)
	return m.refreshCmd()
}

func (m *Model) pendingStrategyProposals() []StrategyProposalView {
	if len(m.strategy.Proposals) == 0 {
		return nil
	}
	out := make([]StrategyProposalView, 0, len(m.strategy.Proposals))
	for _, proposal := range m.strategy.Proposals {
		if strings.EqualFold(strings.TrimSpace(proposal.Status), "pending") {
			out = append(out, proposal)
		}
	}
	return out
}

func summarizeStrategyProposal(proposal StrategyProposalView) string {
	switch strings.ToLower(strings.TrimSpace(proposal.Kind)) {
	case "watchlist":
		add := "none"
		remove := "none"
		if len(proposal.AddSymbols) > 0 {
			add = strings.Join(proposal.AddSymbols, ",")
		}
		if len(proposal.RemoveSymbols) > 0 {
			remove = strings.Join(proposal.RemoveSymbols, ",")
		}
		return fmt.Sprintf("add=%s remove=%s", add, remove)
	case "steering":
		return fmt.Sprintf(
			"profile=%s min_conf=%.2f max_notional=%.2f",
			nonEmpty(proposal.RiskProfile, "n/a"),
			proposal.MinConfidence,
			proposal.MaxPositionNotional,
		)
	default:
		return "details=n/a"
	}
}

func (m *Model) handleStrategyChatStatus() tea.Cmd {
	thread := m.currentStrategyChatThread()
	if thread == nil {
		m.setStatus("strategy chat: no thread available", true)
		return nil
	}
	msgCount := 0
	for _, msg := range m.strategy.Chat.Messages {
		if msg.ThreadID == thread.ID {
			msgCount++
		}
	}
	last := "n/a"
	if !thread.LastMessageAt.IsZero() {
		last = thread.LastMessageAt.Local().Format("2006-01-02 15:04:05")
	}
	m.setStatus(
		fmt.Sprintf("strategy chat: thread #%d \"%s\" messages=%d last=%s", thread.ID, thread.Title, msgCount, last),
		false,
	)
	return nil
}

func (m *Model) handleStrategyChatList() tea.Cmd {
	threads := m.strategy.Chat.Threads
	if len(threads) == 0 {
		m.setStatus("strategy chat threads: (none)", false)
		return nil
	}
	active := m.activeStrategyThreadID()
	parts := make([]string, 0, len(threads))
	for _, thread := range threads {
		prefix := " "
		if thread.ID == active {
			prefix = "*"
		}
		parts = append(parts, fmt.Sprintf("%s#%d %s", prefix, thread.ID, thread.Title))
	}
	m.setStatus("strategy chat threads: "+strings.Join(parts, " | "), false)
	return nil
}

func (m *Model) handleStrategyChatUse(threadID uint) tea.Cmd {
	if threadID == 0 {
		m.setStatus("strategy chat thread id must be a positive integer", true)
		return nil
	}
	m.strategyThreadID = threadID
	m.setStatus(fmt.Sprintf("strategy chat thread selected: #%d", threadID), false)
	return m.refreshCmd()
}

func (m *Model) handleStrategyChatNew(title string) tea.Cmd {
	if m.onStrategyChatCreate == nil {
		m.setStatus("strategy chat create is not configured", true)
		return nil
	}
	createFn := m.onStrategyChatCreate
	title = strings.TrimSpace(title)
	return m.startStrategyChatAsync("creating strategy chat thread", func() strategyChatResultMsg {
		threadID, err := createFn(title)
		if err != nil {
			return strategyChatResultMsg{
				status: fmt.Sprintf("strategy chat create failed: %v", err),
				isErr:  true,
			}
		}
		return strategyChatResultMsg{
			threadID: threadID,
			status:   fmt.Sprintf("strategy chat thread created: #%d", threadID),
			refresh:  true,
		}
	})
}

func (m *Model) handleStrategyChatSay(text string) tea.Cmd {
	if m.onStrategyChatSend == nil {
		m.setStatus("strategy chat send is not configured", true)
		return nil
	}
	text = strings.TrimSpace(text)
	if text == "" {
		m.setStatus("strategy chat message is required", true)
		return nil
	}
	threadID := m.activeStrategyThreadID()
	if threadID == 0 {
		m.setStatus("strategy chat: no thread selected", true)
		return nil
	}
	sendFn := m.onStrategyChatSend
	label := "sending strategy chat message"
	return m.startStrategyChatAsync(label, func() strategyChatResultMsg {
		start := time.Now().UTC()
		if err := sendFn(threadID, text); err != nil {
			return strategyChatResultMsg{
				threadID: threadID,
				status:   fmt.Sprintf("strategy chat send failed: %v", err),
				isErr:    true,
			}
		}
		elapsed := time.Since(start).Round(time.Millisecond)
		return strategyChatResultMsg{
			threadID: threadID,
			status:   fmt.Sprintf("strategy chat sent (thread #%d, %s)", threadID, elapsed),
			refresh:  true,
		}
	})
}

func (m *Model) startStrategyChatAsync(label string, fn func() strategyChatResultMsg) tea.Cmd {
	m.startCommandLoading(label)
	return func() tea.Msg {
		return fn()
	}
}

func (m *Model) activeStrategyThreadID() uint {
	if m.strategyThreadID != 0 {
		return m.strategyThreadID
	}
	if m.strategy.Chat.ActiveThreadID != 0 {
		return m.strategy.Chat.ActiveThreadID
	}
	if len(m.strategy.Chat.Threads) > 0 {
		return m.strategy.Chat.Threads[0].ID
	}
	return 0
}

func (m *Model) currentStrategyChatThread() *StrategyChatThreadView {
	threadID := m.activeStrategyThreadID()
	if threadID == 0 {
		return nil
	}
	return m.findStrategyThread(threadID)
}

func (m *Model) findStrategyThread(threadID uint) *StrategyChatThreadView {
	for i := range m.strategy.Chat.Threads {
		if m.strategy.Chat.Threads[i].ID == threadID {
			return &m.strategy.Chat.Threads[i]
		}
	}
	return nil
}
