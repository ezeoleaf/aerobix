package ui

import (
	"fmt"
	"math"
	"os"
	"os/exec"
	pathpkg "path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"aerobix/domain"
	"aerobix/garminfit"
	"aerobix/paths"
	"aerobix/physics"
	"aerobix/provider"
	"aerobix/provider/mock"
	"aerobix/provider/strava"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
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

	GapPaceText       string
	UphillTimePct     float64
	DownhillBrakeLoad float64
	RSSPoints         float64
	RSSSource         string
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

// chromeAppPad* match appStyle padding so the scroll viewport fills the usable area.
const (
	chromeAppPadV       = 2
	chromeAppPadH       = 4
	chromeHeaderReserve = 2 // tab row + bottom border
	chromeFooterReserve = 3 // two text lines + top border
)

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

	profileIDSetting string

	profilePickerOpen    bool
	profilePickerCursor  int
	profilePickerChoices []string

	createProfileOpen bool

	fitNotifyChan          chan<- tea.Msg
	fitWatcherCtl          *fitWatcherCtl
	pendingGarminFITRescan bool

	mainViewport viewport.Model
}

func NewModel(dataProvider provider.DataProvider, fitNotify chan<- tea.Msg) Model {
	settings := dataProvider.Settings()
	ctl := newFitWatcherCtl()
	if fitNotify != nil {
		ctl.Restart(settings.GarminFITDir, fitNotify)
	}
	m := Model{
		dataProvider:     dataProvider,
		settings:         settings,
		profile:          dataProvider.AthleteProfile(),
		profileIDSetting: paths.ActiveProfile(),
		activityTable:    newActivityTable(nil),
		garminTable:      newActivityTable(nil),
		loading:          true,
		status:           "Press r to reload activities.",
		settingsCursor:   0,
		dashProvider:     "strava",
		preReload:        map[string]physics.PMCPoint{},
		dashDelta:        map[string]dashboardDelta{},
		fitNotifyChan:    fitNotify,
		fitWatcherCtl:    ctl,
		mainViewport:     newMainViewport(),
	}
	return m.applyWindowLayout(80, 24)
}

func newMainViewport() viewport.Model {
	vp := viewport.New(0, 0)
	vp.KeyMap = viewport.KeyMap{
		PageDown: key.NewBinding(
			key.WithKeys("pgdown"),
			key.WithHelp("pgdn", "page down"),
		),
		PageUp: key.NewBinding(
			key.WithKeys("pgup"),
			key.WithHelp("pgup", "page up"),
		),
		HalfPageDown: key.NewBinding(
			key.WithKeys("ctrl+d"),
			key.WithHelp("ctrl+d", "½ page down"),
		),
		HalfPageUp: key.NewBinding(
			key.WithKeys("ctrl+u"),
			key.WithHelp("ctrl+u", "½ page up"),
		),
		Down:  key.NewBinding(),
		Up:    key.NewBinding(),
		Left:  key.NewBinding(),
		Right: key.NewBinding(),
	}
	vp.MouseWheelEnabled = true
	vp.MouseWheelDelta = 3
	vp.Style = lipgloss.NewStyle().
		Padding(0, 1).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240"))
	return vp
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.loadActivitiesCmd(false), spinnerTickCmd())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := m.dispatchUpdate(msg)
	mm := next.(Model)
	if !mm.profilePickerOpen && !mm.createProfileOpen && !mm.editMode {
		mm.mainViewport.SetContent(mm.renderScrollableMainBody())
	}
	return mm, cmd
}

func (m Model) dispatchUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m = m.applyWindowLayout(msg.Width, msg.Height)
		return m, nil
	case tea.MouseMsg:
		if m.profilePickerOpen || m.createProfileOpen || m.editMode {
			return m, nil
		}
		m.mainViewport.SetContent(m.renderScrollableMainBody())
		var vpCmd tea.Cmd
		m.mainViewport, vpCmd = m.mainViewport.Update(msg)
		return m, vpCmd
	case activitiesLoadedMsg:
		m.loading = false
		if msg.err != nil {
			m.status = "Load failed: " + msg.err.Error()
		} else {
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
		}
		if m.pendingGarminFITRescan && m.asyncCh == nil {
			m.pendingGarminFITRescan = false
			m.captureReloadBaseline("garmin")
			m.loading = true
			m.status = "FIT folder queued during Strava reload — importing Garmin…"
			return m, tea.Batch(m.startGarminImportCmd(), spinnerTickCmd())
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
	case garminFITWatchTriggerMsg:
		if len(msg.Paths) == 0 {
			return m, nil
		}
		short := pathpkg.Base(msg.Paths[0])
		if len(msg.Paths) > 1 {
			short += fmt.Sprintf(" +%d more", len(msg.Paths)-1)
		}
		TryDesktopNotify("Aerobix", "New Garmin FIT file — importing: "+short)
		m.status = fmt.Sprintf("FIT folder: new file %s — importing Garmin…", short)
		if m.asyncCh != nil || m.loading {
			m.pendingGarminFITRescan = true
			return m, nil
		}
		m.captureReloadBaseline("garmin")
		m.loading = true
		return m, tea.Batch(m.startGarminImportCmd(), spinnerTickCmd())
	case garminLoadedMsg:
		m.loading = false
		m.asyncCh = nil
		if msg.err != nil {
			m.pendingGarminFITRescan = false
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
		if m.pendingGarminFITRescan {
			m.pendingGarminFITRescan = false
			m.captureReloadBaseline("garmin")
			m.loading = true
			m.status = "FIT folder changed again during import — refreshing Garmin…"
			return m, tea.Batch(m.startGarminImportCmd(), spinnerTickCmd())
		}
		return m, nil
	case tea.KeyMsg:
		if m.profilePickerOpen {
			return m.handleProfilePickerKey(msg)
		}
		if m.createProfileOpen {
			return m.handleCreateProfileKeys(msg)
		}
		if m.editMode {
			return m.handleEditKeys(msg)
		}
		m.mainViewport.SetContent(m.renderScrollableMainBody())
		m.mainViewport, _ = m.mainViewport.Update(msg)
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
		case "P":
			return m.openProfilePicker()
		case "left", "h":
			if m.navCursor > 0 {
				m.navCursor--
				m.mainViewport.GotoTop()
			}
		case "right", "l":
			if m.navCursor < len(navItems)-1 {
				m.navCursor++
				m.mainViewport.GotoTop()
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
			if navItems[m.navCursor] == "Settings" && m.settingsCursor < 13 {
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
		case "n":
			if navItems[m.navCursor] == "Settings" {
				m.createProfileOpen = true
				m.inputBuffer = ""
				m.status = "New profile: type id (a-z, 0-9, -, _), Enter create, Esc cancel."
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

func (m Model) handleCreateProfileKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.createProfileOpen = false
		m.inputBuffer = ""
		m.status = "New profile cancelled."
		return m, nil
	case "enter":
		raw := strings.TrimSpace(m.inputBuffer)
		m.createProfileOpen = false
		m.inputBuffer = ""
		if raw == "" {
			m.status = "New profile: empty name."
			return m, nil
		}
		id, err := paths.CreateProfile(raw)
		if err != nil {
			m.status = "New profile: " + err.Error()
			return m, nil
		}
		return m.switchToProfile(id)
	case "backspace":
		if len(m.inputBuffer) > 0 {
			m.inputBuffer = m.inputBuffer[:len(m.inputBuffer)-1]
		}
		return m, nil
	default:
		if msg.Type == tea.KeyRunes && len(msg.Runes) > 0 {
			m.inputBuffer += string(msg.Runes)
		}
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
	if m.profilePickerOpen {
		return m.renderProfilePickerScreen()
	}
	header := m.renderHeaderTabs()
	body := m.mainViewport.View()
	footer := m.renderFooter()
	stack := lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
	return appStyle.Render(stack)
}

func (m Model) applyWindowLayout(w, h int) Model {
	if w <= 0 {
		w = 80
	}
	if h <= 0 {
		h = 24
	}
	m.width = w
	m.height = h
	contentW := max(40, w-chromeAppPadH)
	innerH := max(8, h-chromeAppPadV)
	vpH := max(6, innerH-chromeHeaderReserve-chromeFooterReserve)
	m.mainViewport.Width = contentW
	m.mainViewport.Height = vpH
	tw := max(contentW-6, 40)
	th := max(6, min(16, vpH/3))
	m.activityTable.SetWidth(tw)
	m.activityTable.SetHeight(th)
	m.garminTable.SetWidth(tw)
	m.garminTable.SetHeight(th)
	return m
}

func (m Model) renderHeaderTabs() string {
	var rows []string
	for i, item := range navItems {
		style := tabStyle
		if i == m.navCursor {
			style = activeTabStyle
		}
		rows = append(rows, style.Render(item))
	}
	bar := lipgloss.JoinHorizontal(lipgloss.Top, rows...)
	w := max(20, m.width-chromeAppPadH)
	return headerBarStyle.Width(w).Render(bar)
}

func (m Model) renderFooter() string {
	w := max(20, m.width-chromeAppPadH)
	keys := "h/l tabs · j/k lists · P profiles · r Strava · g Garmin · p dash · PgUp/PgDn scroll · Ctrl+u/d · wheel · q quit"
	line1 := ansi.Truncate(keys, w, "…")
	st := strings.TrimSpace(m.status)
	if st == "" {
		st = " "
	}
	line2 := ansi.Truncate(st, w, "…")
	return footerStyle.Width(w).Render(mutedStyle.Render(line1) + "\n" + mutedStyle.Render(line2))
}

func (m Model) renderScrollableMainBody() string {
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
	return content + loading
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
	loadAnalytics := subtleBoxStyle.Render(renderLoadAnalytics(currentPMC, currentSummaries))
	weekly := subtleBoxStyle.Render(renderWeeklySummary(currentSummaries))
	runPerf := subtleBoxStyle.Render(renderRunPerformanceSummary(currentSummaries))
	trends := subtleBoxStyle.Render(renderMetricTrends(currentSummaries))
	deltas := subtleBoxStyle.Render(m.renderDashboardDelta())
	explain := subtleBoxStyle.Render(renderMeaningLegend(last))
	selector := subtleBoxStyle.Render(m.renderDashboardProviderSelector())
	return titleStyle.Render("Dashboard") + "\n\n" + selector + "\n\n" + top + "\n\n" + readiness + "\n\n" + deltas + "\n\n" + loadAnalytics + "\n\n" + weekly + "\n\n" + runPerf + "\n\n" + trends + "\n\n" + graph + "\n\n" + explain
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
		fmt.Sprintf("Profile ID (P to switch, s saves): %s", m.profileIDSetting),
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
	if m.createProfileOpen {
		edit = fmt.Sprintf(
			"\n\n%s\n%s\n\n%s",
			titleStyle.Render("New profile id"),
			m.inputBuffer,
			mutedStyle.Render("Enter create · Esc cancel (name is sanitized to letters, digits, -, _)"),
		)
	} else if m.editMode {
		edit = fmt.Sprintf("\n\nEditing: %s", m.inputBuffer)
	}
	return fmt.Sprintf(
		"%s\n\nProvider: %s\nConnected: %t%s\n\n%s\n\nDefaults if zones are empty:\n- Uses 220-age max HR (or override), with 60/70/80/90%% splits\n\nActions:\n- e edit selected field\n- n create new profile folder & switch\n- o toggle Run activities only\n- s save settings\n- a open Strava auth page\n- x exchange auth code\n- g imports Garmin FIT; dropping .fit files into Garmin FIT dir also auto-imports (debounced) with a desktop notify when OS supports it",
		titleStyle.Render("Settings"),
		m.dataProvider.Name(),
		m.settings.Connected,
		profileEnvOverrideNote(),
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
		if block := renderTerrainLoadBlock(s); block != "" {
			terrainCard := cardStyle.Width(max(44, (m.width - 38))).Render(block)
			detailGrid = lipgloss.JoinVertical(lipgloss.Left, detailGrid, "\n"+terrainCard)
		}
	}

	loadBits := fmt.Sprintf("NP %.0f | IF %.2f | TSS %.1f (%s)", s.NP, s.IF, s.TSS, s.TSSSource)
	if s.RSSPoints > 0 {
		loadBits += fmt.Sprintf(" | RSS %.0f (%s)", s.RSSPoints, s.RSSSource)
	}
	loadBits += fmt.Sprintf(" | TRIMP %.1f | EF(speed/HR) %.4f", s.TRIMP, s.EFSpeed)

	return fmt.Sprintf(
		"Details: %s\nDuration %s | Pace %s | AvgHR %.0f bpm\n%s\nDecoupling %.2f%%\nSession type: %s (%s)\nSession verdict: %s\n%s\n\n%s",
		s.Activity.Name, s.Duration, s.AvgPace, s.AvgHR, loadBits, s.Decoupling, s.SessionClass, renderSessionConfidence(s.SessionConf), verdict, s.RunExplanation, detailGrid,
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
	if err := paths.SetActiveProfile(m.profileIDSetting); err != nil {
		return err
	}
	if err := m.dataProvider.UpdateSettings(m.settings); err != nil {
		return err
	}
	m.profile = m.dataProvider.AthleteProfile()
	m.applyFilters()
	m.reloadFITWatcher()
	return nil
}

func (m *Model) reloadFITWatcher() {
	if m.fitWatcherCtl == nil || m.fitNotifyChan == nil {
		return
	}
	m.fitWatcherCtl.Restart(m.settings.GarminFITDir, m.fitNotifyChan)
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
	case 12:
		return m.profileIDSetting
	case 13:
		return m.currentAuthCode()
	default:
		return ""
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
		m.profileIDSetting = strings.TrimSpace(v)
	case 13:
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
		gapText := ""
		uphillPct := 0.0
		brakeLoad := 0.0
		rssPts := 0.0
		rssSrc := ""
		if isRunSport(a.Sport) {
			if g := physics.GradeAdjustedAvgPaceMinKm(a); !math.IsNaN(g) && g > 0 && g < 45 {
				gapText = formatPaceFromMinPerKm(g)
			}
			uphillPct = physics.UphillTimeFraction(a, 2.0) * 100
			brakeLoad = physics.DownhillBrakingLoad(a)
			if settings.FTP > 1 && hasUsablePower(a.Power) {
				rawRSS := physics.RunningSquaredPowerLoad(a.Power, a.TimeSec, settings.FTP)
				rssPts = sanitizeRSS(rawRSS / 36.0)
				rssSrc = "run-power/FTP-ref"
			}
		}
		out = append(out, activitySummary{
			Activity:          a,
			NP:                np,
			IF:                ifVal,
			TSS:               tss,
			TSSSource:         tssSource,
			TRIMP:             trimp,
			EFSpeed:           efSpeed,
			Decoupling:        decoupling,
			AvgHR:             avgHR,
			AvgCadence:        a.AvgCadence,
			VertOscCM:         a.AvgVerticalOscillationCM,
			VertRatio:         vertRatio,
			DecoupleAtMin:     durability.DecouplingStartMinutes,
			DurabilityScore:   durability.Score,
			HRStabilityPct:    durability.HRStabilityPct,
			CadenceStdDev:     cadenceStdDev,
			CadenceDropPct:    cadenceDropPct,
			FormBreakdown:     formBreakdown.Detected,
			FormBreakdownAt:   formBreakdown.StartMin,
			SessionClass:      sessionClass,
			SessionConf:       sessionConf,
			RunExplanation:    runExplanation,
			AvgPace:           formatPace(a.Sport, a.DistanceKM, a.Duration),
			Duration:          formatDurationCompact(a.Duration),
			Zones:             zones,
			ZoneBasis:         basis,
			HRZones:           hrZones,
			GapPaceText:       gapText,
			UphillTimePct:     uphillPct,
			DownhillBrakeLoad: brakeLoad,
			RSSPoints:         rssPts,
			RSSSource:         rssSrc,
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

func sanitizeRSS(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
		return 0
	}
	const maxReasonableRSS = 500.0
	if v > maxReasonableRSS {
		return maxReasonableRSS
	}
	return v
}

func formatPaceFromMinPerKm(minPerKm float64) string {
	if math.IsNaN(minPerKm) || minPerKm <= 0 || minPerKm > 45 {
		return ""
	}
	totalSec := int(minPerKm*60.0 + 0.5)
	m := totalSec / 60
	s := totalSec % 60
	return fmt.Sprintf("%d:%02d/km", m, s)
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
	txt := fmt.Sprintf(
		"Running Economy\nAverage Cadence: %s spm\nCadence consistency (sd): %s spm\nCadence drop late run: %s\nVertical Oscillation: %s cm\nVertical Ratio: %s",
		formatMetricValue(s.AvgCadence, "%.1f"),
		formatMetricValue(s.CadenceStdDev, "%.2f"),
		formatPercentValue(s.CadenceDropPct),
		formatMetricValue(s.VertOscCM, "%.1f"),
		formatVerticalRatioValue(s.VertRatio),
	)
	if s.Activity.AvgStanceTimeMs > 0 {
		txt += fmt.Sprintf("\nAvg stance (GCT proxy): %.0f ms", s.Activity.AvgStanceTimeMs)
	}
	if s.Activity.StrideAsymmetryPct > 0 {
		txt += fmt.Sprintf("\nStride asymmetry: %.1f%%", s.Activity.StrideAsymmetryPct)
	}
	return txt
}

func renderTerrainLoadBlock(s activitySummary) string {
	if s.GapPaceText == "" && s.UphillTimePct < 0.5 && s.DownhillBrakeLoad < 0.01 && s.RSSPoints <= 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Terrain & running stress\n")
	if s.GapPaceText != "" {
		fmt.Fprintf(&b, "Grade-adjusted pace (modeled): %s\n", s.GapPaceText)
	}
	if s.UphillTimePct >= 0.5 {
		fmt.Fprintf(&b, "Uphill moving time (grade ≥ 2%%): %.0f%%\n", s.UphillTimePct)
	}
	if s.DownhillBrakeLoad >= 0.01 {
		fmt.Fprintf(&b, "Descent braking proxy (trend units): %.1f\n", s.DownhillBrakeLoad)
	}
	if s.RSSPoints > 0 {
		fmt.Fprintf(&b, "RSS (∫ power² vs FTP-ref, TSS analog): %.0f (%s)", s.RSSPoints, s.RSSSource)
	}
	return strings.TrimSpace(b.String())
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
		"How to read this:\n- Fitness (CTL): your long-term training load (%s)\n- Fatigue (ATL): your short-term load (last ~7 days)\n- Form (TSB): freshness = CTL - ATL (higher = fresher)\n- ATL/CTL: acute-vs-chronic ratio from the PMC tail (watch for sudden spikes)\n- WSI: weekly stress index (weekly TSS variability composite)\n- IF: workout intensity vs FTP (1.00 = FTP effort)\n- TSS / RSS: classic load vs squared running-power impulse (FTP-ref scale; see runs with power)\n- TRIMP: HR-zone weighted impulse (internal load)\n- EF(speed/HR): aerobic efficiency trend; higher over time is usually better\n- Decoupling: aerobic durability drift; <5%% is usually strong steady-state",
		load,
	)
}

func sumRSSLast7Days(summaries []activitySummary) float64 {
	cutoff := time.Now().AddDate(0, 0, -7)
	var sum float64
	for _, s := range summaries {
		if s.Activity.StartTime.Before(cutoff) {
			continue
		}
		sum += s.RSSPoints
	}
	return sum
}

func renderLoadAnalytics(pmc []physics.PMCPoint, summaries []activitySummary) string {
	la := physics.LoadAnalyticsFromPMC(pmc)
	if !la.HasData {
		return "Load analytics (7d): insufficient history."
	}
	rss7 := sumRSSLast7Days(summaries)
	rssPart := ""
	if rss7 > 0 {
		rssPart = fmt.Sprintf(" | ΣRSS(7d) %.0f", rss7)
	}
	return fmt.Sprintf(
		"Load analytics (7d)\nMonotony %.2f (repetitive load) | Strain %.0f | CTL ramp %.2f / day | WSI %.0f | ATL/CTL %.2f%s",
		la.Monotony,
		la.Strain,
		la.RampPerDay,
		la.WeeklyStressIndex,
		la.ATLCTL,
		rssPart,
	)
}

func profileEnvOverrideNote() string {
	if v := strings.TrimSpace(os.Getenv("AEROBIX_PROFILE")); v != "" {
		return "\n" + mutedStyle.Render("Note: AEROBIX_PROFILE overrides profile in config.json on startup.")
	}
	return ""
}

func renderWeeklySummary(summaries []activitySummary) string {
	if len(summaries) == 0 {
		return "Last 7 days: no sessions"
	}
	cutoff := time.Now().AddDate(0, 0, -7)
	sessions := 0
	totalTSS := 0.0
	rss7 := 0.0
	longest := 0 * time.Minute
	longestName := "-"
	for _, s := range summaries {
		if s.Activity.StartTime.Before(cutoff) {
			continue
		}
		sessions++
		totalTSS += s.TSS
		rss7 += s.RSSPoints
		if s.Activity.Duration > longest {
			longest = s.Activity.Duration
			longestName = s.Activity.Name
		}
	}
	if sessions == 0 {
		return "Last 7 days: no sessions"
	}
	out := fmt.Sprintf(
		"Last 7 days: %d sessions | Total TSS %.1f | Avg TSS %.1f\nLongest session: %s (%s)",
		sessions, totalTSS, totalTSS/float64(sessions), longestName, formatDurationCompact(longest),
	)
	if rss7 > 0 {
		out += fmt.Sprintf("\nΣ RSS (runs with power, 7d): %.0f pts", rss7)
	}
	return out
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

func (m Model) openProfilePicker() (tea.Model, tea.Cmd) {
	ids, err := paths.ListProfiles()
	if err != nil {
		m.status = "Could not list profiles: " + err.Error()
		return m, nil
	}
	m.profilePickerChoices = ids
	m.profilePickerOpen = true
	m.profilePickerCursor = 0
	cur := paths.ActiveProfile()
	for i, id := range ids {
		if id == cur {
			m.profilePickerCursor = i
			break
		}
	}
	return m, nil
}

func (m Model) handleProfilePickerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc", "q":
		m.profilePickerOpen = false
		return m, nil
	case "up", "k":
		if m.profilePickerCursor > 0 {
			m.profilePickerCursor--
		}
		return m, nil
	case "down", "j":
		if m.profilePickerCursor < len(m.profilePickerChoices)-1 {
			m.profilePickerCursor++
		}
		return m, nil
	case "enter":
		if len(m.profilePickerChoices) == 0 {
			m.profilePickerOpen = false
			return m, nil
		}
		id := m.profilePickerChoices[m.profilePickerCursor]
		return m.switchToProfile(id)
	default:
		return m, nil
	}
}

func (m Model) switchToProfile(id string) (tea.Model, tea.Cmd) {
	m.profilePickerOpen = false

	if id == m.profileIDSetting {
		m.status = fmt.Sprintf("Already using profile %s.", id)
		return m, nil
	}

	if err := paths.SetActiveProfile(id); err != nil {
		m.status = "Profile: " + err.Error()
		return m, nil
	}
	m.profileIDSetting = id

	p, err := strava.NewProviderForProfile(id)
	if err != nil {
		m.dataProvider = mock.NewProvider()
		m.status = "Profile switched; Strava unavailable (" + err.Error() + ") — mock data."
	} else {
		m.dataProvider = &p
		m.status = fmt.Sprintf("Switched to profile %s. Loading…", id)
	}
	m.settings = m.dataProvider.Settings()
	m.profile = m.dataProvider.AthleteProfile()
	m.fetchInfo = provider.FetchInfo{}
	m.rawActivities = nil
	m.rawGarminActs = nil
	m.activityCursor = 0
	m.garminCursor = 0
	m.navCursor = 0
	m.dashProvider = "strava"
	m.preReload = map[string]physics.PMCPoint{}
	m.dashDelta = map[string]dashboardDelta{}
	m.asyncCh = nil
	m.loading = true
	m.applyFilters()
	(&m).reloadFITWatcher()
	return m, tea.Batch(m.loadActivitiesCmd(false), spinnerTickCmd())
}

func (m Model) renderProfilePickerScreen() string {
	root := "(unknown)"
	if d, err := paths.AerobixDir(); err == nil {
		root = d
	}
	title := titleStyle.Render("Switch profile")
	var lines []string
	effective := m.profileIDSetting
	for i, id := range m.profilePickerChoices {
		label := id
		if id == effective {
			label += " · active"
		}
		if i == m.profilePickerCursor {
			lines = append(lines, navItemActiveStyle.Render("  ▸ "+label))
		} else {
			lines = append(lines, navItemStyle.Render("    "+label))
		}
	}
	if len(m.profilePickerChoices) == 0 {
		lines = append(lines, mutedStyle.Render("  (no profile folders)"))
	}
	envNote := ""
	if v := strings.TrimSpace(os.Getenv("AEROBIX_PROFILE")); v != "" {
		envNote = "\n" + mutedStyle.Render("AEROBIX_PROFILE="+v+" overrides config on next launch when set.")
	}
	hint := mutedStyle.Render("↑/↓ select · Enter apply · Esc or q close") + "\n" + mutedStyle.Render("Data: "+root) + envNote
	boxW := max(40, min(m.width-4, 72))
	box := cardStyle.Width(boxW).Render(title + "\n\n" + strings.Join(lines, "\n") + "\n\n" + hint)
	if m.width <= 0 || m.height <= 0 {
		return appStyle.Render(box)
	}
	placed := lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box,
		lipgloss.WithWhitespaceChars(" "),
	)
	return appStyle.Render(placed)
}

func fmtSigned(v float64) string {
	return fmt.Sprintf("%+.1f", v)
}
