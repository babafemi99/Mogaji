package main

import (
	"fmt"
	"os"

	"github.com/babafemi99/Mogaji/internal/domain"
	"github.com/babafemi99/Mogaji/internal/engine"
	"github.com/babafemi99/Mogaji/internal/report"
	"github.com/logrusorgru/aurora/v4"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// version is set via ldflags at build time:
//
//	go build -ldflags "-X main.version=1.0.0" ./cmd/mogaji
var version = "dev"

func rootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "mogaji",
		Short:         "Mogaji — open-source financial reconciliation engine",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			// Print banner before every command except version.
			if cmd.Name() != "version" {
				fmt.Println(aurora.Cyan(banner))
				fmt.Println(aurora.Faint("  Named after the Yoruba arbiter who restores order when records conflict."))
				fmt.Println()
			}
		},
	}

	root.AddCommand(reconcileCmd())
	root.AddCommand(versionCmd())

	return root
}

// reconcileCmd builds the `mogaji reconcile` subcommand.
func reconcileCmd() *cobra.Command {
	var (
		mappingPath string
		outputPath  string
		verbose     bool
	)

	cmd := &cobra.Command{
		Use:   "reconcile",
		Short: "Run a reconciliation against the provided mapping file",
		Example: `  mogaji reconcile --mapping mapping.yml --out report.json
  mogaji reconcile --mapping mapping.yml --out report.json --verbose`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runReconcile(mappingPath, outputPath, verbose)
		},
	}

	cmd.Flags().StringVarP(&mappingPath, "mapping", "m", "", "path to the YAML mapping file (required)")
	cmd.Flags().StringVarP(&outputPath, "out", "o", "report.json", "path to write the JSON report")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "print summary to stdout after reconciliation")

	_ = cmd.MarkFlagRequired("mapping")

	return cmd
}

// versionCmd builds the `mogaji version` subcommand.
func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the Mogaji version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("mogaji %s\n", aurora.Cyan(version))
		},
	}
}

// runReconcile is the core reconcile logic, separated from the Cobra handler
// so it can be tested independently.
func runReconcile(mappingPath, outputPath string, verbose bool) error {
	// --- Load and validate config ---
	cfg, err := loadConfig(mappingPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %s\n", aurora.Red("✗"), err)
		return err
	}

	if err := validateConfig(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "%s invalid mapping file: %s\n", aurora.Red("✗"), err)
		return err
	}

	fmt.Fprintf(os.Stderr, "%s run %s starting — %s sources\n",
		aurora.Cyan("→"),
		aurora.Bold(cfg.Run.ID),
		aurora.Yellow(fmt.Sprintf("%d", len(cfg.Sources))),
	)

	// --- Run the engine ---
	eng := engine.New(cfg)
	run := eng.Run()

	// --- Write report ---
	if err := report.Write(run, nil, outputPath); err != nil {
		fmt.Fprintf(os.Stderr, "%s failed to write report: %s\n", aurora.Red("✗"), err)
		return err
	}

	fmt.Fprintf(os.Stderr, "%s report written → %s\n",
		aurora.Green("✓"),
		aurora.Bold(outputPath),
	)

	// --- Handle failed run ---
	if run.Status == domain.RunStatusFailed {
		fmt.Fprintf(os.Stderr, "%s reconciliation failed: %s\n",
			aurora.Red("✗"),
			run.Error,
		)
		return fmt.Errorf(run.Error)
	}

	// --- Print result ---
	if verbose {
		fmt.Print(report.Summary(run))
	} else {
		printCompactSummary(run)
	}

	return nil
}

// printCompactSummary prints a coloured one-line summary to stderr.
func printCompactSummary(run domain.Run) {
	s := run.Summary

	matchColor := aurora.Green
	if s.MatchRatePercent < 95 {
		matchColor = aurora.Yellow
	}
	if s.MatchRatePercent < 80 {
		matchColor = aurora.Red
	}

	fmt.Fprintf(os.Stderr, "\n%s  %s\n",
		aurora.Bold("Status"),
		colorStatus(run.Status),
	)
	fmt.Fprintf(os.Stderr, "%s  %s%%\n",
		aurora.Bold("Match rate"),
		matchColor(fmt.Sprintf("%.2f", s.MatchRatePercent)),
	)
	fmt.Fprintf(os.Stderr, "%s  %d exact  %s missing internal  %s missing external\n\n",
		aurora.Bold("Results"),
		s.ExactMatches,
		aurora.Yellow(fmt.Sprintf("%d", s.MissingInternal)),
		aurora.Yellow(fmt.Sprintf("%d", s.MissingExternal)),
	)
}

// colorStatus returns a coloured RunStatus string.
func colorStatus(status domain.RunStatus) aurora.Value {
	switch status {
	case domain.RunStatusComplete:
		return aurora.Green(string(status))
	case domain.RunStatusFailed:
		return aurora.Red(string(status))
	default:
		return aurora.Yellow(string(status))
	}
}

// loadConfig reads and parses a YAML mapping file into a domain.Config.
func loadConfig(path string) (domain.Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return domain.Config{}, fmt.Errorf("cannot read mapping file %q: %w", path, err)
	}

	var cfg domain.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return domain.Config{}, fmt.Errorf("cannot parse YAML: %w", err)
	}

	return cfg, nil
}

// validateConfig checks that a parsed Config is usable before running.
func validateConfig(cfg domain.Config) error {
	if err := cfg.Run.Validate(); err != nil {
		return err
	}
	if len(cfg.Sources) == 0 {
		return fmt.Errorf("at least one source is required")
	}

	for i, src := range cfg.Sources {
		if err := src.Validate(); err != nil {
			return fmt.Errorf("failed to validate source[%d]: %w", i, err)
		}
	}

	return nil
}
