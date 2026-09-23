package server

// Config is the server's construction options.
//
// It has no Hub field on purpose. Hub participation is decided by pkg/hub's own
// layered config (team code + credential + kill switch), resolved per call, so a
// server-level flag could only ever disagree with it — which is exactly what the
// removed HubSyncer seam did (docs/SYNC_CONTRACT.md §4.4).
type Config struct{}
