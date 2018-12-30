package logrus_sentry

import (
	"fmt"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestSetDefaultLoggerName(t *testing.T) {
	const name = "my_logger"
	a := assert.New(t)

	WithTestDSN(t, func(dsn string, pch <-chan *resultPacket) {
		logger := getTestLogger()
		hook, err := NewSentryHook(dsn, []logrus.Level{
			logrus.ErrorLevel,
		})
		a.NoError(err, "NewSentryHook should be NoError")

		hook.SetDefaultLoggerName(name)
		logger.Hooks.Add(hook)

		logger.Error(message)
		packet := <-pch
		a.Equal(name, packet.Logger, "logger must be set")
	})
}

func TestSetEnvironment(t *testing.T) {
	const env = "test"
	a := assert.New(t)

	WithTestDSN(t, func(dsn string, pch <-chan *resultPacket) {
		logger := getTestLogger()
		hook, err := NewSentryHook(dsn, []logrus.Level{
			logrus.ErrorLevel,
		})
		a.NoError(err, "NewSentryHook should be NoError")

		hook.SetEnvironment(env)
		logger.Hooks.Add(hook)

		logger.Error(message)
		packet := <-pch
		a.Equal(env, packet.Environment, "environment must be set")
	})
}

func TestSetIgnoreErrors(t *testing.T) {
	a := assert.New(t)
	tests := []struct {
		errString string
		isSuccess bool
	}{
		{"", true},
		{"aaa", true},
		{"jskljdasidjiaoklzmxcasifjiklmzx9eijodfsklcmzx", true},
		{"[0-9]+", true},
		{"[0-9", false},
		{"+", false},
	}

	WithTestDSN(t, func(dsn string, pch <-chan *resultPacket) {
		hook, err := NewSentryHook(dsn, []logrus.Level{
			logrus.ErrorLevel,
		})
		a.NoError(err, "NewSentryHook should be NoError")

		for _, tt := range tests {
			target := fmt.Sprintf("%+v", tt)
			err := hook.SetIgnoreErrors(tt.errString)
			switch {
			case !tt.isSuccess:
				a.Error(err, target)
			case tt.isSuccess:
				a.NoError(err, target)
			}
		}
	})
}

func TestSetIncludePaths(t *testing.T) {
	a := assert.New(t)
	paths := []string{
		"aaa",
		"bbb",
		"ccc",
	}

	WithTestDSN(t, func(dsn string, pch <-chan *resultPacket) {
		hook, err := NewSentryHook(dsn, []logrus.Level{
			logrus.ErrorLevel,
		})
		a.NoError(err, "NewSentryHook should be NoError")

		hook.SetIncludePaths(paths)
		a.Equal(paths, hook.client.IncludePaths(), "includePaths must be set")
	})
}

func TestSetRelease(t *testing.T) {
	const releaseVer = "v0.1.0"
	a := assert.New(t)

	WithTestDSN(t, func(dsn string, pch <-chan *resultPacket) {
		logger := getTestLogger()
		hook, err := NewSentryHook(dsn, []logrus.Level{
			logrus.ErrorLevel,
		})
		a.NoError(err, "NewSentryHook should be NoError")

		hook.SetRelease(releaseVer)
		logger.Hooks.Add(hook)

		logger.Error(message)
		packet := <-pch
		a.Equal(releaseVer, packet.Release, "release version must be set")
	})
}

func TestSetSampleRate(t *testing.T) {
	a := assert.New(t)
	tests := []struct {
		rate      float32
		isSuccess bool
	}{
		{0.0, true},
		{0.1, true},
		{0.5, true},
		{0.9, true},
		{1.0, true},
		{-0.1, false},
		{-2, false},
		{1.1, false},
		{2.0, false},
	}

	WithTestDSN(t, func(dsn string, pch <-chan *resultPacket) {
		hook, err := NewSentryHook(dsn, []logrus.Level{
			logrus.ErrorLevel,
		})
		a.NoError(err, "NewSentryHook should be NoError")

		for _, tt := range tests {
			target := fmt.Sprintf("%+v", tt)

			err := hook.SetSampleRate(tt.rate)
			switch {
			case !tt.isSuccess:
				a.Error(err, target)
			case tt.isSuccess:
				a.NoError(err, target)
			}
		}
	})
}

func TestSetServerName(t *testing.T) {
	a := assert.New(t)

	WithTestDSN(t, func(dsn string, pch <-chan *resultPacket) {
		logger := getTestLogger()
		hook, err := NewSentryHook(dsn, []logrus.Level{
			logrus.ErrorLevel,
		})
		a.NoError(err, "NewSentryHook should be NoError")

		hook.SetServerName(server_name)
		logger.Hooks.Add(hook)

		logger.Error(message)
		packet := <-pch
		a.Equal(server_name, packet.ServerName, "server name must be set")
	})
}
