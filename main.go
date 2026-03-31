package main

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"restrail/internal/auth"
	"restrail/internal/cli"
	"restrail/internal/config"
	"restrail/internal/generator"
	"restrail/internal/openapi"
	"restrail/internal/plan"
	"restrail/internal/profile"
	"restrail/internal/report"
	"restrail/internal/runner"
)

// Version is set at build time via -ldflags "-X main.Version=..."
var Version = "dev"

//go:embed profiles/*.profile
var profileFS embed.FS

func main() {
	cli.AppVersion = Version
	cmd := cli.Parse(os.Args[1:])

	switch cmd.Name {
	case "init":
		runInit(cmd)
	case "run":
		if cmd.InitConfig != "" {
			runWithInit(cmd)
		} else if cmd.ConfigFile != "" {
			runFromConfig(cmd)
		} else {
			fatal("use -f <run_config.json> to specify a run config")
		}
	case "discover-profile":
		discoverProfile(cmd)
	}
}

// runInit generates test artifacts from an init config.
func runInit(cmd cli.Command) {
	var initCfg config.InitConfig
	if cmd.ConfigFile != "" {
		var err error
		initCfg, err = config.LoadInitConfig(cmd.ConfigFile)
		if err != nil {
			fatal("%v", err)
		}
	} else {
		initCfg = config.LoadInitConfigFromEnv()
	}
	if err := initCfg.Validate(); err != nil {
		fatal("init configuration error: %v", err)
	}

	result := doInit(initCfg)

	fmt.Printf("Generated %d request files in %s\n", len(result.RequestFiles), result.OutputDir)
	for _, f := range result.RequestFiles {
		fmt.Printf("  %s\n", f)
	}
	fmt.Printf("Execution plan: %s\n", result.PlanFile)
	fmt.Printf("Run config template: %s\n", result.RunConfigFile)
}

// doInit runs the init phase and returns the result.
func doInit(initCfg config.InitConfig) *generator.GenerateResult {
	spec, err := openapi.ParseFile(initCfg.SpecPath)
	if err != nil {
		fatal("parsing spec: %v", err)
	}

	prof, err := loadProfile(initCfg.Profile)
	if err != nil {
		fatal("loading profile: %v", err)
	}

	result, err := generator.Generate(spec, prof, initCfg)
	if err != nil {
		fatal("generating test artifacts: %v", err)
	}
	return result
}

// runWithInit runs init first, then executes using the generated artifacts.
func runWithInit(cmd cli.Command) {
	// Load init config
	initCfg, err := config.LoadInitConfig(cmd.InitConfig)
	if err != nil {
		fatal("loading init config: %v", err)
	}
	if err := initCfg.Validate(); err != nil {
		fatal("init configuration error: %v", err)
	}

	// Run init
	genResult := doInit(initCfg)

	// Load run config from the generated run config, or from -f
	var runCfg config.RunConfig
	if cmd.ConfigFile != "" {
		runCfg, err = config.LoadRunConfig(cmd.ConfigFile)
		if err != nil {
			fatal("loading run config: %v", err)
		}
	} else {
		runCfg, err = config.LoadRunConfig(genResult.RunConfigFile)
		if err != nil {
			fatal("loading generated run config: %v", err)
		}
	}

	// Ensure test_dir points to the generated output
	runCfg.TestDir = genResult.OutputDir
	if err := runCfg.Validate(); err != nil {
		fatal("run configuration error: %v", err)
	}

	executeRun(cmd, runCfg, cmd.Verbose)
}

// runFromConfig loads a RunConfig and executes the plan flow.
func runFromConfig(cmd cli.Command) {
	runCfg, err := config.LoadRunConfig(cmd.ConfigFile)
	if err != nil {
		fatal("%v", err)
	}
	if err := runCfg.Validate(); err != nil {
		fatal("run configuration error: %v", err)
	}
	executeRun(cmd, runCfg, cmd.Verbose)
}

// executeRun runs tests from generated .request files and execution.plan.
func executeRun(cmd cli.Command, runCfg config.RunConfig, verbose bool) {
	// Parse execution plan
	planPath := filepath.Join(runCfg.TestDir, "execution.plan")
	execPlan, err := plan.Parse(planPath)
	if err != nil {
		fatal("parsing execution plan: %v", err)
	}

	// Create authenticator from config
	authenticator := auth.NewAuthenticatorFromConfig(
		runCfg.AuthType, runCfg.TokenURL,
		runCfg.CredsSubject, runCfg.CredsSecret,
		runCfg.CredsClientID, runCfg.CredsClientSecret,
	)

	// Execute using the plan runner
	r := runner.NewPlanRunner(
		strings.TrimRight(runCfg.BaseURL, "/"),
		authenticator,
		runCfg.TestDir,
		runCfg.OptimisticLocking,
		verbose,
		runCfg.BlockedDomains,
	)
	result := r.RunPlan(execPlan)
	result.SpecFile = runCfg.SpecPath
	result.Profile = execPlan.Profile

	printSummary(result)
	generateReports(cmd, runCfg.OutputPath, result)

	if result.Summary.FailedOps > 0 {
		os.Exit(1)
	}
}

func generateReports(cmd cli.Command, outputPath string, result *runner.RunResult) {
	for _, format := range cmd.Formats {
		var reporter interface {
			Generate(*runner.RunResult) ([]byte, error)
			Extension() string
		}
		switch format {
		case "json":
			reporter = &report.JSONReporter{}
		case "markdown":
			reporter = &report.MarkdownReporter{}
		case "html":
			reporter = &report.HTMLReporter{}
		}

		data, err := reporter.Generate(result)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error generating %s report: %v\n", format, err)
			continue
		}

		if outputPath == "" {
			os.Stdout.Write(data)
			os.Stdout.Write([]byte("\n"))
		} else {
			outPath := makeOutputPath(outputPath, reporter.Extension())
			if err := os.WriteFile(outPath, data, 0644); err != nil {
				fmt.Fprintf(os.Stderr, "error writing %s: %v\n", outPath, err)
			} else {
				fmt.Printf("Report written: %s\n", outPath)
			}
		}
	}
}

func discoverProfile(cmd cli.Command) {
	var initCfg config.InitConfig
	if cmd.ConfigFile != "" {
		var err error
		initCfg, err = config.LoadInitConfig(cmd.ConfigFile)
		if err != nil {
			fatal("%v", err)
		}
	} else {
		initCfg = config.LoadInitConfigFromEnv()
	}
	if initCfg.SpecPath == "" {
		fatal("spec path required (set RESTRAIL_SPEC or use -f <config>)")
	}

	spec, err := openapi.ParseFile(initCfg.SpecPath)
	if err != nil {
		fatal("parsing spec: %v", err)
	}

	profiles, err := loadAllProfiles()
	if err != nil {
		fatal("loading profiles: %v", err)
	}

	result, found := profile.DiscoverBest(spec, profiles)
	if !found {
		fmt.Println("No matching profile found.")
		os.Exit(1)
	}

	fmt.Printf("Discovered profile: %s\n", result.ProfileName)
	fmt.Printf("Matched domains: %d\n", result.MatchedDomains)
	for _, d := range result.Domains {
		fmt.Printf("  - %s (%d operations matched)\n", d.Domain, len(d.Mappings))
	}
}

func loadProfile(name string) (*profile.Profile, error) {
	// If name looks like a file path, load from disk
	if strings.Contains(name, "/") || strings.HasSuffix(name, ".profile") {
		data, err := os.ReadFile(name)
		if err != nil {
			return nil, fmt.Errorf("reading profile file %q: %w", name, err)
		}
		profName := strings.TrimSuffix(filepath.Base(name), ".profile")
		return profile.Parse(profName, string(data))
	}

	// Otherwise load from embedded FS
	filename := fmt.Sprintf("profiles/%s.profile", name)
	data, err := profileFS.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("profile %q not found: %w", name, err)
	}
	return profile.Parse(name, string(data))
}

func loadAllProfiles() ([]*profile.Profile, error) {
	entries, err := profileFS.ReadDir("profiles")
	if err != nil {
		return nil, err
	}

	var result []*profile.Profile
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".profile") {
			continue
		}
		profName := strings.TrimSuffix(name, ".profile")
		if profName == "template" {
			continue
		}
		data, err := profileFS.ReadFile("profiles/" + name)
		if err != nil {
			return nil, err
		}
		prof, err := profile.Parse(profName, string(data))
		if err != nil {
			return nil, err
		}
		result = append(result, prof)
	}
	return result, nil
}

func makeOutputPath(base, ext string) string {
	baseExt := filepath.Ext(base)
	if baseExt != "" {
		return strings.TrimSuffix(base, baseExt) + ext
	}
	return base + ext
}

func printSummary(result *runner.RunResult) {
	fmt.Println("=== Restrail Test Results ===")
	fmt.Printf("Profile: %s | Auth: %s\n", result.Profile, result.AuthType)
	if len(result.Dependencies) > 0 {
		fmt.Printf("Dependencies: %d detected\n", len(result.Dependencies))
		for _, dep := range result.Dependencies {
			status := "injected"
			if !dep.Resolved {
				status = "unresolved"
			}
			req := ""
			if dep.Required {
				req = " (required)"
			}
			fmt.Printf("  %s.%s -> %s [%s]%s\n", dep.FromDomain, dep.FieldName, dep.ToDomain, status, req)
		}
	}
	if len(result.CreatedResources) > 0 {
		fmt.Printf("Created resources: %d\n", len(result.CreatedResources))
		for i, cr := range result.CreatedResources {
			fmt.Printf("  %d. %s.%s = %s\n", i+1, cr.Domain, cr.IDField, cr.ID)
		}
	}
	fmt.Printf("Domains: %d passed, %d failed (of %d)\n",
		result.Summary.PassedDomains, result.Summary.FailedDomains, result.Summary.TotalDomains)
	fmt.Printf("Operations: %d passed, %d failed, %d skipped (of %d)\n",
		result.Summary.PassedOps, result.Summary.FailedOps,
		result.Summary.SkippedOps, result.Summary.TotalOps)

	for _, dr := range result.Domains {
		status := "PASS"
		if !dr.Success {
			status = "FAIL"
		}
		fmt.Printf("\n  [%s] %s\n", status, dr.Domain)
		if len(dr.Setup) > 0 {
			fmt.Printf("    Setup (%d operations):\n", len(dr.Setup))
			for _, s := range dr.Setup {
				sStatus := "PASS"
				if s.Operation.Skipped {
					sStatus = "SKIP"
				} else if !s.Operation.Success {
					sStatus = "FAIL"
				}
				fmt.Printf("      [%s] %s/%s %s %s\n", sStatus, s.Domain, s.Operation.OperationName, s.Operation.Method, s.Operation.Path)
			}
		}
		for _, op := range dr.Operations {
			opStatus := "PASS"
			if op.Skipped {
				opStatus = "SKIP"
			} else if !op.Success {
				opStatus = "FAIL"
			}
			fmt.Printf("    [%s] %s %s %s (auth: %s", opStatus, op.OperationName, op.Method, op.Path, op.AuthApplied)
			if len(op.SecurityRequired) > 0 {
				var schemes []string
				for _, s := range op.SecurityRequired {
					schemes = append(schemes, s.SchemeName)
				}
				fmt.Printf(", required: %s", strings.Join(schemes, ","))
			}
			fmt.Print(")")
			if op.Error != "" {
				fmt.Printf(" - %s", op.Error)
			}
			if op.SkipReason != "" {
				fmt.Printf(" - %s", op.SkipReason)
			}
			fmt.Println()
		}
	}
	fmt.Println()
}

func fatal(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(1)
}
