package tail

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Sirupsen/logrus"
	hptail "github.com/hpcloud/tail"
)

const globScanInterval = time.Second

type watchedGlob struct {
	pattern    string
	known      map[string]bool
	activeFile string
	active     *hptail.Tail
}

// WatchEntries returns a stream of independent line channels. Unlike
// GetEntries, it keeps expanding glob paths while dbtail is running so files
// created by timestamp-based log rotation are picked up without a restart.
func WatchEntries(ctx context.Context, conf Config) (<-chan chan string, error) {
	return watchEntries(ctx, conf, globScanInterval)
}

func watchEntries(ctx context.Context, conf Config, scanInterval time.Duration) (<-chan chan string, error) {
	if conf.Type != RotateStyleSyslog {
		return nil, errors.New("Only Syslog style rotation currently supported")
	}
	if err := ensureStateDirectory(&conf); err != nil {
		return nil, err
	}

	var exactFiles []string
	var globs []*watchedGlob
	totalFiles := 0
	for _, path := range conf.Paths {
		if path == "-" || !hasGlobMeta(path) {
			exactFiles = append(exactFiles, path)
			totalFiles++
			continue
		}
		files, err := filepath.Glob(path)
		if err != nil {
			return nil, err
		}
		files = removeStateFiles(files, conf)
		known := make(map[string]bool, len(files))
		for _, file := range files {
			known[file] = true
		}
		globs = append(globs, &watchedGlob{pattern: path, known: known})
		totalFiles += len(files)
		cleanupStaleGlobStates(conf, path, files)
	}
	if totalFiles == 0 {
		return nil, errors.New("After removing missing files and state files from the list, there are no files left to tail")
	}

	streams := make(chan chan string)
	go func() {
		defer close(streams)
		for _, file := range exactFiles {
			if file == "-" {
				if !sendStream(ctx, streams, tailStdIn(ctx)) {
					return
				}
				continue
			}
			lines, _, err := startWatchedFile(ctx, conf, file, totalFiles, false, false)
			if err != nil {
				logrus.WithError(err).WithField("logfile", file).Error("failed to tail logfile")
				continue
			}
			if !sendStream(ctx, streams, lines) {
				return
			}
		}

		for _, watched := range globs {
			files := knownFiles(watched.known)
			for i, file := range files {
				stopAtEOF := i < len(files)-1
				// Globbed files are independently rotated log segments. On restart,
				// resume from state when it exists and backfill from the beginning
				// when it does not.
				lines, tailer, err := startWatchedFile(ctx, conf, file, totalFiles, stopAtEOF, true)
				if err != nil {
					logrus.WithError(err).WithField("logfile", file).Error("failed to tail logfile")
					continue
				}
				if !stopAtEOF {
					watched.activeFile = file
					watched.active = tailer
				}
				if !sendStream(ctx, streams, lines) {
					return
				}
			}
		}

		if conf.Options.Stop || len(globs) == 0 {
			return
		}

		ticker := time.NewTicker(scanInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				for _, watched := range globs {
					if watched.active != nil {
						watched.active.Stop()
						watched.active.Cleanup()
					}
				}
				return
			case <-ticker.C:
				for _, watched := range globs {
					files, err := filepath.Glob(watched.pattern)
					if err != nil {
						logrus.WithError(err).WithField("glob", watched.pattern).Warn("failed to rescan logfile glob")
						continue
					}
					files = removeStateFiles(files, conf)
					present := make(map[string]bool, len(files))
					for _, file := range files {
						present[file] = true
					}
					for file := range watched.known {
						if present[file] {
							continue
						}
						delete(watched.known, file)
						if watched.active != nil && watched.activeFile == file {
							if err := watched.active.Stop(); err != nil {
								logrus.WithError(err).WithField("logfile", file).Warn("failed to stop tailing deleted logfile")
							}
							watched.active.Cleanup()
							watched.active = nil
							watched.activeFile = ""
						}
						removeStateFile(conf, file)
					}
					var discovered []string
					for _, file := range files {
						if !watched.known[file] {
							discovered = append(discovered, file)
						}
					}
					if len(discovered) == 0 {
						continue
					}
					sort.Strings(discovered)
					for _, file := range discovered {
						lines, tailer, err := startWatchedFile(ctx, conf, file, 2, false, true)
						if err != nil {
							logrus.WithError(err).WithField("logfile", file).Error("failed to tail newly discovered logfile")
							continue
						}
						watched.known[file] = true
						previous := watched.active
						watched.activeFile = file
						watched.active = tailer
						if !sendStream(ctx, streams, lines) {
							tailer.Stop()
							tailer.Cleanup()
							return
						}
						// Do not make delivery of the new active logfile wait for a
						// potentially large previous logfile to drain. The old tailer
						// closes its descriptor independently after reaching EOF.
						if previous != nil {
							go stopAtEOFAndCleanup(previous)
						}
					}
				}
			}
		}
	}()
	return streams, nil
}

func stopAtEOFAndCleanup(tailer *hptail.Tail) {
	if err := tailer.StopAtEOF(); err != nil {
		logrus.WithError(err).Warn("failed to stop previous logfile at EOF")
	}
	tailer.Cleanup()
}

// WatchSampledEntries applies tail sampling independently to every file stream.
func WatchSampledEntries(ctx context.Context, conf Config, sampleRate uint) (<-chan chan string, error) {
	unsampled, err := WatchEntries(ctx, conf)
	if err != nil {
		return nil, err
	}
	if sampleRate == 1 {
		return unsampled, nil
	}
	sampledStreams := make(chan chan string)
	go func() {
		defer close(sampledStreams)
		for input := range unsampled {
			output := make(chan string)
			if !sendStream(ctx, sampledStreams, output) {
				return
			}
			go func(in chan string, out chan string) {
				defer close(out)
				for line := range in {
					if !shouldDrop(sampleRate) {
						out <- line
					}
				}
			}(input, output)
		}
	}()
	return sampledStreams, nil
}

func sendStream(ctx context.Context, streams chan chan string, lines chan string) bool {
	select {
	case streams <- lines:
		return true
	case <-ctx.Done():
		return false
	}
}

func removeStateFile(conf Config, logfile string) {
	stateFile := getStateFile(conf, logfile, 2)
	if err := os.Remove(stateFile); err != nil && !os.IsNotExist(err) {
		logrus.WithError(err).WithFields(logrus.Fields{
			"logfile":   logfile,
			"statefile": stateFile,
		}).Warn("failed to remove state for deleted logfile")
	}
}

// cleanupStaleGlobStates removes state left behind while dbtail was stopped.
// It is intentionally limited to patterns ending in .log, where the mapping
// to dbtail's derived state filename is unambiguous.
func cleanupStaleGlobStates(conf Config, pattern string, files []string) {
	if !strings.HasSuffix(pattern, ".log") {
		return
	}
	wanted := make(map[string]bool, len(files))
	for _, file := range files {
		wanted[getStateFile(conf, file, 2)] = true
	}
	statePattern := filepath.Join(filepath.Dir(getStateFile(conf, pattern, 2)),
		strings.TrimSuffix(filepath.Base(pattern), ".log")+".leash.state")
	states, err := filepath.Glob(statePattern)
	if err != nil {
		return
	}
	for _, state := range states {
		if wanted[state] {
			continue
		}
		if err := os.Remove(state); err != nil && !os.IsNotExist(err) {
			logrus.WithError(err).WithField("statefile", state).Warn("failed to remove stale logfile state")
		}
	}
}

func startWatchedFile(ctx context.Context, conf Config, file string, numFiles int, stopAtEOF bool, isNew bool) (chan string, *hptail.Tail, error) {
	stateFile := getStateFile(conf, file, numFiles)
	fileConf := conf
	if isNew {
		if _, err := os.Stat(stateFile); os.IsNotExist(err) {
			fileConf.Options.ReadFrom = "beginning"
		}
	}
	if stopAtEOF {
		fileConf.Options.Stop = true
	}
	tailer, err := getTailer(fileConf, file, stateFile)
	if err != nil {
		return nil, nil, err
	}
	return tailSingleFile(ctx, tailer, file, stateFile), tailer, nil
}

func ensureStateDirectory(conf *Config) error {
	statePath := conf.Options.StateFile
	if statePath == "" || !strings.HasSuffix(statePath, string(os.PathSeparator)) {
		return nil
	}
	return os.MkdirAll(statePath, 0755)
}

func hasGlobMeta(path string) bool {
	return strings.ContainsAny(path, "*?[")
}

func knownFiles(known map[string]bool) []string {
	files := make([]string, 0, len(known))
	for file := range known {
		files = append(files, file)
	}
	sort.Strings(files)
	return files
}
