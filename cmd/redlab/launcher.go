package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	redlab "github.com/redlab/redlab"
)

type launcherScenario struct {
	ID    string
	Title string
}

func runLauncher(in io.Reader, out io.Writer) error {
	reader := bufio.NewReader(in)
	for {
		fmt.Fprintln(out, "")
		fmt.Fprintln(out, "============================================")
		fmt.Fprintln(out, " RedLab - RHEL Troubleshooting Hackathon Lab")
		fmt.Fprintln(out, "============================================")
		fmt.Fprintln(out, "1. Start a participant scenario")
		fmt.Fprintln(out, "2. Open the organizer dashboard")
		fmt.Fprintln(out, "3. Create/manage an event")
		fmt.Fprintln(out, "4. Show command help")
		fmt.Fprintln(out, "5. Exit")
		fmt.Fprint(out, "Select an option [1-5]: ")

		choice, err := launcherLine(reader)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		switch strings.ToLower(choice) {
		case "1":
			played, err := launcherPlayScenario(reader, out)
			if err != nil {
				return err
			}
			if played {
				return nil
			}
		case "2":
			return launcherOpenDashboard(out)
		case "3":
			eventFile, created, err := ensureLauncherEvent(out)
			if err != nil {
				return err
			}
			if !created {
				fmt.Fprintf(out, "Organizer event is ready at %s\n", eventFile)
			}
			fmt.Fprintln(out, "Choose option 2 to manage it in the dashboard.")
		case "4", "help", "h", "?":
			writeLauncherHelp(out)
		case "5", "exit", "quit", "q":
			fmt.Fprintln(out, "Goodbye.")
			return nil
		default:
			fmt.Fprintln(out, "Please enter a number from 1 through 5.")
		}
	}
}

func launcherPlayScenario(reader *bufio.Reader, out io.Writer) (bool, error) {
	scenarios, err := launcherScenarios()
	if err != nil {
		return false, err
	}
	fmt.Fprintln(out, "\nAvailable scenarios:")
	for index, item := range scenarios {
		fmt.Fprintf(out, "%2d. %s (%s)\n", index+1, item.Title, item.ID)
	}
	fmt.Fprintln(out, " 0. Back")
	for {
		fmt.Fprintf(out, "Select a scenario [0-%d]: ", len(scenarios))
		selection, err := launcherLine(reader)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return false, nil
			}
			return false, err
		}
		index, err := strconv.Atoi(selection)
		if err != nil || index < 0 || index > len(scenarios) {
			fmt.Fprintln(out, "Enter one of the scenario numbers shown above.")
			continue
		}
		if index == 0 {
			return false, nil
		}
		fmt.Fprintf(out, "\nStarting %s. Type `lab briefing` for instructions or `exit` to quit.\n", scenarios[index-1].Title)
		return true, playScenario("builtin:"+scenarios[index-1].ID, "PRACTICE", "", reader, out)
	}
}

func launcherScenarios() ([]launcherScenario, error) {
	ids, err := redlab.BuiltinScenarioIDs()
	if err != nil {
		return nil, err
	}
	items := make([]launcherScenario, 0, len(ids))
	for _, id := range ids {
		pkg, diagnostics := loadScenarioArgument("builtin:" + id)
		if len(diagnostics) > 0 {
			return nil, fmt.Errorf("built-in scenario %s is invalid: %s", id, diagnostics[0].Error())
		}
		items = append(items, launcherScenario{ID: id, Title: pkg.Scenario.Metadata.Title})
	}
	return items, nil
}

func launcherOpenDashboard(out io.Writer) error {
	eventFile, created, err := ensureLauncherEvent(out)
	if err != nil {
		return err
	}
	if created {
		fmt.Fprintln(out, "IMPORTANT: Save the organizer recovery secret printed above. It is the dashboard password and is shown only once.")
	}
	fmt.Fprintln(out, "Opening the organizer dashboard. Return to this window and press Ctrl+C to stop RedLab.")
	serve := newServeCommand()
	serve.SetArgs([]string{"--event", eventFile, "--open"})
	serve.SilenceUsage = true
	serve.SilenceErrors = true
	return serve.Execute()
}

func ensureLauncherEvent(out io.Writer) (eventFile string, created bool, err error) {
	root, err := defaultEventDirectory()
	if err != nil {
		return "", false, err
	}
	eventFile = filepath.Join(root, "event.yaml")
	if info, statErr := os.Stat(eventFile); statErr == nil {
		if !info.Mode().IsRegular() {
			return "", false, fmt.Errorf("event path is not a regular file: %s", eventFile)
		}
		return eventFile, false, nil
	} else if !os.IsNotExist(statErr) {
		return "", false, statErr
	}
	if err := initializeEvent(root, out); err != nil {
		return "", false, err
	}
	return eventFile, true, nil
}

func defaultEventDirectory() (string, error) {
	config, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(config, "RedLab", "event"), nil
}

func launcherLine(reader *bufio.Reader) (string, error) {
	line, err := reader.ReadString('\n')
	if err != nil && !(errors.Is(err, io.EOF) && len(line) > 0) {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func writeLauncherHelp(out io.Writer) {
	fmt.Fprintln(out, "\nThe launcher is the easiest way to use RedLab after double-clicking the binary.")
	fmt.Fprintln(out, "Advanced command-line examples:")
	fmt.Fprintln(out, "  redlab-windows-amd64.exe play broken-httpd")
	fmt.Fprintln(out, "  redlab-windows-amd64.exe event init .\\my-event")
	fmt.Fprintln(out, "  redlab-windows-amd64.exe serve --event .\\my-event\\event.yaml --open")
	fmt.Fprintln(out, "  redlab-windows-amd64.exe --help")
}
