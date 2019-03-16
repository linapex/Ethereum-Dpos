
//<developer>
//    <name>linapex 曹一峰</name>
//    <email>linapex@163.com</email>
//    <wx>superexc</wx>
//    <qqgroup>128148617</qqgroup>
//    <url>https://jsq.ink</url>
//    <role>pku engineer</role>
//    <date>2019-03-16 12:09:42</date>
//</624342651935657984>

package metrics

import (
	"sort"
	"testing"
)

func TestMetricsSorting(t *testing.T) {
	var namedMetrics = namedMetricSlice{
		{name: "zzz"},
		{name: "bbb"},
		{name: "fff"},
		{name: "ggg"},
	}

	sort.Sort(namedMetrics)
	for i, name := range []string{"bbb", "fff", "ggg", "zzz"} {
		if namedMetrics[i].name != name {
			t.Fail()
		}
	}
}

