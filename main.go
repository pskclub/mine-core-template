package main

import (
	"log"
	"strings"

	"github.com/pskclub/mine-core-template/cmd"
	"github.com/pskclub/mine-core-template/consts"
)

func main() {
	app, err := cmd.Bootstrap()
	if err != nil {
		log.Fatalf("bootstrap: %v", err)
	}

	// Same binary, one role per deployment: many API replicas, few workers — or
	// "all" to run both in one process. Every role shares cmd.Bootstrap's wiring.
	//
	// The role is read from config, so `ROLE=all` in .env and `APP_ROLE=all` in
	// the environment both work — a compose file sets the latter, local dev the
	// former. Reading os.Getenv directly would silently ignore the .env one.
	switch strings.ToLower(strings.TrimSpace(app.ENV().String("role"))) {
	case consts.RoleWorker:
		cmd.WorkerRun(app)
	case consts.RoleAll:
		cmd.AllRun(app)
	default:
		cmd.APIRun(app)
	}
}
