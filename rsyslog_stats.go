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
	"strings"
)

const (
	metric_prefix = "rsyslog"
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

// All possible labels of a metric value: '{"name":"msg_per_severity", "bucket":"syslog"}'
type RsyslogStatsLabels struct {
	Name, Bucket string
}

// Map of metric values with their labels: '{ {"name":"msg_per_severity", "bucket":"syslog"}: 123 }'
type RsyslogStatsLabeledValues map[RsyslogStatsLabels]RsyslogStatsValue

// Map of metrics of values with their labels: '{ "rsyslog_dynstats_bucket_msg_per_facility": { {"name":"msg_per_severity", "bucket":"syslog"}: 123 } }'
type RsyslogStatsMetrics map[string]RsyslogStatsLabeledValues

type RsyslogStats struct {
	Current        RsyslogStatsMetrics
	ParserFailures int
	StatsParsed    int
}

func NewRsyslogStats() *RsyslogStats {
	rs := new(RsyslogStats)
	rs.ParserFailures = 0
	rs.StatsParsed = 0
	rs.Current = make(RsyslogStatsMetrics)
	return rs
}

// Add metric with the 'name' label and the value (which is float64 internally)
func (rs *RsyslogStats) add(metric_name string, labels RsyslogStatsLabels, value interface{}) {
	sane_metric_name := sanitiseMetricName(metric_name)
	sane_value := RsyslogStatsValue(value.(float64))
	sane_labels := RsyslogStatsLabels{Name: sanitiseMetricName(labels.Name), Bucket: sanitiseMetricName(labels.Bucket)}
	log.Printf("%s[%s] = %d\n", sane_metric_name, sane_labels, sane_value)
	// TODO locking?
	if _, found := rs.Current[sane_metric_name]; !found {
		rs.Current[sane_metric_name] = make(RsyslogStatsLabeledValues)
	}
	rs.Current[sane_metric_name][sane_labels] = sane_value
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
	if name, found = data["name"].(string); !found {
		return rs.failToParse(fmt.Errorf("'name' key is required but not found"), statLine)
	}
	if origin, found = data["origin"].(string); !found {
		if name == "omkafka" {
			origin = "omkafka"
		} else {
			return rs.failToParse(fmt.Errorf("'origin' key is required but not found"), statLine)
		}
	}

	metric_basename = metric_prefix + "_" + origin

	switch origin {
	case "dynstats":
		// Special case for dynamic stats fields reported in <name>.<field> format
		for k, v := range data["values"].(map[string]interface{}) {
			dn, dc := splitRight(k)
			metric_name := metric_basename + "_global_" + dc
			rs.add(metric_name, RsyslogStatsLabels{dn, ""}, v)
		}
	case "dynstats.bucket":
		metric_name := metric_basename
		for k, v := range data["values"].(map[string]interface{}) {
			rs.add(metric_name, RsyslogStatsLabels{name, k}, v)
		}
	default:
		for k, v := range data {
			// skip 'name' and 'origin' fields
			if (k == "name") || (k == "origin") {
				continue
			}

			switch v.(type) {
			case float64:
				metric_name := metric_basename + "_" + k
				rs.add(metric_name, RsyslogStatsLabels{name, ""}, v)
			default:
				rs.failToParse(fmt.Errorf("Wrong value type '%T' for metric '%s'. Must be a number", v, k), statLine)
			}
		}
	}
	rs.StatsParsed++
	return nil
}
