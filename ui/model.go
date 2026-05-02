package ui

import (
	"fmt"
	"math"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"aerobix/domain"
	"aerobix/garminfit"
	"aerobix/physics"
	"aerobix/provider"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/guptarohit/asciigraph"
)

type activitiesLoadedMsg struct {
	activities []domain.Activity
	fetchInfo  provider.FetchInfo
	err        error
}

type garminLoadedMsg struct {
	result garminfit.LoadResult
	err    error
}

type garminProgressMsg struct {
	progress garminfit.Progress
}

type asyncChannelReadyMsg struct {
	ch <-chan tea.Msg
}

type spinnerTickMsg struct{}

type authURLMsg struct {
	url string
	err error
}

type exchangeMsg struct {
	err error
}

type activitySummary struct {
	Activity        domain.Activity
	NP              float64
	IF              float64
	TSS             float64
	TSSSource       string
	TRIMP           float64
	EFSpeed         float64
	Decoupling      float64
	AvgHR           float64
	AvgCadence      float64
	VertOscCM       float64
	VertRatio       float64
	DecoupleAtMin   float64
	DurabilityScore float64
	HRStabilityPct  float64
	CadenceStdDev   float64
	CadenceDropPct  float64
	FormBreakdown   bool
	FormBreakdownAt float64
	SessionClass    string
	SessionConf     float64
	RunExplanation  string
	AvgPace         string
	Duration        string
	Zones           [5]float64
	ZoneBasis       string
	HRZones         [5]float64
}

type dashboardDelta struct {
	Has  bool
	CTL  float64
	ATL  float64
	TSB  float64
	CTLP float64
	ATLP float64
	TSBP float64
}

var navItems = []string{"Dashboard", "Activities", "Garmin (Beta)", "Settings"}

type Model struct {
	width          int
	height         int
	navCursor      int
	activityCursor int
	loading        bool
	status         string

	dataProvider provider.DataProvider
	fetchInfo    provider.FetchInfo
	settings     provider.Settings
	profile      domain.AthleteProfile

	activities    []domain.Activity
	rawActivities []domain.Activity
	summaries     []activitySummary
	pmc           []physics.PMCPoint
	activityTable table.Model
	garminActs    []domain.Activity
	rawGarminActs []domain.Activity
	garminSumm    []activitySummary
	garminTable   table.Model
	garminCursor  int
	dashProvider  string
	preReload     map[string]physics.PMCPoint
	dashDelta     map[string]dashboardDelta

	settingsCursor int
	editMode       bool
	inputBuffer    string
	authCode       string
	asyncCh        <-chan tea.Msg
	spinnerFrame   int
}

func NewModel(dataProvider provider.DataProvider) Model {
	settings := dataProvider.Settings()
	return Model{
		dataProvider:   dataProvider,
		settings:       settings,
		profile:        dataProvider.AthleteProfile(),
		activityTable:  newActivityTable(nil),
		garminTable:    newActivityTable(nil),
		loading:        true,
		status:         "Press r to reload activities.",
		settingsCursor: 0,
		dashProvider:   "strava",
		preReload:      map[string]physics.PMCPoint{},
		dashDelta:      map[string]dashboardDelta{},
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.loadActivitiesCmd(false), spinnerTickCmd())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.activityTable.SetWidth(max(msg.Width-10, 70))
		m.activityTable.SetHeight(max(msg.Height-24, 6))
		m.garminTable.SetWidth(max(msg.Width-10, 70))
		m.garminTable.SetHeight(max(msg.Height-24, 6))
		return m, nil
	case activitiesLoadedMsg:
		m.loading = false
		if msg.err != nil {
			m.status = "Load failed: " + msg.err.Error()
			return m, nil
		}
		m.rawActivities = msg.activities
		m.applyFilters()
		m.updateDashboardDelta("strava")
		m.status = fmt.Sprintf("Loaded %d activities from %s.", len(m.activities), m.dataProvider.Name())
		if msg.fetchInfo.Source != "" {
			m.fetchInfo = msg.fetchInfo
			m.status = fmt.Sprintf(
				"Loaded %d activities (%s at %s).",
				len(m.activities),
				msg.fetchInfo.Source,
				formatFetchTime(msg.fetchInfo.FetchedAt),
			)
		}
		return m, nil
	case spinnerTickMsg:
		if !m.loading {
			return m, nil
		}
		m.spinnerFrame = (m.spinnerFrame + 1) % len(spinnerFrames)
		return m, spinnerTickCmd()
	case authURLMsg:
		if msg.err != nil {
			m.status = "Auth URL error: " + msg.err.Error()
			return m, nil
		}
		m.status = "Opened auth URL in browser. Copy code and paste into Auth Code."
		return m, openBrowserCmd(msg.url)
	case exchangeMsg:
		m.loading = false
		if msg.err != nil {
			m.status = "Code exchange failed: " + msg.err.Error()
			return m, nil
		}
		m.settings = m.dataProvider.Settings()
		m.status = "Connected to Strava. Reloading activities..."
		m.loading = true
		return m, tea.Batch(m.loadActivitiesCmd(true), spinnerTickCmd())
	case garminLoadedMsg:
		m.loading = false
		m.asyncCh = nil
		if msg.err != nil {
			m.status = "Garmin load failed: " + msg.err.Error()
			return m, nil
		}
		m.rawGarminActs = msg.result.Activities
		m.applyFilters()
		m.updateDashboardDelta("garmin")
		m.status = fmt.Sprintf(
			"Garmin import: %d files | %d parsed | %d failed | %d deduped | %d ready.",
			msg.result.TotalFiles,
			msg.result.Imported,
			msg.result.Failed,
			msg.result.Deduped,
			len(m.garminActs),
		)
		return m, nil
	case tea.KeyMsg:
		if m.editMode {
			return m.handleEditKeys(msg)
		}
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "r":
			m.captureReloadBaseline("strava")
			m.loading = true
			return m, tea.Batch(m.loadActivitiesCmd(true), spinnerTickCmd())
		case "g":
			m.captureReloadBaseline("garmin")
			m.loading = true
			return m, tea.Batch(m.startGarminImportCmd(), spinnerTickCmd())
		case "p":
			if navItems[m.navCursor] == "Dashboard" {
				m.toggleDashboardProvider()
				return m, nil
			}
		case "left", "h":
			if m.navCursor > 0 {
				m.navCursor--
			}
		case "right", "l":
			if m.navCursor < len(navItems)-1 {
				m.navCursor++
			}
		case "up", "k":
			if navItems[m.navCursor] == "Activities" {
				if m.activityCursor > 0 {
					m.activityCursor--
				}
				m.activityTable.MoveUp(1)
				m.activityTable.UpdateViewport()
			}
			if navItems[m.navCursor] == "Settings" && m.settingsCursor > 0 {
				m.settingsCursor--
			}
			if navItems[m.navCursor] == "Garmin (Beta)" {
				if m.garminCursor > 0 {
					m.garminCursor--
				}
				m.garminTable.MoveUp(1)
				m.garminTable.UpdateViewport()
			}
		case "down", "j":
			if navItems[m.navCursor] == "Activities" {
				if m.activityCursor < len(m.summaries)-1 {
					m.activityCursor++
				}
				m.activityTable.MoveDown(1)
				m.activityTable.UpdateViewport()
			}
			if navItems[m.navCursor] == "Settings" && m.settingsCursor < 12 {
				m.settingsCursor++
			}
			if navItems[m.navCursor] == "Garmin (Beta)" {
				if m.garminCursor < len(m.garminSumm)-1 {
					m.garminCursor++
				}
				m.garminTable.MoveDown(1)
				m.garminTable.UpdateViewport()
			}
		case "e":
			if navItems[m.navCursor] == "Settings" {
				m.editMode = true
				m.inputBuffer = m.currentSettingValue()
				m.status = "Editing field. Enter to save, Esc to cancel."
			}
		case "s":
			if navItems[m.navCursor] == "Settings" {
				if err := m.persistSettings(); err != nil {
					m.status = "Save failed: " + err.Error()
				} else {
					m.status = "Settings saved."
				}
			}
		case "a":
			if navItems[m.navCursor] == "Settings" {
				return m, m.authURLCmd()
			}
		case "x":
			if navItems[m.navCursor] == "Settings" {
				m.loading = true
				return m, m.exchangeCodeCmd()
			}
		case "o":
			if navItems[m.navCursor] == "Settings" {
				m.settings.RunOnly = !m.settings.RunOnly
				if err := m.persistSettings(); err != nil {
					m.status = "Toggle failed: " + err.Error()
				} else {
					m.status = fmt.Sprintf("Run activities only: %t", m.settings.RunOnly)
				}
			}
		}
	case asyncChannelReadyMsg:
		m.asyncCh = msg.ch
		m.status = "Garmin import started..."
		return m, waitForAsyncMsg(m.asyncCh)
	case garminProgressMsg:
		p := msg.progress
		stage := p.Stage
		if stage == "" {
			stage = "parsing"
		}
		m.status = fmt.Sprintf("Garmin import [%s]: %d/%d processed | %d parsed | %d failed", stage, p.Processed, p.Total, p.Parsed, p.Failed)
		// Keep reading async messages until final garminLoadedMsg arrives.
		if m.asyncCh != nil {
			return m, waitForAsyncMsg(m.asyncCh)
		}
		return m, nil
	}
	return m, nil
}

func (m Model) handleEditKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.editMode = false
		m.inputBuffer = ""
		m.status = "Edit cancelled."
	case "enter":
		m.applyCurrentSetting(m.inputBuffer)
		m.editMode = false
		m.inputBuffer = ""
		m.status = "Field updated. Press s to save."
	case "backspace":
		if len(m.inputBuffer) > 0 {
			m.inputBuffer = m.inputBuffer[:len(m.inputBuffer)-1]
		}
	default:
		// Bubble Tea emits paste as a multi-rune KeyRunes event.
		if msg.Type == tea.KeyRunes && len(msg.Runes) > 0 {
			m.inputBuffer += string(msg.Runes)
		}
	}
	return m, nil
}

func (m Model) View() string {
	tabs := m.renderTabs()
	main := m.renderMain()
	return appStyle.Render(lipgloss.JoinVertical(lipgloss.Left, tabs, "", main))
}

func (m Model) renderTabs() string {
	var rows []string
	for i, item := range navItems {
		style := tabStyle
		if i == m.navCursor {
			style = activeTabStyle
		}
		rows = append(rows, style.Render(item))
	}
	help := mutedStyle.Render("  Keys: h/l nav | j/k move | r Strava reload | g Garmin reload | p dashboard provider | q quit")
	return lipgloss.JoinHorizontal(lipgloss.Top, rows...) + help
}

func (m Model) renderMain() string {
	content := ""
	switch navItems[m.navCursor] {
	case "Dashboard":
		content = m.renderDashboard()
	case "Activities":
		content = m.renderActivities()
	case "Garmin (Beta)":
		content = m.renderGarmin()
	default:
		content = m.renderSettings()
	}
	loading := ""
	if m.loading {
		loading = "\n\n" + spinnerFrames[m.spinnerFrame] + " Loading..."
	}
	status := mutedStyle.Render("\n\n" + m.status)
	return panelStyle.Width(max(m.width-6, 78)).Render(content + loading + status)
}

var spinnerFrames = []string{"|", "/", "-", "\\"}

func spinnerTickCmd() tea.Cmd {
	return tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg {
		return spinnerTickMsg{}
	})
}

func (m Model) renderDashboard() string {
	currentSummaries := m.dashboardSummaries()
	currentPMC := buildPMC(currentSummaries)
	if len(currentPMC) == 0 {
		return titleStyle.Render("Dashboard") + "\n\nNo training load data yet."
	}
	last := currentPMC[len(currentPMC)-1]
	top := lipgloss.JoinHorizontal(
		lipgloss.Top,
		statStyle(fitnessColor).Render(fmt.Sprintf("Fitness (CTL)\n%.1f", last.CTL)),
		statStyle(fatigueColor).Render(fmt.Sprintf("Fatigue (ATL)\n%.1f", last.ATL)),
		statStyle(formColor).Render(fmt.Sprintf("Form (TSB)\n%.1f", last.TSB)),
	)
	trend := make([]float64, 0, len(currentPMC))
	for _, p := range currentPMC {
		trend = append(trend, p.CTL)
	}
	graph := asciigraph.Plot(trend, asciigraph.Height(6), asciigraph.Caption("CTL trend"))
	readiness := subtleBoxStyle.Render(renderReadinessSummary(last))
	weekly := subtleBoxStyle.Render(renderWeeklySummary(currentSummaries))
	runPerf := subtleBoxStyle.Render(renderRunPerformanceSummary(currentSummaries))
	trends := subtleBoxStyle.Render(renderMetricTrends(currentSummaries))
	deltas := subtleBoxStyle.Render(m.renderDashboardDelta())
	explain := subtleBoxStyle.Render(renderMeaningLegend(last))
	selector := subtleBoxStyle.Render(m.renderDashboardProviderSelector())
	return titleStyle.Render("Dashboard") + "\n\n" + selector + "\n\n" + top + "\n\n" + readiness + "\n\n" + deltas + "\n\n" + weekly + "\n\n" + runPerf + "\n\n" + trends + "\n\n" + graph + "\n\n" + explain
}

func (m Model) renderActivities() string {
	details := mutedStyle.Render("No selected activity.")
	if m.activityCursor >= 0 && m.activityCursor < len(m.summaries) {
		details = m.renderDetails(m.summaries[m.activityCursor])
	}
	return titleStyle.Render("Activities") + "\n\n" + m.activityTable.View() + "\n\n" + details
}

func (m Model) renderGarmin() string {
	details := mutedStyle.Render("No selected Garmin activity.")
	if m.garminCursor >= 0 && m.garminCursor < len(m.garminSumm) {
		details = m.renderDetails(m.garminSumm[m.garminCursor])
	}
	return titleStyle.Render("Garmin (Beta)") + "\n\n" + m.garminTable.View() + "\n\n" + details
}

func (m Model) renderSettings() string {
	fields := []string{
		fmt.Sprintf("Athlete Name: %s", m.settings.AthleteName),
		fmt.Sprintf("FTP: %.0f", m.settings.FTP),
		fmt.Sprintf("Age: %d", m.settings.Age),
		fmt.Sprintf("Max HR override: %.0f", m.settings.MaxHeartRate),
		fmt.Sprintf("HR Z1 max (bpm): %.0f", m.settings.HRZone1Max),
		fmt.Sprintf("HR Z2 max (bpm): %.0f", m.settings.HRZone2Max),
		fmt.Sprintf("HR Z3 max (bpm): %.0f", m.settings.HRZone3Max),
		fmt.Sprintf("HR Z4 max (bpm): %.0f", m.settings.HRZone4Max),
		fmt.Sprintf("Garmin FIT dir: %s", maskIfNeeded(m.settings.GarminFITDir, false)),
		fmt.Sprintf("Run activities only: %t", m.settings.RunOnly),
		fmt.Sprintf("Client ID: %s", maskIfNeeded(m.settings.ClientID, false)),
		fmt.Sprintf("Client Secret: %s", maskIfNeeded(m.settings.ClientSecret, true)),
		fmt.Sprintf("Auth Code: %s", maskIfNeeded(m.currentAuthCode(), false)),
	}
	for i := range fields {
		if i == m.settingsCursor {
			fields[i] = navItemActiveStyle.Render("-> " + fields[i])
		} else {
			fields[i] = navItemStyle.Render("   " + fields[i])
		}
	}
	edit := ""
	if m.editMode {
		edit = fmt.Sprintf("\n\nEditing: %s", m.inputBuffer)
	}
	return fmt.Sprintf(
		"%s\n\nProvider: %s\nConnected: %t\n\n%s\n\nDefaults if zones are empty:\n- Uses 220-age max HR (or override), with 60/70/80/90%% splits\n\nActions:\n- e edit selected field\n- o toggle Run activities only\n- s save settings\n- a open Strava auth page\n- x exchange auth code\n- press g anywhere to import Garmin FIT from Garmin FIT dir",
		titleStyle.Render("Settings"),
		m.dataProvider.Name(),
		m.settings.Connected,
		strings.Join(fields, "\n"),
	) + edit
}

func (m Model) renderDetails(s activitySummary) string {
	zones := renderZoneBars(s.Zones)
	hrZones := renderZoneBars(s.HRZones)
	verdict := sessionVerdict(s)
	// series := smoothSeries(pickSparkSeries(s.Activity), 5)
	// spark := asciigraph.Plot(downsample(series, 120), asciigraph.Height(6), asciigraph.Caption(sparkCaption(s.Activity)))

	zonesCard := cardStyle.Width(max(44, (m.width - 38))).Render(
		fmt.Sprintf("Time in Zones (%s)\n%s", s.ZoneBasis, zones),
	)
	// sparkCard := cardStyle.Width(max(44, (m.width-38)/2)).Render(
	// 	fmt.Sprintf("Session Trace\n%s", spark),
	// )
	detailGrid := lipgloss.JoinVertical(lipgloss.Top, zonesCard)
	if isRunSport(s.Activity.Sport) && totalZoneMinutes(s.HRZones) > 0 {
		hrCard := cardStyle.Width(max(44, (m.width - 38))).Render(
			fmt.Sprintf("Heart Rate Zones\n%s", hrZones+"\n"+zoneLegend()),
		)
		detailGrid = lipgloss.JoinVertical(lipgloss.Left, detailGrid, "\n"+hrCard)
	}
	if isRunSport(s.Activity.Sport) {
		economyCard := cardStyle.Width(max(44, (m.width - 38))).Render(
			renderRunningEconomy(s) + "\n" + renderDurabilityBlock(s),
		)
		detailGrid = lipgloss.JoinVertical(lipgloss.Left, detailGrid, "\n"+economyCard)
	}

	return fmt.Sprintf(
		"Details: %s\nDuration %s | Pace %s | AvgHR %.0f bpm\nNP %.0f | IF %.2f | TSS %.1f (%s) | TRIMP %.1f | EF(speed/HR) %.4f\nDecoupling %.2f%%\nSession type: %s (%s)\nSession verdict: %s\n%s\n\n%s",
		s.Activity.Name, s.Duration, s.AvgPace, s.AvgHR, s.NP, s.IF, s.TSS, s.TSSSource, s.TRIMP, s.EFSpeed, s.Decoupling, s.SessionClass, renderSessionConfidence(s.SessionConf), verdict, s.RunExplanation, detailGrid,
	)
}

func (m Model) loadActivitiesCmd(forceRefresh bool) tea.Cmd {
	m.loading = true
	return func() tea.Msg {
		activities, err := m.dataProvider.RecentActivities(0, forceRefresh)
		return activitiesLoadedMsg{activities: activities, fetchInfo: m.dataProvider.FetchInfo(), err: err}
	}
}

func (m Model) startGarminImportCmd() tea.Cmd {
	dir := m.settings.GarminFITDir
	return func() tea.Msg {
		ch := make(chan tea.Msg, 64)
		go func() {
			result, err := garminfit.LoadActivitiesFromDir(dir, func(p garminfit.Progress) {
				ch <- garminProgressMsg{progress: p}
			})
			ch <- garminLoadedMsg{result: result, err: err}
			close(ch)
		}()
		return asyncChannelReadyMsg{ch: ch}
	}
}

func (m Model) authURLCmd() tea.Cmd {
	return func() tea.Msg {
		u, err := m.dataProvider.AuthURL()
		return authURLMsg{url: u, err: err}
	}
}

func (m Model) exchangeCodeCmd() tea.Cmd {
	code := strings.TrimSpace(m.currentAuthCode())
	return func() tea.Msg {
		return exchangeMsg{err: m.dataProvider.ExchangeCode(code)}
	}
}

func openBrowserCmd(url string) tea.Cmd {
	return func() tea.Msg {
		_ = exec.Command("open", url).Run()
		return nil
	}
}

func (m *Model) persistSettings() error {
	if err := m.dataProvider.UpdateSettings(m.settings); err != nil {
		return err
	}
	m.profile = m.dataProvider.AthleteProfile()
	m.applyFilters()
	return nil
}

func (m Model) currentSettingValue() string {
	switch m.settingsCursor {
	case 0:
		return m.settings.AthleteName
	case 1:
		return fmt.Sprintf("%.0f", m.settings.FTP)
	case 2:
		return fmt.Sprintf("%d", m.settings.Age)
	case 3:
		return fmt.Sprintf("%.0f", m.settings.MaxHeartRate)
	case 4:
		return fmt.Sprintf("%.0f", m.settings.HRZone1Max)
	case 5:
		return fmt.Sprintf("%.0f", m.settings.HRZone2Max)
	case 6:
		return fmt.Sprintf("%.0f", m.settings.HRZone3Max)
	case 7:
		return fmt.Sprintf("%.0f", m.settings.HRZone4Max)
	case 8:
		return m.settings.GarminFITDir
	case 9:
		return strconv.FormatBool(m.settings.RunOnly)
	case 10:
		return m.settings.ClientID
	case 11:
		return m.settings.ClientSecret
	default:
		return m.currentAuthCode()
	}
}

func (m *Model) applyCurrentSetting(v string) {
	switch m.settingsCursor {
	case 0:
		m.settings.AthleteName = strings.TrimSpace(v)
	case 1:
		if ftp, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil && ftp > 0 {
			m.settings.FTP = ftp
		}
	case 2:
		if age, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && age > 0 {
			m.settings.Age = age
		}
	case 3:
		if maxHR, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil && maxHR >= 0 {
			m.settings.MaxHeartRate = maxHR
		}
	case 4:
		if z, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil && z >= 0 {
			m.settings.HRZone1Max = z
		}
	case 5:
		if z, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil && z >= 0 {
			m.settings.HRZone2Max = z
		}
	case 6:
		if z, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil && z >= 0 {
			m.settings.HRZone3Max = z
		}
	case 7:
		if z, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil && z >= 0 {
			m.settings.HRZone4Max = z
		}
	case 8:
		m.settings.GarminFITDir = strings.TrimSpace(v)
	case 9:
		if b, err := strconv.ParseBool(strings.ToLower(strings.TrimSpace(v))); err == nil {
			m.settings.RunOnly = b
		}
	case 10:
		m.settings.ClientID = strings.TrimSpace(v)
	case 11:
		m.settings.ClientSecret = strings.TrimSpace(v)
	case 12:
		m.status = "Auth code captured. Press x to exchange."
		m.setAuthCode(v)
	}
}

func (m *Model) setAuthCode(v string) {
	m.authCode = strings.TrimSpace(v)
}

func (m Model) currentAuthCode() string {
	return m.authCode
}

func maskIfNeeded(v string, secret bool) string {
	if v == "" {
		return "(empty)"
	}
	if !secret {
		return v
	}
	if len(v) <= 4 {
		return "****"
	}
	return strings.Repeat("*", len(v)-4) + v[len(v)-4:]
}

func newActivityTable(summaries []activitySummary) table.Model {
	columns := []table.Column{
		{Title: "Date", Width: 12},
		{Title: "Title", Width: 24},
		{Title: "Dur", Width: 7},
		{Title: "Pace", Width: 9},
		{Title: "AvgHR", Width: 7},
		{Title: "TSS", Width: 8},
		{Title: "TSSSrc", Width: 8},
		{Title: "IF", Width: 8},
		{Title: "Decoupling", Width: 14},
	}
	rows := make([]table.Row, 0, len(summaries))
	for _, s := range summaries {
		rows = append(rows, table.Row{
			s.Activity.StartTime.Format("2006-01-02"),
			s.Activity.Name,
			s.Duration,
			s.AvgPace,
			formatAvgHRCell(s.AvgHR),
			fmt.Sprintf("%.1f", s.TSS),
			s.TSSSource,
			fmt.Sprintf("%.2f", s.IF),
			colorDecoupling(s.Decoupling),
		})
	}

	t := table.New(table.WithColumns(columns), table.WithRows(rows), table.WithFocused(true), table.WithHeight(8))
	st := table.DefaultStyles()
	st.Header = st.Header.BorderStyle(lipgloss.NormalBorder()).BorderBottom(true).Bold(true)
	st.Selected = st.Selected.Foreground(lipgloss.Color("230")).Background(lipgloss.Color("62")).Bold(true)
	t.SetStyles(st)
	return t
}

func buildSummaries(activities []domain.Activity, settings provider.Settings) []activitySummary {
	out := make([]activitySummary, 0, len(activities))
	hrBounds := physics.HeartRateZoneBounds(
		settings.Age,
		settings.MaxHeartRate,
		settings.HRZone1Max,
		settings.HRZone2Max,
		settings.HRZone3Max,
		settings.HRZone4Max,
	)
	for _, a := range activities {
		np, _ := physics.NormalizedPowerFromTime(a.Power, a.TimeSec)
		avgHR, _ := physics.AvgHeartRate(a.HeartRate)
		zones, basis := deriveZones(a, settings.FTP, hrBounds)
		hrZones, _ := physics.TimeInHeartRateZonesMinutesWithBounds(a.HeartRate, a.TimeSec, hrBounds)
		trimp := physics.TRIMPFromZones(hrZones)
		ifVal, _ := physics.IntensityFactor(np, settings.FTP)
		tss, _ := physics.TrainingStressScore(int(a.Duration.Seconds()), np, ifVal, settings.FTP)
		tssSource := "power"
		if !hasUsablePower(a.Power) || np <= 0 || tss <= 0 {
			tss = physics.EstimatedTSSFromHRZones(hrZones, int(a.Duration.Seconds()))
			tssSource = "hr-est"
			if tss > 0 {
				// Approximate IF from TSS for display consistency when power is absent.
				hours := a.Duration.Hours()
				if hours > 0 {
					ifVal = math.Sqrt((tss / 100.0) / hours)
				}
			}
		}
		tss = sanitizeTSS(tss)
		decoupling, _ := physics.AerobicDecoupling(a)
		efSpeed, _ := physics.SpeedEfficiencyFactor(a.SpeedMS, avgHR)
		vertRatio, _ := physics.VerticalRatio(a.AvgVerticalOscillationCM, a.AvgStrideLengthM)
		durability, _ := physics.AerobicDurability(a)
		_, cadenceStdDev, cadenceDropPct, _ := physics.CadenceMetrics(a)
		formBreakdown, _ := physics.DetectFormBreakdown(a)
		sessionClass := physics.ClassifySession(a, hrZones)
		sessionConf := physics.SessionClassificationConfidence(a, hrZones, sessionClass)
		runExplanation := physics.ExplainRun(sessionClass, durability, formBreakdown, cadenceDropPct)
		out = append(out, activitySummary{
			Activity:        a,
			NP:              np,
			IF:              ifVal,
			TSS:             tss,
			TSSSource:       tssSource,
			TRIMP:           trimp,
			EFSpeed:         efSpeed,
			Decoupling:      decoupling,
			AvgHR:           avgHR,
			AvgCadence:      a.AvgCadence,
			VertOscCM:       a.AvgVerticalOscillationCM,
			VertRatio:       vertRatio,
			DecoupleAtMin:   durability.DecouplingStartMinutes,
			DurabilityScore: durability.Score,
			HRStabilityPct:  durability.HRStabilityPct,
			CadenceStdDev:   cadenceStdDev,
			CadenceDropPct:  cadenceDropPct,
			FormBreakdown:   formBreakdown.Detected,
			FormBreakdownAt: formBreakdown.StartMin,
			SessionClass:    sessionClass,
			SessionConf:     sessionConf,
			RunExplanation:  runExplanation,
			AvgPace:         formatPace(a.Sport, a.DistanceKM, a.Duration),
			Duration:        formatDurationCompact(a.Duration),
			Zones:           zones,
			ZoneBasis:       basis,
			HRZones:         hrZones,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Activity.StartTime.After(out[j].Activity.StartTime) })
	return out
}

func sanitizeTSS(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
		return 0
	}
	// Guardrail against broken activity records producing unrealistic load spikes.
	// Typical single-session TSS is well below this threshold.
	const maxReasonableSessionTSS = 500.0
	if v > maxReasonableSessionTSS {
		return maxReasonableSessionTSS
	}
	return v
}

func buildPMC(summaries []activitySummary) []physics.PMCPoint {
	daily := map[string]float64{}
	dateMap := map[string]time.Time{}
	for _, s := range summaries {
		d := time.Date(s.Activity.StartTime.Year(), s.Activity.StartTime.Month(), s.Activity.StartTime.Day(), 0, 0, 0, 0, s.Activity.StartTime.Location())
		k := d.Format("2006-01-02")
		daily[k] += s.TSS
		dateMap[k] = d
	}
	keys := make([]string, 0, len(daily))
	for k := range daily {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	loads := make([]physics.DailyLoad, 0, len(keys))
	for _, k := range keys {
		loads = append(loads, physics.DailyLoad{Date: dateMap[k], TSS: daily[k]})
	}
	return physics.PerformanceManagement(loads)
}

func renderZoneBars(z [5]float64) string {
	total := 0.0
	for _, v := range z {
		total += v
	}
	if total <= 0 {
		return "No zone data available."
	}
	const maxWidth = 28
	var lines []string
	for i, v := range z {
		pct := (v / total) * 100
		width := int((pct / 100) * maxWidth)
		if width == 0 && v > 0 {
			width = 1
		}
		line := fmt.Sprintf("Z%d | %-28s %5.1f min (%4.1f%%)", i+1, strings.Repeat("#", width), v, pct)
		lines = append(lines, zoneStyle(i).Render(line))
	}
	return strings.Join(lines, "\n")
}

//nolint:unused
func downsample(v []float64, maxPoints int) []float64 {
	if len(v) <= maxPoints || maxPoints <= 1 {
		return v
	}
	step := len(v) / maxPoints
	if step < 1 {
		step = 1
	}
	out := make([]float64, 0, maxPoints+1)
	for i := 0; i < len(v); i += step {
		out = append(out, v[i])
	}
	return out
}

//nolint:unused
func smoothSeries(v []float64, window int) []float64 {
	if len(v) == 0 || window <= 1 {
		return v
	}
	out := make([]float64, len(v))
	half := window / 2
	for i := range v {
		start := i - half
		if start < 0 {
			start = 0
		}
		end := i + half + 1
		if end > len(v) {
			end = len(v)
		}
		sum := 0.0
		for j := start; j < end; j++ {
			sum += v[j]
		}
		out[i] = sum / float64(end-start)
	}
	return out
}

func colorDecoupling(v float64) string {
	txt := fmt.Sprintf("%.2f%%", v)
	switch {
	case v < 5:
		return goodStyle.Render(txt)
	case v <= 10:
		return warnStyle.Render(txt)
	default:
		return badStyle.Render(txt)
	}
}

func colorVerticalRatio(v float64) string {
	txt := fmt.Sprintf("%.1f%%", v)
	switch {
	case v < 7:
		return goodStyle.Render(txt)
	case v <= 9:
		return warnStyle.Render(txt)
	default:
		return badStyle.Render(txt)
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func deriveZones(a domain.Activity, ftp float64, hrBounds [4]float64) ([5]float64, string) {
	if hasUsablePower(a.Power) && len(a.Power) == len(a.TimeSec) {
		if z, err := physics.TimeInPowerZonesMinutes(a.Power, a.TimeSec, ftp); err == nil {
			return z, "Power"
		}
	}
	if len(a.HeartRate) == len(a.TimeSec) {
		if z, err := physics.TimeInHeartRateZonesMinutesWithBounds(a.HeartRate, a.TimeSec, hrBounds); err == nil {
			return z, "Heart Rate"
		}
	}
	return [5]float64{}, "Unavailable"
}

func hasUsablePower(power []float64) bool {
	if len(power) == 0 {
		return false
	}
	nonZero := 0
	for _, p := range power {
		if p > 0 {
			nonZero++
		}
	}
	return float64(nonZero)/float64(len(power)) > 0.7
}

func formatPace(sport string, distanceKM float64, duration time.Duration) string {
	if distanceKM <= 0 || duration <= 0 {
		return "-"
	}
	s := strings.ToLower(sport)
	if strings.Contains(s, "run") || strings.Contains(s, "walk") {
		secPerKM := duration.Seconds() / distanceKM
		minPart := int(secPerKM) / 60
		secPart := int(secPerKM) % 60
		return fmt.Sprintf("%d:%02d/km", minPart, secPart)
	}
	speed := distanceKM / duration.Hours()
	return fmt.Sprintf("%.1f km/h", speed)
}

func formatDurationCompact(d time.Duration) string {
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	return fmt.Sprintf("%02d:%02d", h, m)
}

func formatAvgHRCell(v float64) string {
	if v <= 0 {
		return "-"
	}
	return fmt.Sprintf("%.0f", v)
}

//nolint:unused
func pickSparkSeries(a domain.Activity) []float64 {
	if hasUsablePower(a.Power) {
		return a.Power
	}
	if len(a.HeartRate) > 0 {
		return a.HeartRate
	}
	return []float64{0}
}

//nolint:unused
func sparkCaption(a domain.Activity) string {
	if hasUsablePower(a.Power) {
		return "Power sparkline"
	}
	return "Heart rate sparkline"
}

func sessionVerdict(s activitySummary) string {
	total := 0.0
	for _, v := range s.Zones {
		total += v
	}
	if total <= 0 {
		return "Insufficient zone data"
	}
	z1z2 := (s.Zones[0] + s.Zones[1]) / total * 100
	z4z5 := (s.Zones[3] + s.Zones[4]) / total * 100
	z3 := s.Zones[2] / total * 100

	switch {
	case z4z5 >= 30:
		return "High intensity day; prioritize recovery in the next 24-48h"
	case z1z2 >= 70:
		return "Aerobic endurance focused; great for base development"
	case z3 >= 35:
		return "Steady tempo/threshold-oriented session"
	default:
		return "Mixed load session; balanced training stimulus"
	}
}

func renderRunningEconomy(s activitySummary) string {
	return fmt.Sprintf(
		"Running Economy\nAverage Cadence: %s spm\nCadence consistency (sd): %s spm\nCadence drop late run: %s\nVertical Oscillation: %s cm\nVertical Ratio: %s",
		formatMetricValue(s.AvgCadence, "%.1f"),
		formatMetricValue(s.CadenceStdDev, "%.2f"),
		formatPercentValue(s.CadenceDropPct),
		formatMetricValue(s.VertOscCM, "%.1f"),
		formatVerticalRatioValue(s.VertRatio),
	)
}

func renderDurabilityBlock(s activitySummary) string {
	breakdown := "not detected"
	if s.FormBreakdown {
		breakdown = formatMinutesValue(s.FormBreakdownAt)
	}
	return fmt.Sprintf(
		"\nAerobic Durability\nTime to decoupling: %s\nHR stability: %s\nDurability score: %s\nForm breakdown: %s",
		formatMinutesValue(s.DecoupleAtMin),
		formatPercentValue(s.HRStabilityPct),
		colorDurabilityScore(s.DurabilityScore),
		breakdown,
	)
}

func zoneStyle(idx int) lipgloss.Style {
	switch idx {
	case 0:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("111"))
	case 1:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("117"))
	case 2:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("150"))
	case 3:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("221"))
	default:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	}
}

func renderReadinessSummary(p physics.PMCPoint) string {
	var formState string
	switch {
	case p.TSB < -15:
		formState = "high fatigue load, prioritize recovery"
	case p.TSB < -5:
		formState = "productive strain, keep quality controlled"
	case p.TSB <= 10:
		formState = "good readiness for normal quality work"
	default:
		formState = "very fresh, suitable for key sessions or testing"
	}
	return fmt.Sprintf("Today: CTL %.1f | ATL %.1f | TSB %.1f\nCoaching read: %s", p.CTL, p.ATL, p.TSB, formState)
}

func renderMeaningLegend(p physics.PMCPoint) string {
	load := "moderate"
	if p.CTL >= 70 {
		load = "high"
	} else if p.CTL < 35 {
		load = "building/base"
	}
	return fmt.Sprintf(
		"How to read this:\n- Fitness (CTL): your long-term training load (%s)\n- Fatigue (ATL): your short-term load (last ~7 days)\n- Form (TSB): freshness = CTL - ATL (higher = fresher)\n- IF: workout intensity vs FTP (1.00 = FTP effort)\n- TSS: training load score (100 ~= 1 hour at FTP)\n- TRIMP: HR-zone weighted impulse (internal load)\n- EF(speed/HR): aerobic efficiency trend; higher over time is usually better\n- Decoupling: aerobic durability drift; <5%% is usually strong steady-state",
		load,
	)
}

func renderWeeklySummary(summaries []activitySummary) string {
	if len(summaries) == 0 {
		return "Last 7 days: no sessions"
	}
	cutoff := time.Now().AddDate(0, 0, -7)
	sessions := 0
	totalTSS := 0.0
	longest := 0 * time.Minute
	longestName := "-"
	for _, s := range summaries {
		if s.Activity.StartTime.Before(cutoff) {
			continue
		}
		sessions++
		totalTSS += s.TSS
		if s.Activity.Duration > longest {
			longest = s.Activity.Duration
			longestName = s.Activity.Name
		}
	}
	if sessions == 0 {
		return "Last 7 days: no sessions"
	}
	return fmt.Sprintf(
		"Last 7 days: %d sessions | Total TSS %.1f | Avg TSS %.1f\nLongest session: %s (%s)",
		sessions, totalTSS, totalTSS/float64(sessions), longestName, formatDurationCompact(longest),
	)
}

func renderRunPerformanceSummary(summaries []activitySummary) string {
	if len(summaries) == 0 {
		return "Run metrics: no data"
	}
	activities := make([]domain.Activity, 0, len(summaries))
	efSum := 0.0
	efCount := 0
	trimpSum := 0.0
	for _, s := range summaries {
		activities = append(activities, s.Activity)
		if s.EFSpeed > 0 {
			efSum += s.EFSpeed
			efCount++
		}
		trimpSum += s.TRIMP
	}
	cs, err := physics.CriticalSpeed(activities)
	csText := "n/a"
	dPrimeText := "n/a"
	if err == nil {
		pace := 0.0
		if cs.CSMS > 0 {
			pace = 1000.0 / (cs.CSMS * 60.0)
		}
		csText = fmt.Sprintf("%.2f m/s (%.2f min/km)", cs.CSMS, pace)
		dPrimeText = fmt.Sprintf("%.0f m", cs.DPrimeM)
	}
	efText := "n/a"
	if efCount > 0 {
		efText = fmt.Sprintf("%.4f", efSum/float64(efCount))
	}
	return fmt.Sprintf("Run performance:\n- Critical Speed: %s\n- D': %s\n- Avg EF(speed/HR): %s\n- Total TRIMP (loaded set): %.1f", csText, dPrimeText, efText, trimpSum)
}

func zoneLegend() string {
	return mutedStyle.Render("Legend: Z1 easy | Z2 endurance | Z3 tempo | Z4 threshold | Z5 VO2+")
}

func isRunSport(sport string) bool {
	s := strings.ToLower(sport)
	return strings.Contains(s, "run") || strings.Contains(s, "walk") || strings.Contains(s, "trail")
}

func totalZoneMinutes(z [5]float64) float64 {
	total := 0.0
	for _, v := range z {
		total += v
	}
	return total
}

func formatMetricValue(v float64, format string) string {
	if v <= 0 || math.IsNaN(v) || math.IsInf(v, 0) {
		return "n/a"
	}
	return fmt.Sprintf(format, v)
}

func formatVerticalRatioValue(v float64) string {
	if v <= 0 || math.IsNaN(v) || math.IsInf(v, 0) {
		return "n/a"
	}
	return colorVerticalRatio(v)
}

func formatPercentValue(v float64) string {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return "n/a"
	}
	return fmt.Sprintf("%.1f%%", v)
}

func formatMinutesValue(v float64) string {
	if v <= 0 || math.IsNaN(v) || math.IsInf(v, 0) {
		return "not detected"
	}
	return fmt.Sprintf("%.0f min", v)
}

func colorDurabilityScore(v float64) string {
	if v <= 0 || math.IsNaN(v) || math.IsInf(v, 0) {
		return "n/a"
	}
	txt := fmt.Sprintf("%.0f/100", v)
	switch {
	case v >= 80:
		return goodStyle.Render(txt)
	case v >= 60:
		return warnStyle.Render(txt)
	default:
		return badStyle.Render(txt)
	}
}

func renderSessionConfidence(v float64) string {
	switch {
	case v >= 0.8:
		return goodStyle.Render("high confidence")
	case v >= 0.6:
		return warnStyle.Render("medium confidence")
	default:
		return badStyle.Render("low confidence")
	}
}

func renderMetricTrends(summaries []activitySummary) string {
	now := time.Now()
	last7Start := now.AddDate(0, 0, -7)
	prev7Start := now.AddDate(0, 0, -14)
	last28Start := now.AddDate(0, 0, -28)
	prev28Start := now.AddDate(0, 0, -56)

	last7EF, prev7EF := avgRange(summaries, last7Start, now, "ef"), avgRange(summaries, prev7Start, last7Start, "ef")
	last7TR, prev7TR := avgRange(summaries, last7Start, now, "trimp"), avgRange(summaries, prev7Start, last7Start, "trimp")
	last7Dec, prev7Dec := avgRange(summaries, last7Start, now, "dec"), avgRange(summaries, prev7Start, last7Start, "dec")

	last28EF, prev28EF := avgRange(summaries, last28Start, now, "ef"), avgRange(summaries, prev28Start, last28Start, "ef")
	last28TR, prev28TR := avgRange(summaries, last28Start, now, "trimp"), avgRange(summaries, prev28Start, last28Start, "trimp")
	last28Dec, prev28Dec := avgRange(summaries, last28Start, now, "dec"), avgRange(summaries, prev28Start, last28Start, "dec")

	return fmt.Sprintf(
		"Trends (7d vs previous 7d / 28d vs previous 28d)\n- EF(speed/HR): %s / %s\n- TRIMP avg: %s / %s\n- Decoupling avg: %s / %s",
		trendText(last7EF, prev7EF, false),
		trendText(last28EF, prev28EF, false),
		trendText(last7TR, prev7TR, false),
		trendText(last28TR, prev28TR, false),
		trendText(last7Dec, prev7Dec, true),
		trendText(last28Dec, prev28Dec, true),
	)
}

func avgRange(summaries []activitySummary, start, end time.Time, metric string) float64 {
	sum := 0.0
	count := 0
	for _, s := range summaries {
		t := s.Activity.StartTime
		if t.Before(start) || !t.Before(end) {
			continue
		}
		v := 0.0
		switch metric {
		case "ef":
			v = s.EFSpeed
		case "trimp":
			v = s.TRIMP
		case "dec":
			v = s.Decoupling
		}
		if v > 0 {
			sum += v
			count++
		}
	}
	if count == 0 {
		return 0
	}
	return sum / float64(count)
}

func trendText(current, previous float64, lowerIsBetter bool) string {
	if current <= 0 || previous <= 0 {
		return "n/a"
	}
	delta := ((current - previous) / previous) * 100
	icon := "->"
	better := delta > 0
	if lowerIsBetter {
		better = delta < 0
	}
	switch {
	case delta > 1:
		icon = "↑"
	case delta < -1:
		icon = "↓"
	}
	quality := "neutral"
	if better {
		quality = "better"
	} else if delta != 0 {
		quality = "worse"
	}
	return fmt.Sprintf("%.3f (%s %.1f%%, %s)", current, icon, delta, quality)
}

func formatFetchTime(ts string) string {
	if ts == "" {
		return "unknown"
	}
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return ts
	}
	return t.Format("2006-01-02 15:04")
}

func waitForAsyncMsg(ch <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return nil
		}
		return msg
	}
}

func (m *Model) toggleDashboardProvider() {
	if m.dashProvider == "strava" {
		if len(m.garminSumm) == 0 {
			m.status = "Garmin dashboard source unavailable: no Garmin activities loaded."
			return
		}
		m.dashProvider = "garmin"
		m.status = "Dashboard source set to Garmin."
		return
	}
	m.dashProvider = "strava"
	m.status = "Dashboard source set to Strava."
}

func (m Model) dashboardSummaries() []activitySummary {
	if m.dashProvider == "garmin" {
		return m.garminSumm
	}
	return m.summaries
}

func (m Model) renderDashboardProviderSelector() string {
	strava := tabStyle.Render("Strava")
	garmin := tabStyle.Render("Garmin")
	if m.dashProvider == "strava" {
		strava = activeTabStyle.Render("Strava")
	}
	if len(m.garminSumm) == 0 {
		garmin = mutedStyle.Render("Garmin (disabled)")
	} else if m.dashProvider == "garmin" {
		garmin = activeTabStyle.Render("Garmin")
	}
	return "Dashboard source (press p): " + strava + " " + garmin
}

func (m *Model) applyFilters() {
	m.activities = filterActivitiesBySettings(m.rawActivities, m.settings)
	m.garminActs = filterActivitiesBySettings(m.rawGarminActs, m.settings)

	m.summaries = buildSummaries(m.activities, m.settings)
	m.garminSumm = buildSummaries(m.garminActs, m.settings)
	m.pmc = buildPMC(m.summaries)

	m.activityTable = newActivityTable(m.summaries)
	if m.activityCursor >= len(m.summaries) {
		m.activityCursor = max(len(m.summaries)-1, 0)
	}
	m.activityTable.SetCursor(m.activityCursor)

	m.garminTable = newActivityTable(m.garminSumm)
	if m.garminCursor >= len(m.garminSumm) {
		m.garminCursor = max(len(m.garminSumm)-1, 0)
	}
	m.garminTable.SetCursor(m.garminCursor)
}

func filterActivitiesBySettings(activities []domain.Activity, settings provider.Settings) []domain.Activity {
	if !settings.RunOnly {
		return activities
	}
	filtered := make([]domain.Activity, 0, len(activities))
	for _, a := range activities {
		if isRunSport(a.Sport) {
			filtered = append(filtered, a)
		}
	}
	return filtered
}

func (m *Model) captureReloadBaseline(providerKey string) {
	if p, ok := latestPMCForProvider(m, providerKey); ok {
		m.preReload[providerKey] = p
	}
}

func (m *Model) updateDashboardDelta(providerKey string) {
	before, ok := m.preReload[providerKey]
	if !ok {
		return
	}
	after, ok := latestPMCForProvider(m, providerKey)
	if !ok {
		return
	}
	m.dashDelta[providerKey] = dashboardDelta{
		Has:  true,
		CTL:  after.CTL - before.CTL,
		ATL:  after.ATL - before.ATL,
		TSB:  after.TSB - before.TSB,
		CTLP: pctChange(before.CTL, after.CTL),
		ATLP: pctChange(before.ATL, after.ATL),
		TSBP: pctChange(before.TSB, after.TSB),
	}
	delete(m.preReload, providerKey)
}

func latestPMCForProvider(m *Model, providerKey string) (physics.PMCPoint, bool) {
	var summaries []activitySummary
	if providerKey == "garmin" {
		summaries = m.garminSumm
	} else {
		summaries = m.summaries
	}
	pmc := buildPMC(summaries)
	if len(pmc) == 0 {
		return physics.PMCPoint{}, false
	}
	return pmc[len(pmc)-1], true
}

func pctChange(before, after float64) float64 {
	if before == 0 {
		return 0
	}
	return ((after - before) / math.Abs(before)) * 100.0
}

func (m Model) renderDashboardDelta() string {
	key := m.dashProvider
	delta, ok := m.dashDelta[key]
	if !ok || !delta.Has {
		return "Change since last reload: n/a (reload current provider to populate)."
	}
	return fmt.Sprintf(
		"Change since last reload\nCTL %s (%+.1f%%) | ATL %s (%+.1f%%) | TSB %s (%+.1f%%)",
		fmtSigned(delta.CTL),
		delta.CTLP,
		fmtSigned(delta.ATL),
		delta.ATLP,
		fmtSigned(delta.TSB),
		delta.TSBP,
	)
}

func fmtSigned(v float64) string {
	return fmt.Sprintf("%+.1f", v)
}
