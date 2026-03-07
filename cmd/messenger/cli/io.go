package cli

import (
	pb "babylontower/pkg/proto"
)

// UserInterface defines the interface for CLI user interaction.
// This abstraction allows for testing and alternative UI implementations.
type UserInterface interface {
	// Output methods
	Output(message string)
	ShowBanner(version, publicKey string)
	ShowHelp()
	ShowMessage(msg *pb.Message, sender string, isOutgoing bool)
	ShowContactList(contacts []*pb.Contact)
	ShowError(err error)
	ShowSuccess(message string)
	ShowInfo(message string)

	// Input methods (for future interactive features)
	// ReadLine(prompt string) (string, error)
	// ReadPassword(prompt string) (string, error)
}

// ConsoleUI implements UserInterface for console-based interaction.
type ConsoleUI struct {
	output func(string)
}

// NewConsoleUI creates a new ConsoleUI instance.
func NewConsoleUI(output func(string)) *ConsoleUI {
	return &ConsoleUI{
		output: output,
	}
}

// Output writes a message to the output.
func (ui *ConsoleUI) Output(message string) {
	if ui.output != nil {
		ui.output(message)
	}
}

// ShowBanner displays the application banner.
func (ui *ConsoleUI) ShowBanner(version, publicKey string) {
	ui.Output(FormatBanner(version, publicKey))
}

// ShowHelp displays help information.
func (ui *ConsoleUI) ShowHelp() {
	ui.Output(FormatHelp())
}

// ShowMessage formats and displays a message.
func (ui *ConsoleUI) ShowMessage(msg *pb.Message, sender string, isOutgoing bool) {
	ui.Output(FormatMessage(msg, sender, isOutgoing))
}

// ShowContactList displays a list of contacts.
func (ui *ConsoleUI) ShowContactList(contacts []*pb.Contact) {
	ui.Output(FormatContactList(contacts))
}

// ShowError displays an error message.
func (ui *ConsoleUI) ShowError(err error) {
	if err != nil {
		ui.Output(FormatError(err))
	}
}

// ShowSuccess displays a success message.
func (ui *ConsoleUI) ShowSuccess(message string) {
	ui.Output(FormatSuccess(message))
}

// ShowInfo displays an informational message.
func (ui *ConsoleUI) ShowInfo(message string) {
	ui.Output(FormatInfo(message))
}

// Ensure ConsoleUI implements UserInterface
var _ UserInterface = (*ConsoleUI)(nil)
