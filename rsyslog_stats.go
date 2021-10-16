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
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Sanitise metric name
func sanitiseMetricName(name string) string {
	reNonAlNum := regexp.MustCompile("[^_a-zA-Z0-9]")
	reUnderscores := regexp.MustCompile("_+")
	nn := strings.ToLower(name)
	// replace all non-alnum chars by underscore
	nn = reNonAlNum.ReplaceAllLiteralString(nn, "_")
	// squash multiple underscores
	nn = reUnderscores.ReplaceAllLiteralString(nn, "_")
	// strip trailing underscore
	nn = strings.TrimRight(nn, "_")

	return nn
}

// Split dynstats counter stats by "." from right
func splitRight(str string) (string, string) {
	i := strings.LastIndexAny(str, ".")

	return str[:i], str[i+1:]
}

func appendMetric(m RsyslogStatsMetrics, metricName string, labels RsyslogStatsLabels, value interface{}) RsyslogStatsMetrics {
	saneMetricName := sanitiseMetricName(metricName)
	saneValue := RsyslogStatsValue(value.(float64))

	if _, found := m[saneMetricName]; !found {
		m[saneMetricName] = make(RsyslogStatsLabeledValues)
	}

	m[saneMetricName][labels] = saneValue

	return m
}

func getValue(value interface{}) (rv float64, e error) {
	switch v := value.(type) {
	case float64:
		rv = v
	case string:
		rv, e = strconv.ParseFloat(v, 64)
	default:
		e = fmt.Errorf("cannot convert '%T' to float64: %w", value, strconv.ErrSyntax)
	}

	return rv, e
}

// Metric value type
type RsyslogStatsValue int

// Label: {name="main Q"} -> { Name: "name", Value: "main Q" }
// Just one label per value is used at the moment
type RsyslogStatsLabels struct {
	Name  string
	Value string
}

// Map of metric values with their labels: { {name="main Q"}: 123, ...}
type RsyslogStatsLabeledValues map[RsyslogStatsLabels]RsyslogStatsValue

// Map of metrics: '{ "rsyslog_core_queue_discarded_full": { {"name":"main Q"}: 123 }, ... }, ...'
type RsyslogStatsMetrics map[string]RsyslogStatsLabeledValues

// Main structure
type RsyslogStats struct {
	sync.RWMutex
	Metrics        RsyslogStatsMetrics
	ParserFailures int
	ParsedMessages int
	ParseTimestamp int64
	MetricPrefix   string
	NameField      string
	OriginField    string

	parsersByType map[rsyslogStatType]parserForType
}

// RsyslogStats constructor
func NewRsyslogStats() *RsyslogStats {
	rs := new(RsyslogStats)
	rs.MetricPrefix = "rsyslog"
	rs.NameField = "name"
	rs.OriginField = "origin"
	rs.ParserFailures = 0
	rs.ParsedMessages = 0
	rs.Metrics = make(RsyslogStatsMetrics)

	rs.parsersByType = map[rsyslogStatType]parserForType{
		rtDynstatGlobal: rs.parseDynstatsGlobal,
		rtDynstatBucket: rs.parseDynstatsBucket,
		rtSender:        rs.parseSenderStats,
		rtNamed:         rs.parseNamedStats,
		rtDefault:       rs.parseDefault,
	}

	return rs
}

// Add collected metrics from `m`
func (rs *RsyslogStats) add(m RsyslogStatsMetrics) {
	for metric, data := range m {
		rs.Lock()
		for labels, value := range data {
			if _, found := rs.Metrics[metric]; !found {
				rs.Metrics[metric] = RsyslogStatsLabeledValues{}
			}

			rs.Metrics[metric][labels] = value
		}
		rs.Unlock()
	}
}

// Parsing error wrapper
func (rs *RsyslogStats) failToParse(err error, source string) error {
	log.Printf("%s! JSON string is %s", err, source)
	rs.ParserFailures++

	return errors.Unwrap(err)
}

// Parsers

type rsyslogStatType int32

const (
	rtDefault rsyslogStatType = iota
	rtDynstatGlobal
	rtDynstatBucket
	rtNamed
	rtSender
)

type parserForType func(string, string, map[string]interface{}) (RsyslogStatsMetrics, []error)

// Parse global dynstats counters
func (rs *RsyslogStats) parseDynstatsGlobal(name, origin string, data map[string]interface{}) (RsyslogStatsMetrics, []error) {
	m := RsyslogStatsMetrics{}
	metricName := rs.MetricPrefix + "_" + origin + "_" + name

	for field, value := range data["values"].(map[string]interface{}) {
		cname, counter := splitRight(field)
		appendMetric(m, metricName+"_"+counter, RsyslogStatsLabels{"counter", cname}, value)
	}

	return m, nil
}

// Parse dynstats.bucket counters
func (rs *RsyslogStats) parseDynstatsBucket(name, origin string, data map[string]interface{}) (RsyslogStatsMetrics, []error) {
	m := RsyslogStatsMetrics{}
	metricName := rs.MetricPrefix + "_" + origin + "_" + name

	for counter, value := range data["values"].(map[string]interface{}) {
		appendMetric(m, metricName, RsyslogStatsLabels{"bucket", counter}, value)
	}

	return m, nil
}

// Parse sender stats
func (rs *RsyslogStats) parseSenderStats(name, origin string, data map[string]interface{}) (RsyslogStatsMetrics, []error) {
	errs := []error{}
	v, e := getValue(data["messages"])

	if e != nil {
		return nil, append(errs, e)
	}

	m := RsyslogStatsMetrics{}
	l := RsyslogStatsLabels{"sender", data["sender"].(string)}
	metricName := rs.MetricPrefix + "_" + "sender_stat_messages"
	appendMetric(m, metricName, l, v)

	return m, nil
}

// Parse "named" counters (core.queue, core.action)
func (rs *RsyslogStats) parseNamedStats(name, origin string, data map[string]interface{}) (RsyslogStatsMetrics, []error) {
	errs := []error{}
	m := RsyslogStatsMetrics{}
	l := RsyslogStatsLabels{"name", name}
	metricName := rs.MetricPrefix + "_" + origin

	for counter, value := range data {
		if counter == rs.NameField || counter == rs.OriginField {
			continue
		}

		if v, e := getValue(value); e != nil {
			errs = append(errs, e)
		} else {
			appendMetric(m, metricName+"_"+counter, l, v)
		}
	}

	return m, errs
}

// Parse common (unlabeled) counters
func (rs *RsyslogStats) parseDefault(name, origin string, data map[string]interface{}) (RsyslogStatsMetrics, []error) {
	errs := []error{}
	m := RsyslogStatsMetrics{}
	l := RsyslogStatsLabels{}
	metricName := rs.MetricPrefix + "_" + origin + "_" + name

	for counter, value := range data {
		if counter == rs.NameField || counter == rs.OriginField {
			continue
		}

		if v, e := getValue(value); e != nil {
			errs = append(errs, e)
		} else {
			appendMetric(m, metricName+"_"+counter, l, v)
		}
	}

	return m, errs
}

// Identify statLine type
func (rs *RsyslogStats) identify(data map[string]interface{}) (name string, origin string, st rsyslogStatType, e error) {
	var found bool

	name, found = data[rs.NameField].(string)
	if !found {
		e = fmt.Errorf("'%s' field is required but not found", rs.NameField)
	}

	origin, found = data[rs.OriginField].(string)
	if !found {
		switch name {
		case "omkafka": // omkafka missing origin hack (issue #1508, pre-8.27)
			origin = "omkafka"
		case "_sender_stat": // senders.keepTrack stats hack - https://github.com/rsyslog/rsyslog/pull/4601
			origin = "impstats"
		default:
			e = fmt.Errorf("'%s' field is required but not found", rs.OriginField)
		}
	}

	st = rtNamed // default type

	switch origin {
	case "dynstats":
		st = rtDynstatGlobal
	case "dynstats.bucket":
		st = rtDynstatBucket
	default:
		switch name {
		case "_sender_stat":
			st = rtSender
		}
	}

	return
}

// Parse JSON line and store metrics
func (rs *RsyslogStats) Parse(statLine string) error {
	var (
		data   map[string]interface{}
		name   string
		origin string
	)

	err := json.Unmarshal([]byte(statLine), &data)
	if err != nil {
		return rs.failToParse(fmt.Errorf("cannot parse JSON: %w", err), statLine)
	}

	name, origin, rsType, err := rs.identify(data)
	if err != nil {
		return rs.failToParse(err, statLine)
	}

	m, errs := rs.parsersByType[rsType](name, origin, data)

	for _, e := range errs {
		rs.failToParse(e, statLine)
	}

	rs.add(m)

	rs.ParsedMessages++
	rs.ParseTimestamp = time.Now().Unix()

	return nil
}
