# **c**onvenient **a**udio **c**onverter [![Go Reference](https://pkg.go.dev/badge/golang.org/x/example.svg)](https://pkg.go.dev/github.com/vanillaiice/cac) [![Go Report Card](https://goreportcard.com/badge/github.com/vanillaiice/cac)](https://goreportcard.com/report/github.com/vanillaiice/cac)

cac is a command line tool to convert audio files using ffmpeg.

# Install

```sh
$ go install github.com/vanillaiice/cac@latest
```

# Usage

```sh
# convert all opus files in a directory to mp3 to the 'converted' folder
$ cac --dir ~/Music --sources ".opus" --target ".mp3" -out-dir converted --create-out-dir
```

# Help

```sh
NAME:
   cac - conveniently convert audio files using ffmpeg

USAGE:
   cac [global options]

VERSION:
   v0.2.2

AUTHOR:
   vanillaiice

GLOBAL OPTIONS:
   --command string                                             ffmpeg convert command with Go template placeholders (default: "ffmpeg -y -i \"{{ .InputFile }}\" \"{{ .OutputFile }}\"")
   --dir DIRECTORY, -d DIRECTORY                                convert files in DIRECTORY
   --files FILES, -f FILES [ --files FILES, -f FILES ]          convert FILES
   --target string, -t string                                   convert files to target extension (default: ".mp3")
   --except string, -e string [ --except string, -e string ]    do not convert files with specified extensions (only applicable with --dir flag)
   --sources string, -s string [ --sources string, -s string ]  convert files with specified extensions (only applicable with --dir flag, takes precedence over --except flag)
   --out-dir string, -o string                                  output directory of processed files (default: ".")
   --create-out-dir, -c                                         create output directory if it does not exist (default: false)
   --delete, -D                                                 delete original files after processing (default: false)
   --quiet, -q                                                  only show error logs (default: false)
   --help, -h                                                   show help
   --version, -v                                                print the version
```

# Author

vanillaiice

# License

GPLv3
