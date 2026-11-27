package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/hasankaradmir/karga-sdk/api"
)

func newVolumeCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{Use: "volume", Short: "Back up and restore a release's volumes"}
	var backup api.VolumeBackupRequest
	b := &cobra.Command{
		Use:   "backup <release> <volume> <file.tar.gz>",
		Short: "Copy a volume's files to a backup file (0600, must not exist)",
		Long: "Copy a volume's files to a gzipped tar through a workload that mounts it. The workloads writing to\n" +
			"it, and those that depend on them, are stopped meanwhile for a consistent copy, and started again in\n" +
			"order, ready, before the file is written. --live copies while they run: not consistent for an\n" +
			"application that writes (a database).",
		Args: cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			backup.Release, backup.Volume = args[0], args[1]
			if backup.Live {
				_, _ = fmt.Fprintln(a.stderr, "Warning: a live copy of a volume an application writes to may not be consistent; without --live its workloads are stopped meanwhile")
			}
			s := a.backend()
			var res *api.VolumeResult
			err := writeNew(args[2], func(w io.Writer) (err error) {
				res, err = s.BackupVolume(cmd.Context(), backup, w)
				return err
			}, func(err error) bool { return errors.Is(err, api.ErrNotReady) }) // the copy is complete
			if errors.Is(err, api.ErrNotReady) {
				return fmt.Errorf("%s was written; %w", args[2], err)
			}
			if err != nil {
				return err
			}
			return a.report(volumeReport(backup.Release, backup.Volume, args[2], res),
				"volume %s of %s written to %s%s\n", backup.Volume, backup.Release, args[2], downFor(res))
		},
	}
	b.Flags().BoolVar(&backup.Live, "live", false, "copy while the workloads run (not consistent for a database)")
	var restore api.VolumeRestoreRequest
	r := &cobra.Command{
		Use:   "restore <release> <volume> <file.tar.gz>",
		Short: "Replace a volume's files with a backup (its workloads are stopped meanwhile)",
		Long: "Replace a volume's files with a backup's. The workloads mounting it, and those that depend on them,\n" +
			"are stopped meanwhile and started again in order, ready. The volume's files are kept first as its\n" +
			"restore point: 'karga volume undo-restore' puts them back. --merge writes the backup over the files\n" +
			"instead, keeping those it does not have.",
		Args: cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			restore.Release, restore.Volume, restore.Name = args[0], args[1], filepath.Base(args[2])
			if err := a.confirm(cmd, restore.Risk()); err != nil {
				return err
			}
			restore.Confirmed = true
			f, err := os.Open(args[2])
			if err != nil {
				return err
			}
			defer func() { _ = f.Close() }()
			progress, done := a.volumeProgress()
			res, err := a.backend().RestoreVolume(cmd.Context(), restore, f, progress)
			done()
			if err != nil {
				return err
			}
			msg := fmt.Sprintf("volume %s of %s restored from %s%s\n", restore.Volume, restore.Release, args[2], downFor(res))
			if res != nil && res.RestorePoint != "" {
				msg += fmt.Sprintf("Its previous files are kept: 'karga volume undo-restore %s %s' puts them back.\n", restore.Release, restore.Volume)
			} else if restore.NoRestorePoint {
				msg += "It has no restore point now: its previous files cannot be brought back.\n"
			}
			return a.report(volumeReport(restore.Release, restore.Volume, args[2], res), "%s", msg)
		},
	}
	r.Flags().BoolVar(&restore.Force, "from-other-volume", false, "restore a backup taken from another volume or release")
	r.Flags().BoolVar(&restore.Merge, "merge", false, "write the backup over the volume's files, keeping those it does not have")
	r.Flags().BoolVar(&restore.NoRestorePoint, "no-restore-point", false, "keep no restore point and remove the volume's older one (the current files cannot be brought back)")
	var undo api.VolumeUndoRestoreRequest
	u := &cobra.Command{
		Use:   "undo-restore <release> <volume>",
		Short: "Put back the files a volume had before its last restore",
		Long: "Put back a volume's restore point, the files it had before its last restore, the way a restore\n" +
			"does: its workloads, and those that depend on them, are stopped meanwhile.",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			undo.Release, undo.Volume = args[0], args[1]
			if err := a.confirm(cmd, undo.Risk()); err != nil {
				return err
			}
			undo.Confirmed = true
			progress, done := a.volumeProgress()
			res, err := a.backend().UndoRestoreVolume(cmd.Context(), undo, progress)
			done()
			if err != nil {
				return err
			}
			return a.report(volumeReport(undo.Release, undo.Volume, "", res),
				"volume %s of %s is back as it was before its last restore%s\n", undo.Volume, undo.Release, downFor(res))
		},
	}
	cmd.AddCommand(b, r, u)
	return cmd
}

// volumeProgress shows a volume operation's progress on stderr, except for JSON or YAML output.
func (a *app) volumeProgress() (func(api.Event), func()) {
	if a.structured() {
		return nil, func() {}
	}
	p := newProgressPrinter(a.stderr)
	return p.event, p.finish
}

// downFor says which workloads a volume operation stopped and for how long.
func downFor(res *api.VolumeResult) string {
	if res == nil || len(res.Stopped) == 0 {
		return ""
	}
	verb := "was"
	if len(res.Stopped) > 1 {
		verb = "were"
	}
	return fmt.Sprintf(" (%s %s down for %s)", strings.Join(res.Stopped, ", "), verb, time.Duration(res.Downtime).Round(100*time.Millisecond))
}

func volumeReport(rel, vol, file string, res *api.VolumeResult) map[string]any {
	out := map[string]any{"release": rel, "volume": vol}
	if file != "" {
		out["file"] = file
	}
	if res != nil {
		out["stopped"], out["downtime"], out["restorePoint"] = res.Stopped, res.Downtime, res.RestorePoint
	}
	return out
}

// writeNew creates path (0600, must not exist) and fills it with fn; on failure the file is removed,
// unless keep says the error leaves it complete.
func writeNew(path string, fn func(io.Writer) error, keep func(error) bool) (err error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil && (keep == nil || !keep(err)) {
			_ = os.Remove(path)
		}
	}()
	return fn(f)
}
