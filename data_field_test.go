package logrus_sentry

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"testing"

	"github.com/getsentry/raven-go"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestLen(t *testing.T) {
	a := assert.New(t)

	tests := []struct {
		fieldSize int
	}{
		{0},   // empty fields
		{1},   // "0"
		{2},   // "0", "1"
		{9},   // "0", "1", "2" ... "8"
		{100}, // "0", "1", "2" ... "99"
	}

	for _, tt := range tests {
		target := fmt.Sprintf("%+v", tt)

		fields := logrus.Fields{}
		for i, max := 0, tt.fieldSize; i < max; i++ {
			fields[strconv.Itoa(i)] = struct{}{}
		}

		df := dataField{
			data: fields,
		}
		a.Equal(tt.fieldSize, df.len(), "dataField.Len() should equal fieldSize", target)
	}
}

func TestIsOmit(t *testing.T) {
	a := assert.New(t)

	omitList := map[string]struct{}{
		"key_1": {},
		"key_2": {},
		"key_3": {},
		"key_4": {},
	}

	tests := []struct {
		key      string
		expected bool
	}{
		{"key_1", true},
		{"key_2", true},
		{"key_3", true},
		{"key_4", true},
		{"not_key", false},
		{"foo", false},
		{"bar", false},
		{"_key_1", false},
		{"key_1_", false},
	}

	for _, tt := range tests {
		target := fmt.Sprintf("%+v", tt)

		df := dataField{
			omitList: omitList,
		}
		a.Equal(tt.expected, df.isOmit(tt.key), target)
	}
}

func TestGetLogger(t *testing.T) {
	a := assert.New(t)

	tests := []struct {
		key         string
		value       interface{}
		expected    bool
		description string
	}{
		{"logger", "test_logger", true, "valid logger"},
		{"logger", "", true, "valid logger"},
		{"not_logger", "test_logger", false, "invalid key"},
		{"logger", 1, false, "invalid value type"},
		{"logger", true, false, "invalid value type"},
		{"logger", struct{}{}, false, "invalid value type"},
	}

	for _, tt := range tests {
		target := fmt.Sprintf("%+v", tt)

		fields := logrus.Fields{}
		fields[tt.key] = tt.value

		df := newDataField(fields)
		logger, ok := df.getLogger()
		a.Equal(tt.expected, ok, target)
		if ok {
			a.Equal(tt.value, logger, target)
			a.True(df.isOmit("logger"), "`logger` should be in omitList")
		} else {
			a.False(df.isOmit("logger"), "`logger` should not be in omitList")
		}
	}
}

func TestGetServerName(t *testing.T) {
	a := assert.New(t)

	tests := []struct {
		key         string
		value       interface{}
		expected    bool
		description string
	}{
		{"server_name", "test_server_name", true, "valid server name"},
		{"server_name", "", true, "valid server name"},
		{"not_server_name", "test_server_name", false, "invalid key"},
		{"server_name", 1, false, "invalid value type"},
		{"server_name", true, false, "invalid value type"},
		{"server_name", struct{}{}, false, "invalid value type"},
	}

	for _, tt := range tests {
		target := fmt.Sprintf("%+v", tt)

		fields := logrus.Fields{}
		fields[tt.key] = tt.value

		df := newDataField(fields)
		serverName, ok := df.getServerName()
		a.Equal(tt.expected, ok, target)
		if ok {
			a.Equal(tt.value, serverName, target)
			a.True(df.isOmit("server_name"), "`server_name` should be in omitList")
		} else {
			a.False(df.isOmit("server_name"), "`server_name` should not be in omitList")
		}
	}
}

func TestGetTags(t *testing.T) {
	a := assert.New(t)

	tests := []struct {
		key         string
		value       interface{}
		expected    bool
		description string
	}{
		{"tags", raven.Tags{{Key: "key", Value: "value"}}, true, "valid tags"},
		{"tags", raven.Tags{}, true, "valid tags"},
		{"not_tags", raven.Tags{{Key: "key", Value: "value"}}, false, "invalid key"},
		{"tags", &raven.Tags{}, false, "invalid value type"},
		{"tags", "test_tags", false, "invalid value type"},
		{"tags", 1, false, "invalid value type"},
		{"tags", true, false, "invalid value type"},
		{"tags", struct{}{}, false, "invalid value type"},
	}

	for _, tt := range tests {
		target := fmt.Sprintf("%+v", tt)

		fields := logrus.Fields{}
		fields[tt.key] = tt.value

		df := newDataField(fields)
		tags, ok := df.getTags()
		a.Equal(tt.expected, ok, target)
		if ok {
			a.Equal(tt.value, tags, target)
			a.True(df.isOmit("tags"), "`tags` should be in omitList")
		} else {
			a.False(df.isOmit("tags"), "`tags` should not be in omitList")
		}
	}
}

func TestGetFingerprint(t *testing.T) {
	a := assert.New(t)

	tests := []struct {
		key         string
		value       interface{}
		expected    bool
		description string
	}{
		{"fingerprint", []string{"a", "fingerprint"}, true, "valid fingerprint"},
		{"fingerprint", []string{}, true, "valid fingerprint"},
		{"not_fingerprint", []string{"a", "fingerprint"}, false, "invalid key"},
		{"fingerprint", []int{}, false, "invalid value type"},
		{"fingerprint", "test_fingerprint", false, "invalid value type"},
		{"fingerprint", 1, false, "invalid value type"},
		{"fingerprint", true, false, "invalid value type"},
		{"fingerprint", struct{}{}, false, "invalid value type"},
	}

	for _, tt := range tests {
		target := fmt.Sprintf("%+v", tt)

		fields := logrus.Fields{}
		fields[tt.key] = tt.value

		df := newDataField(fields)
		fingerprint, ok := df.getFingerprint()
		a.Equal(tt.expected, ok, target)
		if ok {
			a.Equal(tt.value, fingerprint, target)
			a.True(df.isOmit("fingerprint"), "`fingerprint` should be in omitList")
		} else {
			a.False(df.isOmit("fingerprint"), "`fingerprint` should not be in omitList")
		}
	}
}

func TestGetError(t *testing.T) {
	a := assert.New(t)

	tests := []struct {
		key         string
		value       interface{}
		expected    bool
		description string
	}{
		{"error", errors.New("error type"), true, "valid error"},
		{"error", errors.New(""), true, "valid error"},
		{"not_error", errors.New("error type"), false, "invalid key"},
		{"error", "test_error", false, "invalid value type"},
		{"error", 1, false, "invalid value type"},
		{"error", true, false, "invalid value type"},
		{"error", struct{}{}, false, "invalid value type"},
	}

	for _, tt := range tests {
		target := fmt.Sprintf("%+v", tt)

		fields := logrus.Fields{}
		fields[tt.key] = tt.value

		df := newDataField(fields)
		err, ok := df.getError()
		a.Equal(tt.expected, ok, target)
		if ok {
			a.Equal(tt.value, err, target)
			a.True(df.isOmit("error"), "`error` should be in omitList")
		} else {
			a.False(df.isOmit("error"), "`error` should not be in omitList")
		}
	}
}

func TestGetHTTPRequest(t *testing.T) {
	a := assert.New(t)

	httpReq, _ := http.NewRequest("GET", "/", nil)
	ravenReq := raven.NewHttp(httpReq)

	tests := []struct {
		key         string
		value       interface{}
		expected    bool
		description string
	}{
		{"http_request", httpReq, true, "valid http_request"},
		{"not_http_request", httpReq, false, "invalid key"},
		{"http_request", http.Request{}, false, "invalid value type"},
		{"http_request", "test_http_request", false, "invalid value type"},
		{"http_request", 1, false, "invalid value type"},
		{"http_request", true, false, "invalid value type"},
		{"http_request", struct{}{}, false, "invalid value type"},
		{"http_request", raven.NewHttp(httpReq), true, "valid raven http_request"},
		{"http_request", raven.Http{}, false, "invalid raven http_request"},
	}

	for _, tt := range tests {
		target := fmt.Sprintf("%+v", tt)

		fields := logrus.Fields{}
		fields[tt.key] = tt.value

		df := newDataField(fields)
		req, ok := df.getHTTPRequest()
		a.Equal(tt.expected, ok, target)
		if ok {
			a.Equal(ravenReq, req, target)
			a.True(df.isOmit("http_request"), "`http_request` should be in omitList")
		} else {
			a.False(df.isOmit("http_request"), "`http_request` should not be in omitList")
		}
	}
}

func TestGetEventID(t *testing.T) {
	a := assert.New(t)

	tests := []struct {
		key         string
		value       interface{}
		expected    bool
		description string
	}{
		{"event_id", "ffffffff-ffff-ffff-ffff-ffffffffffff", true, "valid event id"},
		{"event_id", "ffffffffffffffffffffffffffffffff", true, "valid event id"},
		{"event_id", "urn:uuid:ffffffff-ffff-ffff-ffff-ffffffffffff", true, "valid event id"},
		{"not_event_id", "ffffffff-ffff-ffff-ffff-ffffffffffff", false, "invalid key"},
		{"event_id", "test_event_id", false, "invalid uuid format"},
		{"event_id", "ffffffff-ffff-ffff-ffff-ffffffffffffZ", false, "invalid uuid format"},
		{"event_id", "Zffffffff-ffff-ffff-ffff-ffffffffffff", false, "invalid uuid format"},
		{"event_id", 1, false, "invalid value type"},
		{"event_id", true, false, "invalid value type"},
		{"event_id", struct{}{}, false, "invalid value type"},
	}

	for _, tt := range tests {
		target := fmt.Sprintf("%+v", tt)

		fields := logrus.Fields{}
		fields[tt.key] = tt.value

		df := newDataField(fields)
		eventID, ok := df.getEventID()
		a.Equal(tt.expected, ok, target)
		if ok {
			a.Equal("ffffffffffffffffffffffffffffffff", eventID, target)
			a.True(df.isOmit("event_id"), "`event_id` should be in omitList")
		} else {
			a.False(df.isOmit("event_id"), "`event_id` should not be in omitList")
		}
	}
}

func TestGetUser(t *testing.T) {
	a := assert.New(t)

	tests := []struct {
		key         string
		value       interface{}
		expected    bool
		description string
	}{
		{"user", &raven.User{}, true, "valid user"},
		{"user", raven.User{}, true, "valid user"},
		{"not_user", &raven.User{}, false, "invalid key"},
		{"user", "test_user", false, "invalid value type"},
		{"user", 1, false, "invalid value type"},
		{"user", true, false, "invalid value type"},
		{"user", struct{}{}, false, "invalid value type"},
	}

	for _, tt := range tests {
		target := fmt.Sprintf("%+v", tt)

		fields := logrus.Fields{}
		fields[tt.key] = tt.value

		df := newDataField(fields)
		user, ok := df.getUser()
		a.Equal(tt.expected, ok, target)
		if ok {
			a.IsType(&raven.User{}, user, target)
			a.True(df.isOmit("user"), "`user` should be in omitList")
		} else {
			a.False(df.isOmit("user"), "`user` should not be in omitList")
		}
	}
}

func TestGetUserFromString(t *testing.T) {
	a := assert.New(t)

	tests := []struct {
		data        map[string]interface{}
		expected    bool
		description string
	}{
		{map[string]interface{}{
			"user_name":  "name",
			"user_email": "example@example.com",
			"user_id":    "A0001",
			"user_ip":    "0.0.0.0",
		}, true, "valid user"},
		{map[string]interface{}{"user_name": "name"}, true, "valid user"},
		{map[string]interface{}{"user_email": "example@example.com"}, true, "valid user"},
		{map[string]interface{}{"user_id": "A0001"}, true, "valid user"},
		{map[string]interface{}{"user_ip": "0.0.0.0"}, true, "valid user"},
		{map[string]interface{}{"user_name": ""}, false, "invalid user: empty user_name"},
		{map[string]interface{}{"user_email": ""}, false, "invalid user: empty user_email"},
		{map[string]interface{}{"user_id": ""}, false, "invalid user: empty user_id"},
		{map[string]interface{}{"user_ip": ""}, false, "invalid user: empty user_ip"},
		{map[string]interface{}{
			"user_name":  1,
			"user_email": true,
			"user_id":    errors.New("user_id"),
			"user_ip":    "",
		}, false, "invalid types"},
	}

	for _, tt := range tests {
		target := fmt.Sprintf("%+v", tt)

		fields := logrus.Fields(tt.data)

		df := newDataField(fields)
		user, ok := df.getUser()
		a.Equal(tt.expected, ok, target)
		if ok {
			a.IsType(&raven.User{}, user, target)
		}
	}
}
