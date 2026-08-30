package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/redscaresu/infrafactory/internal/livestore"
)

// newLiveCmd groups the commands that deal with infrastructure which
// deliberately outlives the run that created it. Everything under `run`
// and `test` promises the opposite -- apply, destroy, sweep -- so the
// exception gets its own verb rather than another flag on `run`.
func newLiveCmd(cfg *rootConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "live",
		Short: "Inspect infrastructure deliberately left running past the run that created it",
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "ls",
		Short: "List live deployments, their remaining TTL, and anything unaccounted for",
		// NoArgs like its siblings: without it cobra silently swallows a
		// mistyped `live ls <id>` and prints the whole listing, which a
		// script filtering on that argument reads as a match.
		Args: cobra.NoArgs,
		RunE: cfg.withRuntime("live ls", runLiveListCommand),
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "teardown <deployment-id>",
		Short: "Destroy one live deployment, sweep the account, and release the record",
		Args:  cobra.ExactArgs(1),
		RunE:  cfg.withRuntime("live teardown", runLiveTeardownCommand),
	})

	reap := &cobra.Command{
		Use:   "reap",
		Short: "Destroy every live deployment whose TTL has run out",
		Args:  cobra.NoArgs,
		RunE:  cfg.withRuntime("live reap", runLiveReapCommand),
	}
	reap.Flags().Bool("dry-run", false, "Report what would be destroyed without destroying it")
	cmd.AddCommand(reap)

	return cmd
}

func runLiveListCommand(cmd *cobra.Command, _ []string, runtime *CommandRuntime) error {
	store := livestore.NewFilesystemStore(runtime.LiveStoreRoot())

	deployments, unreadable, err := store.List()
	if err != nil {
		return &CLIError{Op: "live ls", Code: errorCodeCommandFailed, Err: err}
	}

	mode, err := outputModeFromCommand(cmd)
	if err != nil {
		return err
	}

	now := time.Now()
	if mode == OutputModeJSON {
		err = renderLiveJSON(cmd.OutOrStdout(), deployments, unreadable, now)
	} else {
		err = renderLiveTable(cmd.OutOrStdout(), deployments, unreadable, now)
	}
	if err != nil {
		return err
	}

	// A record the store could not decode may still describe real,
	// billing infrastructure. Reporting it and exiting zero would make
	// "we could not check" look exactly like "nothing is running".
	if len(unreadable) > 0 {
		return &CLIError{
			Op:   "live ls",
			Code: errorCodeCommandFailed,
			Err:  fmt.Errorf("%d live record(s) could not be read; they may describe running infrastructure", len(unreadable)),
		}
	}

	return nil
}

func renderLiveTable(out io.Writer, deployments []livestore.Deployment, unreadable []error, now time.Time) error {
	if len(deployments) == 0 && len(unreadable) == 0 {
		_, err := fmt.Fprintln(out, "No live deployments.")
		return err
	}

	if len(deployments) > 0 {
		w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tSCENARIO\tIMAGE\tSTATE\tTTL\tPROJECT\tADDRESS")
		for _, d := range deployments {
			state := string(d.State)
			if d.Undecodable {
				state = "UNREADABLE"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				d.ID, orDash(d.Scenario), imageRef(d), state, ttlLabel(d, now), orDash(d.ProjectID), orDash(d.Address))
		}
		if err := w.Flush(); err != nil {
			return err
		}
	}

	if len(unreadable) > 0 {
		fmt.Fprintf(out, "\n%d unreadable record(s) — these may be real infrastructure:\n", len(unreadable))
		for _, err := range unreadable {
			fmt.Fprintf(out, "  %s\n", err)
		}
	}

	return nil
}

// liveListJSON is a listing, not a staged run, so it does not pretend to
// be an OutputResult. Callers parsing this get deployments and the count
// of things the store could not account for.
type liveListJSON struct {
	Schema      string               `json:"schema"`
	Deployments []liveDeploymentJSON `json:"deployments"`
	Unreadable  []string             `json:"unreadable"`
}

type liveDeploymentJSON struct {
	livestore.Deployment
	Expired      bool   `json:"expired"`
	TimeToLive   string `json:"time_to_live"`
	TTLSeconds   int64  `json:"time_to_live_seconds"`
	ImageWithTag string `json:"image_with_tag,omitempty"`
}

func renderLiveJSON(out io.Writer, deployments []livestore.Deployment, unreadable []error, now time.Time) error {
	payload := liveListJSON{
		Schema:      "infrafactory.live.list.v1",
		Deployments: make([]liveDeploymentJSON, 0, len(deployments)),
		Unreadable:  make([]string, 0, len(unreadable)),
	}

	for _, d := range deployments {
		ttl := d.TimeToLive(now)
		payload.Deployments = append(payload.Deployments, liveDeploymentJSON{
			Deployment:   d,
			Expired:      d.Expired(now),
			TimeToLive:   ttl.Round(time.Second).String(),
			TTLSeconds:   int64(ttl.Seconds()),
			ImageWithTag: imageRef(d),
		})
	}
	for _, err := range unreadable {
		payload.Unreadable = append(payload.Unreadable, err.Error())
	}

	encoded, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("encode live list: %w", err)
	}

	_, err = fmt.Fprintf(out, "%s\n", encoded)
	return err
}

func imageRef(d livestore.Deployment) string {
	if d.Image == "" {
		return "-"
	}
	if d.Tag == "" {
		return d.Image
	}
	return d.Image + ":" + d.Tag
}

func orDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

// ttlLabel renders remaining life. Expired is spelled out rather than
// shown as "0s" because the two read very differently at a glance, and
// the operator's next action differs.
func ttlLabel(d livestore.Deployment, now time.Time) string {
	if d.State == livestore.StateReleased {
		return "-"
	}
	if d.Expired(now) {
		return "EXPIRED"
	}
	return d.TimeToLive(now).Round(time.Second).String()
}

func resolveLiveStoreRoot() string {
	if root := os.Getenv("INFRAFACTORY_LIVESTORE_ROOT"); root != "" {
		return root
	}
	return livestore.DefaultRoot
}
