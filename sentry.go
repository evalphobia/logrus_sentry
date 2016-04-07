package logrus_sentry

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/getsentry/raven-go"
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

func getAndDel(d logrus.Fields, key string) (string, bool) {
	if value, ok := d[key].(string); ok {
		delete(d, key)
		return value, true
	} else {
		return "", false
	}
}

func getAndDelRequest(d logrus.Fields, key string) (*http.Request, bool) {
	if value, ok := d[key].(*http.Request); ok {
		delete(d, key)
		return value, true
	} else {
		return nil, false
	}
}

// SentryHook delivers logs to a sentry server.
type SentryHook struct {
	// Timeout sets the time to wait for a delivery error from the sentry server.
	// If this is set to zero the server will not wait for any response and will
	// consider the message correctly sent
	Timeout                 time.Duration
	StacktraceConfiguration StackTraceConfiguration

	client *raven.Client
	levels []logrus.Level
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
		client: client,
		levels: levels,
	}, nil
}

func formatExtraData(fields logrus.Fields) (ret map[string]interface{}) {
	ret = make(map[string]interface{}, len(fields))
	for key, value := range fields {
		switch value := value.(type) {
		case json.Marshaler:
			ret[key] = value
		case error:
			ret[key] = value.Error()
		case fmt.Stringer:
			ret[key] = value.String()
		default:
			ret[key] = value
		}
	}
	return
}

// Called when an event should be sent to sentry
// Special fields that sentry uses to give more information to the server
// are extracted from entry.Data (if they are found)
// These fields are: logger, server_name and http_request
func (hook *SentryHook) Fire(entry *logrus.Entry) error {
	packet := &raven.Packet{
		Message:   entry.Message,
		Timestamp: raven.Timestamp(entry.Time),
		Level:     severityMap[entry.Level],
		Platform:  "go",
	}

	d := entry.Data

	if logger, ok := getAndDel(d, "logger"); ok {
		packet.Logger = logger
	}
	if serverName, ok := getAndDel(d, "server_name"); ok {
		packet.ServerName = serverName
	}
	if req, ok := getAndDelRequest(d, "http_request"); ok {
		packet.Interfaces = append(packet.Interfaces, raven.NewHttp(req))
	}
	if user, ok := getUserContext(d); ok {
		packet.Interfaces = append(packet.Interfaces, user)
	}
	if eventID, ok := getEventID(d); ok {
		packet.EventID = eventID
	}
	stConfig := &hook.StacktraceConfiguration
	if stConfig.Enable && entry.Level <= stConfig.Level {
		currentStacktrace := raven.NewStacktrace(stConfig.Skip, stConfig.Context, stConfig.InAppPrefixes)
		packet.Interfaces = append(packet.Interfaces, currentStacktrace)
	}
	packet.Extra = formatExtraData(d)

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

// Levels returns the available logging levels.
func (hook *SentryHook) Levels() []logrus.Level {
	return hook.levels
}
