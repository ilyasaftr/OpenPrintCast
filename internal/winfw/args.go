package winfw

// Command line of the elevated copy: SetupFlag runs the firewall setup and
// exits; AllowPublicFlag adds the Public profile and is only ever passed
// after the user confirmed it.
const (
	SetupFlag       = "--setup-firewall"
	AllowPublicFlag = "--allow-public"
)

// Command is what a command line asks for.
type Command struct {
	// Setup is set when the line asks for the firewall setup.
	Setup       bool
	AllowPublic bool
	// Invalid is set when the line asks for the setup in a way that is not
	// understood (an unknown flag), which must not start the app.
	Invalid bool
}

// ParseCommand reads the program arguments (without the program name).
// Lines that do not mention SetupFlag are not a command.
func ParseCommand(args []string) Command {
	var cmd Command
	for _, arg := range args {
		switch arg {
		case SetupFlag:
			cmd.Setup = true
		case AllowPublicFlag:
			cmd.AllowPublic = true
		default:
			cmd.Invalid = true
		}
	}
	if !cmd.Setup {
		return Command{}
	}
	return cmd
}
