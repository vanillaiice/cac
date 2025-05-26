# **c**onvenient **a**udio **c**onverter

cac is a command line tool to convert audio files using ffmpeg.

# Usage

```sh
Usage of ./cac:
  -command string
    	ffmpeg convert command with Go template placeholders (default "ffmpeg -y -i \"{{ .InputFile }}\" \"{{ .OutputFile }}\"")
  -create
    	create output directory if it does not exist
  -debug
    	show detailed debug information
  -delete
    	delete original files after converting/moving
  -except-exts string
    	do not convert files with specified extensions
  -output string
    	output directory of converted/moved files (default "converted")
  -quiet
    	only show error logs
  -source-exts string
    	convert files with specified extensions
  -src string
    	convert files from specified directory
  -target-ext string
    	convert to specified extension (default ".mp3")
```

# Author

vanillaiice

# License

GPLv3
