/*
 * Export rsyslog counters as prometheus metrics
 *
 * Copyright (c) 2021, Yury Bushmelev <jay4mail@gmail.com>
 * All rights reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package main

import (
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

// sanitiseMetricName
func TestRsyslogStatsSanitiseMetricName(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		input  string
		output string
	}{
		{"a1_b2_c3", "a1_b2_c3"},
		{"a1__b2___c3", "a1_b2_c3"},
		{"a1%b2!c3", "a1_b2_c3"},
		{"a1!@#$%^&*()b2+)(*&^%$#@!~c3", "a1_b2_c3"},
	}

	for _, c := range tests {
		if want, got := c.output, sanitiseMetricName(c.input); want != got {
			t.Errorf("want '%s', got '%s'", want, got)
		}
	}
}

// splitRight
func TestRsyslogStatsSplitRight(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		input string
		left  string
		right string
	}{
		{"a1.c3", "a1", "c3"},
		{"a1.b2.c3", "a1.b2", "c3"},
		{"a1..b2...c3", "a1..b2..", "c3"},
		{"a1.", "a1", ""},
		{"a1..", "a1.", ""},
		{".c3", "", "c3"},
	}

	for _, c := range tests {
		left, right := splitRight(c.input)
		if c.left != left || c.right != right {
			t.Errorf("want (%s, %s), got (%s, %s)", c.left, c.right, left, right)
		}
	}
}

// appendMetric
func TestRsyslogStatsAppendMetric(t *testing.T) {
	t.Parallel()

	got := RsyslogStatsMetrics{}
	got = appendMetric(got, "Rsyslog_Test_123_", RsyslogStatsLabels{"name", "t123.1"}, 1.123)
	got = appendMetric(got, "Rsyslog_Test_123_", RsyslogStatsLabels{"name", "t123.2"}, 2.234)
	got = appendMetric(got, "Rsyslog_Test_345_", RsyslogStatsLabels{"name", "t345"}, 3.345)

	want := RsyslogStatsMetrics{
		"rsyslog_test_123": {
			RsyslogStatsLabels{"name", "t123.1"}: 1,
			RsyslogStatsLabels{"name", "t123.2"}: 2,
		},
		"rsyslog_test_345": {
			RsyslogStatsLabels{"name", "t345"}: 3,
		},
	}

	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("RsyslogStatsMetrics mismatch (-want +got):\n%s", diff)
	}
}

// getValue
func TestRsyslogStatsGetValue(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		input interface{}
		value float64
		err   error
	}{
		{1.234, 1.234, nil},
		{"1.234", 1.234, nil},
		{"1.2.3.4", 0, strconv.ErrSyntax},
		{true, 0, strconv.ErrSyntax},
	}

	for _, c := range tests {
		want := c.value
		got, err := getValue(c.input)

		if err != nil && !errors.Is(err, c.err) {
			t.Errorf("errors mismatch (%#v != %#v)", err, c.err)
		}

		if want != got {
			t.Errorf("values mismatch")
		}
	}
}

// add
func TestRsyslogStatsAdd(t *testing.T) {
	t.Parallel()

	rs := NewRsyslogStats()
	rs.add(
		RsyslogStatsMetrics{
			"rsyslog_test_123": {
				RsyslogStatsLabels{"name", "t123.1"}: 1,
				RsyslogStatsLabels{"name", "t123.2"}: 2,
			},
		},
	)
	rs.add(
		RsyslogStatsMetrics{
			"rsyslog_test_345": {
				RsyslogStatsLabels{"name", "t345"}: 3,
			},
		},
	)

	got := rs.Metrics

	want := RsyslogStatsMetrics{
		"rsyslog_test_123": {
			RsyslogStatsLabels{"name", "t123.1"}: 1,
			RsyslogStatsLabels{"name", "t123.2"}: 2,
		},
		"rsyslog_test_345": {
			RsyslogStatsLabels{"name", "t345"}: 3,
		},
	}

	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("RsyslogStatsMetrics mismatch (-want +got):\n%s", diff)
	}
}

// parseDynstatsGlobal
func TestRsyslogStatsParseDynstatsGlobal(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		input  map[string]interface{}
		output RsyslogStatsMetrics
	}{
		{
			map[string]interface{}{"name": "global", "origin": "dynstats", "values": map[string]interface{}{"msg_per_facility.new_metric_add": 1.0, "msg_per_facility.ops_overflow": 2.0, "msg_per_facility.no_metric": 3.0, "msg_per_facility.metrics_purged": 4.0, "msg_per_facility.ops_ignored": 5.0}},
			RsyslogStatsMetrics{
				"rsyslog_dynstats_global_new_metric_add": {RsyslogStatsLabels{"counter", "msg_per_facility"}: 1},
				"rsyslog_dynstats_global_ops_overflow":   {RsyslogStatsLabels{"counter", "msg_per_facility"}: 2},
				"rsyslog_dynstats_global_no_metric":      {RsyslogStatsLabels{"counter", "msg_per_facility"}: 3},
				"rsyslog_dynstats_global_metrics_purged": {RsyslogStatsLabels{"counter", "msg_per_facility"}: 4},
				"rsyslog_dynstats_global_ops_ignored":    {RsyslogStatsLabels{"counter", "msg_per_facility"}: 5},
			},
		},
	}

	rs := NewRsyslogStats()
	for _, c := range tests {
		got, errs := rs.parseDynstatsGlobal(c.input["name"].(string), c.input["origin"].(string), c.input)
		for _, e := range errs {
			t.Errorf("%v", e)
		}

		if diff := cmp.Diff(c.output, got); diff != "" {
			t.Errorf("RsyslogStatsMetrics mismatch (-want +got):\n%s", diff)
		}
	}
}

// parseDynstatsBucket
func TestRsyslogStatsParseDynstatsBucket(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		input  map[string]interface{}
		output RsyslogStatsMetrics
	}{
		{
			map[string]interface{}{"name": "msg_per_facility", "origin": "dynstats.bucket", "values": map[string]interface{}{"mail": 1.0, "auth": 2.0, "local": 3.0}},
			RsyslogStatsMetrics{"rsyslog_dynstats_bucket_msg_per_facility": {RsyslogStatsLabels{"bucket", "mail"}: 1, RsyslogStatsLabels{"bucket", "auth"}: 2, RsyslogStatsLabels{"bucket", "local"}: 3}},
		},
	}

	rs := NewRsyslogStats()
	for _, c := range tests {
		got, errs := rs.parseDynstatsBucket(c.input["name"].(string), c.input["origin"].(string), c.input)
		for _, e := range errs {
			t.Errorf("%v", e)
		}

		if diff := cmp.Diff(c.output, got); diff != "" {
			t.Errorf("RsyslogStatsMetrics mismatch (-want +got):\n%s", diff)
		}
	}
}

// parseSenderStats
func TestRsyslogStatsParseSenderStats(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		input  map[string]interface{}
		output RsyslogStatsMetrics
	}{
		{
			map[string]interface{}{"name": "_sender_stat", "origin": "impstats", "sender": "test1.host.tld", "messages": "1"},
			RsyslogStatsMetrics{"rsyslog_sender_stat_messages": {RsyslogStatsLabels{"sender", "test1.host.tld"}: 1}},
		},
		{
			map[string]interface{}{"name": "_sender_stat", "origin": "impstats", "sender": "test2.host.tld", "messages": 42.0},
			RsyslogStatsMetrics{"rsyslog_sender_stat_messages": {RsyslogStatsLabels{"sender", "test2.host.tld"}: 42}},
		},
	}

	rs := NewRsyslogStats()
	for _, c := range tests {
		got, errs := rs.parseSenderStats(c.input["name"].(string), c.input["origin"].(string), c.input)
		for _, e := range errs {
			t.Errorf("%v", e)
		}

		if diff := cmp.Diff(c.output, got); diff != "" {
			t.Errorf("RsyslogStatsMetrics mismatch (-want +got):\n%s", diff)
		}
	}
}

// parseNamedStats
func TestRsyslogStatsParseNamedStats(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		input  map[string]interface{}
		output RsyslogStatsMetrics
	}{
		{
			map[string]interface{}{"name": "stats", "origin": "core.queue", "size": 1.0, "enqueued": 42.0, "full": 0.0, "maxqsize": 2.0},
			RsyslogStatsMetrics{
				"rsyslog_core_queue_size":     {RsyslogStatsLabels{"name", "stats"}: 1},
				"rsyslog_core_queue_enqueued": {RsyslogStatsLabels{"name", "stats"}: 42},
				"rsyslog_core_queue_full":     {RsyslogStatsLabels{"name", "stats"}: 0},
				"rsyslog_core_queue_maxqsize": {RsyslogStatsLabels{"name", "stats"}: 2},
			},
		},
	}

	rs := NewRsyslogStats()
	for _, c := range tests {
		got, errs := rs.parseNamedStats(c.input["name"].(string), c.input["origin"].(string), c.input)
		for _, e := range errs {
			t.Errorf("%v", e)
		}

		if diff := cmp.Diff(c.output, got); diff != "" {
			t.Errorf("RsyslogStatsMetrics mismatch (-want +got):\n%s", diff)
		}
	}
}

// parseDefault
func TestRsyslogStatsParseDefault(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		input  map[string]interface{}
		output RsyslogStatsMetrics
	}{
		{
			map[string]interface{}{"name": "resource-usage", "origin": "impstats", "openfiles": 42.0, "nvcsw": 123.0},
			RsyslogStatsMetrics{
				"rsyslog_impstats_resource_usage_openfiles": {RsyslogStatsLabels{}: 42},
				"rsyslog_impstats_resource_usage_nvcsw":     {RsyslogStatsLabels{}: 123},
			},
		},
	}

	rs := NewRsyslogStats()
	for _, c := range tests {
		got, errs := rs.parseDefault(c.input["name"].(string), c.input["origin"].(string), c.input)
		for _, e := range errs {
			t.Errorf("%v", e)
		}

		if diff := cmp.Diff(c.output, got); diff != "" {
			t.Errorf("RsyslogStatsMetrics mismatch (-want +got):\n%s", diff)
		}
	}
}

// identify
// FIXME test for errors
func TestRsyslogStatsIdentify(t *testing.T) {
	t.Parallel()

	type identifyRetValType struct {
		Name, Origin string
		Rstype       rsyslogStatType
		Err          error
	}

	var tests = []struct {
		input  map[string]interface{}
		output identifyRetValType
	}{
		{
			map[string]interface{}{"name": "global", "origin": "dynstats", "values": map[string]interface{}{"msg_per_facility.new_metric_add": 1.0, "msg_per_facility.ops_overflow": 2.0, "msg_per_facility.no_metric": 3.0, "msg_per_facility.metrics_purged": 4.0, "msg_per_facility.ops_ignored": 5.0}},
			identifyRetValType{"global", "dynstats", rtDynstatGlobal, nil},
		},
		{
			map[string]interface{}{"name": "msg_per_facility", "origin": "dynstats.bucket", "values": map[string]interface{}{"mail": 1.0, "auth": 2.0, "local": 3.0}},
			identifyRetValType{"msg_per_facility", "dynstats.bucket", rtDynstatBucket, nil},
		},
		{
			map[string]interface{}{"name": "_sender_stat", "origin": "impstats", "sender": "test1.host.tld", "messages": "1"},
			identifyRetValType{"_sender_stat", "impstats", rtSender, nil},
		},
		{
			map[string]interface{}{"name": "stats", "origin": "core.queue", "size": 1.0, "enqueued": 42.0, "full": 0.0, "maxqsize": 2.0},
			identifyRetValType{"stats", "core.queue", rtNamed, nil},
		},
	}

	var got identifyRetValType

	rs := NewRsyslogStats()
	for _, c := range tests {
		got.Name, got.Origin, got.Rstype, got.Err = rs.identify(c.input)
		if diff := cmp.Diff(c.output, got); diff != "" {
			t.Errorf("RsyslogStatsMetrics mismatch (-want +got):\n%s", diff)
		}
	}
}

// Parse
// FIXME test for errors
func TestRsyslogStatsParse(t *testing.T) {
	t.Parallel()

	inputs := [...]string{
		`{"name": "global", "origin": "dynstats", "values": {"msg_per_facility.new_metric_add": 1, "msg_per_facility.ops_overflow": 2, "msg_per_facility.no_metric": 3, "msg_per_facility.metrics_purged": 4, "msg_per_facility.ops_ignored": 5}}`,
		`{"name": "msg_per_facility", "origin": "dynstats.bucket", "values": {"mail": 1, "auth": 2, "local": 3}}`,
		`{"name": "_sender_stat", "origin": "impstats", "sender": "test1.host.tld", "messages": "1"}`,
		`{"name": "_sender_stat", "origin": "impstats", "sender": "test2.host.tld", "messages": 42}`,
		`{"name": "stats", "origin": "core.queue", "size": 1, "enqueued": 42, "full": 0, "maxqsize": 2}`,
		`{"name": "resource-usage", "origin": "impstats", "openfiles": 42, "nvcsw": 123}`,
	}

	output := struct {
		metrics        RsyslogStatsMetrics
		parserFailures int
		parsedMessages int
		parseTimestamp int64
	}{
		metrics: RsyslogStatsMetrics{
			"rsyslog_dynstats_global_new_metric_add": {RsyslogStatsLabels{"counter", "msg_per_facility"}: 1},
			"rsyslog_dynstats_global_ops_overflow":   {RsyslogStatsLabels{"counter", "msg_per_facility"}: 2},
			"rsyslog_dynstats_global_no_metric":      {RsyslogStatsLabels{"counter", "msg_per_facility"}: 3},
			"rsyslog_dynstats_global_metrics_purged": {RsyslogStatsLabels{"counter", "msg_per_facility"}: 4},
			"rsyslog_dynstats_global_ops_ignored":    {RsyslogStatsLabels{"counter", "msg_per_facility"}: 5},
			"rsyslog_dynstats_bucket_msg_per_facility": {
				RsyslogStatsLabels{"bucket", "mail"}:  1,
				RsyslogStatsLabels{"bucket", "auth"}:  2,
				RsyslogStatsLabels{"bucket", "local"}: 3,
			},
			"rsyslog_sender_stat_messages": {
				RsyslogStatsLabels{"sender", "test1.host.tld"}: 1,
				RsyslogStatsLabels{"sender", "test2.host.tld"}: 42,
			},
			"rsyslog_core_queue_size":     {RsyslogStatsLabels{"name", "stats"}: 1},
			"rsyslog_core_queue_enqueued": {RsyslogStatsLabels{"name", "stats"}: 42},
			"rsyslog_core_queue_full":     {RsyslogStatsLabels{"name", "stats"}: 0},
			"rsyslog_core_queue_maxqsize": {RsyslogStatsLabels{"name", "stats"}: 2},
			"rsyslog_impstats_openfiles":  {RsyslogStatsLabels{"name", "resource-usage"}: 42},
			"rsyslog_impstats_nvcsw":      {RsyslogStatsLabels{"name", "resource-usage"}: 123},
		},
		parserFailures: 0,
		parsedMessages: len(inputs),
		parseTimestamp: time.Now().Unix(),
	}

	rs := NewRsyslogStats()
	for _, c := range inputs {
		rs.Parse(c)
	}

	if diff := cmp.Diff(output.metrics, rs.Metrics); diff != "" {
		t.Errorf("RsyslogStatsMetrics mismatch (-want +got):\n%s", diff)
	}

	if want, got := output.parserFailures, rs.ParserFailures; want != got {
		t.Errorf("ParserFailures mismatch: want '%d', got '%d'", want, got)
	}

	if want, got := output.parsedMessages, rs.ParsedMessages; want != got {
		t.Errorf("ParsedMessages mismatch: want '%d', got '%d'", want, got)
	}

	// Not really sure if it's good idea at all
	if want, got := output.parseTimestamp, rs.ParseTimestamp; want > got {
		t.Errorf("Wrong ParseTimestamp: want '%d' > got '%d'", want, got)
	}
}
