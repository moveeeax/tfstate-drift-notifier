// Command tfstate-drift-notifier runs `terraform plan -json` (or reads a
// pre-captured plan stream), classifies the resource changes into a compact
// drift report, optionally posts it to Slack, and exits 2 when drift is found.
package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/moveeeax/tfstate-drift-notifier/internal/drift"
	"github.com/moveeeax/tfstate-drift-notifier/internal/slack"
	"github.com/spf13/cobra"
)

// Exit codes follow `terraform plan -detailed-exitcode`: 0 = no drift,
// 2 = drift present, 1 = error.
const (
	exitNoDrift = 0
	exitError   = 1
	exitDrift   = 2
)

var (
	chdir     string
	planFile  string
	slackHook string
	jsonOut   bool
	tfBin     string
)

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tfstate-drift-notifier",
		Short: "Detect Terraform drift on a schedule and alert only when it appears",
		Long: "Runs `terraform plan -json`, classifies resource changes " +
			"(create/update/delete/replace), prints a compact report, and exits 2 " +
			"only when real drift is found. Optionally posts to a Slack webhook.",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          run,
	}
	f := cmd.Flags()
	f.StringVar(&chdir, "chdir", ".", "directory to run terraform in")
	f.StringVar(&planFile, "plan-file", "", "read a pre-captured `terraform plan -json` stream from this file instead of running terraform")
	f.StringVar(&slackHook, "slack-webhook", "", "Slack incoming webhook URL to post the report to")
	f.BoolVar(&jsonOut, "json", false, "emit the report as JSON")
	f.StringVar(&tfBin, "terraform-bin", "terraform", "terraform binary to invoke")
	return cmd
}

// planStream returns the plan JSON stream, either from a file or by running
// terraform. The returned closer must be called by the caller.
func planStream() (io.ReadCloser, func() error, error) {
	if planFile != "" {
		f, err := os.Open(planFile)
		if err != nil {
			return nil, nil, err
		}
		return f, f.Close, nil
	}
	cmd := exec.Command(tfBin, "plan", "-json", "-detailed-exitcode", "-input=false")
	cmd.Dir = chdir
	cmd.Stderr = os.Stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, fmt.Errorf("start terraform: %w", err)
	}
	// terraform exits 2 on drift; that is expected and not a failure here.
	closer := func() error {
		if err := cmd.Wait(); err != nil {
			if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == exitDrift {
				return nil
			}
			return err
		}
		return nil
	}
	return out, closer, nil
}

func run(cmd *cobra.Command, _ []string) error {
	stream, closer, err := planStream()
	if err != nil {
		return err
	}
	rep, perr := drift.Parse(stream)
	if perr != nil {
		return perr
	}
	if err := closer(); err != nil {
		return fmt.Errorf("terraform failed: %w", err)
	}

	if jsonOut {
		enc := jsonEncoder(cmd.OutOrStdout())
		if err := enc(rep); err != nil {
			return err
		}
	} else {
		fmt.Fprintln(cmd.OutOrStdout(), rep.Text())
	}

	if slackHook != "" {
		if err := slack.Post(slackHook, rep.Slack()); err != nil {
			return fmt.Errorf("slack notify: %w", err)
		}
	}

	if rep.HasDrift() {
		exit(exitDrift)
	}
	return nil
}

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		exit(exitError)
	}
}
