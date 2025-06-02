package convert

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// ConvertFileOpts hold options used when converting files.
type ConvertFileOpts struct {
	Command         string
	Path            string
	TargetExtension string
	OutDir          string
	DeleteOriginal  bool
	Quiet           bool
}

// fileActionType is the type of action resulting from a ConvertFile call.
type fileActionType int

const (
	fileActionFail    fileActionType = iota // converting the file failed.
	fileActionConvert                       // converting the file succeeded.
	fileActionMove                          // the file was moved/copied.
	fileActionSkip                          // the file was skipped
)

func ConvertFile(convertFileOpts *ConvertFileOpts) (error, fileActionType) {
	ext := filepath.Ext(convertFileOpts.Path)
	fileName := strings.TrimSuffix(filepath.Base(convertFileOpts.Path), ext) + convertFileOpts.TargetExtension
	outputPath := filepath.Join(convertFileOpts.OutDir, fileName)

	if ext == convertFileOpts.TargetExtension {
		if convertFileOpts.Path == outputPath {
			if !convertFileOpts.Quiet {
				log.Printf("already in output directory - skipping: %s\n", convertFileOpts.Path)
			}
			return nil, fileActionSkip
		}

		if convertFileOpts.DeleteOriginal {
			if !convertFileOpts.Quiet {
				log.Printf("moving: %s -> %s\n", convertFileOpts.Path, outputPath)
			}
			if err := os.Rename(convertFileOpts.Path, outputPath); err != nil {
				return fmt.Errorf("failed to move file %s: %w", convertFileOpts.Path, err), fileActionFail
			}
		} else {
			if !convertFileOpts.Quiet {
				log.Printf("copying: %s -> %s\n", convertFileOpts.Path, outputPath)
			}
			if err := copyFile(convertFileOpts.Path, outputPath); err != nil {
				return fmt.Errorf("failed to copy file %s: %w", convertFileOpts.Path, err), fileActionFail
			}
		}

		if !convertFileOpts.Quiet {
			log.Printf("moved/copied: %s -> %s\n", convertFileOpts.Path, outputPath)
		}

		return nil, fileActionMove
	}

	if !convertFileOpts.Quiet {
		log.Printf("converting: %s -> %s\n", convertFileOpts.Path, outputPath)
	}

	command, err := generateConvertCommand(convertFileOpts.Command, convertFileOpts.Path, outputPath)
	if err != nil {
		return fmt.Errorf("failed to generate command for %s: %w", convertFileOpts.Path, err), fileActionFail
	}

	if err = runCommand(command, convertFileOpts.Quiet); err != nil {
		return fmt.Errorf("failed to convert %s: %w", convertFileOpts.Path, err), fileActionFail
	}

	if convertFileOpts.DeleteOriginal {
		if !convertFileOpts.Quiet {
			log.Printf("deleting original file: %s\n", convertFileOpts.Path)
		}
		if err := os.Remove(convertFileOpts.Path); err != nil {
			return fmt.Errorf("failed to delete original file %s: %w", convertFileOpts.Path, err), fileActionFail
		}
	}

	if !convertFileOpts.Quiet {
		log.Printf("converted: %s -> %s\n", convertFileOpts.Path, outputPath)
	}

	return nil, fileActionConvert
}
