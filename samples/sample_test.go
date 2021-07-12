package samples

import (
	htt "github.com/caiorcferreira/helm-template-testing"
	"testing"
)

func TestSampleChart(t *testing.T) {
	htt.TestChartTemplate(t, "./chart", "./tests")
}
