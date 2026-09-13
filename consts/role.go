// Package consts holds values that more than one module has to agree on.
//
// Like models, it is part of the shared kernel: constants only, no behaviour,
// and no import of anything in this project. A value used by exactly one module
// belongs in that module — this is for the ones where two packages disagreeing
// would be a bug.
package consts

// Role selects what the binary runs, read from config like any other key:
// `ROLE=all` in .env, or `APP_ROLE=all` in the environment (which is how
// docker-compose and Kubernetes set it). Both resolve to the same key.
//
// Note the asymmetry that catches people out: the APP_ prefix belongs to OS
// environment variables only. `APP_ROLE=all` written *inside* .env binds to the
// key "app_role" and is ignored.
const (
	RoleAPI    = "api"    // HTTP only
	RoleWorker = "worker" // scheduled jobs only
	RoleAll    = "all"    // both, in one process — single replica only
)
