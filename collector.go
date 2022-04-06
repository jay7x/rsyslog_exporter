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
	_ "net/http/pprof"

	"github.com/prometheus/client_golang/prometheus"
)

// RsyslogStatsCollector is the prometheus collector implementation
type RsyslogStatsCollector struct {
	RS *RsyslogStats
}

// NewRsyslogStatsCollector constructor
func NewRsyslogStatsCollector(rs *RsyslogStats) *RsyslogStatsCollector {
	return &RsyslogStatsCollector{RS: rs}
}

// Describe metrics
func (rsc *RsyslogStatsCollector) Describe(ch chan<- *prometheus.Desc) {}

// Collect metrics
func (rsc *RsyslogStatsCollector) Collect(ch chan<- prometheus.Metric) {
	var mType prometheus.ValueType

	rsc.RS.RLock()

	for metricName, labeledValues := range rsc.RS.Metrics {
		for labels, value := range labeledValues {
			switch metricName {
			case "rsyslog_core_queue_size":
				mType = prometheus.GaugeValue
			default:
				mType = prometheus.CounterValue
			}

			desc := prometheus.NewDesc(metricName, "", []string{labels.Name}, nil)
			ch <- prometheus.MustNewConstMetric(desc, mType, float64(value), labels.Value)
		}
	}

	rsc.RS.RUnlock()

	// export internal counters
	ch <- prometheus.MustNewConstMetric(
		prometheus.NewDesc(
			"rsyslog_exporter_parser_failures",
			"Amount of rsyslog stats parsing failures",
			nil, nil,
		),
		prometheus.CounterValue,
		float64(rsc.RS.ParserFailures),
	)

	ch <- prometheus.MustNewConstMetric(
		prometheus.NewDesc(
			"rsyslog_exporter_parsed_messages",
			"Amount of rsyslog stat messages parsed",
			nil, nil,
		),
		prometheus.CounterValue,
		float64(rsc.RS.ParsedMessages),
	)

	ch <- prometheus.MustNewConstMetric(
		prometheus.NewDesc(
			"rsyslog_exporter_parse_timestamp",
			"Latest parse Unix timestamp",
			nil, nil,
		),
		prometheus.CounterValue,
		float64(rsc.RS.ParseTimestamp),
	)
}
