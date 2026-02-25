package eventmeta

import (
	"fmt"
	"strconv"
	"strings"

	"helix-tui/internal/domain"
	"helix-tui/internal/symbols"
)

type AgentMode struct {
	Mode      string
	AgentType string
	Watchlist []string
}

func EncodeAgentMode(v AgentMode) string {
	return fmt.Sprintf(
		"mode=%s agent=%s watchlist=%s",
		strings.ToLower(strings.TrimSpace(v.Mode)),
		strings.ToLower(strings.TrimSpace(v.AgentType)),
		strings.Join(symbols.Normalize(v.Watchlist), ","),
	)
}

func DecodeAgentMode(details string) (AgentMode, bool) {
	fields := parseFields(details)
	out := AgentMode{
		Mode:      strings.ToLower(strings.TrimSpace(fields["mode"])),
		AgentType: strings.ToLower(strings.TrimSpace(fields["agent"])),
		Watchlist: parseCSVSymbols(fields["watchlist"]),
	}
	if out.Mode == "" && out.AgentType == "" && len(out.Watchlist) == 0 {
		return AgentMode{}, false
	}
	return out, true
}

type AlpacaConfig struct {
	Env         string
	Endpoint    string
	Feed        string
	Credentials string
}

func EncodeAlpacaConfig(v AlpacaConfig) string {
	return fmt.Sprintf(
		"env=%s endpoint=%s feed=%s credentials=%s",
		strings.ToLower(strings.TrimSpace(v.Env)),
		strings.TrimSpace(v.Endpoint),
		strings.ToLower(strings.TrimSpace(v.Feed)),
		strings.ToLower(strings.TrimSpace(v.Credentials)),
	)
}

func DecodeAlpacaConfig(details string) (AlpacaConfig, bool) {
	fields := parseFields(details)
	out := AlpacaConfig{
		Env:         strings.ToLower(strings.TrimSpace(fields["env"])),
		Endpoint:    strings.TrimSpace(fields["endpoint"]),
		Feed:        strings.ToLower(strings.TrimSpace(fields["feed"])),
		Credentials: strings.ToLower(strings.TrimSpace(fields["credentials"])),
	}
	if out.Env == "" && out.Endpoint == "" && out.Feed == "" && out.Credentials == "" {
		return AlpacaConfig{}, false
	}
	return out, true
}

type IdentityConfig struct {
	Agent string
	Human string
	Alias string
}

func EncodeIdentityConfig(v IdentityConfig) string {
	return fmt.Sprintf(
		"agent=%s human=%s alias=%s",
		sanitizeField(v.Agent),
		sanitizeField(v.Human),
		sanitizeField(v.Alias),
	)
}

func DecodeIdentityConfig(details string) (IdentityConfig, bool) {
	fields := parseFields(details)
	out := IdentityConfig{
		Agent: restoreField(fields["agent"]),
		Human: restoreField(fields["human"]),
		Alias: restoreField(fields["alias"]),
	}
	if out.Agent == "" && out.Human == "" && out.Alias == "" {
		return IdentityConfig{}, false
	}
	return out, true
}

type AgentPowerState struct {
	State        string
	Prev         string
	Reason       string
	NextInterval string
}

func EncodeAgentPowerState(v AgentPowerState) string {
	return fmt.Sprintf(
		"state=%s prev=%s reason=%s next_interval=%s",
		strings.ToLower(strings.TrimSpace(v.State)),
		strings.ToLower(strings.TrimSpace(v.Prev)),
		strings.ToLower(strings.TrimSpace(v.Reason)),
		strings.TrimSpace(v.NextInterval),
	)
}

func DecodeAgentPowerState(details string) (AgentPowerState, bool) {
	fields := parseFields(details)
	out := AgentPowerState{
		State:        strings.ToLower(strings.TrimSpace(fields["state"])),
		Prev:         strings.ToLower(strings.TrimSpace(fields["prev"])),
		Reason:       strings.ToLower(strings.TrimSpace(fields["reason"])),
		NextInterval: strings.TrimSpace(fields["next_interval"]),
	}
	if out.State == "" && out.Prev == "" && out.Reason == "" && out.NextInterval == "" {
		return AgentPowerState{}, false
	}
	return out, true
}

type StrategyMode struct {
	Enabled  bool
	Interval string
	Model    string
}

func EncodeStrategyMode(v StrategyMode) string {
	return fmt.Sprintf(
		"enabled=%t interval=%s model=%s",
		v.Enabled,
		strings.TrimSpace(v.Interval),
		strings.TrimSpace(v.Model),
	)
}

func DecodeStrategyMode(details string) (StrategyMode, bool) {
	fields := parseFields(details)
	enabled := strings.EqualFold(strings.TrimSpace(fields["enabled"]), "true")
	out := StrategyMode{
		Enabled:  enabled,
		Interval: strings.TrimSpace(fields["interval"]),
		Model:    strings.TrimSpace(fields["model"]),
	}
	if fields["enabled"] == "" && out.Interval == "" && out.Model == "" {
		return StrategyMode{}, false
	}
	return out, true
}

type CompliancePosture struct {
	AccountType    string
	PatternDayTrad bool
	DayTrades      int
}

func EncodeCompliancePosture(status domain.ComplianceStatus) string {
	return fmt.Sprintf(
		"enabled=%t account_type=%s avoid_pdt=%t avoid_gfv=%t pdt=%t day_trades=%d max_day_trades_5d=%d equity=%.2f min_equity_for_pdt=%.2f local_unsettled=%.2f broker_unsettled=%.2f drift_detected=%t",
		status.Enabled,
		status.AccountType,
		status.AvoidPDT,
		status.AvoidGoodFaith,
		status.PatternDayTrader,
		status.DayTradeCount,
		status.MaxDayTrades5D,
		status.Equity,
		status.MinEquityForPDT,
		status.LocalUnsettledProceeds,
		status.BrokerUnsettledProceeds,
		status.UnsettledDriftDetected,
	)
}

func DecodeCompliancePosture(details string) (CompliancePosture, bool) {
	fields := parseFields(details)
	dayTrades, ok := parseInt(fields["day_trades"])
	if !ok {
		dayTrades = 0
	}
	out := CompliancePosture{
		AccountType:    strings.TrimSpace(fields["account_type"]),
		PatternDayTrad: strings.EqualFold(strings.TrimSpace(fields["pdt"]), "true"),
		DayTrades:      dayTrades,
	}
	if out.AccountType == "" && fields["pdt"] == "" && fields["day_trades"] == "" {
		return CompliancePosture{}, false
	}
	return out, true
}

type ComplianceDrift struct {
	State           string
	LocalUnsettled  float64
	BrokerUnsettled float64
	Drift           float64
}

func EncodeComplianceDrift(status domain.ComplianceStatus) string {
	state := "clear"
	if status.UnsettledDriftDetected {
		state = "detected"
	}
	return fmt.Sprintf(
		"state=%s account_type=%s local_unsettled=%.2f broker_unsettled=%.2f drift=%.2f tolerance=%.2f",
		state,
		status.AccountType,
		status.LocalUnsettledProceeds,
		status.BrokerUnsettledProceeds,
		status.UnsettledDrift,
		status.UnsettledDriftTolerance,
	)
}

func DecodeComplianceDrift(details string) (ComplianceDrift, bool) {
	fields := parseFields(details)
	local, _ := parseFloat(fields["local_unsettled"])
	broker, _ := parseFloat(fields["broker_unsettled"])
	drift, _ := parseFloat(fields["drift"])
	out := ComplianceDrift{
		State:           strings.ToLower(strings.TrimSpace(fields["state"])),
		LocalUnsettled:  local,
		BrokerUnsettled: broker,
		Drift:           drift,
	}
	if out.State == "" && fields["local_unsettled"] == "" && fields["broker_unsettled"] == "" && fields["drift"] == "" {
		return ComplianceDrift{}, false
	}
	return out, true
}

func parseFields(details string) map[string]string {
	fields := map[string]string{}
	for _, part := range strings.Fields(details) {
		if !strings.Contains(part, "=") {
			continue
		}
		key, value, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		fields[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	return fields
}

func parseCSVSymbols(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	return symbols.Normalize(strings.Split(raw, ","))
}

func sanitizeField(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "n/a"
	}
	return strings.ReplaceAll(value, " ", "_")
}

func restoreField(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "n/a" {
		return ""
	}
	return strings.ReplaceAll(value, "_", " ")
}

func parseFloat(raw string) (float64, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

func parseInt(raw string) (int, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0, false
	}
	return v, true
}
