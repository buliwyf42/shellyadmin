package firmware

import (
	"errors"
	"fmt"
	"testing"

	"shellyadmin/internal/core/shellyclient"
)

func TestIsUnreachableSeparatesSilenceFromRefusal(t *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{nil, false},
		{shellyclient.ErrAuthRequired, false},
		{shellyclient.ErrAuthLockout, false},
		{fmt.Errorf("rpc: %w", shellyclient.ErrAuthRequired), false},
		{errors.New("shelly returned 500"), false},
		{errors.New("dial tcp 192.168.211.117:80: connect: no route to host"), true},
		{errors.New("context deadline exceeded"), true},
		{errors.New("dial tcp: i/o timeout"), true},
		{errors.New("connect: connection refused"), true},
		{errors.New(`lookup shelly-x.local: no such host`), true},
		{errors.New("connect: network is unreachable"), true},
		{errors.New("connect: host is down"), true},
	}
	for _, tc := range cases {
		if got := isUnreachable(tc.err); got != tc.want {
			t.Errorf("isUnreachable(%v) = %v, want %v", tc.err, got, tc.want)
		}
	}
}
