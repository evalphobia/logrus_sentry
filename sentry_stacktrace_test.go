package logrus_sentry

import (
	"strings"
	"testing"

	"github.com/getsentry/raven-go"
	pkgerrors "github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

func TestSentryStacktrace(t *testing.T) {
	WithTestDSN(t, func(dsn string, pch <-chan *resultPacket) {
		logger := getTestLogger()
		hook, err := NewSentryHook(dsn, []logrus.Level{
			logrus.ErrorLevel,
			logrus.InfoLevel,
		})
		if err != nil {
			t.Fatal(err.Error())
		}
		logger.Hooks.Add(hook)

		logger.Error(message)
		packet := <-pch
		stacktraceSize := len(packet.Stacktrace.Frames)
		if stacktraceSize != 0 {
			t.Error("Stacktrace should be empty as it is not enabled")
		}

		hook.StacktraceConfiguration.Enable = true

		logger.Error(message) // this is the call that the last frame of stacktrace should capture
		expectedLineno := 33  //this should be the line number of the previous line
		packet = <-pch
		stacktraceSize = len(packet.Stacktrace.Frames)
		if stacktraceSize == 0 {
			t.Error("Stacktrace should not be empty")
		}
		lastFrame := packet.Stacktrace.Frames[stacktraceSize-2]
		expectedSuffix := "logrus_sentry/sentry_stacktrace_test.go"
		if !strings.HasSuffix(lastFrame.Filename, expectedSuffix) {
			t.Errorf("File name should have ended with %s, was %s", expectedSuffix, lastFrame.Filename)
		}
		if lastFrame.Lineno != expectedLineno {
			t.Errorf("Line number should have been %d, was %d", expectedLineno, lastFrame.Lineno)
		}
		if lastFrame.InApp {
			t.Error("Frame should not be identified as in_app without prefixes")
		}

		hook.StacktraceConfiguration.InAppPrefixes = []string{"github.com/sirupsen/logrus"}
		hook.StacktraceConfiguration.Context = 2
		hook.StacktraceConfiguration.Skip = 2

		logger.Error(message)
		packet = <-pch
		stacktraceSize = len(packet.Stacktrace.Frames)
		if stacktraceSize == 0 {
			t.Error("Stacktrace should not be empty")
		}
		lastFrame = packet.Stacktrace.Frames[stacktraceSize-1]
		expectedFilename := "github.com/sirupsen/logrus/entry.go"
		if lastFrame.Filename != expectedFilename {
			t.Errorf("File name should have been %s, was %s", expectedFilename, lastFrame.Filename)
		}
		if !lastFrame.InApp {
			t.Error("Frame should be identified as in_app")
		}

		logger.WithError(myStacktracerError{}).Error(message) // use an error that implements Stacktracer
		packet = <-pch
		var frames []*raven.StacktraceFrame
		if packet.Exception.Stacktrace != nil {
			frames = packet.Exception.Stacktrace.Frames
		}
		if len(frames) != 1 || frames[0].Filename != expectedStackFrameFilename {
			t.Error("Stacktrace should be taken from err if it implements the Stacktracer interface")
		}

		logger.WithError(pkgerrors.Wrap(myStacktracerError{}, "wrapped")).Error(message) // use an error that wraps a Stacktracer
		packet = <-pch
		if packet.Exception.Stacktrace != nil {
			frames = packet.Exception.Stacktrace.Frames
		}
		expectedCulprit := "wrapped: myStacktracerError!"
		if packet.Culprit != expectedCulprit {
			t.Errorf("Expected culprit of '%s', got '%s'", expectedCulprit, packet.Culprit)
		}
		if len(frames) != 1 || frames[0].Filename != expectedStackFrameFilename {
			t.Error("Stacktrace should be taken from err if it implements the Stacktracer interface")
		}

		logger.WithError(pkgerrors.New("errorX")).Error(message) // use an error that implements pkgErrorStackTracer
		packet = <-pch
		if packet.Exception.Stacktrace != nil {
			frames = packet.Exception.Stacktrace.Frames
		}
		expectedPkgErrorsStackTraceFilename := "testing/testing.go"
		expectedFrameCount := 4
		expectedCulprit = "errorX"
		if packet.Culprit != expectedCulprit {
			t.Errorf("Expected culprit of '%s', got '%s'", expectedCulprit, packet.Culprit)
		}
		if len(frames) != expectedFrameCount {
			t.Errorf("Expected %d frames, got %d", expectedFrameCount, len(frames))
		}
		if !strings.HasSuffix(frames[0].Filename, expectedPkgErrorsStackTraceFilename) {
			t.Error("Stacktrace should be taken from err if it implements the pkgErrorStackTracer interface")
		}

		// zero stack frames
		defer func() {
			if err := recover(); err != nil {
				t.Error("Zero stack frames should not cause panic")
			}
		}()
		hook.StacktraceConfiguration.Skip = 1000
		logger.Error(message)
		<-pch // check panic
	})
}
