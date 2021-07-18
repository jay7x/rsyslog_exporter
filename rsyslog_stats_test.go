/*
 * bla bla bla
 */

/*
global dynstats:
{ "name": "global", "origin": "dynstats", "values": { "msg_per_facility.ops_overflow": 0, "msg_per_facility.new_metric_add": 4, "msg_per_facility.no_metric": 0, "msg_per_facility.metrics_purged": 0, "msg_per_facility.ops_ignored": 0, "msg_per_facility.purge_triggered": 0, "msg_per_severity.ops_overflow": 0, "msg_per_severity.new_metric_add": 1, "msg_per_severity.no_metric": 0, "msg_per_severity.metrics_purged": 0, "msg_per_severity.ops_ignored": 0, "msg_per_severity.purge_triggered": 0 } }
*/

package main

import (
	"testing"
)

/* Nothing to test yet
func TestNewRsyslogStats(t *testing.T) {
  rs := NewRsyslogStats()
}
*/

func TestRsyslogStatsSanitiseMetricName(t *testing.T) {
	testNames := []string{
		"a1_b2_c3",
		"a1__b2___c3",
		"a1%b2!c3",
		"a1!@#$%^&*()b2+)(*&^%$#@!~c3",
	}

	rs := NewRsyslogStats()
	for _, v := range testNames {
		if want, got := "a1_b2_c3", rs.sanitiseMetricName(v); want != got {
			t.Errorf("want '%s', got '%s'", want, got)
		}
	}
}
