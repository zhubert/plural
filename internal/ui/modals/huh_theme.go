package modals

import (
	"charm.land/bubbles/v2/help"
	tea "charm.land/bubbletea/v2"
	huh "charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	"github.com/zhubert/plural/internal/keys"
)

// initHuhForm initializes a huh form eagerly so it renders correctly
// immediately. Call this in every modal constructor after creating the form.
func initHuhForm(form *huh.Form) {
	form.Init()
}

// huhFormUpdate is the common Update logic for huh-based modals.
// It intercepts Enter and Escape (handled by the app-layer modal handlers)
// and delegates everything else to the huh form.
func huhFormUpdate(form *huh.Form, initialized *bool, msg tea.Msg) (*huh.Form, tea.Cmd) {
	// Legacy lazy init path for safety — forms should be eagerly initialized
	// via initHuhForm() in constructors, but handle the case where they aren't.
	if !*initialized {
		*initialized = true
		initCmd := form.Init()
		m, updateCmd := form.Update(msg)
		form = m.(*huh.Form)
		return form, tea.Batch(initCmd, updateCmd)
	}

	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		switch keyMsg.String() {
		case keys.Enter, keys.Escape:
			// Don't let huh handle these — the app-layer modal handlers do
			return form, nil
		}
	}

	m, cmd := form.Update(msg)
	form = m.(*huh.Form)
	return form, cmd
}

// ModalTheme returns a huh theme that matches the current modal color palette.
// This is called each time a huh form is created to pick up the current theme colors.
func ModalTheme() huh.Theme {
	return huh.ThemeFunc(func(isDark bool) *huh.Styles {
		t := huh.ThemeBase(isDark)

		// Focused field styles — active field with left border indicator
		t.Focused.Base = lipgloss.NewStyle().
			PaddingLeft(1).
			BorderStyle(lipgloss.NormalBorder()).
			BorderLeft(true).
			BorderForeground(ColorPrimary)
		t.Focused.Card = t.Focused.Base
		t.Focused.Title = lipgloss.NewStyle().Foreground(ColorText).Bold(true)
		t.Focused.Description = lipgloss.NewStyle().Foreground(ColorTextMuted).Italic(true)
		t.Focused.ErrorIndicator = lipgloss.NewStyle().Foreground(ColorWarning).SetString(" *")
		t.Focused.ErrorMessage = lipgloss.NewStyle().Foreground(ColorWarning)

		// Select styles
		t.Focused.SelectSelector = lipgloss.NewStyle().Foreground(ColorPrimary).SetString("> ")
		t.Focused.NextIndicator = lipgloss.NewStyle().Foreground(ColorPrimary).MarginLeft(1).SetString("→")
		t.Focused.PrevIndicator = lipgloss.NewStyle().Foreground(ColorPrimary).MarginRight(1).SetString("←")
		t.Focused.Option = lipgloss.NewStyle().Foreground(ColorText)

		// MultiSelect styles
		t.Focused.MultiSelectSelector = lipgloss.NewStyle().Foreground(ColorPrimary).SetString("> ")
		t.Focused.SelectedOption = lipgloss.NewStyle().Foreground(ColorSecondary)
		t.Focused.SelectedPrefix = lipgloss.NewStyle().Foreground(ColorSecondary).SetString("[x] ")
		t.Focused.UnselectedOption = lipgloss.NewStyle().Foreground(ColorText)
		t.Focused.UnselectedPrefix = lipgloss.NewStyle().Foreground(ColorTextMuted).SetString("[ ] ")

		// Confirm button styles
		t.Focused.FocusedButton = lipgloss.NewStyle().
			Padding(0, 2).
			MarginRight(1).
			Foreground(ColorTextInverse).
			Background(ColorPrimary)
		t.Focused.BlurredButton = lipgloss.NewStyle().
			Padding(0, 2).
			MarginRight(1).
			Foreground(ColorTextMuted)

		// Text input styles
		t.Focused.TextInput.Cursor = lipgloss.NewStyle().Foreground(ColorPrimary)
		t.Focused.TextInput.Placeholder = lipgloss.NewStyle().Foreground(ColorTextMuted)
		t.Focused.TextInput.Prompt = lipgloss.NewStyle().Foreground(ColorPrimary)
		t.Focused.TextInput.Text = lipgloss.NewStyle().Foreground(ColorText)

		// Blurred field styles — inactive field with hidden border
		t.Blurred = t.Focused
		t.Blurred.Base = lipgloss.NewStyle().
			PaddingLeft(2)
		t.Blurred.Card = t.Blurred.Base
		t.Blurred.NextIndicator = lipgloss.NewStyle()
		t.Blurred.PrevIndicator = lipgloss.NewStyle()

		// Group styles
		t.Group.Title = lipgloss.NewStyle().Foreground(ColorSecondary).Bold(true)
		t.Group.Description = lipgloss.NewStyle().Foreground(ColorTextMuted)

		// Minimal field separator
		t.FieldSeparator = lipgloss.NewStyle().SetString("\n")

		// Help styles
		t.Help = help.New().Styles

		return t
	})
}
