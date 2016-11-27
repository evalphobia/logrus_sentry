package logrus_sentry

import (
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/getsentry/raven-go"
	"github.com/pkg/errors"
)

var (
	severityMap = map[logrus.Level]raven.Severity{
		logrus.DebugLevel: raven.DEBUG,
		logrus.InfoLevel:  raven.INFO,
		logrus.WarnLevel:  raven.WARNING,
		logrus.ErrorLevel: raven.ERROR,
		logrus.FatalLevel: raven.FATAL,
		logrus.PanicLevel: raven.FATAL,
	}
)

// SentryHook delivers logs to a sentry server.
type SentryHook struct {
	// Timeout sets the time to wait for a delivery error from the sentry server.
	// If this is set to zero the server will not wait for any response and will
	// consider the message correctly sent
	Timeout                 time.Duration
	StacktraceConfiguration StackTraceConfiguration

	client *raven.Client
	levels []logrus.Level

	ignoreFields map[string]struct{}
	extraFilters map[string]func(interface{}) interface{}
}

// The Stacktracer interface allows an error type to return a raven.Stacktrace.
type Stacktracer interface {
	GetStacktrace() *raven.Stacktrace
}

type causer interface {
	Cause() error
}

type pkgErrorStackTracer interface {
	StackTrace() errors.StackTrace
}

// StackTraceConfiguration allows for configuring stacktraces
type StackTraceConfiguration struct {
	// whether stacktraces should be enabled
	Enable bool
	// the level at which to start capturing stacktraces
	Level logrus.Level
	// how many stack frames to skip before stacktrace starts recording
	Skip int
	// the number of lines to include around a stack frame for context
	Context int
	// the prefixes that will be matched against the stack frame.
	// if the stack frame's package matches one of these prefixes
	// sentry will identify the stack frame as "in_app"
	InAppPrefixes []string
}

// NewSentryHook creates a hook to be added to an instance of logger
// and initializes the raven client.
// This method sets the timeout to 100 milliseconds.
func NewSentryHook(DSN string, levels []logrus.Level) (*SentryHook, error) {
	client, err := raven.New(DSN)
	if err != nil {
		return nil, err
	}
	return NewWithClientSentryHook(client, levels)
}

// NewWithTagsSentryHook creates a hook with tags to be added to an instance
// of logger and initializes the raven client. This method sets the timeout to
// 100 milliseconds.
func NewWithTagsSentryHook(DSN string, tags map[string]string, levels []logrus.Level) (*SentryHook, error) {
	client, err := raven.NewWithTags(DSN, tags)
	if err != nil {
		return nil, err
	}
	return NewWithClientSentryHook(client, levels)
}

// NewWithClientSentryHook creates a hook using an initialized raven client.
// This method sets the timeout to 100 milliseconds.
func NewWithClientSentryHook(client *raven.Client, levels []logrus.Level) (*SentryHook, error) {
	return &SentryHook{
		Timeout: 100 * time.Millisecond,
		StacktraceConfiguration: StackTraceConfiguration{
			Enable:        false,
			Level:         logrus.ErrorLevel,
			Skip:          5,
			Context:       0,
			InAppPrefixes: nil,
		},
		client:       client,
		levels:       levels,
		ignoreFields: make(map[string]struct{}),
		extraFilters: make(map[string]func(interface{}) interface{}),
	}, nil
}

// Fire is called when an event should be sent to sentry
// Special fields that sentry uses to give more information to the server
// are extracted from entry.Data (if they are found)
// These fields are: error, logger, server_name, http_request, tags
func (hook *SentryHook) Fire(entry *logrus.Entry) error {

	packet := raven.NewPacket(entry.Message)
	packet.Timestamp = raven.Timestamp(entry.Time)
	packet.Level = severityMap[entry.Level]
	packet.Platform = "go"

	d := entry.Data

	if logger, ok := getAndDelString(d, "logger"); ok {
		packet.Logger = logger
	}
	if serverName, ok := getAndDelString(d, "server_name"); ok {
		packet.ServerName = serverName
	}
	if req, ok := getAndDelRequest(d, "http_request"); ok {
		packet.Interfaces = append(packet.Interfaces, raven.NewHttp(req))
	}
	if tags, ok := getAndDeleteTags(d, "tags"); ok {
		packet.Tags = tags
	}
	if user, ok := getUserContext(d); ok {
		packet.Interfaces = append(packet.Interfaces, user)
	}
	if eventID, ok := getEventID(d); ok {
		packet.EventID = eventID
	}

	stConfig := &hook.StacktraceConfiguration
	if stConfig.Enable && entry.Level <= stConfig.Level {
		if err, ok := getAndDelError(d, logrus.ErrorKey); ok {
			var currentStacktrace *raven.Stacktrace
			currentStacktrace, err = hook.findStacktraceAndCause(err)
			if currentStacktrace == nil {
				currentStacktrace = raven.NewStacktrace(stConfig.Skip, stConfig.Context, stConfig.InAppPrefixes)
			}
			exc := raven.NewException(err, currentStacktrace)
			packet.Interfaces = append(packet.Interfaces, exc)
			packet.Culprit = err.Error()
		} else {
			currentStacktrace := raven.NewStacktrace(stConfig.Skip, stConfig.Context, stConfig.InAppPrefixes)
			packet.Interfaces = append(packet.Interfaces, currentStacktrace)
		}
	}

	dataExtra := hook.formatExtraData(d)
	if packet.Extra == nil {
		packet.Extra = dataExtra
	} else {
		for k, v := range dataExtra {
			packet.Extra[k] = v
		}
	}

	_, errCh := hook.client.Capture(packet, nil)
	timeout := hook.Timeout
	if timeout != 0 {
		timeoutCh := time.After(timeout)
		select {
		case err := <-errCh:
			return err
		case <-timeoutCh:
			return fmt.Errorf("no response from sentry server in %s", timeout)
		}
	}
	return nil
}

func (hook *SentryHook) findStacktraceAndCause(err error) (*raven.Stacktrace, error) {
	errCause := errors.Cause(err)
	var stacktrace *raven.Stacktrace
	var stackErr errors.StackTrace
	for err != nil {
		// Find the earliest *raven.Stacktrace, or error.StackTrace
		if tracer, ok := err.(Stacktracer); ok {
			stacktrace = tracer.GetStacktrace()
			stackErr = nil
		} else if tracer, ok := err.(pkgErrorStackTracer); ok {
			stacktrace = nil
			stackErr = tracer.StackTrace()
		}
		if cause, ok := err.(causer); ok {
			err = cause.Cause()
		} else {
			break
		}
	}
	if stackErr != nil {
		stacktrace = hook.convertStackTrace(stackErr)
	}
	return stacktrace, errCause
}

// convertStackTrace converts an errors.StackTrace into a natively consumable
// *raven.Stacktrace
func (hook *SentryHook) convertStackTrace(st errors.StackTrace) *raven.Stacktrace {
	stConfig := &hook.StacktraceConfiguration
	stFrames := []errors.Frame(st)
	frames := make([]*raven.StacktraceFrame, 0, len(stFrames))
	for i := range stFrames {
		pc := uintptr(stFrames[i])
		fn := runtime.FuncForPC(pc)
		file, line := fn.FileLine(pc)
		frames = append(frames, raven.NewStacktraceFrame(pc, file, line, stConfig.Context, stConfig.InAppPrefixes))
	}
	return &raven.Stacktrace{Frames: frames}
}

// Levels returns the available logging levels.
func (hook *SentryHook) Levels() []logrus.Level {
	return hook.levels
}

// SetRelease sets release tag.
func (hook *SentryHook) SetRelease(release string) {
	hook.client.SetRelease(release)
}

// SetEnvironment sets environment tag.
func (hook *SentryHook) SetEnvironment(environment string) {
	hook.client.SetEnvironment(environment)
}

// AddIgnore adds field name to ignore.
func (hook *SentryHook) AddIgnore(name string) {
	hook.ignoreFields[name] = struct{}{}
}

// AddExtraFilter adds a custom filter function.
func (hook *SentryHook) AddExtraFilter(name string, fn func(interface{}) interface{}) {
	hook.extraFilters[name] = fn
}

func (hook *SentryHook) formatExtraData(fields logrus.Fields) (result map[string]interface{}) {
	// create a map for passing to Sentry's extra data
	result = make(map[string]interface{}, len(fields))
	for k, v := range fields {
		if _, ok := hook.ignoreFields[k]; ok {
			continue
		}
		if fn, ok := hook.extraFilters[k]; ok {
			v = fn(v) // apply custom filter
		} else {
			v = formatData(v) // use default formatter
		}
		result[k] = v
	}
	return result
}

// formatData returns value as a suitable format.
func formatData(value interface{}) (formatted interface{}) {
	switch value := value.(type) {
	case json.Marshaler:
		return value
	case error:
		return value.Error()
	case fmt.Stringer:
		return value.String()
	default:
		return value
	}
}

func getEventID(d logrus.Fields) (string, bool) {
	eventID, ok := d["event_id"].(string)

	if !ok {
		return "", false
	}

	//verify eventID is 32 characters hexadecimal string (UUID4)
	uuid := parseUUID(eventID)

	if uuid == nil {
		return "", false
	}

	return uuid.noDashString(), true
}

func getAndDelString(d logrus.Fields, key string) (string, bool) {
	if value, ok := d[key].(string); ok {
		delete(d, key)
		return value, true
	}
	return "", false
}

func getAndDelError(d logrus.Fields, key string) (error, bool) {
	if value, ok := d[key].(error); ok {
		delete(d, key)
		return value, true
	}
	return nil, false
}

func getAndDelRequest(d logrus.Fields, key string) (*http.Request, bool) {
	if value, ok := d[key].(*http.Request); ok {
		delete(d, key)
		return value, true
	}
	return nil, false
}

func getAndDeleteTags(d logrus.Fields, key string) (raven.Tags, bool) {
	if value, ok := d[key].(raven.Tags); ok {
		delete(d, key)
		return value, true
	}
	return nil, false
}

func getUserContext(d logrus.Fields) (*raven.User, bool) {
	if v, ok := d["user"]; ok {
		switch val := v.(type) {
		case *raven.User:
			return val, true

		case raven.User:
			return &val, true
		}
	}

	username, _ := d["user_name"].(string)
	email, _ := d["user_email"].(string)
	id, _ := d["user_id"].(string)
	ip, _ := d["user_ip"].(string)

	if username == "" && email == "" && id == "" && ip == "" {
		return nil, false
	}

	return &raven.User{
		ID:       id,
		Username: username,
		Email:    email,
		IP:       ip,
	}, true
}
