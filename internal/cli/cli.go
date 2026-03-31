package cli

import (
	"fmt"
	"os"
	"strings"
)

// AppVersion is set by main before calling Parse, used for --version output.
var AppVersion = "dev"

type Command struct {
	Name       string
	Formats    []string
	ConfigFile string
	InitConfig string // for `run -i <path>` / `run --init <path>`
	Verbose    bool
}

func Parse(args []string) Command {
	if len(args) == 0 {
		printHelp()
		os.Exit(0)
	}

	cmd := Command{
		Formats: []string{"json"},
	}

	i := 0
	for i < len(args) {
		arg := args[i]
		switch {
		case arg == "--version":
			fmt.Printf("restrail %s\n", AppVersion)
			os.Exit(0)
		case arg == "-h" || arg == "--help" || arg == "help":
			printHelp()
			os.Exit(0)
		case arg == "-f":
			i++
			if i >= len(args) {
				fmt.Fprintln(os.Stderr, "error: -f requires a file path")
				os.Exit(1)
			}
			cmd.ConfigFile = args[i]
		case strings.HasPrefix(arg, "-f="):
			cmd.ConfigFile = strings.TrimPrefix(arg, "-f=")
		case arg == "-v" || arg == "--verbose":
			cmd.Verbose = true
		case arg == "-i" || arg == "--init":
			i++
			if i >= len(args) {
				fmt.Fprintln(os.Stderr, "error: -i/--init requires a file path")
				os.Exit(1)
			}
			cmd.InitConfig = args[i]
		case strings.HasPrefix(arg, "-i="):
			cmd.InitConfig = strings.TrimPrefix(arg, "-i=")
		case strings.HasPrefix(arg, "--init="):
			cmd.InitConfig = strings.TrimPrefix(arg, "--init=")
		case arg == "--format":
			i++
			if i >= len(args) {
				fmt.Fprintln(os.Stderr, "error: --format requires a value (json,markdown,html)")
				os.Exit(1)
			}
			cmd.Formats = parseFormats(args[i])
		case strings.HasPrefix(arg, "--format="):
			cmd.Formats = parseFormats(strings.TrimPrefix(arg, "--format="))
		case arg == "run" || arg == "discover-profile" || arg == "init":
			cmd.Name = arg
		default:
			fmt.Fprintf(os.Stderr, "error: unknown argument %q\n", arg)
			printHelp()
			os.Exit(1)
		}
		i++
	}

	if cmd.Name == "" {
		fmt.Fprintln(os.Stderr, "error: no command specified")
		printHelp()
		os.Exit(1)
	}

	return cmd
}

func parseFormats(s string) []string {
	var formats []string
	for _, f := range strings.Split(s, ",") {
		f = strings.TrimSpace(f)
		switch f {
		case "json", "markdown", "html":
			formats = append(formats, f)
		default:
			fmt.Fprintf(os.Stderr, "error: unknown format %q (valid: json, markdown, html)\n", f)
			os.Exit(1)
		}
	}
	if len(formats) == 0 {
		return []string{"json"}
	}
	return formats
}

func printHelp() {
	fmt.Print(`restrail - Guardrailing your REST-Endpoints

Usage:
  restrail <command> [flags]

Commands:
  init                        Generate test artifacts (.request files, execution plan)
  run                         Run integration tests against the API
  discover-profile            Discover which profile matches the spec

Flags:
  -f <path>            Load configuration from a JSON file
  -i, --init <path>    (run only) Run init before run using init config at <path>
  --format <formats>   Output formats, comma-separated (json,markdown,html)
                        Default: json
  -v, --verbose        Enable verbose logging (detailed request/response info)
  --version            Show version
  -h, --help           Show this help message

Environment Variables:
  RESTRAIL_BASE_URL            Base URL where the API is served
  RESTRAIL_SPEC                Path to the OpenAPI spec file (JSON)
  RESTRAIL_CREDS_SUBJECT       Username or client ID (empty for unauthenticated)
  RESTRAIL_CREDS_SECRET        Password or secret (empty for unauthenticated)
  RESTRAIL_CREDS_CLIENT_ID     OAuth2 client ID (enables password grant)
  RESTRAIL_CREDS_CLIENT_SECRET OAuth2 client secret (optional, for confidential clients)
  RESTRAIL_OUTPUT              Path to store results (default: stdout)
  RESTRAIL_OUTPUT_DIR          Output directory for init (default: restrail-tests)
  RESTRAIL_PROFILE             Profile name (e.g., ddd)
  RESTRAIL_BLOCKED_DOMAINS     Comma-separated list of domains to skip during run

Init Config (JSON):
  {
    "spec": "api.json",
    "profile": "ddd",
    "output_dir": "restrail-tests",
    "optimistic_locking": false
  }

Run Config (JSON):
  {
    "base_url": "http://localhost:8080",
    "test_dir": "restrail-tests",
    "creds_subject": "",
    "creds_secret": "",
    "creds_client_id": "",
    "creds_client_secret": "",
    "output": "",
    "blocked_domains": []
  }

Examples:
  restrail -f init.json init
  restrail -f run_config.json run
  restrail -f run_config.json run --format markdown,html
  restrail -f run_config.json -i init.json run
  restrail -f config.json run                              (legacy)
  RESTRAIL_SPEC=api.json restrail discover-profile
`)
}
