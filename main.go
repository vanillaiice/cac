package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"text/template"

	"github.com/mattn/go-shellwords"
)

// TODO: add support for handling files

func main() {
	// Check that ffmpeg is installed and available in PATH
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			log.Fatal("ffmpeg binary not found in PATH\n")
		} else {
			log.Fatal(err)
		}
	}

	// Define command line flags
	var src string
	flag.StringVar(&src, "src", "", "convert files from specified directory")

	var targetExtension string
	flag.StringVar(&targetExtension, "target-ext", ".mp3", "convert to specified extension")

	var exemptedExtensions string
	flag.StringVar(&exemptedExtensions, "except-exts", "", "do not convert files with specified extensions")

	var sourceExtensions string
	flag.StringVar(&sourceExtensions, "source-exts", "", "convert files with specified extensions")

	var outputDir string
	flag.StringVar(&outputDir, "output", "converted", "output directory of converted/moved files")

	var createOutputDir bool
	flag.BoolVar(&createOutputDir, "create", false, "create output directory if it does not exist")

	var deleteOriginalFile bool
	flag.BoolVar(&deleteOriginalFile, "delete", false, "delete original files after converting/moving")

	var quiet bool
	flag.BoolVar(&quiet, "quiet", false, "only show error logs")

	var debug bool
	flag.BoolVar(&debug, "debug", false, "show detailed debug information")

	var ffmpegConvertCommand string
	defaultConvertCommand := `ffmpeg -y -i "{{ .InputFile }}" "{{ .OutputFile }}"`
	flag.StringVar(&ffmpegConvertCommand, "command", defaultConvertCommand, "ffmpeg convert command with Go template placeholders")

	flag.Parse()

	// Validate required parameters
	if src == "" {
		log.Fatal("source directory (-src) is required")
	}

	// Verify source directory exists
	if _, err := os.Stat(src); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			log.Fatalf("directory %q does not exist", src)
		} else {
			log.Fatal(err)
		}
	}

	if !quiet {
		fmt.Printf("starting audio conversion...\n")
		fmt.Printf("source directory: %s\n", src)
		fmt.Printf("target extension: %s\n", targetExtension)
		fmt.Printf("output directory: %s\n", outputDir)
	}

	// Check if output directory exists, create if needed
	if _, err := os.Stat(outputDir); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if createOutputDir {
				if !quiet {
					fmt.Printf("creating output directory: %s\n", outputDir)
				}
				if err := os.MkdirAll(outputDir, os.ModePerm); err != nil {
					log.Fatalf("failed to create output directory: %v", err)
				}
			} else {
				log.Fatalf("directory %q does not exist", outputDir)
			}
		} else {
			log.Fatal(err)
		}
	}

	// Set up concurrency control - limit goroutines to number of CPU cores
	maxGoroutines := runtime.NumCPU()
	sem := make(chan struct{}, maxGoroutines)
	var wg sync.WaitGroup

	// Error collection - collect all errors instead of stopping on first one
	var collectedErrors []error
	var errorMutex sync.Mutex

	if !quiet {
		fmt.Printf("using %d worker threads for parallel processing\n", maxGoroutines)
	}

	// Parse and process extension lists
	var exemptedExtensionsSlice []string
	if exemptedExtensions != "" {
		exemptedExtensionsSlice = strings.Split(exemptedExtensions, ",")
		// Trim whitespace from each extension
		for i, ext := range exemptedExtensionsSlice {
			exemptedExtensionsSlice[i] = strings.TrimSpace(ext)
		}
		if !quiet {
			fmt.Printf("exempted extensions: %v\n", exemptedExtensionsSlice)
		}
	}

	var sourceExtensionsSlice []string
	if sourceExtensions != "" {
		sourceExtensionsSlice = strings.Split(sourceExtensions, ",")
		// Trim whitespace from each extension
		for i, ext := range sourceExtensionsSlice {
			sourceExtensionsSlice[i] = strings.TrimSpace(ext)
		}
		if !quiet {
			fmt.Printf("source extensions filter: %v\n", sourceExtensionsSlice)
		}
	}

	if !quiet {
		fmt.Printf("\nscanning directory for files...\n")
	}

	// Statistics tracking
	var processedFiles, skippedFiles, movedFiles, failedFiles int
	var statsMutex sync.Mutex

	// Function to safely add errors to the collection
	addError := func(err error) {
		errorMutex.Lock()
		collectedErrors = append(collectedErrors, err)
		errorMutex.Unlock()

		statsMutex.Lock()
		failedFiles++
		statsMutex.Unlock()
	}

	// Walk through source directory and process files
	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// Log the walking error but continue processing
			addError(fmt.Errorf("error accessing %s: %w", path, err))
			return nil // Continue walking despite this error
		}

		// Skip directories
		if d.IsDir() {
			if !quiet && path != src {
				fmt.Printf("entering directory: %s\n", path)
			}
			return nil
		}

		ext := filepath.Ext(d.Name())
		fileName := strings.TrimSuffix(d.Name(), ext)

		// Determine if this file should be processed
		shouldConvert := false
		shouldMoveOrCopy := false

		// Check if file matches our source extension criteria
		matchesSourceCriteria := false
		if len(sourceExtensionsSlice) > 0 {
			// If source extensions are specified, file must match one of them
			matchesSourceCriteria = slices.Contains(sourceExtensionsSlice, ext)
		} else {
			// If no source extensions specified, process all files except exempted ones
			matchesSourceCriteria = !slices.Contains(exemptedExtensionsSlice, ext)
		}

		if !quiet {
			fmt.Printf("found file: %s (extension: %s) - ", d.Name(), ext)
		}

		if matchesSourceCriteria {
			if ext == targetExtension {
				// File is already in target format, should be moved/copied
				shouldMoveOrCopy = true
				if !quiet {
					fmt.Printf("already target format - will move/copy\n")
				}
			} else {
				// File needs conversion
				shouldConvert = true
				if !quiet {
					fmt.Printf("will convert\n")
				}
			}
		} else {
			if !quiet {
				fmt.Printf("skipped (doesn't match source criteria)\n")
			}
			statsMutex.Lock()
			skippedFiles++
			statsMutex.Unlock()
		}

		if debug {
			fmt.Printf("DEBUG: File: %s, Ext: %s, Target: %s, MatchesCriteria: %t, ShouldConvert: %t, ShouldMove: %t\n",
				d.Name(), ext, targetExtension, matchesSourceCriteria, shouldConvert, shouldMoveOrCopy)
		}

		if shouldConvert {
			statsMutex.Lock()
			processedFiles++
			statsMutex.Unlock()

			wg.Add(1)
			go func(inputPath, fileName string) {
				defer wg.Done()
				// Acquire semaphore for concurrency control
				sem <- struct{}{}
				defer func() { <-sem }()

				outputPath := filepath.Join(outputDir, fileName+targetExtension)

				if !quiet {
					fmt.Printf("converting: %s -> %s\n", inputPath, outputPath)
				}

				// Generate the conversion command from template
				command, err := generateConvertCommand(ffmpegConvertCommand, inputPath, outputPath)
				if err != nil {
					addError(fmt.Errorf("failed to generate command for %s: %w", inputPath, err))
					return
				}

				// Execute the conversion command
				if err = runCommand(command, quiet); err != nil {
					addError(fmt.Errorf("failed to convert %s: %w", inputPath, err))
					return
				}

				// Delete original file if requested
				if deleteOriginalFile {
					if !quiet {
						fmt.Printf("deleting original file: %s\n", inputPath)
					}
					if err := os.Remove(inputPath); err != nil {
						addError(fmt.Errorf("failed to delete original file %s: %w", inputPath, err))
						return
					}
				}

				if !quiet {
					fmt.Printf("✓ converted: %s -> %s\n", inputPath, outputPath)
				}
			}(path, fileName)
		} else if shouldMoveOrCopy {
			// Handle files that are already in target format
			outputPath := filepath.Join(outputDir, d.Name())

			// Check if source and destination are the same file
			if path == outputPath {
				if !quiet {
					fmt.Printf("already in output directory - skipping\n")
				}
				statsMutex.Lock()
				skippedFiles++
				statsMutex.Unlock()
			} else {
				statsMutex.Lock()
				movedFiles++
				statsMutex.Unlock()

				if deleteOriginalFile {
					if !quiet {
						fmt.Printf("moving: %s -> %s\n", path, outputPath)
					}
					if err := os.Rename(path, outputPath); err != nil {
						addError(fmt.Errorf("failed to move file %s: %w", path, err))
						return nil // Continue processing other files
					}
				} else {
					if !quiet {
						fmt.Printf("copying: %s -> %s\n", path, outputPath)
					}
					if err := copyFile(path, outputPath); err != nil {
						addError(fmt.Errorf("failed to copy file %s: %w", path, err))
						return nil // Continue processing other files
					}
				}
				if !quiet {
					fmt.Printf("✓ moved/copied: %s -> %s\n", path, outputPath)
				}
			}
		}

		return nil
	})

	if err != nil {
		addError(fmt.Errorf("error walking directory: %w", err))
	}

	if !quiet {
		fmt.Printf("\nwaiting for all conversions to complete...\n")
	}

	wg.Wait()

	// Report final results
	if !quiet {
		fmt.Printf("\n=== conversion summary ===\n")
		fmt.Printf("files converted: %d\n", processedFiles-failedFiles)
		fmt.Printf("files moved/copied: %d\n", movedFiles)
		fmt.Printf("files skipped: %d\n", skippedFiles)
		fmt.Printf("files failed: %d\n", failedFiles)
		fmt.Printf("total files processed: %d\n", processedFiles+movedFiles+skippedFiles)
	}

	// Report all collected errors
	if len(collectedErrors) > 0 {
		fmt.Printf("\n=== errors encountered ===\n")
		for i, err := range collectedErrors {
			fmt.Printf("Error %d: %v\n", i+1, err)
		}
		fmt.Printf("\n❌ completed with %d errors\n", len(collectedErrors))
		os.Exit(1)
	} else {
		if !quiet {
			fmt.Println("✅ all conversions completed successfully!")
		}
	}
}

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

	// Show command output unless in quiet mode
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

	// Ensure we're copying a regular file, not a directory or special file
	if !sourceFileStat.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", src)
	}

	// Open source file for reading
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	// Create destination file
	destination, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destination.Close()

	_, err = io.Copy(destination, source)
	return err
}
