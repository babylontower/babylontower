package ui

import (
	"context"
	"time"

	"gioui.org/app"
	"gioui.org/font/gofont"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	babylonapp "babylontower/pkg/app"
	"babylontower/pkg/storage"
)

// App represents the Gio UI application
type App struct {
	window *app.Window
	theme  *material.Theme
	ui     *UIState

	// Application services
	coreApp      babylonapp.Application
	storage      storage.Storage
	messenger    babylonapp.Messenger
	groupManager babylonapp.GroupManager

	// Context for cancellation
	ctx    context.Context
	cancel context.CancelFunc

	// Message handling
	messageEvents <-chan *babylonapp.MessageEvent
	contacts      []ContactInfo
	messages      map[int][]MessageInfo
}

// ContactInfo holds contact information for display
type ContactInfo struct {
	Name        string
	PublicKey   []byte
	IsOnline    bool
	LastSeen    time.Time
	DisplayName string
}

// MessageInfo holds message information for display
type MessageInfo struct {
	Text       string
	IsOutgoing bool
	Timestamp  time.Time
}

// UIState holds the UI state
type UIState struct {
	// Current theme (light/dark)
	isDarkMode bool
	theme      *Theme

	// Column selectors
	selectedContact int
	selectedChat    int

	// Widgets for lists
	contactList  widget.List
	chatList     widget.List
	settingsList widget.List

	// Input fields
	messageInput widget.Editor
	searchInput  widget.Editor

	// Buttons
	sendButton widget.Clickable

	// Contact buttons (dynamically created)
	contactButtons []widget.Clickable

	// Settings buttons (dynamically created)
	settingsButtons []widget.Clickable

	// Current view (for settings panel)
	currentSettingsTab int

	// Settings editor
	settings *SettingsState

	// Settings panel visibility
	showSettingsPanel bool
}

// AppConfig holds Gio UI application configuration
type AppConfig struct {
	// Initial theme (false = light, true = dark)
	DarkMode bool
	// Window title
	Title string
	// Window size
	Width, Height unit.Dp
}

// NewApp creates a new Gio UI application
func NewApp(coreApp babylonapp.Application, cfg *AppConfig) *App {
	ctx, cancel := context.WithCancel(context.Background())

	window := &app.Window{}

	theme := material.NewTheme()
	theme.Shaper = text.NewShaper(text.WithCollection(gofont.Collection()))

	uiTheme := NewTheme(LightTheme())
	if cfg.DarkMode {
		uiTheme = NewTheme(DarkTheme())
	}

	uiState := &UIState{
		isDarkMode:         cfg.DarkMode,
		theme:              uiTheme,
		selectedContact:    -1,
		selectedChat:       -1,
		contactButtons:     make([]widget.Clickable, 0),
		settingsButtons:    make([]widget.Clickable, 0),
		currentSettingsTab: 0,
		showSettingsPanel:  false,
		settings:           NewSettingsState(nil, "2.0", "unknown", "unknown"),
	}

	// Setup message input
	uiState.messageInput.SingleLine = true
	uiState.messageInput.Submit = true

	// Setup search input
	uiState.searchInput.SingleLine = true

	// Setup list configurations
	uiState.contactList.Axis = layout.Vertical
	uiState.chatList.Axis = layout.Vertical
	uiState.settingsList.Axis = layout.Vertical

	gioApp := &App{
		window:        window,
		theme:         theme,
		ui:            uiState,
		coreApp:       coreApp,
		storage:       coreApp.Storage(),
		messenger:     coreApp.Messenger(),
		groupManager:  coreApp.Groups(),
		ctx:           ctx,
		cancel:        cancel,
		messageEvents: coreApp.MessageEvents(),
		contacts:      make([]ContactInfo, 0),
		messages:      make(map[int][]MessageInfo),
	}

	// Load contacts
	gioApp.loadContacts()

	// Start message listener
	go gioApp.listenForMessages()

	return gioApp
}

// Run starts the UI event loop
func (a *App) Run() error {
	// Setup window
	a.window.Option(app.Size(unit.Dp(1200), unit.Dp(800)))
	a.window.Option(app.MinSize(unit.Dp(800), unit.Dp(600)))

	var ops op.Ops

	for {
		e := a.window.Event()
		if e == nil {
			continue
		}
		switch e := e.(type) {
		case app.DestroyEvent:
			return e.Err

		case app.FrameEvent:
			gtx := layout.Context{
				Constraints: layout.Constraints{
					Max: e.Size,
				},
				Metric: e.Metric,
				Now:    e.Now,
				Source: e.Source,
				Ops:    &ops,
			}

			// Layout the UI
			a.layout(gtx)

			// Render the frame
			e.Frame(&ops)
		}
	}
}

// Stop gracefully shuts down the UI
func (a *App) Stop() {
	a.cancel()
}

// ToggleTheme switches between light and dark mode
func (a *App) ToggleTheme() {
	a.ui.isDarkMode = !a.ui.isDarkMode
	if a.ui.isDarkMode {
		a.ui.theme = NewTheme(DarkTheme())
	} else {
		a.ui.theme = NewTheme(LightTheme())
	}
}

// loadContacts loads contacts from storage
func (a *App) loadContacts() {
	if a.messenger == nil {
		return
	}

	statuses, err := a.messenger.GetAllContactStatuses()
	if err != nil {
		logger.Debugw("failed to get contact statuses", "error", err)
		return
	}

	a.contacts = make([]ContactInfo, 0, len(statuses))
	for _, status := range statuses {
		contact := ContactInfo{
			Name:        string(status.PubKey),
			PublicKey:   status.PubKey,
			IsOnline:    status.IsOnline,
			DisplayName: status.DisplayName,
		}
		a.contacts = append(a.contacts, contact)
	}

	logger.Debugw("loaded contacts", "count", len(a.contacts))
}

// listenForMessages listens for incoming messages
func (a *App) listenForMessages() {
	if a.messageEvents == nil {
		return
	}

	for {
		select {
		case <-a.ctx.Done():
			return
		case event, ok := <-a.messageEvents:
			if !ok {
				return
			}
			a.handleMessageEvent(event)
		}
	}
}

// handleMessageEvent processes an incoming message event
func (a *App) handleMessageEvent(event *babylonapp.MessageEvent) {
	logger.Debugw("received message event", "from", string(event.ContactPubKey))

	// Find the contact index
	contactIdx := -1
	for i, contact := range a.contacts {
		if string(contact.PublicKey) == string(event.ContactPubKey) {
			contactIdx = i
			break
		}
	}

	// If contact not found, add them
	if contactIdx == -1 {
		contact := ContactInfo{
			Name:      string(event.ContactPubKey),
			PublicKey: event.ContactPubKey,
			IsOnline:  true,
		}
		a.contacts = append(a.contacts, contact)
		contactIdx = len(a.contacts) - 1
	}

	// Add message to the contact's message list
	msg := MessageInfo{
		Text:       event.Message.Text,
		IsOutgoing: false,
		Timestamp:  time.Unix(int64(event.Message.Timestamp), 0),
	}

	a.messages[contactIdx] = append(a.messages[contactIdx], msg)
	logger.Debugw("message added", "contact", contactIdx, "text", event.Message.Text)
}

// sendMessage sends a message to the selected contact
func (a *App) sendMessage() error {
	if a.ui.selectedContact < 0 || a.ui.selectedContact >= len(a.contacts) {
		return nil
	}

	text := a.ui.messageInput.Text()
	if text == "" {
		return nil
	}

	contact := a.contacts[a.ui.selectedContact]

	// Send message via messenger
	_, err := a.messenger.SendMessageToContact(
		text,
		contact.PublicKey,
		nil, // X25519 key would be needed in a full implementation
	)
	if err != nil {
		logger.Errorw("failed to send message", "error", err)
		return err
	}

	// Add outgoing message to local list
	msg := MessageInfo{
		Text:       text,
		IsOutgoing: true,
		Timestamp:  time.Now(),
	}
	a.messages[a.ui.selectedContact] = append(a.messages[a.ui.selectedContact], msg)

	// Clear input
	a.ui.messageInput.SetText("")

	logger.Debugw("message sent", "to", contact.Name, "text", text)
	return nil
}
