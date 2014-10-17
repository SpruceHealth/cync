package main

/*
TODO:
	- Configurable AWS region (currently us-east-1 only)
	- Configurable S3 permissions (currently private only)
	- Configurable optional S3 headers (currently always server-side encryption)
	- Configurable mimetypes
*/

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path"

	"github.com/sprucehealth/backend/libs/aws"
)

var (
	config = struct {
		AWSKeys aws.Keys
		DryRun  bool
		Verbose bool
	}{}
	commands []*command
)

type command struct {
	Name  string
	Flags *flag.FlagSet
	Func  func(*command)
}

func awsKeys() aws.Keys {
	if config.AWSKeys.AccessKey == "" {
		config.AWSKeys = aws.KeysFromEnvironment()
		if config.AWSKeys.AccessKey == "" {
			cred, err := aws.CredentialsForRole("")
			if err == nil {
				config.AWSKeys = cred.Keys()
			}
		}
	}
	return config.AWSKeys
}

func errorExit(s string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, s, a...)
	os.Exit(2)
}

func logError(s string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, s, a...)
}

func logVerbose(s string, a ...interface{}) {
	if config.Verbose {
		fmt.Printf(s, a...)
	}
}

func flagUsage() {
	fmt.Fprintf(os.Stderr, "Usage: %s [options] <command> [command options] ...\n", path.Base(os.Args[0]))
	flag.PrintDefaults()
	fmt.Fprintf(os.Stderr, "\nCommands:\n\n")
	for _, c := range commands {
		fmt.Fprintf(os.Stderr, "%s\n", c.Name)
		c.Flags.PrintDefaults()
	}
	os.Exit(1)
}

func parseFlags() {
	flag.Usage = flagUsage
	flag.StringVar(&config.AWSKeys.AccessKey, "aws-access-key", "", "AWS Access Key ID")
	flag.StringVar(&config.AWSKeys.SecretKey, "aws-secret-key", "", "AWS Secret Key")
	flag.StringVar(&config.AWSKeys.Token, "aws-token", "", "AWS Token")
	flag.BoolVar(&config.DryRun, "d", false, "Dry run. Output actions that would be taken but don't actually do anything")
	flag.BoolVar(&config.Verbose, "v", false, "Verbose output")
	flag.Parse()
	if len(flag.Args()) < 1 {
		flagUsage()
	}
	if config.DryRun {
		config.Verbose = true
	}
}

func main() {
	log.SetFlags(0)
	parseFlags()

	cmd := flag.Arg(0)
	for _, c := range commands {
		if c.Name == cmd {
			c.Flags.Parse(flag.Args()[1:])
			c.Func(c)
			return
		}
	}

	errorExit("Unknown command: %s\n", cmd)
}
