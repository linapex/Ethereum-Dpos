
//<developer>
//    <name>linapex 曹一峰</name>
//    <email>linapex@163.com</email>
//    <wx>superexc</wx>
//    <qqgroup>128148617</qqgroup>
//    <url>https://jsq.ink</url>
//    <role>pku engineer</role>
//    <date>2019-03-16 12:09:42</date>
//</624342651788857344>

package metrics

import (
	"fmt"
	"io"
	"sort"
	"time"
)

//写入排序定期将给定注册表中的每个指标写入
//给定IO.WRITER。
func Write(r Registry, d time.Duration, w io.Writer) {
	for range time.Tick(d) {
		WriteOnce(r, w)
	}
}

//WriteOnce将给定注册表中的度量值排序并写入给定的
//作者。
func WriteOnce(r Registry, w io.Writer) {
	var namedMetrics namedMetricSlice
	r.Each(func(name string, i interface{}) {
		namedMetrics = append(namedMetrics, namedMetric{name, i})
	})

	sort.Sort(namedMetrics)
	for _, namedMetric := range namedMetrics {
		switch metric := namedMetric.m.(type) {
		case Counter:
			fmt.Fprintf(w, "counter %s\n", namedMetric.name)
			fmt.Fprintf(w, "  count:       %9d\n", metric.Count())
		case Gauge:
			fmt.Fprintf(w, "gauge %s\n", namedMetric.name)
			fmt.Fprintf(w, "  value:       %9d\n", metric.Value())
		case GaugeFloat64:
			fmt.Fprintf(w, "gauge %s\n", namedMetric.name)
			fmt.Fprintf(w, "  value:       %f\n", metric.Value())
		case Healthcheck:
			metric.Check()
			fmt.Fprintf(w, "healthcheck %s\n", namedMetric.name)
			fmt.Fprintf(w, "  error:       %v\n", metric.Error())
		case Histogram:
			h := metric.Snapshot()
			ps := h.Percentiles([]float64{0.5, 0.75, 0.95, 0.99, 0.999})
			fmt.Fprintf(w, "histogram %s\n", namedMetric.name)
			fmt.Fprintf(w, "  count:       %9d\n", h.Count())
			fmt.Fprintf(w, "  min:         %9d\n", h.Min())
			fmt.Fprintf(w, "  max:         %9d\n", h.Max())
			fmt.Fprintf(w, "  mean:        %12.2f\n", h.Mean())
			fmt.Fprintf(w, "  stddev:      %12.2f\n", h.StdDev())
			fmt.Fprintf(w, "  median:      %12.2f\n", ps[0])
			fmt.Fprintf(w, "  75%%:         %12.2f\n", ps[1])
			fmt.Fprintf(w, "  95%%:         %12.2f\n", ps[2])
			fmt.Fprintf(w, "  99%%:         %12.2f\n", ps[3])
			fmt.Fprintf(w, "  99.9%%:       %12.2f\n", ps[4])
		case Meter:
			m := metric.Snapshot()
			fmt.Fprintf(w, "meter %s\n", namedMetric.name)
			fmt.Fprintf(w, "  count:       %9d\n", m.Count())
			fmt.Fprintf(w, "  1-min rate:  %12.2f\n", m.Rate1())
			fmt.Fprintf(w, "  5-min rate:  %12.2f\n", m.Rate5())
			fmt.Fprintf(w, "  15-min rate: %12.2f\n", m.Rate15())
			fmt.Fprintf(w, "  mean rate:   %12.2f\n", m.RateMean())
		case Timer:
			t := metric.Snapshot()
			ps := t.Percentiles([]float64{0.5, 0.75, 0.95, 0.99, 0.999})
			fmt.Fprintf(w, "timer %s\n", namedMetric.name)
			fmt.Fprintf(w, "  count:       %9d\n", t.Count())
			fmt.Fprintf(w, "  min:         %9d\n", t.Min())
			fmt.Fprintf(w, "  max:         %9d\n", t.Max())
			fmt.Fprintf(w, "  mean:        %12.2f\n", t.Mean())
			fmt.Fprintf(w, "  stddev:      %12.2f\n", t.StdDev())
			fmt.Fprintf(w, "  median:      %12.2f\n", ps[0])
			fmt.Fprintf(w, "  75%%:         %12.2f\n", ps[1])
			fmt.Fprintf(w, "  95%%:         %12.2f\n", ps[2])
			fmt.Fprintf(w, "  99%%:         %12.2f\n", ps[3])
			fmt.Fprintf(w, "  99.9%%:       %12.2f\n", ps[4])
			fmt.Fprintf(w, "  1-min rate:  %12.2f\n", t.Rate1())
			fmt.Fprintf(w, "  5-min rate:  %12.2f\n", t.Rate5())
			fmt.Fprintf(w, "  15-min rate: %12.2f\n", t.Rate15())
			fmt.Fprintf(w, "  mean rate:   %12.2f\n", t.RateMean())
		}
	}
}

type namedMetric struct {
	name string
	m    interface{}
}

//namedmetricslice是实现sort.interface的namedmetrics切片。
type namedMetricSlice []namedMetric

func (nms namedMetricSlice) Len() int { return len(nms) }

func (nms namedMetricSlice) Swap(i, j int) { nms[i], nms[j] = nms[j], nms[i] }

func (nms namedMetricSlice) Less(i, j int) bool {
	return nms[i].name < nms[j].name
}

