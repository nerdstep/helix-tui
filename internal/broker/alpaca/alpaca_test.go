package alpaca

import (
	"testing"

	"github.com/alpacahq/alpaca-trade-api-go/v3/marketdata"
)

func TestNormalizeFeed(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want marketdata.Feed
	}{
		{name: "default empty", in: "", want: marketdata.IEX},
		{name: "iex", in: "iex", want: marketdata.IEX},
		{name: "sip uppercase", in: "SIP", want: marketdata.SIP},
		{name: "delayed sip", in: "delayed_sip", want: marketdata.DelayedSIP},
		{name: "boats", in: "boats", want: marketdata.BOATS},
		{name: "overnight", in: "overnight", want: marketdata.Overnight},
		{name: "unknown defaults", in: "nope", want: marketdata.IEX},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeFeed(tt.in)
			if got != tt.want {
				t.Fatalf("normalizeFeed(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
