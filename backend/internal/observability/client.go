package observability

import (
	"fmt"
	"time"

	"github.com/getsentry/sentry-go"
	sentrygin "github.com/getsentry/sentry-go/gin"
	sentrylogrus "github.com/getsentry/sentry-go/logrus"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

type Client struct {
	enabled bool
	logHook sentrylogrus.Hook
}

func DisabledClient() *Client {
	return &Client{}
}

func Init(cfg Config, logger *logrus.Logger) (*Client, error) {
	if !cfg.Enabled {
		return DisabledClient(), nil
	}

	options := sentry.ClientOptions{
		Dsn:                   cfg.DSN,
		Environment:           cfg.Environment,
		Release:               cfg.Release,
		ServerName:            cfg.Service,
		EnableTracing:         cfg.EnableTracing,
		TracesSampleRate:      cfg.TracesSampleRate,
		SendDefaultPII:        false,
		AttachStacktrace:      true,
		BeforeSend:            ScrubEvent,
		BeforeSendLog:         ScrubLog,
		BeforeSendTransaction: ScrubTransaction,
		Tags: map[string]string{
			"service": cfg.Service,
		},
	}

	if err := sentry.Init(options); err != nil {
		return DisabledClient(), err
	}

	client := &Client{enabled: true}
	if cfg.EnableLogs && logger != nil {
		sentryClient := sentry.CurrentHub().Client()
		if sentryClient == nil {
			return DisabledClient(), fmt.Errorf("sentry client unavailable after initialization")
		}

		logLevels := nonTerminatingLogLevels(cfg.LogLevels)
		if len(logLevels) > 0 {
			hook := sentrylogrus.NewLogHookFromClient(logLevels, sentryClient)
			hook.AddTags(map[string]string{"service": cfg.Service})
			logger.AddHook(hook)
			client.logHook = hook
		}
	}

	return client, nil
}

func nonTerminatingLogLevels(levels []logrus.Level) []logrus.Level {
	filtered := make([]logrus.Level, 0, len(levels))
	seen := make(map[logrus.Level]struct{}, len(levels))
	for _, level := range levels {
		switch level {
		case logrus.TraceLevel, logrus.DebugLevel, logrus.InfoLevel, logrus.WarnLevel, logrus.ErrorLevel:
			if _, ok := seen[level]; ok {
				continue
			}
			seen[level] = struct{}{}
			filtered = append(filtered, level)
		}
	}

	return filtered
}

func (c *Client) Enabled() bool {
	return c != nil && c.enabled
}

func (c *Client) Flush(timeout time.Duration) bool {
	if c == nil || !c.enabled {
		return true
	}

	flushed := true
	if c.logHook != nil {
		flushed = c.logHook.Flush(timeout) && flushed
	}

	return sentry.Flush(timeout) && flushed
}

func GinMiddlewares(client *Client) []gin.HandlerFunc {
	if client == nil || !client.Enabled() {
		return nil
	}

	return []gin.HandlerFunc{
		sentrygin.New(sentrygin.Options{
			Repanic:         true,
			WaitForDelivery: false,
			Timeout:         2 * time.Second,
		}),
		GinContextMiddleware(),
	}
}
