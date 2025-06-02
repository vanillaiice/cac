package convert

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"text/template"

	"github.com/mattn/go-shellwords"
)

// convertCmdTemplateData holds the template data for the conversion command.
type convertCmdTemplateData struct {
	InputFile  string
	OutputFile string
}

// generateConvertCommand generates the conversion command from the template.
// It replaces placeholders like {{ .InputFile }} and {{ .OutputFile }} with actual values.
func generateConvertCommand(tmpl, inputFile, outputFile string) (string, error) {
	template, err := template.New("tmpl").Parse(tmpl)
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	if err = template.Execute(&sb, convertCmdTemplateData{
		InputFile:  inputFile,
		OutputFile: outputFile,
	}); err != nil {
		return "", err
	}

	return sb.String(), nil
}

// splitCommand splits a command string into arguments, handling quoted strings properly.
// This allows commands with spaces in file paths to work correctly.
func splitCommand(command string) ([]string, error) {
	return shellwords.Parse(command)
}

// runCommand executes a shell command and handles output based on quiet flag.
func runCommand(command string, quiet bool) error {
	var cmd *exec.Cmd
	commandParts, err := splitCommand(command)
	if err != nil {
		return err
	}
	lenCmdStringParts := len(commandParts)

	if lenCmdStringParts == 0 {
		return fmt.Errorf("invalid command: %s", command)
	} else if lenCmdStringParts == 1 {
		cmd = exec.Command(commandParts[0])
	} else {
		cmd = exec.Command(commandParts[0], commandParts[1:]...)
	}

	if !quiet {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}

	return cmd.Run()
}

// copyFile copies a file from src to dst, preserving file permissions.
// Used when moving files that are already in the correct format.
func copyFile(src, dst string) error {
	sourceFileStat, err := os.Stat(src)
	if err != nil {
		return err
	}

	if !sourceFileStat.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", src)
	}

	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	destination, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destination.Close()

	_, err = io.Copy(destination, source)
	return err
}
