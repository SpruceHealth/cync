package main

import (
	"flag"
	"fmt"
	"io"
	"mime"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sync"

	"github.com/sprucehealth/backend/libs/aws"
	"github.com/sprucehealth/backend/libs/aws/s3"
)

var syncConfig = struct {
	DeleteSource bool
	Excludes     []*regexp.Regexp
}{}

func init() {
	cmd := &command{
		Name: "sync",
		Func: syncCmd,
	}
	cmd.Flags = flag.NewFlagSet("sync", flag.ExitOnError)
	cmd.Flags.Usage = flagUsage
	cmd.Flags.BoolVar(&syncConfig.DeleteSource, "delete-source", false, "Delete source file after successful copy (i.e. move the file)")
	cmd.Flags.Var(regexSliceVar{&syncConfig.Excludes}, "exclude", "A regex of files to ignore")
	commands = append(commands, cmd)
}

type file struct {
	Path   string
	Info   os.FileInfo
	Open   func() (io.ReadCloser, error)
	Delete func() error
}

func syncCmd(cmd *command) {
	args := cmd.Flags.Args()

	if len(args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s [options] sync [sync options] SourceURL DestinationURL\n", path.Base(os.Args[0]))
		cmd.Flags.PrintDefaults()
		os.Exit(1)
	}

	src, err := url.Parse(args[0])
	if err != nil {
		errorExit("Failed to parse source URL '%s': %s\n", args[0], err.Error())
	}
	dest, err := url.Parse(args[1])
	if err != nil {
		errorExit("Failed to parse destination URL '%s': %s\n", args[1], err.Error())
	}

	if src.Scheme != "" && src.Scheme != "file" {
		errorExit("Source may only be a local path\n")
	}
	if (src.Scheme == "file" && src.Host != "") || (dest.Scheme == "file" && dest.Host != "") {
		errorExit("Scheme 'file' requires a blank host\n")
	}

	if dest.Scheme != "s3" {
		errorExit("Currently destination may only be S3\n")
	}

	srcPath, err := filepath.Abs(src.Path)
	if err != nil {
		errorExit("Unable to get absoluate source path: %s\n", err.Error())
	}

	walkCh := make(chan *file, 10)
	go func() {
		filepath.Walk(srcPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				logError("Error walking path %s: %s\n", path, err.Error())
			} else if !info.IsDir() {
				relPath := path[len(srcPath)+1:]
				walkCh <- &file{
					Path: relPath,
					Info: info,
					Open: func() (io.ReadCloser, error) {
						return os.Open(path)
					},
					Delete: func() error {
						return os.Remove(path)
					},
				}
			}
			return nil
		})
		close(walkCh)
	}()

	var wg sync.WaitGroup

	wg.Add(1)
	transferCh := make(chan *file, 10)
	go func() {
		s3c := &s3.S3{
			Region: aws.USEast,
			Client: &aws.Client{
				Auth: awsKeys(),
			},
		}
		bucket := dest.Host
		prefix := dest.Path
		switch {
		case len(prefix) == 0 || (len(prefix) == 1 && prefix[0] == '/'):
		default:
			if prefix[0] == '/' {
				prefix = prefix[1:]
			}
			if prefix[len(prefix)-1] != '/' {
				prefix += "/"
			}
		}

		headers := map[string][]string{"x-amz-server-side-encryption": []string{"AES256"}}
		for f := range transferCh {
			dpath := prefix + f.Path

			if config.Verbose {
				s := *src
				s.Path = path.Join(s.Path, f.Path)
				d := *dest
				d.Path = dpath
				logVerbose("Transfering %s -> %s\n", s.String(), d.String())
			}

			if !config.DryRun {
				rc, err := f.Open()
				if err != nil {
					logVerbose("Failed to open %s: %s\n", f.Path, err.Error())
					continue
				}
				contentType := mime.TypeByExtension(path.Ext(f.Path))
				if contentType == "" {
					// TODO: contentType := http.DetectContentType(data)
					contentType = "application/binary"
				}
				err = s3c.PutFrom(bucket, dpath, rc, f.Info.Size(), contentType, s3.Private, headers)
				rc.Close()
				if err != nil {
					logError("Failed to transfer %s: %s\n", f.Path, err.Error())
				} else if syncConfig.DeleteSource {
					if err := f.Delete(); err != nil {
						logError("Failed to delete %s\n", f.Path)
					}
				}
			}
		}
		wg.Done()
	}()

	for f := range walkCh {
		skip := false
		for _, e := range syncConfig.Excludes {
			if e.MatchString(f.Path) {
				skip = true
				break
			}
		}
		if skip {
			logVerbose("Skipping %s\n", f.Path)
			continue
		}
		transferCh <- f
	}
	close(transferCh)

	wg.Wait()
}
