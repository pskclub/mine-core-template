// Package cmd is the entry point for each role the binary can run: api, worker,
// or both in one process.
//
// It decides how this deployment is configured — CORS, timeouts, which
// scheduler — and nothing about what the service does. Which modules exist and
// what protects them lives in cmd.NewAPI, so a change to the route list
// never touches process startup and a change to startup never touches the
// routes.
package cmd

import (
	"time"

	core "github.com/pskclub/mine-core/v2"
)

// Bootstrap builds the App once at startup. The App owns every long-lived
// resource (connection pools, logger, Sentry client) and is shared by the API
// and the worker — pools are never rebuilt per request or per job.
//
// A connection is only opened when it is configured, so a service can run on
// SQL alone. Nothing returns nil for it: ctx.Cache() gives a cache that misses
// every read and drops every write, so cache-aside code runs either way, and
// ctx.MQ() / ctx.Storage() give handles whose every call fails naming the
// configuration that is missing — a message nobody receives is not something to
// degrade quietly about.
func Bootstrap() (*core.App, core.IError) {
	env, err := core.NewEnv()
	if err != nil {
		return nil, err
	}
	cfg := env.Config()

	opts := make([]core.Option, 0, 3)

	if cfg.DBConnectionString != "" || cfg.DBHost != "" {
		db, err := core.NewDatabase(env,
			core.WithMaxOpenConns(20),
			core.WithMaxIdleConns(5),
			core.WithConnMaxLifetime(time.Hour),
		)
		if err != nil {
			return nil, err
		}
		opts = append(opts, core.WithSQL("default", db))
	}

	if cfg.CacheConnectionString != "" || cfg.CacheHost != "" {
		cache, err := core.NewCache(env)
		if err != nil {
			return nil, err
		}
		opts = append(opts, core.WithCache("default", cache))
	}

	if cfg.MQConnectionString != "" || cfg.MQHost != "" {
		mq, err := core.NewMQ(env)
		if err != nil {
			return nil, err
		}
		opts = append(opts, core.WithMQ(mq))
	}

	return core.NewApp(env, opts...)
}
