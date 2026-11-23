// Package jobhistory persists the finished print jobs of the bridge so the
// Jobs page survives restarts. It stores one core.JobEntry per line (JSON
// Lines), appends in O(1), tolerates a torn or corrupt line, and compacts
// the file atomically so it never grows without bound. It knows nothing of
// the UI or of when jobs finish: callers hand it entries.
package jobhistory
