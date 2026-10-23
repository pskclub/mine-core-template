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

	// The integrations follow the same rule as the pools above: opened only
	// when configured, and a handle that fails loudly otherwise. Without these
	// lines a service that set S3_* or EMAIL_* would still get the disabled
	// handle, and every upload or mail would fail naming configuration that is
	// in fact present — the kind of mistake that looks like the framework's.
	if cfg.S3Bucket != "" {
		storage, err := core.NewStorage(env)
		if err != nil {
			return nil, err
		}
		opts = append(opts, core.WithStorage(storage))
	}

	if cfg.EmailServer != "" {
		mailer, err := core.NewMailer(env)
		if err != nil {
			return nil, err
		}
		opts = append(opts, core.WithMailer(mailer))
	}

	if cfg.FirebaseCredential != "" {
		pusher, err := core.NewPusherFromEnv(env)
		if err != nil {
			return nil, err
		}
		opts = append(opts, core.WithPusher(pusher))
	}

	return core.NewApp(env, opts...)
}
