package ai

import (
	"fmt"
	"strings"
)

type CoachMessage struct {
	Role    string
	Content string
}

func PrepareContext(last7TSS float64, ctl, atl, tsb float64, recentDecoupling float64) string {
	return fmt.Sprintf(
		"Training context:\n- Last 7d TSS: %.1f\n- CTL: %.1f\n- ATL: %.1f\n- TSB: %.1f\n- Recent decoupling: %.2f%%",
		last7TSS, ctl, atl, tsb, recentDecoupling,
	)
}

func GetAthleteBio(ctl, atl, tsb, recentDecoupling float64) string {
	return fmt.Sprintf(
		"Athlete bio:\nCTL %.1f, ATL %.1f, TSB %.1f, recent decoupling %.2f%%.",
		ctl, atl, tsb, recentDecoupling,
	)
}

func BuildCoachPrompt(systemContext string, history []CoachMessage, userMessage string) string {
	var b strings.Builder
	b.WriteString("You are Aerobix Coach, concise and practical.\n")
	b.WriteString(systemContext)
	b.WriteString("\n\nConversation:\n")
	for _, m := range history {
		role := strings.ToLower(strings.TrimSpace(m.Role))
		if role == "" {
			role = "user"
		}
		b.WriteString("- " + role + ": " + strings.TrimSpace(m.Content) + "\n")
	}
	b.WriteString("\nLatest user question:\n")
	b.WriteString(strings.TrimSpace(userMessage))
	return b.String()
}
