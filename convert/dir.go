package convert

import (
	"fmt"
	"io/fs"
	"log"
	"path/filepath"
	"runtime"
	"slices"
	"sync"
)

// ConvertDirOpts hold options used when converting files in a directory.
type ConvertDirOpts struct {
	Command         string
	Dir             string
	Sources         []string
	Except          []string
	TargetExtension string
	OutDir          string
	DeleteOriginal  bool
	Quiet           bool
}

// ConvertDir converts files in a directory.
func ConvertDir(convertDirOpts *ConvertDirOpts) error {
	maxGoroutines := runtime.NumCPU()
	sem := make(chan struct{}, maxGoroutines)
	var wg sync.WaitGroup

	if !convertDirOpts.Quiet {
		log.Printf("using %d worker threads for parallel processing\n", maxGoroutines)
	}

	var collectedErrors []error
	var errorMutex, statsMutex sync.Mutex
	var processedFiles, skippedFiles, movedFiles, failedFiles int

	addError := func(err error) {
		errorMutex.Lock()
		collectedErrors = append(collectedErrors, err)
		errorMutex.Unlock()
	}

	err := filepath.WalkDir(convertDirOpts.Dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			addError(fmt.Errorf("error accessing %s: %w", path, err))
			return nil
		}

		if d.IsDir() {
			return nil
		}

		wg.Add(1)
		go func(inputPath string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			var shouldProcess bool
			if slices.Contains(convertDirOpts.Sources, convertDirOpts.TargetExtension) {
				shouldProcess = true
			} else if !slices.Contains(convertDirOpts.Except, convertDirOpts.TargetExtension) {
				shouldProcess = true
			}

			if shouldProcess {
				err, fileAction := ConvertFile(&ConvertFileOpts{
					Command:         convertDirOpts.Command,
					Path:            inputPath,
					TargetExtension: convertDirOpts.TargetExtension,
					OutDir:          convertDirOpts.OutDir,
					DeleteOriginal:  convertDirOpts.DeleteOriginal,
					Quiet:           convertDirOpts.Quiet,
				})

				if err != nil {
					addError(err)
				}

				statsMutex.Lock()
				switch fileAction {
				case fileActionFail:
					failedFiles++
				case fileActionConvert:
					processedFiles++
				case fileActionMove:
					movedFiles++
				case fileActionSkip:
					skippedFiles++
				}
				statsMutex.Unlock()

			}
		}(path)

		return nil
	})

	if err != nil {
		addError(fmt.Errorf("error walking directory: %w", err))
	}

	if !convertDirOpts.Quiet {
		log.Printf("\nwaiting for all conversions to complete...\n")
	}

	wg.Wait()

	if len(collectedErrors) > 0 {
		log.Printf("\n=== errors encountered ===\n")
		for i, err := range collectedErrors {
			log.Printf("Error %d: %v\n", i+1, err)
		}
		return fmt.Errorf("\ncompleted with %d errors\n", len(collectedErrors))
	}

	if !convertDirOpts.Quiet {
		log.Printf("\n=== conversion summary ===\n")
		log.Printf("files converted: %d\n", processedFiles-failedFiles)
		log.Printf("files moved/copied: %d\n", movedFiles)
		log.Printf("files skipped: %d\n", skippedFiles)
		log.Printf("files failed: %d\n", failedFiles)
		log.Printf("total files processed: %d\n", processedFiles+movedFiles+skippedFiles)
	}

	return nil
}
