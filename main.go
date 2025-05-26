package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"sync"
	"text/template"
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
	defaultConvertCommand := "ffmpeg -y -i \"{{ .InputFile }}\" \"{{ .OutputFile }}\""
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
	var firstErr error
	var errMutex sync.Mutex

	// Set up context for cancellation on first error
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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
	var processedFiles, skippedFiles, movedFiles int

	// Walk through source directory and process files
	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
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
			skippedFiles++
		}

		if debug {
			fmt.Printf("DEBUG: File: %s, Ext: %s, Target: %s, MatchesCriteria: %t, ShouldConvert: %t, ShouldMove: %t\n",
				d.Name(), ext, targetExtension, matchesSourceCriteria, shouldConvert, shouldMoveOrCopy)
		}

		if shouldConvert {
			// Check for context cancellation before starting goroutine
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			processedFiles++
			wg.Add(1)
			go func(inputPath, fileName string) {
				defer wg.Done()
				// Acquire semaphore for concurrency control
				select {
				case sem <- struct{}{}:
					defer func() { <-sem }()
				case <-ctx.Done():
					return
				}

				outputPath := filepath.Join(outputDir, fileName+targetExtension)

				if !quiet {
					fmt.Printf("converting: %s -> %s\n", inputPath, outputPath)
				}

				// Generate the conversion command from template
				command, err := generateConvertCommand(ffmpegConvertCommand, inputPath, outputPath)
				if err != nil {
					errMutex.Lock()
					if firstErr == nil {
						firstErr = err
						cancel()
					}
					errMutex.Unlock()
					return
				}

				// Execute the conversion command
				if err = runCommand(command, quiet); err != nil {
					errMutex.Lock()
					if firstErr == nil {
						firstErr = err
						cancel()
					}
					errMutex.Unlock()
					return
				}

				// Delete original file if requested
				if deleteOriginalFile {
					if !quiet {
						fmt.Printf("deleting original file: %s\n", inputPath)
					}
					if err := os.Remove(inputPath); err != nil {
						errMutex.Lock()
						if firstErr == nil {
							firstErr = fmt.Errorf("failed to delete original file %s: %w", inputPath, err)
							cancel()
						}
						errMutex.Unlock()
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
				skippedFiles++
			} else {
				movedFiles++

				if deleteOriginalFile {
					if !quiet {
						fmt.Printf("moving: %s -> %s\n", path, outputPath)
					}
					if err := os.Rename(path, outputPath); err != nil {
						return fmt.Errorf("failed to move file %s: %w", path, err)
					}
				} else {
					if !quiet {
						fmt.Printf("copying: %s -> %s\n", path, outputPath)
					}
					if err := copyFile(path, outputPath); err != nil {
						return fmt.Errorf("failed to copy file %s: %w", path, err)
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
		log.Fatalf("error walking directory: %v", err)
	}

	if !quiet {
		fmt.Printf("\nwaiting for all conversions to complete...\n")
	}

	wg.Wait()

	// Check for errors and report final results
	if firstErr != nil {
		log.Fatalf("error converting files: %v", firstErr)
	} else {
		if !quiet {
			fmt.Printf("\n=== conversion summary ===\n")
			fmt.Printf("files converted: %d\n", processedFiles)
			fmt.Printf("files moved/copied: %d\n", movedFiles)
			fmt.Printf("files skipped: %d\n", skippedFiles)
			fmt.Printf("total files processed: %d\n", processedFiles+movedFiles+skippedFiles)
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
func splitCommand(command string) []string {
	re := regexp.MustCompile(`(?:[^\s'"]+|['"][^'"]*['"])`)
	matches := re.FindAllString(command, -1)
	// Remove quotes from quoted arguments
	for i, match := range matches {
		if len(match) > 1 && (match[0] == '"' || match[0] == '\'') && match[len(match)-1] == match[0] {
			matches[i] = match[1 : len(match)-1]
		}
	}
	return matches
}

// runCommand executes a shell command and handles output based on quiet flag.
func runCommand(command string, quiet bool) error {
	var cmd *exec.Cmd
	commandParts := splitCommand(command)
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
