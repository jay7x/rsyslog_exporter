/*
 * bla bla bla
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
)

// Sanitise metric name
func sanitiseMetricName(name string) string {
	re_nonalnum := regexp.MustCompile("[^_a-zA-Z0-9]")
	re_underscores := regexp.MustCompile("_+")
	nn := strings.ToLower(name)
	// replace all non-alnum chars by underscore
	nn = re_nonalnum.ReplaceAllLiteralString(nn, "_")
	// squash multiple underscores
	nn = re_underscores.ReplaceAllLiteralString(nn, "_")
	// strip trailing underscore
	nn = strings.TrimRight(nn, "_")
	return nn
}

// Split dynstats counter stats by "." from right
func splitRight(str string) (string, string) {
	i := strings.LastIndexAny(str, ".")
	return str[:i], str[i+1:]
}

// Metric value type
type RsyslogStatsValue int

// Labels pair: {name="main Q",counter="discarder.full"}
type RsyslogStatsLabels struct {
	Name, Counter string
}

// Map of metric values with their labels: { {name="main Q",counter="discarded.full"}: 123, ...}
type RsyslogStatsLabeledValues map[RsyslogStatsLabels]RsyslogStatsValue

// Map of metrics: '{ "rsyslog_core_queue": { {"name":"main Q", "counter":"discarded.full"}: 123 }, ... }, ...'
type RsyslogStatsMetrics map[string]RsyslogStatsLabeledValues

type RsyslogStats struct {
	Current        RsyslogStatsMetrics
	ParserFailures int
	StatsParsed    int
	MetricPrefix   string
	NameField      string
	OriginField    string
}

func NewRsyslogStats() *RsyslogStats {
	rs := new(RsyslogStats)
	rs.MetricPrefix = "rsyslog"
	rs.NameField = "name"
	rs.OriginField = "origin"
	rs.ParserFailures = 0
	rs.StatsParsed = 0
	rs.Current = make(RsyslogStatsMetrics)
	return rs
}

// Add metric with labels
func (rs *RsyslogStats) add(metric_name string, labels RsyslogStatsLabels, value interface{}) {
	sane_metric_name := sanitiseMetricName(metric_name)
	sane_value := RsyslogStatsValue(value.(float64))
	log.Printf("%s[%s, %s] = %d\n", sane_metric_name, labels.Name, labels.Counter, sane_value)
	// TODO locking?
	if _, found := rs.Current[sane_metric_name]; !found {
		rs.Current[sane_metric_name] = make(RsyslogStatsLabeledValues)
	}
	rs.Current[sane_metric_name][labels] = sane_value
}

func (rs *RsyslogStats) failToParse(err error, source string) error {
	log.Printf("%s! JSON string is %s", err, source)
	rs.ParserFailures++
	return errors.Unwrap(err)
}

func (rs *RsyslogStats) Parse(statLine string) error {
	log.Printf("-- %s\n", statLine)

	var data map[string]interface{}
	var name string
	var origin string
	var metric_basename string

	err := json.Unmarshal([]byte(statLine), &data)
	if err != nil {
		return rs.failToParse(fmt.Errorf("Cannot parse JSON: %w", err), statLine)
	}

	var found bool

	// sanity check for 'name' and 'origin' presence
	if name, found = data[rs.NameField].(string); !found {
		return rs.failToParse(fmt.Errorf("'%s' field is required but not found", rs.NameField), statLine)
	}

	if origin, found = data[rs.OriginField].(string); !found {
		switch name {
		case "omkafka": // omkafka missing origin hack (issue #1508, pre-8.27)
			origin = "omkafka"
		case "_sender_stat": // senders.keepTrack stats hack - https://github.com/rsyslog/rsyslog/pull/4601
			origin = "_sender_stat"
		default:
			return rs.failToParse(fmt.Errorf("'%s' field is required but not found", rs.OriginField), statLine)
		}
	}

	// senders.keepTrack stats hack - rewrite origin & name
	if name == "_sender_stat" {
		origin = "_sender_stat"
		name = data["sender"].(string)
	}

	metric_basename = rs.MetricPrefix + "_" + origin

	skip_fields := map[string]bool{
		rs.OriginField: true,
		rs.NameField:   true,
	}

	if values, found := data["values"].(map[string]interface{}); found {
		if origin == "dynstats" {
			// Special case for dynstats fields reported in <name>.<field> format
			for field, value := range values {
				name, counter := splitRight(field)
				rs.add(metric_basename, RsyslogStatsLabels{name, counter}, value)
			}
		} else {
			// case for dynstats.bucket and any other possible "values"
			for counter, value := range values {
				rs.add(metric_basename, RsyslogStatsLabels{name, counter}, value)
			}
		}
	} else {
		for counter, value := range data {
			// skip some fields (origin & name e.g.)
			if _, found := skip_fields[counter]; found {
				continue
			}

			// senders.keepTrack stats hack - skip "sender" field just in this message
			if origin == "senders" && counter == "sender" {
				continue
			}

			switch value.(type) {
			case float64:
				rs.add(metric_basename, RsyslogStatsLabels{name, counter}, value)
			case string:
				if v, err := strconv.ParseFloat(value.(string), 64); err != nil {
					rs.failToParse(fmt.Errorf("Cannot convert field '%s' to float: %w!", counter, err), statLine)
				} else {
					rs.add(metric_basename, RsyslogStatsLabels{name, counter}, v)
				}
			default:
				rs.failToParse(fmt.Errorf("Wrong value type '%T' for the field '%s'!", value, counter), statLine)
			}
		}
	}
	rs.StatsParsed++
	return nil
}
