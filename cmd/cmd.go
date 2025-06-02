package cmd

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"

	"github.com/urfave/cli/v3"
	"github.com/vanillaiice/cac/convert"
	"github.com/vanillaiice/cac/version"
)

// Run runs the cli command.
func Run(ctx context.Context) {
	cmd := cli.Command{
		Name:                  "cac",
		Usage:                 "conveniently convert audio files using ffmpeg",
		Authors:               []any{"vanillaiice"},
		Suggest:               true,
		EnableShellCompletion: true,
		Version:               version.Version,
		Before: func(ctx context.Context, c *cli.Command) (context.Context, error) {
			if _, err := exec.LookPath("ffmpeg"); err != nil {
				if errors.Is(err, exec.ErrNotFound) {
					return ctx, errors.New("ffmpeg binary not found in PATH")
				} else {
					return ctx, err
				}
			}
			return ctx, nil
		},
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "command",
				Usage: "ffmpeg convert command with Go template placeholders",
				Value: `ffmpeg -y -i "{{ .InputFile }}" "{{ .OutputFile }}"`,
			},
			&cli.StringFlag{
				Name:    "dir",
				Usage:   "convert files in `DIRECTORY`",
				Aliases: []string{"d"},
			},
			&cli.StringSliceFlag{
				Name:    "files",
				Usage:   "convert `FILES`",
				Aliases: []string{"f"},
			},
			&cli.StringFlag{
				Name:    "target",
				Usage:   "convert files to target extension",
				Aliases: []string{"t"},
				Value:   ".mp3",
				Validator: func(s string) error {
					if s == "" {
						return fmt.Errorf("target extension cannot be empty  string")
					}
					return nil
				},
			},
			&cli.StringSliceFlag{
				Name:    "except",
				Usage:   "do not convert files with specified extensions",
				Aliases: []string{"e"},
			},
			&cli.StringSliceFlag{
				Name:    "sources",
				Usage:   "convert files with specified extensions",
				Aliases: []string{"s"},
			},
			&cli.StringFlag{
				Name:    "out-dir",
				Usage:   "output directory of processed files",
				Value:   ".",
				Aliases: []string{"o"},
			},
			&cli.BoolFlag{
				Name:    "create-out-dir",
				Usage:   "create output directory if it does not exist",
				Value:   false,
				Aliases: []string{"c"},
			},
			&cli.BoolFlag{
				Name:    "delete",
				Usage:   "delete original files after converting/moving",
				Value:   false,
				Aliases: []string{"D"},
			},
			&cli.BoolFlag{
				Name:    "quiet",
				Usage:   "only show error logs",
				Value:   false,
				Aliases: []string{"q"},
			},
		},
		Action: func(ctx context.Context, c *cli.Command) error {
			if c.String("dir") == "" && len(c.StringSlice("files")) < 0 {
				return fmt.Errorf("input directory (--dir) or file(s) (--files) are required")
			}

			if c.String("dir") != "" {
				if _, err := os.Stat(c.String("dir")); err != nil {
					if errors.Is(err, os.ErrNotExist) {
						return fmt.Errorf("directory %q does not exist", c.String("dir"))
					} else {
						return err
					}
				}

				if _, err := os.Stat(c.String("out-dir")); err != nil {
					if errors.Is(err, os.ErrNotExist) {
						if c.Bool("create-out-dir") {
							if !c.Bool("quiet") {
								log.Printf("creating output directory: %s\n", c.String("out-dir"))
							}
							if err := os.MkdirAll(c.String("out-dir"), os.ModePerm); err != nil {
								return fmt.Errorf("failed to create output directory: %v", err)
							}
						} else {
							return fmt.Errorf("directory %q does not exist", c.String("out-dir"))
						}
					} else {
						return err
					}
				}

				if !c.Bool("quiet") {
					log.Printf("starting audio conversion...\n")
					log.Printf("source directory: %s\n", c.String("dir"))
					log.Printf("target extension: %s\n", c.String("target"))
					log.Printf("output directory: %s\n", c.String("out-dir"))
				}

				return convert.ConvertDir(&convert.ConvertDirOpts{
					Command:         c.String("command"),
					Dir:             c.String("dir"),
					Sources:         c.StringSlice("sources"),
					Except:          c.StringSlice("except"),
					TargetExtension: c.String("target"),
					OutDir:          c.String("out-dir"),
					DeleteOriginal:  c.Bool("delete"),
					Quiet:           c.Bool("quiet"),
				})
			}

			// TODO: consider using goroutines for parallel processing
			if len(c.StringSlice("files")) > 0 {
				errors := []error{}

				for _, f := range c.StringSlice("files") {
					err, _ := convert.ConvertFile(&convert.ConvertFileOpts{
						Command:         c.String("command"),
						Path:            f,
						TargetExtension: c.String("target"),
						OutDir:          c.String("out-dir"),
						DeleteOriginal:  c.Bool("delete"),
						Quiet:           c.Bool("quiet"),
					})
					if err != nil {
						errors = append(errors, err)
					}
				}

				for _, err := range errors {
					fmt.Errorf("%s", err)
				}
			}

			return nil
		},
	}

	if err := cmd.Run(ctx, os.Args); err != nil {
		log.Fatal(err)
	}
}
